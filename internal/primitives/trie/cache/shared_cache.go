package cache

import (
	"sync"

	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/ChainSafe/gossamer/pkg/trie/triedb"
	"github.com/maypok86/otter"
)

// / The shared node cache.
// /
// / Internally this stores all cached nodes in a [`LruMap`]. It ensures that when updating the
// / cache, that the cache stays within its allowed bounds.
type SharedNodeCache[H runtime.Hash] struct {
	// 	/// The cached nodes, ordered by least recently used.
	// 	pub(super) lru: SharedNodeCacheMap<H>,
	cache        otter.Cache[H, triedb.CachedNode[H]]
	itemsEvicted uint
}

func NewSharedNodeCache[H runtime.Hash](sizeBytes uint) *SharedNodeCache[H] {
	snc := SharedNodeCache[H]{}
	itemsEvictedPtr := &snc.itemsEvicted
	var err error
	snc.cache, err = otter.MustBuilder[H, triedb.CachedNode[H]](int(sizeBytes)).
		Cost(func(hash H, node triedb.CachedNode[H]) uint32 {
			return uint32(node.ByteSize())
		}).
		DeletionListener(func(_ H, _ triedb.CachedNode[H], cause otter.DeletionCause) {
			if cause == otter.Size {
				(*itemsEvictedPtr)++
			}
		}).
		Build()
	if err != nil {
		panic(err)
	}
	return &snc
}

type UpdateItem[H runtime.Hash] struct {
	Hash H
	NodeCached[H]
}

// / Update the cache with the `list` of nodes which were either newly added or accessed.
func (snc *SharedNodeCache[H]) Update(list []UpdateItem[H]) {
	accessCount := uint(0)
	addCount := uint(0)

	snc.itemsEvicted = 0
	maxItemsEvicted := uint(snc.cache.Size()*100) / SharedNodeCacheMaxReplacePercent
	for _, ui := range list {
		if ui.NodeCached.FromSharedCache {
			_, ok := snc.cache.Get(ui.Hash)
			if ok {
				accessCount++
				if accessCount >= SharedNodeCacheMaxPromotedKeys {
					// Stop when we've promoted a large enough number of items.
					break
				}
				continue
			}
		}

		ok := snc.cache.Set(ui.Hash, ui.NodeCached.Node)
		if ok {
			addCount++
		}

		if snc.itemsEvicted > maxItemsEvicted {
			// Stop when we've evicted a big enough chunk of the shared cache.
			break
		}
	}

	// tracing::debug!(
	// 	target: super::LOG_TARGET,
	// 	"Updated the shared node cache: {} accesses, {} new values, {}/{} evicted (length = {}, inline size={}/{}, heap size={}/{})",
	// 	access_count,
	// 	add_count,
	// 	self.lru.limiter().items_evicted,
	// 	self.lru.limiter().max_items_evicted,
	// 	self.lru.len(),
	// 	self.lru.memory_usage(),
	// 	self.lru.limiter().max_inline_size,
	// 	self.lru.limiter().heap_size,
	// 	self.lru.limiter().max_heap_size,
	// );
}

func (snc *SharedNodeCache[H]) Reset() {
	snc.cache.Clear()
}

type ValueCacheKeyHash[H runtime.Hash] struct {
	StorageRoot H
	StorageKey  string
}

func (vckh ValueCacheKeyHash[H]) ValueCacheKey() ValueCacheKey[H] {
	return ValueCacheKey[H]{
		StorageRoot: vckh.StorageRoot,
		StorageKey:  []byte(vckh.StorageKey),
	}
}

// / The key type that is being used to address a [`CachedValue`].
// #[derive(Eq)]
// pub(super) struct ValueCacheKey<H> {
type ValueCacheKey[H runtime.Hash] struct {
	// /// The storage root of the trie this key belongs to.
	// pub storage_root: H,
	StorageRoot H
	// /// The key to access the value in the storage.
	// pub storage_key: Arc<[u8]>,
	StorageKey []byte
	// /// The hash that identifies this instance of `storage_root` and `storage_key`.
	// pub hash: ValueCacheKeyHash,
	// Hash ValueCacheKeyHash[H]
}

func (vck ValueCacheKey[H]) ValueCacheKeyHash() ValueCacheKeyHash[H] {
	return ValueCacheKeyHash[H]{
		StorageRoot: vck.StorageRoot,
		StorageKey:  string(vck.StorageKey),
	}
}

// / The shared value cache.
// /
// / The cache ensures that it stays in the configured size bounds.
type SharedValueCache[H runtime.Hash] struct {
	cache        otter.Cache[ValueCacheKeyHash[H], triedb.CachedValue[H]]
	itemsEvicted uint
}

