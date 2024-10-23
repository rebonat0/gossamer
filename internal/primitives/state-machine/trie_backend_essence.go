package statemachine

import (
	"bytes"
	"log"
	"slices"
	"sync"

	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/ChainSafe/gossamer/internal/primitives/storage"
	"github.com/ChainSafe/gossamer/internal/primitives/trie"
	"github.com/ChainSafe/gossamer/internal/primitives/trie/recorder"
	ptrie "github.com/ChainSafe/gossamer/pkg/trie"
	triedb "github.com/ChainSafe/gossamer/pkg/trie/triedb"
	"golang.org/x/exp/constraints"
)

type IterState uint

const (
	Pending IterState = iota
	FinishedComplete
	FinishedIncomplete
)

// / A raw iterator over the storage.
type RawIter[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	stopOnImcompleteDatabase bool
	skipIfFirst              *StorageKey
	root                     H
	childInfo                storage.ChildInfo
	trieIter                 triedb.TrieDBRawIterator[H, Hasher]
	state                    IterState
}

func prepare[H runtime.Hash, Hasher runtime.Hasher[H], R any](
	ri *RawIter[H, Hasher],
	backend *TrieBackendEssence[H, Hasher],
	callback func(*triedb.TrieDB[H, Hasher], triedb.TrieDBRawIterator[H, Hasher]) (*R, error),
) (*R, error) {
	if ri.state != Pending {
		return nil, nil
	}

	var result *R
	var err error
	withTrieDB[H, Hasher](backend, ri.root, ri.childInfo, func(db *triedb.TrieDB[H, Hasher]) {
		result, err = callback(db, ri.trieIter)
	})
	if err != nil {
		ri.state = FinishedIncomplete
		// if matches!(*error, TrieError::IncompleteDatabase(_)) &&
		// 			self.stop_on_incomplete_database
		// 		{
		// 			None
		// 		} else {
		// 			Some(Err(format!("TrieDB iteration error: {}", error)))
		// 		}
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	ri.state = FinishedComplete
	return nil, nil
}

func (ri *RawIter[H, Hasher]) NextKey(backend *TrieBackend[H, Hasher]) (StorageKey, error) {
	skipIfFirst := ri.skipIfFirst
	ri.skipIfFirst = nil

	key, err := prepare[H, Hasher, []byte](
		ri,
		&backend.Essence,
		func(trie *triedb.TrieDB[H, Hasher], trieIter triedb.TrieDBRawIterator[H, Hasher]) (*[]byte, error) {
			result, err := trieIter.NextKey()
			if err != nil {
				return nil, err
			}
			if skipIfFirst != nil {
				if result != nil {
					if slices.Equal(result, *skipIfFirst) {
						result, err = trieIter.NextKey()
						if err != nil {
							return nil, err
						}
					}
				}
			}
			return &result, nil
		})

	if key == nil {
		return nil, err
	}
	var storageKey StorageKey
	storageKey = StorageKey(*key)
	return storageKey, nil
}

func (ri *RawIter[H, Hasher]) NextKeyValue(backend *TrieBackend[H, Hasher]) (*StorageKeyValue, error) {
	skipIfFirst := ri.skipIfFirst
	ri.skipIfFirst = nil

	pair, err := prepare[H, Hasher, StorageKeyValue](
		ri,
		&backend.Essence,
		func(trie *triedb.TrieDB[H, Hasher], trieIter triedb.TrieDBRawIterator[H, Hasher]) (*StorageKeyValue, error) {
			result, err := trieIter.NextItem()
			if err != nil {
				return nil, err
			}
			if skipIfFirst != nil {
				if result != nil {
					if bytes.Equal(result.Key, *skipIfFirst) {
						result, err = trieIter.NextItem()
						if err != nil {
							return nil, err
						}
					}
				}
			}
			return &StorageKeyValue{result.Key, result.Value}, nil
		})
	return pair, err
}

func (ri *RawIter[H, Hasher]) Complete() bool {
	return ri.state == FinishedComplete
}

// / Patricia trie-based pairs storage essence.
// pub struct TrieBackendEssence<S: TrieBackendStorage<H>, H: Hasher, C> {
type TrieBackendEssence[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	storage TrieBackendStorage[H]
	root    H
	empty   H
	cache   struct {
		childRoot map[string]*H
		sync.RWMutex
	}
	// pub(crate) trie_node_cache: Option<C>,
	trieNodeCache TrieCacheProvider[H]
	// #[cfg(feature = "std")]
	// pub(crate) recorder: Option<Recorder<H>>,
	recorder *recorder.Recorder[H]
}

func newTrieBackendEssence[H runtime.Hash, Hasher runtime.Hasher[H]](
	storage TrieBackendStorage[H],
	root H,
	cache TrieCacheProvider[H],
	recorder *recorder.Recorder[H],
) TrieBackendEssence[H, Hasher] {
	return TrieBackendEssence[H, Hasher]{
		storage: storage,
		root:    root,
		cache: struct {
			childRoot map[string]*H
			sync.RWMutex
		}{childRoot: make(map[string]*H)},
		trieNodeCache: cache,
		recorder:      recorder,
	}
}

// / Get backend storage reference.
func (tbe *TrieBackendEssence[H, Hasher]) BackendStorage() TrieBackendStorage[H] {
	return tbe.storage
}

// /// Get backend storage mutable reference.
// pub fn backend_storage_mut(&mut self) -> &mut S {
// 	&mut self.storage
// }

// / Get trie root.
func (tbe *TrieBackendEssence[H, Hasher]) Root() H {
	return tbe.root
}

// / Set trie root. This is useful for testing.
func (tbe *TrieBackendEssence[H, Hasher]) SetRoot(root H) {
	tbe.resetCache()
	tbe.root = root
}

func (tbe *TrieBackendEssence[H, Hasher]) resetCache() {
	tbe.cache = struct {
		childRoot map[string]*H
		sync.RWMutex
	}{childRoot: make(map[string]*H)}
}

// /// Consumes self and returns underlying storage.
// pub fn into_storage(self) -> S {
// 	self.storage
// }

func withRecorderAndCache[H runtime.Hash, Hasher runtime.Hasher[H]](
	tbe *TrieBackendEssence[H, Hasher],
	storageRoot *H,
	// TODO: try and remove return params on callback
	callback func(triedb.TrieRecorder, triedb.TrieCache[H]),
) {
	root := tbe.root
	if storageRoot != nil {
		root = *storageRoot
	}

	var cache triedb.TrieCache[H]
	if tbe.trieNodeCache != nil {
		cache = tbe.trieNodeCache.TrieCache(root)
	}

	var recorder triedb.TrieRecorder
	if tbe.recorder != nil {
		recorder = tbe.recorder.TrieRecorder(root)
	}
	callback(recorder, cache)
}

// / Call the given closure passing it the recorder and the cache.
// /
// / This function must only be used when the operation in `callback` is
// / calculating a `storage_root`. It is expected that `callback` returns
// / the new storage root. This is required to register the changes in the cache
// / for the correct storage root. The given `storage_root` corresponds to the root of the "old"
// / trie. If the value is not given, `self.root` is used.
func withRecorderAndCacheForStorageRoot[H runtime.Hash, Hasher runtime.Hasher[H], R any](
	tbe *TrieBackendEssence[H, Hasher],
	storageRoot *H,
	callback func(triedb.TrieRecorder, triedb.TrieCache[H]) (*H, R),
) R {
	root := tbe.root
	if storageRoot != nil {
		root = *storageRoot
	}

	var recorder triedb.TrieRecorder
	if tbe.recorder != nil {
		recorder = tbe.recorder.TrieRecorder(root)
	}

	if tbe.trieNodeCache != nil {
		cache := tbe.trieNodeCache.NewTrieCache()
		newRoot, r := callback(recorder, cache)
		if newRoot != nil {
			tbe.trieNodeCache.Merge(cache, *newRoot)
		}
		return r
	} else {
		_, r := callback(recorder, nil)
		return r
	}
}

// / Calls the given closure with a [`TrieDb`] constructed for the given
// / storage root and (optionally) child trie.
func withTrieDB[H runtime.Hash, Hasher runtime.Hasher[H]](
	tbe *TrieBackendEssence[H, Hasher],
	root H,
	childInfo storage.ChildInfo,
	callback func(*triedb.TrieDB[H, Hasher]),
) {
	var backend hashdb.HashDB[H] = tbe
	var db hashdb.HashDB[H] = tbe
	if childInfo != nil {
		db = trie.NewKeySpacedDB[H](backend, childInfo.Keyspace())
	}

	withRecorderAndCache(tbe, &root, func(recorder triedb.TrieRecorder, cache triedb.TrieCache[H]) {
		trieDB := triedb.NewTrieDB[H, Hasher](root, db, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
		trieDB.SetVersion(ptrie.V1)
		callback(trieDB)
	})
}

// / Access the root of the child storage in its parent trie
func (tbe *TrieBackendEssence[H, Hasher]) childRoot(childInfo storage.ChildInfo) (*H, error) {
	tbe.cache.RLock()
	{
		result, ok := tbe.cache.childRoot[string(childInfo.StorageKey())]
		tbe.cache.RUnlock()
		if ok {
			return result, nil
		}
	}

	r, err := tbe.Storage(childInfo.PrefixedStorageKey())
	if err != nil {
		return nil, err
	}
	var result *H
	if r != nil {
		h := (*(new(Hasher))).NewHash(r)
		result = &h
	}
	tbe.cache.Lock()
	defer tbe.cache.Unlock()
	tbe.cache.childRoot[string(childInfo.StorageKey())] = result

	return result, nil
}

// / Returns the hash value
func (tbe *TrieBackendEssence[H, Hasher]) StorageHash(key []byte) (hash *H, err error) {
	withRecorderAndCache[H, Hasher](tbe, nil, func(recorder triedb.TrieRecorder, cache triedb.TrieCache[H]) {
		trieDB := triedb.NewTrieDB[H, Hasher](tbe.root, tbe, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
		trieDB.SetVersion(ptrie.V1)
		hash, err = trieDB.GetHash(key)
	})
	return
}

// / Get the value of storage at given key.
func (tbe *TrieBackendEssence[H, Hasher]) Storage(key []byte) (val StorageValue, err error) {
	withRecorderAndCache(tbe, nil, func(recorder triedb.TrieRecorder, cache triedb.TrieCache[H]) {
		val, err = trie.ReadTrieValue[H, Hasher](tbe, tbe.root, key, recorder, cache)
	})
	return
}

// / Create a raw iterator over the storage.
func (tbe *TrieBackendEssence[H, Hasher]) RawIter(args IterArgs) (RawIter[H, Hasher], error) {
	var root H
	if args.ChildInfo != nil {
		childRoot, err := tbe.childRoot(args.ChildInfo)
		if err != nil {
			return RawIter[H, Hasher]{}, err
		}
		if childRoot != nil {
			root = *childRoot
		} else {
			return RawIter[H, Hasher]{
				state: FinishedComplete,
			}, nil
		}
	}

	if root == (*new(H)) {
		return RawIter[H, Hasher]{
			state: FinishedComplete,
		}, nil
	}

	var trieIter *triedb.TrieDBRawIterator[H, Hasher]
	var err error
	withTrieDB[H, Hasher](tbe, root, args.ChildInfo, func(db *triedb.TrieDB[H, Hasher]) {
		var prefix []byte
		if args.Prefix != nil {
			prefix = *args.Prefix
		}

		if args.StartAt != nil {
			trieIter, err = triedb.NewPrefixedTrieDBRawIteratorThenSeek[H, Hasher](db, prefix, *args.StartAt)
		} else {
			trieIter, err = triedb.NewPrefixedTrieDBRawIterator[H, Hasher](db, prefix)
		}
	})
	if err != nil {
		return RawIter[H, Hasher]{}, err
	}

	var skipIfFirst *StorageKey
	if args.StartAtExclusive {
		if args.StartAt != nil {
			storageKey := StorageKey(*args.StartAt)
			skipIfFirst = &storageKey
		}
	}

	return RawIter[H, Hasher]{
		stopOnImcompleteDatabase: args.StopOnIncompleteDatabase,
		skipIfFirst:              skipIfFirst,
		childInfo:                args.ChildInfo,
		root:                     root,
		trieIter:                 *trieIter,
		state:                    Pending,
	}, nil
}

func (tbe *TrieBackendEssence[H, Hasher]) StorageRoot(delta []struct {
	Key   []byte
	Value []byte
}, stateVersion storage.StateVersion) (H, *trie.PrefixedMemoryDB[H, Hasher]) {
	writeOverlay := trie.NewPrefixedMemoryDB[H, Hasher]()

	root := withRecorderAndCacheForStorageRoot[H, Hasher, H](tbe, nil, func(recorder triedb.TrieRecorder, cache triedb.TrieCache[H]) (*H, H) {
		eph := newEphemeral[H, Hasher](tbe.BackendStorage(), writeOverlay)
		root, err := trie.DeltaTrieRoot[H, Hasher](eph, tbe.root, delta, recorder, cache, stateVersion)
		if err != nil {
			log.Printf("WARN: failed to write to trie: %v", err)
			return nil, tbe.root
		}
		return &root, root
	})

	return root, writeOverlay
}

func (tbe *TrieBackendEssence[H, Hasher]) Get(key H, prefix hashdb.Prefix) []byte {
	if key == tbe.empty {
		val := []byte{0}
		return val
	}
	val, err := tbe.storage.Get(key, prefix)
	if err != nil {
		log.Printf("WARN: failed to write to trie: %v\n", err)
		return nil
	}
	return val
}

func (tbe *TrieBackendEssence[H, Hasher]) Contains(key H, prefix hashdb.Prefix) bool {
	return tbe.Get(key, prefix) != nil
}

func (tbe *TrieBackendEssence[H, Hasher]) Insert(prefix hashdb.Prefix, value []byte) H {
	panic("unimplemented")
}

func (tbe *TrieBackendEssence[H, Hasher]) Emplace(key H, prefix hashdb.Prefix, value []byte) {
	panic("unimplemented")
}

func (tbe *TrieBackendEssence[H, Hasher]) Remove(key H, prefix hashdb.Prefix) {
	panic("unimplemented")
}

type ephemeral[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	storage TrieBackendStorage[H]
	overlay *trie.PrefixedMemoryDB[H, Hasher]
}

func newEphemeral[H runtime.Hash, Hasher runtime.Hasher[H]](
	storage TrieBackendStorage[H],
	overlay *trie.PrefixedMemoryDB[H, Hasher],
) *ephemeral[H, Hasher] {
	return &ephemeral[H, Hasher]{
		storage, overlay,
	}
}

func (e *ephemeral[H, Hasher]) Get(key H, prefix hashdb.Prefix) []byte {
	val := e.overlay.Get(key, prefix)
	if val == nil {
		val, err := e.storage.Get(key, prefix)
		if err != nil {
			log.Printf("WARN: failed to read from DB: %v\n", err)
			return nil
		}
		return val
	}
	return val
}

func (e *ephemeral[H, Hasher]) Contains(key H, prefix hashdb.Prefix) bool {
	return e.Get(key, prefix) != nil
}

func (e *ephemeral[H, Hasher]) Insert(prefix hashdb.Prefix, value []byte) H {
	return e.overlay.Insert(prefix, value)
}

func (e *ephemeral[H, Hasher]) Emplace(key H, prefix hashdb.Prefix, value []byte) {
	e.overlay.Emplace(key, prefix, value)
}

func (e *ephemeral[H, Hasher]) Remove(key H, prefix hashdb.Prefix) {
	e.overlay.Remove(key, prefix)
}

// / Key-value pairs storage that is used by trie backend essence.
type TrieBackendStorage[H constraints.Ordered] interface {
	Get(key H, prefix hashdb.Prefix) ([]byte, error)
}
