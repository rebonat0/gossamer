// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package erasure_test

import (
	"errors"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/erasure"
	libtrie "github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

func TestObtainChunks(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name              string
		nValidators       uint
		dataHex           string
		expectedChunksHex []string
		expectedError     error
	}{
		// generated all these values using `roundtrip_proof_encoding()` function from polkadot.
		// https://github.com/paritytech/polkadot/blob/9b1fc27cec47f01a2c229532ee7ab79cc5bb28ef/erasure-coding/src/lib.rs#L413-L418
		{
			name:              "1_validators",
			nValidators:       1,
			dataHex:           "0x04020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			expectedChunksHex: []string{},
			expectedError:     errors.New("expected at least 2 validators"),
		},
		{
			name:              "2_validators with zero sized data",
			nValidators:       2,
			dataHex:           "0x",
			expectedChunksHex: []string{},
			expectedError:     erasure.ErrZeroSizedData,
		},
		{
			name:        "2_validators",
			nValidators: 2,
			dataHex:     "0x04020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			expectedChunksHex: []string{
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
		},
		{
			name:        "3_validators",
			nValidators: 3,
			dataHex:     "0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			expectedChunksHex: []string{
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
		},
		{
			name:        "4_validators",
			nValidators: 4,
			dataHex:     "0x10020202020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			expectedChunksHex: []string{
				"0x100202000000000000000000000000000000000000000000",
				"0x020200000000000000000000000000000000000000000000",
				"0x3f60019f0000000000000000000000000000000000000000",
				"0x2d60039f0000000000000000000000000000000000000000",
			},
		},
		{
			name:        "5_validators",
			nValidators: 5,
			dataHex:     "0x2002020202020202020000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			expectedChunksHex: []string{
				"0x2002020202000000000000000000000000000000000000000000",
				"0x0202020200000000000000000000000000000000000000000000",
				"0x1a670202019f0000000000000000000000000000000000000000",
				"0x38670202039f0000000000000000000000000000000000000000",
				"0x948a020209f70000000000000000000000000000000000000000",
			},
		},
		{
			name:        "6_validators",
			nValidators: 6,
			dataHex:     "0x40020202020202020202020202020202020000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			expectedChunksHex: []string{
				"0x400202020202020202000000000000000000000000000000000000000000",
				"0x020202020202020200000000000000000000000000000000000000000000",
				"0xf069020202020202019f0000000000000000000000000000000000000000",
				"0xb269020202020202039f0000000000000000000000000000000000000000",
				"0x211702020202020209f70000000000000000000000000000000000000000",
				"0x63170202020202020bf70000000000000000000000000000000000000000",
			},
		},
		{
			name:        "7_validators",
			nValidators: 7,
			dataHex:     "0x8002020202020202020202020202020202020202020202020202020202020202020000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			expectedChunksHex: []string{
				"0x8002020202020202020202020202020202000000000000000000000000000000000000000000",
				"0x0202020202020202020202020202020200000000000000000000000000000000000000000000",
				"0x64740202020202020202020202020202019f0000000000000000000000000000000000000000",
				"0xe6740202020202020202020202020202039f0000000000000000000000000000000000000000",
				"0xf60b020202020202020202020202020209f70000000000000000000000000000000000000000",
				"0x740b02020202020202020202020202020bf70000000000000000000000000000000000000000",
				"0x127d02020202020202020202020202020a680000000000000000000000000000000000000000",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res, err := erasure.ObtainChunks(c.nValidators, common.MustHexToBytes(c.dataHex))
			require.Equal(t, c.expectedError, err)

			if err == nil {
				var expectedChunks [][]byte
				for _, chunk := range c.expectedChunksHex {
					expectedChunks = append(expectedChunks, common.MustHexToBytes(chunk))
				}
				require.Equal(t, c.nValidators, uint(len(res)))
				require.Equal(t, expectedChunks, res)
			}
		})
	}

}

