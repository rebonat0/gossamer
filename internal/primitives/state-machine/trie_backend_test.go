package statemachine

import (
	"github.com/ChainSafe/gossamer/internal/primitives/core/hash"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
)

var _ StorageIterator[hash.H256, runtime.BlakeTwo256] = &RawIter[hash.H256, runtime.BlakeTwo256]{}
