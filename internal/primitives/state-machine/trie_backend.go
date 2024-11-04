package statemachine

import (
	"bytes"
	"fmt"
	"sync"

	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/ChainSafe/gossamer/internal/primitives/storage"
	"github.com/ChainSafe/gossamer/internal/primitives/trie"
	"github.com/ChainSafe/gossamer/internal/primitives/trie/cache"
	"github.com/ChainSafe/gossamer/internal/primitives/trie/recorder"
	"github.com/ChainSafe/gossamer/pkg/scale"
	triedb "github.com/ChainSafe/gossamer/pkg/trie/triedb"
)

// pub trait TrieCacheProvider<H: Hasher> {
type TrieCacheProvider[H runtime.Hash, Cache any] interface {
	// 	/// Cache type that implements [`trie_db::TrieCache`].
	// 	type Cache<'a>: TrieCacheT<sp_trie::NodeCodec<H>> + 'a
	// 	where
	// 		Self: 'a;

	// 	/// Return a [`trie_db::TrieDB`] compatible cache.
	// 	///
	// 	/// The `storage_root` parameter *must* be the storage root of the trie this cache is used for.
	// 	///
	// 	/// NOTE: Implementors should use the `storage_root` to differentiate between storage keys that
	// 	/// may belong to different tries.
	// 	fn as_trie_db_cache(&self, storage_root: H::Out) -> Self::Cache<'_>;
	TrieCache(storageRoot H) (cache Cache, unlock func())

	// 	/// Returns a cache that can be used with a [`trie_db::TrieDBMut`].
	// 	///
	// 	/// When finished with the operation on the trie, it is required to call [`Self::merge`] to
	// 	/// merge the cached items for the correct `storage_root`.
	// 	fn as_trie_db_mut_cache(&self) -> Self::Cache<'_>;
	TrieCacheMut() (cache Cache, unlock func())

	// /// Merge the cached data in `other` into the provider using the given `new_root`.
	// ///
	// /// This must be used for the cache returned by [`Self::as_trie_db_mut_cache`] as otherwise the
	// /// cached data is just thrown away.
	// fn merge<'a>(&'a self, other: Self::Cache<'a>, new_root: H::Out);
	Merge(other Cache, newRoot H)
}

type cachedIter[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	lastKey []byte
	iter    RawIter[H, Hasher]
}

// / Patricia trie-based backend. Transaction type is an overlay of changes to commit.
// pub struct TrieBackend<S: TrieBackendStorage<H>, H: Hasher, C = DefaultCache<H>> {
type TrieBackend[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	// pub(crate) essence: TrieBackendEssence<S, H, C>,
	essence TrieBackendEssence[H, Hasher]
	// next_storage_key_cache: CacheCell<Option<CachedIter<S, H, C>>>,
	nextStorageKeyCache    *cachedIter[H, Hasher]
	nextStorageKeyCacheMtx sync.Mutex
}

func NewTrieBackend[H runtime.Hash, Hasher runtime.Hasher[H]](
	storage TrieBackendStorage[H],
	root H,
	cache TrieCacheProvider[H, *cache.TrieCache[H]],
	recorder *recorder.Recorder[H],
) *TrieBackend[H, Hasher] {
	return &TrieBackend[H, Hasher]{
		essence: newTrieBackendEssence[H, Hasher](storage, root, cache, recorder),
	}
}

// / Wrap the given [`TrieBackend`].
// /
// / This can be used for example if all accesses to the trie should
// / be recorded while some other functionality still uses the non-recording
// / backend.
// /
// / The backend storage and the cache will be taken from `other`.
func NewWrappedTrieBackend[H runtime.Hash, Hasher runtime.Hasher[H]](
	other *TrieBackend[H, Hasher],
) *TrieBackend[H, Hasher] {
	return NewTrieBackend[H, Hasher](
		other.essence.BackendStorage(),
		other.essence.root,
		other.essence.trieNodeCache,
		nil,
	)
}

// / Create a backend used for checking the proof, using `H` as hasher.
// /
// / `proof` and `root` must match, i.e. `root` must be the correct root of `proof` nodes.
func NewProofCheckTrieBackend[H runtime.Hash, Hasher runtime.Hasher[H]](
	root H, proof trie.StorageProof,
) (*TrieBackend[H, Hasher], error) {
	db := trie.ToMemoryDB[H, Hasher](proof)

	if db.Contains(root, hashdb.EmptyPrefix) {
		return NewTrieBackend[H, Hasher](HashDBTrieBackendStorage[H]{db}, root, nil, nil), nil
	}
	return nil, fmt.Errorf("invalid execution proof")
}

func (tb *TrieBackend[H, Hasher]) Storage(key []byte) (StorageValue, error) {
	return tb.essence.Storage(key)
}

func (tb *TrieBackend[H, Hasher]) StorageHash(key []byte) (*H, error) {
	return tb.essence.StorageHash(key)
}

func (tb *TrieBackend[H, Hasher]) ChildStorage(childInfo storage.ChildInfo, key []byte) (StorageValue, error) {
	return tb.essence.ChildStorage(childInfo, key)
}

func (tb *TrieBackend[H, Hasher]) ChildStorageHash(childInfo storage.ChildInfo, key []byte) (*H, error) {
	return tb.essence.ChildStorageHash(childInfo, key)
}

func (tb *TrieBackend[H, Hasher]) ClosestMerkleValue(key []byte) (triedb.MerkleValue[H], error) {
	return tb.essence.ClosestMerkleValue(key)
}

func (tb *TrieBackend[H, Hasher]) ChildClosestMerkleValue(childInfo storage.ChildInfo, key []byte) (triedb.MerkleValue[H], error) {
	return tb.essence.ChildClosestMerkleValue(childInfo, key)
}

