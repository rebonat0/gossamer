// Copyright 2024 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package inmemory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryTrieIterator(t *testing.T) {
	tt := NewEmptyTrie()

	tt.Put([]byte("some_other_storage:XCC:ZZZ"), []byte("0x10"))
	tt.Put([]byte("yet_another_storage:BLABLA:YYY:JJJ"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:AAA"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:CCC"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:DDD"), []byte("0x10"))
	tt.Put([]byte("account_storage:JJK:EEE"), []byte("0x10"))

	iter := NewInMemoryTrieIterator(WithTrie(tt))
	require.Equal(t, []byte("account_storage:ABC:AAA"), iter.NextKey())
	require.Equal(t, []byte("account_storage:ABC:CCC"), iter.NextKey())
	require.Equal(t, []byte("account_storage:ABC:DDD"), iter.NextKey())
	require.Equal(t, []byte("account_storage:JJK:EEE"), iter.NextKey())
	require.Equal(t, []byte("some_other_storage:XCC:ZZZ"), iter.NextKey())
	require.Equal(t, []byte("yet_another_storage:BLABLA:YYY:JJJ"), iter.NextKey())
	require.Nil(t, iter.NextKey())
}

func TestInMemoryIteratorGetAllKeysWithPrefix(t *testing.T) {
	tt := NewEmptyTrie()

	tt.Put([]byte("services_storage:serviceA:19090"), []byte("0x10"))
	tt.Put([]byte("services_storage:serviceB:22222"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:AAA"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:CCC"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:DDD"), []byte("0x10"))
	tt.Put([]byte("account_storage:JJK:EEE"), []byte("0x10"))

	prefix := []byte("account_storage")

	keys := make([][]byte, 0)
	for key := range tt.PrefixedKeys(prefix) {
		keys = append(keys, key)
	}

	expectedKeys := [][]byte{
		[]byte("account_storage:ABC:AAA"),
		[]byte("account_storage:ABC:CCC"),
		[]byte("account_storage:ABC:DDD"),
		[]byte("account_storage:JJK:EEE"),
	}

	require.Equal(t, expectedKeys, keys)
}

func TestInMemoryIteratorGetAllKeysWithPrefixIncluded(t *testing.T) {
	tt := NewEmptyTrie()

	tt.Put([]byte("services_storage:serviceA:19090"), []byte("0x10"))
	tt.Put([]byte("services_storage:serviceB:22222"), []byte("0x10"))
	tt.Put([]byte("account_storage"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:AAA"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:CCC"), []byte("0x10"))
	tt.Put([]byte("account_storage:ABC:DDD"), []byte("0x10"))
	tt.Put([]byte("account_storage:JJK:EEE"), []byte("0x10"))

	prefix := []byte("account_storage")

	keys := make([][]byte, 0)
	for key := range tt.PrefixedKeys(prefix) {
		keys = append(keys, key)
	}

	expectedKeys := [][]byte{
		[]byte("account_storage"),
		[]byte("account_storage:ABC:AAA"),
		[]byte("account_storage:ABC:CCC"),
		[]byte("account_storage:ABC:DDD"),
		[]byte("account_storage:JJK:EEE"),
	}

	require.Equal(t, expectedKeys, keys)
}
