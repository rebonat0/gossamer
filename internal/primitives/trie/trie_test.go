package trie

import (
	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	"github.com/ChainSafe/gossamer/internal/primitives/core/hash"
)

var (
	_ hashdb.HashDB[hash.H256] = &KeySpacedDB[hash.H256]{}
)
