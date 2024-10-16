package recorder

import (
	"fmt"
	"testing"

	"github.com/ChainSafe/gossamer/internal/database"
	hashdb "github.com/ChainSafe/gossamer/internal/hash-db"
	memorydb "github.com/ChainSafe/gossamer/internal/memory-db"
	"github.com/ChainSafe/gossamer/internal/primitives/core/hash"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	ptrie "github.com/ChainSafe/gossamer/internal/primitives/trie"
	"github.com/ChainSafe/gossamer/pkg/trie"
	"github.com/ChainSafe/gossamer/pkg/trie/triedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

type hashConstructor[H runtime.Hash] func(b []byte) H
type memoryDBWrapper[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	ptrie.MemoryDB[H, Hasher]
	hashConstructor[H]
}

func newMemoryDBWrapper() *memoryDBWrapper[hash.H256, runtime.BlakeTwo256] {
	return &memoryDBWrapper[hash.H256, runtime.BlakeTwo256]{
		hashConstructor: func(b []byte) hash.H256 {
			return hash.H256(b)
		},
		MemoryDB: *ptrie.NewMemoryDB[hash.H256, runtime.BlakeTwo256](),
	}
}

func (mdbw *memoryDBWrapper[H, Hasher]) Get(key []byte) (value []byte, err error) {
	hash := mdbw.hashConstructor(key)
	val := mdbw.MemoryDB.Get(hash, hashdb.EmptyPrefix)
	if val == nil {
		return nil, fmt.Errorf("missing value")
	}
	value = val
	return value, nil
}
func (mdbw *memoryDBWrapper[H, Hasher]) Put(key, value []byte) error {
	k := mdbw.MemoryDB.Insert(hashdb.EmptyPrefix, value)
	if string(k.Bytes()) != string(key) {
		panic("huh? should be the same")
	}
	return nil
}
func (mdbw *memoryDBWrapper[H, Hasher]) Del(key []byte) error {
	mdbw.MemoryDB.Remove(mdbw.hashConstructor(key), hashdb.EmptyPrefix)
	return nil
}
func (mdbw *memoryDBWrapper[H, Hasher]) Flush() error {
	return nil
}

type MemoryBatch[H runtime.Hash, Hasher runtime.Hasher[H]] struct {
	*memoryDBWrapper[H, Hasher]
}

func (b *MemoryBatch[H, Hasher]) Close() error {
	return nil
}

func (*MemoryBatch[H, Hasher]) Reset() {}

func (b *MemoryBatch[H, Hasher]) ValueSize() int {
	return 1
}
func (mdbw *memoryDBWrapper[H, Hasher]) NewBatch() database.Batch {
	return &MemoryBatch[H, Hasher]{
		memoryDBWrapper: mdbw}
}

func makeValue(i uint8) []byte {
	val := make([]byte, 64)
	for j := 0; j < len(val); j++ {
		val[j] = byte(i)
	}
	return val
}

var testData []struct {
	Key   []byte
	Value []byte
} = []struct {
	Key   []byte
	Value []byte
}{
	{
		Key:   []byte("key1"),
		Value: makeValue(1),
	},
	{
		Key:   []byte("key2"),
		Value: makeValue(2),
	},
	{
		Key:   []byte("key3"),
		Value: makeValue(3),
	},
	{
		Key:   []byte("key4"),
		Value: makeValue(4),
	},
}

type MemoryDB = memorydb.MemoryDB[
	hash.H256, runtime.BlakeTwo256, hash.H256, memorydb.HashKey[hash.H256],
]

func newMemoryDB() *MemoryDB {
	mdb := MemoryDB(memorydb.NewMemoryDB[
		hash.H256, runtime.BlakeTwo256, hash.H256, memorydb.HashKey[hash.H256],
	]([]byte{0}))
	return &mdb
}

func createTrie(t *testing.T) (db *MemoryDB, root hash.H256) {
	t.Helper()
	db = newMemoryDB()
	trieDB := triedb.NewEmptyTrieDB[hash.H256, runtime.BlakeTwo256](db)
	trieDB.SetVersion(trie.V1)

	for _, td := range testData {
		err := trieDB.Put(td.Key, td.Value)
		require.NoError(t, err)
	}

	root, err := trieDB.Hash()
	require.NoError(t, err)

	return db, root
}
func TestRecorder(t *testing.T) {
	db, root := createTrie(t)
	rec := Recorder[hash.H256]{inner: newRecorderInner[hash.H256]()}

	{
		trieRecorder := rec.TrieRecorder(root)
		trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
		trieDB.SetVersion(trie.V1)
		require.Equal(t, testData[0].Value, trieDB.Get(testData[0].Key))
	}

	storageProof := rec.DrainStorageProof()
	memDB := ptrie.ToMemoryDB[hash.H256, runtime.BlakeTwo256](storageProof)

	// Check that we recorded the required data
	trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, memDB)
	trieDB.SetVersion(trie.V1)
	require.Equal(t, testData[0].Value, trieDB.Get(testData[0].Key))
}

type recorderStats struct {
	AccessedNodes int
	RecordedKeys  int
	EstimatedSize int
}

