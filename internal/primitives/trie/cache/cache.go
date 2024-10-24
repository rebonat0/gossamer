package cache

import (
	"sync"

	costlru "github.com/ChainSafe/gossamer/internal/cost-lru"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/ChainSafe/gossamer/pkg/trie/triedb"
	"github.com/elastic/go-freelru"
)

// / The maximum number of existing keys in the shared cache that a single local cache
// / can promote to the front of the LRU cache in one go.
// /
// / If we have a big shared cache and the local cache hits all of those keys we don't
// / want to spend forever bumping all of them.
const SharedNodeCacheMaxPromotedKeys = uint(1792)

// / Same as [`SHARED_NODE_CACHE_MAX_PROMOTED_KEYS`].
const SharedValueCacheMaxPromotedKeys = uint(1792)

// / The maximum portion of the shared cache (in percent) that a single local
// / cache can replace in one go.
// /
// / We don't want a single local cache instance to have the ability to replace
// / everything in the shared cache.
const SharedNodeCacheMaxReplacePercent = uint(33)

// / Same as [`SHARED_NODE_CACHE_MAX_REPLACE_PERCENT`].
const SharedValueCacheMaxReplacePercent = uint(33)

// / The maximum size of the memory allocated on the heap by the local cache, in bytes.
// /
// / The size of the node cache should always be bigger than the value cache. The value
// / cache is only holding weak references to the actual values found in the nodes and
// / we account for the size of the node as part of the node cache.
const LocalNodeCacheMaxSize = uint(8 * 1024 * 1024)

// / Same as [`LOCAL_NODE_CACHE_MAX_HEAP_SIZE`].
const LocalValueCacheMaxSize = uint(2 * 1024 * 1024)

// / An internal struct to store the cached trie nodes.
type NodeCached[H runtime.Hash] struct {
	/// The cached node.
	Node triedb.CachedNode[H]
	/// Whether this node was fetched from the shared cache or not.
	FromSharedCache bool
}

// / Returns the number of bytes allocated on the heap by this node.
func (nc NodeCached[H]) ByteSize() uint {
	return nc.Node.ByteSize()
}

// / The local trie cache.
// /
// / This cache should be used per state instance created by the backend. One state instance is
// / referring to the state of one block. It will cache all the accesses that are done to the state
// / which could not be fullfilled by the [`SharedTrieCache`]. These locally cached items are merged
// / back to the shared trie cache when this instance is dropped.
// /
// / When using [`Self::as_trie_db_cache`] or [`Self::as_trie_db_mut_cache`], it will lock Mutexes.
// / So, it is important that these methods are not called multiple times, because they otherwise
// / deadlock.
type LocalTrieCache[H runtime.Hash] struct {
	/// The shared trie cache that created this instance.
	shared *SharedTrieCache[H]

	/// The local cache for the trie nodes.
	nodeCache    *costlru.LRU[H, NodeCached[H]]
	nodeCacheMtx sync.Mutex

	/// The local cache for the values.
	valueCache    *costlru.LRU[ValueCacheKeyHash[H], triedb.CachedValue[H]]
	valueCacheMtx sync.Mutex

	/// Keeps track of all values accessed in the shared cache.
	///
	/// This will be used to ensure that these nodes are brought to the front of the lru when this
	/// local instance is merged back to the shared cache. This can actually lead to collision when
	/// two [`ValueCacheKey`]s with different storage roots and keys map to the same hash. However,
	/// as we only use this set to update the lru position it is fine, even if we bring the wrong
	/// value to the top. The important part is that we always get the correct value from the value
	/// cache for a given key.
	sharedValueCacheAccess    *freelru.LRU[ValueCacheKeyHash[H], any]
	sharedValueCacheAccessMtx sync.Mutex

	stats trieHitStats
}

