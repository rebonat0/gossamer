// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package parachain

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	parachaintypes "github.com/ChainSafe/gossamer/lib/parachain/types"
	"github.com/stretchr/testify/require"
)

func TestEncodeStatementFetchingRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		request        StatementFetchingRequest
		expectedEncode []byte
	}{
		{
			// expected encoding is generated by running rust test code:
			// fn statement_request() {
			// 	let hash4 = Hash::repeat_byte(4);
			// 	let statement_fetching_request = StatementFetchingRequest{
			// 		relay_parent: hash4,
			// 		candidate_hash: CandidateHash(hash4)
			// 	};
			// 	println!(
			// 		"statement_fetching_request encode => {:?}\n\n",
			// 		statement_fetching_request.encode()
			// 	);
			// }
			name: "all_4_in_hash",
			request: StatementFetchingRequest{
				RelayParent:   getDummyHash(4),
				CandidateHash: CandidateHash{Value: getDummyHash(4)},
			},
			expectedEncode: common.MustHexToBytes(testDataStatement["all4InCommonHash"]),
		},
		{
			name: "all_7_in_hash",
			request: StatementFetchingRequest{
				RelayParent:   getDummyHash(7),
				CandidateHash: CandidateHash{Value: getDummyHash(7)},
			},
			expectedEncode: common.MustHexToBytes(testDataStatement["all7InCommonHash"]),
		},
		{
			name: "random_hash",
			request: StatementFetchingRequest{
				RelayParent: common.MustHexToHash("0x677811d2f3ded2489685468dbdb2e4fa280a249fba9356acceb2e823820e2c19"),
				CandidateHash: CandidateHash{
					Value: common.MustHexToHash("0x677811d2f3ded2489685468dbdb2e4fa280a249fba9356acceb2e823820e2c19"),
				},
			},
			expectedEncode: common.MustHexToBytes(testDataStatement["hexOfStatementFetchingRequest"]),
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			actualEncode, err := c.request.Encode()
			require.NoError(t, err)
			require.Equal(t, c.expectedEncode, actualEncode)
		})
	}
}

func TestStatementFetchingResponse(t *testing.T) {
	t.Parallel()

	testHash := common.MustHexToHash("0x677811d2f3ded2489685468dbdb2e4fa280a249fba9356acceb2e823820e2c19")

	var collatorID parachaintypes.CollatorID
	tempCollatID := common.MustHexToBytes("0x48215b9d322601e5b1a95164cea0dc4626f545f98343d07f1551eb9543c4b147")
	copy(collatorID[:], tempCollatID)

	var collatorSignature parachaintypes.CollatorSignature
	tempSignature := common.MustHexToBytes(testDataStatement["collatorSignature"])
	copy(collatorSignature[:], tempSignature)

	missingDataInStatement := MissingDataInStatement{
		Descriptor: parachaintypes.CandidateDescriptor{
			ParaID:                      uint32(1),
			RelayParent:                 testHash,
			Collator:                    collatorID,
			PersistedValidationDataHash: testHash,
			PovHash:                     testHash,
			ErasureRoot:                 testHash,
			Signature:                   collatorSignature,
			ParaHead:                    testHash,
			ValidationCodeHash:          parachaintypes.ValidationCodeHash(testHash),
		},
		Commitments: parachaintypes.CandidateCommitments{
			UpwardMessages:    []parachaintypes.UpwardMessage{{1, 2, 3}},
			NewValidationCode: &parachaintypes.ValidationCode{1, 2, 3},
			HeadData: parachaintypes.HeadData{
				Data: []byte{1, 2, 3},
			},
			ProcessedDownwardMessages: uint32(5),
			HrmpWatermark:             uint32(0),
		},
	}

	encodedValue := common.MustHexToBytes(testDataStatement["hexOfStatementFetchingResponse"])

	t.Run("encode_statement_fetching_response", func(t *testing.T) {
		t.Parallel()

		response := NewStatementFetchingResponse()
		err := response.SetValue(missingDataInStatement)
		require.NoError(t, err)

		actualEncode, err := response.Encode()
		require.NoError(t, err)

		require.Equal(t, encodedValue, actualEncode)
	})

	t.Run("Decode_statement_fetching_response", func(t *testing.T) {
		t.Parallel()

		response := NewStatementFetchingResponse()
		err := response.Decode(encodedValue)
		require.NoError(t, err)

		actualData, err := response.Value()
		require.NoError(t, err)

		require.EqualValues(t, missingDataInStatement, actualData)
	})
}
