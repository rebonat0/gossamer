package wrapper

import (
	"fmt"

	"github.com/ChainSafe/gossamer/internal/database"
	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
)

type hashConstructor[H runtime.Hash] func(b []byte) H

type Wrapper[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	hashdb.HashDB[H, []byte]
}

func New[H runtime.Hash, Hasher runtime.Hasher[H]](db hashdb.HashDB[H, []byte]) *Wrapper[H, Hasher] {
	return &Wrapper[H, Hasher]{
		HashDB: db,
	}
}

func (mdbw *Wrapper[H, Hasher]) Get(key []byte) (value []byte, err error) {
	hash := (*(new(Hasher))).Hash(key)
	val := mdbw.HashDB.Get(hash, hashdb.EmptyPrefix)
	if val == nil {
		return nil, fmt.Errorf("missing value")
	}
	value = []byte(*val)
	return value, nil
}
func (mdbw *Wrapper[H, Hasher]) Put(key, value []byte) error {
	k := mdbw.HashDB.Insert(hashdb.EmptyPrefix, value)
	if string(k.Bytes()) != string(key) {
		panic("huh? should be the same")
	}
	return nil
}
func (mdbw *Wrapper[H, Hasher]) Del(key []byte) error {
	mdbw.HashDB.Remove((*(new(Hasher))).Hash(key), hashdb.EmptyPrefix)
	return nil
}
func (mdbw *Wrapper[H, Hasher]) Flush() error {
	return nil
}

type MemoryBatch[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	*Wrapper[H, Hasher]
}

func (b *MemoryBatch[H, Hasher]) Close() error {
	return nil
}

func (*MemoryBatch[H, Hasher]) Reset() {}

func (b *MemoryBatch[H, Hasher]) ValueSize() int {
	return 1
}
func (mdbw *Wrapper[H, Hasher]) NewBatch() database.Batch {
	return &MemoryBatch[H, Hasher]{
		Wrapper: mdbw,
	}
}
