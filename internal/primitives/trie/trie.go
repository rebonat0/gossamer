package trie

import (
	"slices"

	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	memorydb "github.com/ChainSafe/gossamer/internal/memory-db"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	triedb "github.com/ChainSafe/gossamer/pkg/trie/triedb"
)

// / Reexport from `hash_db`, with genericity set for `Hasher` trait.
// / This uses a `KeyFunction` for prefixing keys internally (avoiding
// / key conflict for non random keys).
// pub type PrefixedMemoryDB<H> = memory_db::MemoryDB<H, memory_db::PrefixedKey<H>, trie_db::DBValue>;
type PrefixedMemoryDB[Hash runtime.Hash, Hasher hashdb.Hasher[Hash]] struct {
	memorydb.MemoryDB[Hash, Hasher, string, memorydb.PrefixedKey[Hash]]
}

func NewPrefixedMemoryDB[Hash runtime.Hash, Hasher hashdb.Hasher[Hash]]() *PrefixedMemoryDB[Hash, Hasher] {
	return &PrefixedMemoryDB[Hash, Hasher]{
		memorydb.NewMemoryDB[Hash, Hasher, string, memorydb.PrefixedKey[Hash]]([]byte{0}),
	}
}

// / Reexport from `hash_db`, with genericity set for `Hasher` trait.
// / This uses a noops `KeyFunction` (key addressing must be hashed or using
// / an encoding scheme that avoid key conflict).
// pub type MemoryDB<H> = memory_db::MemoryDB<H, memory_db::HashKey<H>, trie_db::DBValue>;
type MemoryDB[Hash runtime.Hash, Hasher runtime.Hasher[Hash]] struct {
	memorydb.MemoryDB[Hash, Hasher, Hash, memorydb.HashKey[Hash]]
}

func NewMemoryDB[Hash runtime.Hash, Hasher runtime.Hasher[Hash]]() *MemoryDB[Hash, Hasher] {
	return &MemoryDB[Hash, Hasher]{
		MemoryDB: memorydb.NewMemoryDB[Hash, Hasher, Hash, memorydb.HashKey[Hash]]([]byte{0}),
	}
}

// pub type TrieHash<L> = <<L as TrieLayout>::Hash as Hasher>::Out;

// / Builder for creating a [`TrieDB`].
// pub type TrieDBBuilder<'a, 'cache, L> = trie_db::TrieDBBuilder<'a, 'cache, L>;
// type TrieDBBuilder[Hash, DB, Cache, Layout any] triedb.TrieDBBuilder[Hash, DB, Cache, Layout]

// / Determine a trie root given a hash DB and delta values.
func DeltaTrieRoot[H runtime.Hash, Hasher runtime.Hasher[H]](
	db hashdb.HashDB[H],
	root H,
	delta []struct {
		Key   []byte
		Value []byte
	},
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) (H, error) {
	trieDB := triedb.NewTrieDB[H, Hasher](root, db, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)

	slices.SortStableFunc(delta, func(a struct {
		Key   []byte
		Value []byte
	}, b struct {
		Key   []byte
		Value []byte
	}) int {
		if string(a.Key) < string(b.Key) {
			return -1
		} else if string(a.Key) == string(b.Key) {
			return 0
		} else {
			return 1
		}
	})

	for i, kv := range delta {
		_ = i
		if kv.Value != nil {
			err := trieDB.Put(kv.Key, kv.Value)
			if err != nil {
				return *(new(H)), err
			}
		} else {
			err := trieDB.Delete(kv.Key)
			if err != nil {
				return *(new(H)), err
			}
		}
	}

	hash, err := trieDB.Hash()
	return hash, err
}

// / Read a value from the trie.
func ReadTrieValue[H runtime.Hash, Hasher runtime.Hasher[H]](
	db hashdb.HashDB[H],
	root H,
	key []byte,
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) ([]byte, error) {
	trieDB := triedb.NewTrieDB[H, Hasher](root, db, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)
	b, err := triedb.GetWith(trieDB, key, func(data []byte) []byte { return data })
	if err != nil {
		return nil, err
	}
	if b != nil {
		return *b, nil
	}
	return nil, nil
}

// / Read a value from the trie with given Query.
func ReadTrieValueWith[H runtime.Hash, Hasher runtime.Hasher[H]](
	db hashdb.HashDB[H],
	root H,
	key []byte,
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
	query triedb.Query[[]byte],
) ([]byte, error) {
	trieDB := triedb.NewTrieDB[H, Hasher](root, db, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)
	b, err := triedb.GetWith[H, Hasher, []byte](trieDB, key, query)
	if err != nil {
		return nil, err
	}
	if b != nil {
		return *b, nil
	}
	return nil, nil
}

