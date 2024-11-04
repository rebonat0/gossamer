package trie

import (
	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/tidwall/btree"
)

// / A proof that some set of key-value pairs are included in the storage trie. The proof contains
// / the storage values so that the partial storage backend can be reconstructed by a verifier that
// / does not already have access to the key-value pairs.
// /
// / The proof consists of the set of serialized nodes in the storage trie accessed when looking up
// / the keys covered by the proof. Verifying the proof requires constructing the partial trie from
// / the serialized nodes and performing the key lookups.
// #[derive(Debug, PartialEq, Eq, Clone, Encode, Decode, TypeInfo)]
// pub struct StorageProof {
type StorageProof struct {
	// 	trie_nodes: BTreeSet<Vec<u8>>,
	trieNodes btree.Set[string]
}

func ToMemoryDB[H runtime.Hash, Hasher runtime.Hasher[H]](sp StorageProof) *MemoryDB[H, Hasher] {
	db := NewMemoryDB[H, Hasher]()
	sp.trieNodes.Scan(func(v string) bool {
		db.Insert(hashdb.EmptyPrefix, []byte(v))
		return true
	})
	return db
}

// / Constructs a storage proof from a subset of encoded trie nodes in a storage backend.
func NewStorageProof(trieNodes [][]byte) StorageProof {
	set := btree.Set[string]{}
	for _, trieNode := range trieNodes {
		set.Insert(string(trieNode))
	}
	return StorageProof{
		trieNodes: set,
	}
}

func (sp *StorageProof) Empty() bool {
	return sp.trieNodes.Len() == 0
}

// / Convert into an iterator over encoded trie nodes in lexicographical order constructed
// / from the proof.
func (sp *StorageProof) Nodes() [][]byte {
	var ret [][]byte
	sp.trieNodes.Scan(func(v string) bool {
		ret = append(ret, []byte(v))
		return true
	})
	return ret
}