func (tb *TrieBackend[H, Hasher]) ExistsStorage(key []byte) (bool, error) {
	h, err := tb.StorageHash(key)
	if err != nil {
		return false, err
	}
	if h != nil {
		return true, nil
	}
	return false, nil
}

func (tb *TrieBackend[H, Hasher]) ExistsChildStorage(childInfo storage.ChildInfo, key []byte) (bool, error) {
	h, err := tb.ChildStorageHash(childInfo, key)
	if err != nil {
		return false, err
	}
	if h != nil {
		return true, nil
	}
	return false, nil
}

func (tb *TrieBackend[H, Hasher]) NextStorageKey(key []byte) (StorageKey, error) {
	var isCached bool
	tb.nextStorageKeyCacheMtx.Lock()
	defer tb.nextStorageKeyCacheMtx.Unlock()
	var cache *cachedIter[H, Hasher]
	if tb.nextStorageKeyCache != nil {
		isCached = bytes.Equal(tb.nextStorageKeyCache.lastKey, key)
	} else {
		tb.nextStorageKeyCache = &cachedIter[H, Hasher]{}
	}
	cache = tb.nextStorageKeyCache

	if !isCached {
		iter, err := tb.essence.RawIter(IterArgs{
			StartAt:          key,
			StartAtExclusive: true,
		})
		if err != nil {
			return nil, err
		}
		cache.iter = *iter
		tb.nextStorageKeyCache = cache
	}

	nextKey, err := cache.iter.NextKey(tb)
	if err != nil {
		return nil, err
	}
	if nextKey == nil {
		return nil, nil
	}

	// cache.lastKey = nil
	cache.lastKey = nextKey

	return nextKey, nil
}

func (tb *TrieBackend[H, Hasher]) NextChildStorageKey(childInfo storage.ChildInfo, key []byte) (StorageKey, error) {
	return tb.essence.NextChildStorageKey(childInfo, key)
}

func (tb *TrieBackend[H, Hasher]) RawIter(args IterArgs) (StorageIterator[H, Hasher], error) {
	return tb.essence.RawIter(args)
}

func (tb *TrieBackend[H, Hasher]) StorageRoot(delta []struct {
	Key   []byte
	Value []byte
}, stateVersion storage.StateVersion) (H, BackendTransaction[H, Hasher]) {
	h, pmdb := tb.essence.StorageRoot(delta, stateVersion)
	return h, BackendTransaction[H, Hasher]{pmdb}
}

func (tb *TrieBackend[H, Hasher]) ChildStorageRoot(childInfo storage.ChildInfo, delta []struct {
	Key   []byte
	Value []byte
}, stateVersion storage.StateVersion) (H, bool, BackendTransaction[H, Hasher]) {
	h, b, pmdb := tb.essence.ChildStorageRoot(childInfo, delta, stateVersion)
	return h, b, BackendTransaction[H, Hasher]{pmdb}
}

func (tb *TrieBackend[H, Hasher]) Pairs(args IterArgs) (PairsIter[H, Hasher], error) {
	rawIter, err := tb.RawIter(args)
	if err != nil {
		return PairsIter[H, Hasher]{}, err
	}
	return PairsIter[H, Hasher]{
		backend: tb,
		rawIter: rawIter,
	}, nil
}

func (tb *TrieBackend[H, Hasher]) Keys(args IterArgs) (KeysIter[H, Hasher], error) {
	rawIter, err := tb.RawIter(args)
	if err != nil {
		return KeysIter[H, Hasher]{}, err
	}
	return KeysIter[H, Hasher]{
		backend: tb,
		rawIter: rawIter,
	}, nil
}

func (tb *TrieBackend[H, Hasher]) FullStorageRoot(
	delta []struct {
		Key   []byte
		Value []byte
	},
	childDeltas []struct {
		storage.ChildInfo
		Delta []struct {
			Key   []byte
			Value []byte
		}
	},
	stateVersion storage.StateVersion,
) (H, BackendTransaction[H, Hasher]) {
	type ChildRoot struct {
		StorageKey       []byte
		EncodedChildRoot []byte
	}
	var childRoots []ChildRoot
	var txs BackendTransaction[H, Hasher] = NewBackendTransaction[H, Hasher]()

	// child first
	for _, cd := range childDeltas {
		childInfo := cd.ChildInfo
		childDelta := cd.Delta

		childRoot, empty, childTxs := tb.ChildStorageRoot(childInfo, childDelta, stateVersion)
		prefixedStorageKey := childInfo.PrefixedStorageKey()

		txs.Consolidate(&childTxs.MemoryDB)
		if empty {
			childRoots = append(childRoots, ChildRoot{
				StorageKey: prefixedStorageKey,
			})
		} else {
			childRoots = append(childRoots, ChildRoot{
				StorageKey:       prefixedStorageKey,
				EncodedChildRoot: scale.MustMarshal(childRoot),
			})
		}
	}

	chainedDelta := delta
	for _, cr := range childRoots {
		chainedDelta = append(chainedDelta, struct {
			Key   []byte
			Value []byte
		}{
			Key:   cr.StorageKey,
			Value: cr.EncodedChildRoot,
		})
	}
	root, parentTxs := tb.StorageRoot(chainedDelta, stateVersion)
	txs.Consolidate(&parentTxs.MemoryDB)

	return root, txs
}

func (tb *TrieBackend[H, Hasher]) ExtractProof() *trie.StorageProof {
	recorder := tb.essence.recorder
	tb.essence.recorder = nil
	if recorder != nil {
		proof := recorder.DrainStorageProof()
		return &proof
	}
	return nil
}