// / Read the [`trie_db::MerkleValue`] of the node that is the closest descendant for
// / the provided key.
func ReadTrieFirstDescendantValue[H runtime.Hash, Hasher runtime.Hasher[H]](
	db hashdb.HashDB[H],
	root H,
	key []byte,
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) (triedb.MerkleValue[H], error) {
	trieDB := triedb.NewTrieDB(root, db, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)

	return trieDB.LookupFirstDescendant(key)
}

// / Determine the empty trie root.
func EmptyTrieRoot[H runtime.Hash, Hasher runtime.Hasher[H]]() H {
	hasher := *new(Hasher)
	root := hasher.Hash([]byte{0})
	return root
}

// / Determine the empty child trie root.
func EmptyChildTrieRoot[H runtime.Hash, Hasher runtime.Hasher[H]]() H {
	return EmptyTrieRoot[H, Hasher]()
}

// / Determine a child trie root given a hash DB and delta values. H is the default hasher,
// / but a generic implementation may ignore this type parameter and use other hashers.
func ChildDeltaTrieRoot[H runtime.Hash, Hasher runtime.Hasher[H]](
	keyspace []byte,
	db hashdb.HashDB[H],
	root H,
	delta []struct {
		Key   []byte
		Value []byte
	},
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) (H, error) {
	ksdb := NewKeySpacedDB(db, keyspace)
	return DeltaTrieRoot[H, Hasher](ksdb, root, delta, recorder, cache, stateVersion)
}

// / Read a value from the child trie.
func ReadChildTrieValue[H runtime.Hash, Hasher runtime.Hasher[H]](
	keyspace []byte,
	db hashdb.HashDB[H],
	root H,
	key []byte,
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) ([]byte, error) {
	ksdb := NewKeySpacedDB[H](db, keyspace)
	trieDB := triedb.NewTrieDB(
		root, ksdb, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)
	val, err := triedb.GetWith[H, Hasher, []byte](trieDB, key, func(data []byte) []byte { return data })
	if err != nil {
		return nil, err
	}
	if val != nil {
		return *val, nil
	}
	return nil, nil
}

// / Read a hash from the child trie.
func ReadChildTrieHash[H runtime.Hash, Hasher runtime.Hasher[H]](
	keyspace []byte,
	db hashdb.HashDB[H],
	root H,
	key []byte,
	recorder triedb.TrieRecorder,
	cache triedb.TrieCache[H],
	stateVersion triedb.TrieLayout,
) (*H, error) {
	ksdb := NewKeySpacedDB[H](db, keyspace)
	trieDB := triedb.NewTrieDB(
		root, ksdb, triedb.WithCache[H, Hasher](cache), triedb.WithRecorder[H, Hasher](recorder))
	trieDB.SetVersion(stateVersion)
	return trieDB.GetHash(key)
}

// / `HashDB` implementation that append a encoded prefix (unique id bytes) in addition to the
// / prefix of every key value.
type KeySpacedDB[Hash comparable] struct {
	db       hashdb.HashDB[Hash]
	keySpace []byte
}

func NewKeySpacedDB[Hash comparable](db hashdb.HashDB[Hash], ks []byte) *KeySpacedDB[Hash] {
	return &KeySpacedDB[Hash]{
		db:       db,
		keySpace: ks,
	}
}

// / Utility function used to merge some byte data (keyspace) and `prefix` data
// / before calling key value database primitives.
func keyspaceAsPrefix(ks []byte, prefix hashdb.Prefix) hashdb.Prefix {
	result := ks
	result = append(result, prefix.Key...)
	return hashdb.Prefix{
		Key:    result,
		Padded: prefix.Padded,
	}
}

func (tbe *KeySpacedDB[H]) Get(key H, prefix hashdb.Prefix) []byte {
	derivedPrefix := keyspaceAsPrefix(tbe.keySpace, prefix)
	return tbe.db.Get(key, derivedPrefix)
}

func (tbe *KeySpacedDB[H]) Contains(key H, prefix hashdb.Prefix) bool {
	derivedPrefix := keyspaceAsPrefix(tbe.keySpace, prefix)
	return tbe.db.Contains(key, derivedPrefix)
}

func (tbe *KeySpacedDB[H]) Insert(prefix hashdb.Prefix, value []byte) H {
	derivedPrefix := keyspaceAsPrefix(tbe.keySpace, prefix)
	h := tbe.db.Insert(derivedPrefix, value)
	return h
}

func (tbe *KeySpacedDB[H]) Emplace(key H, prefix hashdb.Prefix, value []byte) {
	derivedPrefix := keyspaceAsPrefix(tbe.keySpace, prefix)
	tbe.db.Emplace(key, derivedPrefix, value)
}

func (tbe *KeySpacedDB[H]) Remove(key H, prefix hashdb.Prefix) {
	derivedPrefix := keyspaceAsPrefix(tbe.keySpace, prefix)
	tbe.db.Remove(key, derivedPrefix)
}