func (ltc *LocalTrieCache[H]) commit() {
	// tracing::debug!(
	// 	target: LOG_TARGET,
	// 	"Local node trie cache dropped: {}",
	// 	self.stats.node_cache
	// );

	// tracing::debug!(
	// 	target: LOG_TARGET,
	// 	"Local value trie cache dropped: {}",
	// 	self.stats.value_cache
	// );

	ltc.shared.Lock()
	defer ltc.shared.Unlock()

	sharedInner := &ltc.shared.inner

	updateItems := make([]UpdateItem[H], 0)
	ltc.nodeCacheMtx.Lock()
	for _, hash := range ltc.nodeCache.Keys() {
		node, ok := ltc.nodeCache.Peek(hash)
		if !ok {
			panic("node should be found")
		}
		updateItems = append(updateItems, UpdateItem[H]{
			Hash:       hash,
			NodeCached: node,
		})
	}
	ltc.nodeCache.Purge()
	ltc.nodeCacheMtx.Unlock()
	sharedInner.nodeCache.Update(updateItems)

	added := make([]SharedValueCacheAdded[H], 0)
	ltc.valueCacheMtx.Lock()
	for _, key := range ltc.valueCache.Keys() {
		value, ok := ltc.valueCache.Get(key)
		if !ok {
			panic("value should be found")
		}
		added = append(added, SharedValueCacheAdded[H]{
			ValueCacheKey: key.ValueCacheKey(),
			CachedValue:   value,
		})
	}
	ltc.valueCache.Purge()
	ltc.valueCacheMtx.Unlock()

	ltc.sharedValueCacheAccessMtx.Lock()
	accessed := ltc.sharedValueCacheAccess.Keys()
	ltc.sharedValueCacheAccess.Purge()
	ltc.sharedValueCacheAccessMtx.Unlock()

	sharedInner.valueCache.Update(added, accessed)
}

// / Return self as a [`TrieDB`](trie_db::TrieDB) compatible cache.
// /
// / The given `storage_root` needs to be the storage root of the trie this cache is used for.
func (ltc *LocalTrieCache[H]) TrieCache(storageRoot H) (cache triedb.TrieCache[H], unlock func()) {
	ltc.valueCacheMtx.Lock()
	ltc.sharedValueCacheAccessMtx.Lock()

	valueCache := forStorageRootValueCache[H]{
		storageRoot:            storageRoot,
		localValueCache:        ltc.valueCache,
		sharedValueCacheAccess: ltc.sharedValueCacheAccess,
	}

	ltc.nodeCacheMtx.Lock()

	unlock = func() {
		ltc.valueCacheMtx.Unlock()
		ltc.sharedValueCacheAccessMtx.Unlock()
		ltc.nodeCacheMtx.Unlock()
	}
	return &TrieCache[H]{
		sharedCache: ltc.shared,
		localCache:  ltc.nodeCache,
		valueCache:  &valueCache,
		stats:       ltc.stats,
	}, unlock
}

// / Return self as [`TrieDBMut`](trie_db::TrieDBMut) compatible cache.
// /
// / After finishing all operations with [`TrieDBMut`](trie_db::TrieDBMut) and having obtained
// / the new storage root, [`TrieCache::merge_into`] should be called to update this local
// / cache instance. If the function is not called, cached data is just thrown away and not
// / propagated to the shared cache. So, accessing these new items will be slower, but nothing
// / would break because of this.
func (ltc *LocalTrieCache[H]) TrieCacheMut() (cache triedb.TrieCache[H], unlock func()) {
	ltc.nodeCacheMtx.Lock()
	return &TrieCache[H]{
		sharedCache: ltc.shared,
		localCache:  ltc.nodeCache,
		valueCache:  &freshValueCache[H]{},
		stats:       ltc.stats,
	}, ltc.nodeCacheMtx.Unlock
}

// / A struct to gather hit/miss stats to aid in debugging the performance of the cache.
//
//	struct HitStats {
//		shared_hits: AtomicU64,
//		shared_fetch_attempts: AtomicU64,
//		local_hits: AtomicU64,
//		local_fetch_attempts: AtomicU64,
//	}
type hitStats struct{}

// / A struct to gather hit/miss stats for the node cache and the value cache.
//
//	struct TrieHitStats {
//		node_cache: HitStats,
//		value_cache: HitStats,
//	}
type trieHitStats struct{}

// / The abstraction of the value cache for the [`TrieCache`].
type valueCache[H runtime.Hash] interface {
	get(key []byte, sharedCache *SharedTrieCache[H], stats hitStats) triedb.CachedValue[H]
	insert(key []byte, value triedb.CachedValue[H])
}