func newRecorderStats(recorder *Recorder[hash.H256]) (rs recorderStats) {
	recorder.innerMtx.Lock()
	defer recorder.innerMtx.Unlock()

	var recordedKeys int
	allKeys := maps.Values(recorder.inner.recordedKeys)
	for _, keys := range allKeys {
		recordedKeys = recordedKeys + len(maps.Keys(keys))
	}

	rs.RecordedKeys = recordedKeys
	rs.AccessedNodes = len(recorder.inner.accessedNodes)
	rs.EstimatedSize = int(recorder.EstimateEncodedSize())
	return
}

func TestRecorder_TransactionsRollback(t *testing.T) {
	db, root := createTrie(t)
	rec := Recorder[hash.H256]{inner: newRecorderInner[hash.H256]()}
	stats := make([]recorderStats, 0)
	stats = append(stats, recorderStats{})

	for i := 0; i < 4; i++ {
		rec.StartTransaction()
		{
			trieRecorder := rec.TrieRecorder(root)
			trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
			trieDB.SetVersion(trie.V1)
			assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
		}
		stats = append(stats, newRecorderStats(&rec))
	}

	assert.Equal(t, 4, len(rec.inner.transactions))

	for i := 0; i < 5; i++ {
		assert.Equal(t, stats[4-i], newRecorderStats(&rec))

		storageProof := rec.StorageProof()
		memDB := ptrie.ToMemoryDB[hash.H256, runtime.BlakeTwo256](storageProof)

		trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, memDB)
		trieDB.SetVersion(trie.V1)

		// Check that the required data is still present.
		for a := 0; a < 4; a++ {
			if a < 4-i {
				assert.Equal(t, testData[a].Value, trieDB.Get(testData[a].Key))
			} else {
				// All the data that we already rolled back, should be gone!
				assert.Nil(t, trieDB.Get(testData[a].Key))
			}
		}

		if i < 4 {
			err := rec.RollBackTransaction()
			assert.NoError(t, err)
		}
	}
	assert.Equal(t, 0, len(rec.inner.transactions))
}

func TestRecorder_TransactionsCommit(t *testing.T) {
	db, root := createTrie(t)
	rec := Recorder[hash.H256]{inner: newRecorderInner[hash.H256]()}

	for i := 0; i < 4; i++ {
		rec.StartTransaction()
		{
			trieRecorder := rec.TrieRecorder(root)
			trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
			trieDB.SetVersion(trie.V1)
			assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
		}
	}

	stats := newRecorderStats(&rec)
	assert.Equal(t, 4, len(rec.inner.transactions))

	for i := 0; i < 4; i++ {
		err := rec.CommitTransaction()
		assert.NoError(t, err)
	}
	assert.Equal(t, 0, len(rec.inner.transactions))
	assert.Equal(t, stats, newRecorderStats(&rec))

	storageProof := rec.StorageProof()
	memDB := ptrie.ToMemoryDB[hash.H256, runtime.BlakeTwo256](storageProof)

	// Check that we recorded the required data
	trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, memDB)
	trieDB.SetVersion(trie.V1)

	// Check that the required data is still present.
	for i := 0; i < 4; i++ {
		assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
	}
}

func TestRecorder_TransactionsCommitAndRollback(t *testing.T) {
	db, root := createTrie(t)
	rec := Recorder[hash.H256]{inner: newRecorderInner[hash.H256]()}

	for i := 0; i < 2; i++ {
		rec.StartTransaction()
		{
			trieRecorder := rec.TrieRecorder(root)
			trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
			trieDB.SetVersion(trie.V1)
			assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
		}
	}

	err := rec.RollBackTransaction()
	assert.NoError(t, err)

	for i := 2; i < 4; i++ {
		rec.StartTransaction()
		{
			trieRecorder := rec.TrieRecorder(root)
			trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
			trieDB.SetVersion(trie.V1)
			assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
		}
	}

	err = rec.RollBackTransaction()
	assert.NoError(t, err)

	assert.Equal(t, 2, len(rec.inner.transactions))

	for i := 0; i < 2; i++ {
		err := rec.CommitTransaction()
		assert.NoError(t, err)
	}

	assert.Equal(t, 0, len(rec.inner.transactions))

	storageProof := rec.StorageProof()
	memDB := ptrie.ToMemoryDB[hash.H256, runtime.BlakeTwo256](storageProof)

	// Check that we recorded the required data
	trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, memDB)
	trieDB.SetVersion(trie.V1)

	// Check that the required data is still present.
	for i := 0; i < 4; i++ {
		if i%2 == 0 {
			assert.Equal(t, testData[i].Value, trieDB.Get(testData[i].Key))
		} else {
			assert.Nil(t, trieDB.Get(testData[i].Key))
		}
	}
}

func TestRecorder_TransactionAccessedKeys(t *testing.T) {
	key := testData[0].Key
	db, root := createTrie(t)
	rec := Recorder[hash.H256]{inner: newRecorderInner[hash.H256]()}

	{
		trieRecorder := rec.TrieRecorder(root)
		assert.Equal(t, trieRecorder.TrieNodesRecordedForKey(key), triedb.RecordedNone)
	}

	rec.StartTransaction()
	{
		trieRecorder := rec.TrieRecorder(root)
		trieDB := triedb.NewTrieDB[hash.H256, runtime.BlakeTwo256](root, db, triedb.WithRecorder[hash.H256, runtime.BlakeTwo256](trieRecorder))
		trieDB.SetVersion(trie.V1)

		// trieDB.
		// TODO: update TrieDB to support GetHash method
	}
	fmt.Println(key, db)
}