func NewSharedValueCache[H runtime.Hash](size uint) *SharedValueCache[H] {
	var svc SharedValueCache[H]
	itemsEvictedPtr := &svc.itemsEvicted
	var err error

	svc.cache, err = otter.MustBuilder[ValueCacheKeyHash[H], triedb.CachedValue[H]](int(size)).
		Cost(func(key ValueCacheKeyHash[H], value triedb.CachedValue[H]) uint32 {
			keyCost := uint32(len(key.StorageKey))
			switch value := value.(type) {
			case triedb.NonExistingCachedValue[H]:
				return keyCost + 1
			case triedb.ExistingHashCachedValue[H]:
				return keyCost + uint32(value.Hash.Length())
			case triedb.ExistingCachedValue[H]:
				return keyCost + uint32(value.Hash.Length()+len(value.Data))
			default:
				panic("unreachable")
			}
		}).
		DeletionListener(func(_ ValueCacheKeyHash[H], _ triedb.CachedValue[H], cause otter.DeletionCause) {
			if cause == otter.Size {
				(*itemsEvictedPtr)++
			}
		}).
		Build()
	if err != nil {
		panic(err)
	}
	return &svc
}

type SharedValueCacheAdded[H runtime.Hash] struct {
	ValueCacheKey[H]
	triedb.CachedValue[H]
}

func (svc *SharedValueCache[H]) Update(added []SharedValueCacheAdded[H], accessed []ValueCacheKeyHash[H]) {
	accessCount := uint(0)
	addCount := uint(0)

	for _, hash := range accessed {
		// Access every node in the map to put it to the front.
		//
		// Since we are only comparing the hashes here it may lead us to promoting the wrong
		// values as the most recently accessed ones. However this is harmless as the only
		// consequence is that we may accidentally prune a recently used value too early.
		_, ok := svc.cache.Get(hash)
		if ok {
			accessCount++
		}
	}

	// Insert all of the new items which were *not* found in the shared cache.
	//
	// Limit how many items we'll replace in the shared cache in one go so that
	// we don't evict the whole shared cache nor we keep spinning our wheels
	// evicting items which we've added ourselves in previous iterations of this loop.
	svc.itemsEvicted = 0
	maxItemsEvicted := uint(svc.cache.Size()) * 100 / SharedValueCacheMaxReplacePercent

	for _, svca := range added {
		set := svc.cache.Set(svca.ValueCacheKey.ValueCacheKeyHash(), svca.CachedValue)
		if set {
			addCount++
		}

		if svc.itemsEvicted > maxItemsEvicted {
			// Stop when we've evicted a big enough chunk of the shared cache.
			break
		}
	}

	// tracing::debug!(
	// 	target: super::LOG_TARGET,
	// 	"Updated the shared value cache: {} accesses, {} new values, {}/{} evicted (length = {}, known_storage_keys = {}, inline size={}/{}, heap size={}/{})",
	// 	access_count,
	// 	add_count,
	// 	self.lru.limiter().items_evicted,
	// 	self.lru.limiter().max_items_evicted,
	// 	self.lru.len(),
	// 	self.lru.limiter().known_storage_keys.len(),
	// 	self.lru.memory_usage(),
	// 	self.lru.limiter().max_inline_size,
	// 	self.lru.limiter().heap_size,
	// 	self.lru.limiter().max_heap_size
	// );
}

func (snc *SharedValueCache[H]) Reset() {
	snc.cache.Clear()
}

// / The inner of [`SharedTrieCache`].
// pub(super) struct SharedTrieCacheInner<H: Hasher> {
type SharedTrieCacheInner[H runtime.Hash] struct {
	// node_cache: SharedNodeCache<H::Out>,
	nodeCache *SharedNodeCache[H]
	// value_cache: SharedValueCache<H::Out>,
	valueCache *SharedValueCache[H]
}

// / The shared trie cache.
// /
// / It should be instantiated once per node. It will hold the trie nodes and values of all
// / operations to the state. To not use all available memory it will ensure to stay in the
// / bounds given via the [`CacheSize`] at startup.
// /
// / The instance of this object can be shared between multiple threads.
// pub struct SharedTrieCache<H: Hasher> {
type SharedTrieCache[H runtime.Hash] struct {
	// inner: Arc<RwLock<SharedTrieCacheInner<H>>>,
	inner SharedTrieCacheInner[H]
	mtx   sync.RWMutex
}

// / Create a new [SharedTrieCache].
func NewSharedTrieCache[H runtime.Hash](size uint) SharedTrieCache[H] {
	totalBudget := size

	// Split our memory budget between the two types of caches.
	valueCacheBudget := uint(float32(totalBudget) * 0.20) // 20% for the value cache
	nodeCacheBudget := totalBudget - valueCacheBudget     // 80% for the node cache

	return SharedTrieCache[H]{
		inner: SharedTrieCacheInner[H]{
			nodeCache:  NewSharedNodeCache[H](nodeCacheBudget),
			valueCache: NewSharedValueCache[H](valueCacheBudget),
		},
	}
}