// / The value cache is fresh, aka not yet associated to any storage root.
// / This is used for example when a new trie is being build, to cache new values.
type freshValueCache[H runtime.Hash] map[string]triedb.CachedValue[H]

func (fvc *freshValueCache[H]) get(key []byte, sharedCache *SharedTrieCache[H], stats hitStats) triedb.CachedValue[H] {
	// stats.local_fetch_attempts.fetch_add(1, Ordering::Relaxed);
	val, ok := (*fvc)[string(key)]
	if ok {
		// stats.local_hits.fetch_add(1, Ordering::Relaxed);
		return val
	} else {
		return nil
	}
}
func (fvc *freshValueCache[H]) insert(key []byte, value triedb.CachedValue[H]) {
	(*fvc)[string(key)] = value
}

// / The value cache is already bound to a specific storage root.
type forStorageRootValueCache[H runtime.Hash] struct {
	sharedValueCacheAccess *freelru.LRU[ValueCacheKeyHash[H], any]
	localValueCache        *costlru.LRU[ValueCacheKeyHash[H], triedb.CachedValue[H]]
	storageRoot            H
	// The shared value cache needs to be temporarily locked when reading from it
	// so we need to clone the value that is returned, but we need to be able to
	// return a reference to the value, so we just buffer it here.
	bufferedValue triedb.CachedValue[H]
}

func (fsrvc forStorageRootValueCache[H]) get(key []byte, sharedCache *SharedTrieCache[H], stats hitStats) triedb.CachedValue[H] {
	// We first need to look up in the local cache and then the shared cache.
	// It can happen that some value is cached in the shared cache, but the
	// weak reference of the data can not be upgraded anymore. This for example
	// happens when the node is dropped that contains the strong reference to the data.
	//
	// So, the logic of the trie would lookup the data and the node and store both
	// in our local caches.
	vck := ValueCacheKey[H]{
		StorageKey:  key,
		StorageRoot: fsrvc.storageRoot,
	}
	val, ok := fsrvc.localValueCache.Peek(vck.ValueCacheKeyHash())
	if ok {
		// stats.local_hits.fetch_add(1, Ordering::Relaxed);
		return val
	}

	// stats.shared_fetch_attempts.fetch_add(1, Ordering::Relaxed);
	sharedVal := sharedCache.PeekValueByHash(vck.ValueCacheKeyHash(), fsrvc.storageRoot, key)
	if sharedVal != nil {
		// stats.shared_hits.fetch_add(1, Ordering::Relaxed);
		fsrvc.sharedValueCacheAccess.Add(vck.ValueCacheKeyHash(), any(nil))
		fsrvc.bufferedValue = sharedVal
		return fsrvc.bufferedValue
	}

	return nil
}
func (fsrvc forStorageRootValueCache[H]) insert(key []byte, value triedb.CachedValue[H]) {
	vck := ValueCacheKey[H]{
		StorageKey:  key,
		StorageRoot: fsrvc.storageRoot,
	}
	fsrvc.localValueCache.Add(vck.ValueCacheKeyHash(), value)
}

// / The actual [`TrieCache`](trie_db::TrieCache) implementation.
// /
// / If this instance was created for using it with a [`TrieDBMut`](trie_db::TrieDBMut), it needs to
// / be merged back into the [`LocalTrieCache`] with [`Self::merge_into`] after all operations are
// / done.
type TrieCache[H runtime.Hash] struct {
	sharedCache *SharedTrieCache[H]
	localCache  *costlru.LRU[H, NodeCached[H]]
	valueCache  valueCache[H]
	stats       trieHitStats
}

// / Merge this cache into the given [`LocalTrieCache`].
// /
// / This function is only required to be called when this instance was created through
// / [`LocalTrieCache::as_trie_db_mut_cache`], otherwise this method is a no-op. The given
// / `storage_root` is the new storage root that was obtained after finishing all operations
// / using the [`TrieDBMut`](trie_db::TrieDBMut).
func (tc *TrieCache[H]) MergeInto(local *LocalTrieCache[H], storageRoot H) {
	cache, ok := tc.valueCache.(*freshValueCache[H])
	if !ok {
		return
	}

	if len(*cache) != 0 {
		local.valueCacheMtx.Lock()
		defer local.valueCacheMtx.Unlock()

		vck := ValueCacheKey[H]{
			StorageRoot: storageRoot,
		}
		for k, v := range *cache {
			vck.StorageKey = []byte(k)
			ok, _ := local.valueCache.Add(vck.ValueCacheKeyHash(), v)
			if !ok {
				panic("huh?")
			}
		}
	}
}