func TestReconstruct(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		nValidators     uint
		chunksHex       []string
		expectedDataHex string
		expectedError   error
	}{
		// generated all these values using `roundtrip_proof_encoding()` function from polkadot.
		// https://github.com/paritytech/polkadot/blob/9b1fc27cec47f01a2c229532ee7ab79cc5bb28ef/erasure-coding/src/lib.rs#L413-L418
		{
			name:            "1_validator_with_zero_sized_chunks",
			nValidators:     1,
			expectedDataHex: "0x",
			chunksHex:       []string{},
			expectedError:   erasure.ErrZeroSizedChunks,
		},
		{
			name:            "1_validators",
			nValidators:     1,
			expectedDataHex: "0x",
			chunksHex: []string{
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedError: errors.New("expected at least 2 validators"),
		},
		{
			name:            "2_validators",
			nValidators:     2,
			expectedDataHex: "0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			chunksHex: []string{
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "3_validators",
			nValidators:     3,
			expectedDataHex: "0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			chunksHex: []string{
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "4_validators",
			nValidators:     4,
			expectedDataHex: "0x100202020200000000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			chunksHex: []string{
				"0x100202000000000000000000000000000000000000000000",
				"0x020200000000000000000000000000000000000000000000",
				"0x3f60019f0000000000000000000000000000000000000000",
				"0x2d60039f0000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "5_validators",
			nValidators:     5,
			expectedDataHex: "0x20020202020202020200000000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			chunksHex: []string{
				"0x2002020202000000000000000000000000000000000000000000",
				"0x0202020200000000000000000000000000000000000000000000",
				"0x1a670202019f0000000000000000000000000000000000000000",
				"0x38670202039f0000000000000000000000000000000000000000",
				"0x948a020209f70000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "6_validators",
			nValidators:     6,
			expectedDataHex: "0x400202020202020202020202020202020200000000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			chunksHex: []string{
				"0x400202020202020202000000000000000000000000000000000000000000",
				"0x020202020202020200000000000000000000000000000000000000000000",
				"0xf069020202020202019f0000000000000000000000000000000000000000",
				"0xb269020202020202039f0000000000000000000000000000000000000000",
				"0x211702020202020209f70000000000000000000000000000000000000000",
				"0x63170202020202020bf70000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "7_validators",
			nValidators:     7,
			expectedDataHex: "0x80020202020202020202020202020202020202020202020202020202020202020200000000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			chunksHex: []string{
				"0x8002020202020202020202020202020202000000000000000000000000000000000000000000",
				"0x0202020202020202020202020202020200000000000000000000000000000000000000000000",
				"0x64740202020202020202020202020202019f0000000000000000000000000000000000000000",
				"0xe6740202020202020202020202020202039f0000000000000000000000000000000000000000",
				"0xf60b020202020202020202020202020209f70000000000000000000000000000000000000000",
				"0x740b02020202020202020202020202020bf70000000000000000000000000000000000000000",
				"0x127d02020202020202020202020202020a680000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
		{
			name:            "7_validators_with_missing_chunks",
			nValidators:     7,
			expectedDataHex: "0x80020202020202020202020202020202020202020202020202020202020202020200000000000000000000000000000000000000000000000000000000000000000000000000000000000000", //nolint:lll
			chunksHex: []string{
				"0x8002020202020202020202020202020202000000000000000000000000000000000000000000",
				"0x0202020202020202020202020202020200000000000000000000000000000000000000000000",
				"0x64740202020202020202020202020202019f0000000000000000000000000000000000000000",
				"0xe6740202020202020202020202020202039f0000000000000000000000000000000000000000",
				"0xf60b020202020202020202020202020209f70000000000000000000000000000000000000000",
			},
			expectedError: nil,
		},
	}

	for _, d := range testCases {
		d := d
		t.Run(d.name, func(t *testing.T) {
			t.Parallel()

			var chunks [][]byte
			for _, chunk := range d.chunksHex {
				chunks = append(chunks, common.MustHexToBytes(chunk))
			}

			actualData, err := erasure.Reconstruct(d.nValidators, chunks)
			require.Equal(t, err, d.expectedError)

			if actualData == nil {
				require.Equal(t, common.MustHexToBytes(d.expectedDataHex), []byte{})
			} else {
				require.Equal(t, common.MustHexToBytes(d.expectedDataHex), actualData)
			}
		})
	}
}

func TestChunksToTrie(t *testing.T) {
	t.Parallel()
	var testCases = []struct {
		name            string
		chunksHex       []string
		expectedRootHex string
	}{
		// generated all these values using `roundtrip_proof_encoding()` function from polkadot.
		// https://github.com/paritytech/polkadot/blob/9b1fc27cec47f01a2c229532ee7ab79cc5bb28ef/erasure-coding/src/lib.rs#L413-L418
		{
			name: "2_chunks",
			chunksHex: []string{
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0402000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedRootHex: "0x513489282098e960bfd57ed52d62838ce9395f3f59257f1f40fadd02261a7991",
		},
		{
			name: "3_chunks",
			chunksHex: []string{
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"0x0802020000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedRootHex: "0x57aff6950c28545a43ae9ed83acbc87dd50cf548d8712e3ba9e2f074e333a84c",
		},
		{
			name: "4_chunks",
			chunksHex: []string{
				"0x100202000000000000000000000000000000000000000000",
				"0x020200000000000000000000000000000000000000000000",
				"0x3f60019f0000000000000000000000000000000000000000",
				"0x2d60039f0000000000000000000000000000000000000000",
			},
			expectedRootHex: "0x083c17b6cceaf3a5e062bb93ea31a690a218d4ca654f42454b1d639033f1ec9a",
		},
		{
			name: "5_chunks",
			chunksHex: []string{
				"0x2002020202000000000000000000000000000000000000000000",
				"0x0202020200000000000000000000000000000000000000000000",
				"0x1a670202019f0000000000000000000000000000000000000000",
				"0x38670202039f0000000000000000000000000000000000000000",
				"0x948a020209f70000000000000000000000000000000000000000",
			},
			expectedRootHex: "0x5496b487c0c849eaf194ad8e283eed0fe188e507c62b93f9a2659489f3a12d89",
		},
		{
			name: "6_chunks",
			chunksHex: []string{
				"0x400202020202020202000000000000000000000000000000000000000000",
				"0x020202020202020200000000000000000000000000000000000000000000",
				"0xf069020202020202019f0000000000000000000000000000000000000000",
				"0xb269020202020202039f0000000000000000000000000000000000000000",
				"0x211702020202020209f70000000000000000000000000000000000000000",
				"0x63170202020202020bf70000000000000000000000000000000000000000",
			},
			expectedRootHex: "0xc20501e40e6dd45a9b71a66c9da8bdd3b11ab0722579d26516f7ae1fdb3e3ad2",
		},
		{
			name: "7_chunks",
			chunksHex: []string{
				"0x8002020202020202020202020202020202000000000000000000000000000000000000000000",
				"0x0202020202020202020202020202020200000000000000000000000000000000000000000000",
				"0x64740202020202020202020202020202019f0000000000000000000000000000000000000000",
				"0xe6740202020202020202020202020202039f0000000000000000000000000000000000000000",
				"0xf60b020202020202020202020202020209f70000000000000000000000000000000000000000",
				"0x740b02020202020202020202020202020bf70000000000000000000000000000000000000000",
				"0x127d02020202020202020202020202020a680000000000000000000000000000000000000000",
			},
			expectedRootHex: "0xdbc4bde5cf7f7eaa16baec41f9a2c1f800a992b3a4b81c5d1ec66dcf1a4d7f18",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var chunks [][]byte
			for _, chunk := range c.chunksHex {
				chunks = append(chunks, common.MustHexToBytes(chunk))
			}

			trie, err := erasure.ChunksToTrie(chunks)
			require.NoError(t, err)

			root, err := trie.Hash(libtrie.NoMaxInlineValueSize)
			require.NoError(t, err)

			require.Equal(t, c.expectedRootHex, root.String())
		})
	}
}