// / Create a new [`LocalTrieCache`](super::LocalTrieCache) instance from this shared cache.
func (stc *SharedTrieCache[H]) LocalTrieCache() LocalTrieCache[H] {
	nodeCache, err := otter.MustBuilder[H, NodeCached[H]](int(LocalNodeCacheMaxSize)).
		Cost(func(hash H, node NodeCached[H]) uint32 {
			return uint32(node.ByteSize())
		}).
		Build()
	if err != nil {
		panic(err)
	}

	valueCache, err := otter.MustBuilder[ValueCacheKeyHash[H], triedb.CachedValue[H]](int(LocalValueCacheMaxSize)).
		Cost(func(key ValueCacheKeyHash[H], value triedb.CachedValue[H]) uint32 {
			keyCost := uint32(len(key.StorageKey))
			switch value := value.(type) {
			case triedb.NonExistingCachedValue[H]:
				return keyCost + 1
			case triedb.ExistingHashCachedValue[H]:
				return keyCost + uint32(value.Hash.Length())
			case triedb.ExistingCachedValue[H]:
				return keyCost + uint32(value.Hash.Length()+len(value.Data))
			default:
				panic("unreachable")
			}
		}).
		Build()
	if err != nil {
		panic(err)
	}

	sharedValueCacheAccess, err := otter.MustBuilder[ValueCacheKeyHash[H], any](int(SharedValueCacheMaxPromotedKeys)).
		Build()
	if err != nil {
		panic(err)
	}

	return LocalTrieCache[H]{
		shared:                 stc,
		nodeCache:              nodeCache,
		valueCache:             valueCache,
		sharedValueCacheAccess: sharedValueCacheAccess,
	}
}

func (stc *SharedTrieCache[H]) Lock() {
	stc.mtx.Lock()
}

func (stc *SharedTrieCache[H]) Unlock() {
	stc.mtx.Unlock()
}

// / Get a copy of the node for `key`.
// /
// / This will temporarily lock the shared cache for reading.
// /
// / This doesn't change the least recently order in the internal [`LruMap`].
func (stc *SharedTrieCache[H]) PeekNode(key H) triedb.CachedNode[H] {
	stc.mtx.RLock()
	defer stc.mtx.RUnlock()
	node, ok := stc.inner.nodeCache.cache.Extension().GetQuietly(key)
	if ok {
		return node
	}
	return nil
}

// / Get a copy of the [`CachedValue`] for `key`.
// /
// / This will temporarily lock the shared cache for reading.
// /
// / This doesn't reorder any of the elements in the internal [`LruMap`].
func (stc *SharedTrieCache[H]) PeekValueByHash(hash ValueCacheKeyHash[H], storageRoot H, storageKey []byte) triedb.CachedValue[H] {
	stc.mtx.RLock()
	defer stc.mtx.RUnlock()
	val, ok := stc.inner.valueCache.cache.Extension().GetQuietly(hash)
	if ok {
		return val
	}
	return nil
}

// / Reset the node cache.
func (stc *SharedTrieCache[H]) ResetNodeCache() {
	stc.mtx.Lock()
	defer stc.mtx.Unlock()
	stc.inner.nodeCache.Reset()
}

// / Reset the value cache.
func (stc *SharedTrieCache[H]) ResetValueCache() {
	stc.mtx.Lock()
	defer stc.mtx.Unlock()
	stc.inner.valueCache.Reset()
}

func (stc *SharedTrieCache[H]) usedMemorySize() uint {
	stc.mtx.RLock()
	defer stc.mtx.RUnlock()
	var totalCost uint32

	nodeCacheKeys := make([]H, 0)
	stc.inner.nodeCache.cache.Range(func(key H, value triedb.CachedNode[H]) bool {
		nodeCacheKeys = append(nodeCacheKeys, key)
		return true
	})
	for _, key := range nodeCacheKeys {
		entry, ok := stc.inner.nodeCache.cache.Extension().GetEntryQuietly(key)
		if !ok {
			panic("huh?")
		}
		totalCost += entry.Cost()
	}
	valueCacheKeys := make([]ValueCacheKeyHash[H], 0)
	stc.inner.valueCache.cache.Range(func(key ValueCacheKeyHash[H], value triedb.CachedValue[H]) bool {
		valueCacheKeys = append(valueCacheKeys, key)
		return true
	})
	for _, key := range valueCacheKeys {
		entry, ok := stc.inner.valueCache.cache.Extension().GetEntryQuietly(key)
		if !ok {
			panic("huh?")
		}
		totalCost += entry.Cost()
	}
	return uint(totalCost)
}