func (tc *TrieCache[H]) GetOrInsertNode(hash H, fetchNode func() (triedb.CachedNode[H], error)) (triedb.CachedNode[H], error) {
	var isLocalCacheHit bool = true
	// self.stats.node_cache.local_fetch_attempts.fetch_add(1, Ordering::Relaxed);

	// First try to grab the node from the local cache.
	var err error
	var node *NodeCached[H]
	local, ok := tc.localCache.Get(hash)
	if !ok {
		isLocalCacheHit = false

		// It was not in the local cache; try the shared cache.
		// self.stats.node_cache.shared_fetch_attempts.fetch_add(1, Ordering::Relaxed);
		shared := tc.sharedCache.PeekNode(hash)
		if shared != nil {
			// self.stats.node_cache.shared_hits.fetch_add(1, Ordering::Relaxed);
			// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from shared cache");
			node = &NodeCached[H]{Node: shared, FromSharedCache: true}
		} else {
			// It was not in the shared cache; try fetching it from the database.
			var fetched triedb.CachedNode[H]
			fetched, err = fetchNode()
			if err != nil {
				// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from database failed");
				return nil, err
			} else {
				// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from database");
				node = &NodeCached[H]{Node: fetched, FromSharedCache: false}
			}
		}
		tc.localCache.Add(hash, *node)
	} else {
		node = &local
	}

	if isLocalCacheHit {
		// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from local cache");
		// self.stats.node_cache.local_hits.fetch_add(1, Ordering::Relaxed);
	}

	if node == nil {
		panic("you can always insert at least one element into the local cache; qed")
	}
	return node.Node, nil
}

func (tc *TrieCache[H]) GetNode(hash H) triedb.CachedNode[H] {
	var isLocalCacheHit bool = true
	// self.stats.node_cache.local_fetch_attempts.fetch_add(1, Ordering::Relaxed);

	// First try to grab the node from the local cache.
	var node *NodeCached[H]
	nodeCached, ok := tc.localCache.Get(hash)
	if !ok {
		isLocalCacheHit = false

		// / It was not in the local cache; try the shared cache.
		// self.stats.node_cache.shared_fetch_attempts.fetch_add(1, Ordering::Relaxed);
		peeked := tc.sharedCache.PeekNode(hash)
		if peeked != nil {
			// self.stats.node_cache.shared_hits.fetch_add(1, Ordering::Relaxed);
			// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from shared cache");
			node = &NodeCached[H]{Node: peeked, FromSharedCache: true}
		} else {
			// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from cache failed");
			return nil
		}
	} else {
		node = &nodeCached
	}

	if isLocalCacheHit {
		// tracing::trace!(target: LOG_TARGET, ?hash, "Serving node from local cache");
		// self.stats.node_cache.local_hits.fetch_add(1, Ordering::Relaxed);
	}

	if node == nil {
		panic("you can always insert at least one element into the local cache; qed")
	}
	return node.Node
}

func (tc *TrieCache[H]) GetValue(key []byte) triedb.CachedValue[H] {
	cached := tc.valueCache.get(key, tc.sharedCache, hitStats{})

	// tracing::trace!(
	// 	target: LOG_TARGET,
	// 	key = ?sp_core::hexdisplay::HexDisplay::from(&key),
	// 	found = res.is_some(),
	// 	"Looked up value for key",
	// );

	return cached
}

func (tc *TrieCache[H]) SetValue(key []byte, value triedb.CachedValue[H]) {
	// tracing::trace!(
	// 	target: LOG_TARGET,
	// 	key = ?sp_core::hexdisplay::HexDisplay::from(&key),
	// 	"Caching value for key",
	// );

	tc.valueCache.insert(key, value)
}
