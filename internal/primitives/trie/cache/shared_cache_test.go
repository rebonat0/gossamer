package cache

import (
	"bytes"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/internal/primitives/core/hash"
	"github.com/ChainSafe/gossamer/pkg/trie/triedb"
	"github.com/stretchr/testify/require"
)

func Test_SharedValueCache(t *testing.T) {
	cache := NewSharedValueCache[hash.H256](110)

	key := bytes.Repeat([]byte{0}, 10)
	root0 := hash.NewRandomH256()
	root1 := hash.NewRandomH256()

	vck0 := ValueCacheKey[hash.H256]{
		StorageRoot: root0,
		StorageKey:  key,
	}
	vck1 := ValueCacheKey[hash.H256]{
		StorageRoot: root1,
		StorageKey:  key,
	}

	cache.Update([]SharedValueCacheAdded[hash.H256]{
		{
			ValueCacheKey: vck0,
			CachedValue:   triedb.NonExistingCachedValue[hash.H256]{},
		},
		{
			ValueCacheKey: vck1,
			CachedValue:   triedb.NonExistingCachedValue[hash.H256]{},
		},
	}, nil)

	// Ensure that the basics are working
	require.Equal(t, 2, cache.cache.Size())

	// Just accessing a key should not change anything on the size and number of entries.
	cache.Update(nil, []ValueCacheKeyHash[hash.H256]{
		vck0.ValueCacheKeyHash(),
	})
	require.Equal(t, 2, cache.cache.Size())

	// Updating the cache again with exactly the same data should not change anything.
	cache.Update([]SharedValueCacheAdded[hash.H256]{
		{
			ValueCacheKey: vck0,
			CachedValue:   triedb.NonExistingCachedValue[hash.H256]{},
		},
		{
			ValueCacheKey: vck1,
			CachedValue:   triedb.NonExistingCachedValue[hash.H256]{},
		},
	}, nil)
	require.Equal(t, 2, cache.cache.Size())

	var added []SharedValueCacheAdded[hash.H256]
	// Add 10 other entries and this should move out two of the initial entries.
	for i := 1; i < 11; i++ {
		added = append(added, SharedValueCacheAdded[hash.H256]{
			ValueCacheKey: ValueCacheKey[hash.H256]{
				StorageRoot: root0,
				StorageKey:  bytes.Repeat([]byte{uint8(i)}, 10),
			},
			CachedValue: triedb.NonExistingCachedValue[hash.H256]{},
		})
	}
	cache.Update(added, nil)

	// Sleep a little for eviction
	time.Sleep(5 * time.Millisecond)
	require.Equal(t, 10, cache.cache.Size())

	val, ok := cache.cache.Extension().GetQuietly(ValueCacheKey[hash.H256]{
		StorageRoot: root0,
		StorageKey:  bytes.Repeat([]byte{1}, 10),
	}.ValueCacheKeyHash())
	require.True(t, ok)
	require.Equal(t, triedb.NonExistingCachedValue[hash.H256]{}, val)

	// this was never inserted
	val, ok = cache.cache.Extension().GetQuietly(ValueCacheKey[hash.H256]{
		StorageRoot: root1,
		StorageKey:  bytes.Repeat([]byte{1}, 10),
	}.ValueCacheKeyHash())
	require.False(t, ok)

	val, ok = cache.cache.Extension().GetQuietly(vck0.ValueCacheKeyHash())
	require.False(t, ok)
	val, ok = cache.cache.Extension().GetQuietly(vck1.ValueCacheKeyHash())
	require.False(t, ok)

	// cache.Update([]SharedValueCacheAdded[hash.H256]{
	// 	{
	// 		ValueCacheKey: ValueCacheKey[hash.H256]{
	// 			StorageRoot: root0,
	// 			StorageKey:  bytes.Repeat([]byte{10}, 10),
	// 		},
	// 		CachedValue: triedb.NonExistingCachedValue[hash.H256]{},
	// 	},
	// }, nil)
}
