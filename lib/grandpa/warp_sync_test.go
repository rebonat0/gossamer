// Copyright 2024 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package grandpa

import (
	"encoding/hex"
	"errors"
	"math/rand"
	"slices"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	primitives "github.com/ChainSafe/gossamer/internal/primitives/consensus/grandpa"
	ced25519 "github.com/ChainSafe/gossamer/internal/primitives/core/ed25519"
	"github.com/ChainSafe/gossamer/internal/primitives/core/hash"
	"github.com/ChainSafe/gossamer/internal/primitives/keyring/ed25519"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
	"github.com/ChainSafe/gossamer/internal/primitives/runtime/generic"
	"github.com/ChainSafe/gossamer/lib/common"
	grandpa "github.com/ChainSafe/gossamer/pkg/finality-grandpa"
	"github.com/ChainSafe/gossamer/pkg/scale"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGenerateWarpSyncProofBlockNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock block state to return not found block
	blockStateMock := NewMockBlockState(ctrl)
	blockStateMock.EXPECT().GetHeader(common.EmptyHash).Return(nil, errors.New("not found")).AnyTimes()

	provider := &WarpSyncProofProvider{
		blockState: blockStateMock,
	}

	// Check errMissingStartBlock returned by provider
	_, err := provider.Generate(common.EmptyHash)
	require.Error(t, err)
	require.ErrorIs(t, err, errMissingStartBlock)
}

func TestGenerateWarpSyncProofBlockNotFinalized(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock block state to return not found block
	bestBlockHeader := &types.Header{
		Number:     2,
		ParentHash: common.MustBlake2bHash([]byte("1")),
	}

	notFinalizedBlockHeader := &types.Header{
		Number:     3,
		ParentHash: common.MustBlake2bHash([]byte("2")),
	}

	blockStateMock := NewMockBlockState(ctrl)
	blockStateMock.EXPECT().GetHeader(notFinalizedBlockHeader.Hash()).Return(notFinalizedBlockHeader, nil).AnyTimes()
	blockStateMock.EXPECT().GetHighestFinalisedHeader().Return(bestBlockHeader, nil).AnyTimes()

	provider := &WarpSyncProofProvider{
		blockState: blockStateMock,
	}

	// Check errMissingStartBlock returned by provider
	_, err := provider.Generate(notFinalizedBlockHeader.Hash())
	require.Error(t, err)
	require.ErrorIs(t, err, errStartBlockNotFinalized)
}

// This test generates a small blockchain with authority set changes and expected
// justifications to create a warp sync proof and verify it.
//
//nolint:lll
func TestGenerateAndVerifyWarpSyncProofOk(t *testing.T) {
	t.Parallel()

	type signedPrecommit = grandpa.SignedPrecommit[hash.H256, uint32, primitives.AuthoritySignature, primitives.AuthorityID]
	type preCommit = grandpa.Precommit[hash.H256, uint32]

	// Initialize mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	blockStateMock := NewMockBlockState(ctrl)
	grandpaStateMock := NewMockGrandpaState(ctrl)

	// Set authorities
	availableAuthorities := ed25519.AvailableAuthorities
	genesisAuthorities := primitives.AuthorityList{
		primitives.AuthorityIDWeight{
			AuthorityID:     ed25519.Alice.Pair().Public().(ced25519.Public),
			AuthorityWeight: 1,
		},
	}
	currentAuthorities := []ed25519.Keyring{ed25519.Alice}

	// Set initial values for the scheduled changes
	currentSetId := primitives.SetID(0)
	authoritySetChanges := []uint{}

	// Genesis block
	genesis := &types.Header{
		ParentHash: common.MustBlake2bHash([]byte("genesis")),
		Number:     1,
	}

	// All blocks headers
	headers := []*types.Header{
		genesis,
	}

	const maxBlocks = 100

	// Create blocks with their scheduled changes and justifications
	for n := uint(1); n <= maxBlocks; n++ {
		lastBlockHeader := headers[len(headers)-1]

		newAuthorities := []ed25519.Keyring{}

		digest := types.NewDigest()

		// Authority set change happens every 10 blocks
		if n != 0 && n%10 == 0 {
			// Pick new random authorities
			nAuthorities := rand.Intn(len(availableAuthorities)-1) + 1
			require.GreaterOrEqual(t, nAuthorities, 1)

			rand.Shuffle(len(availableAuthorities), func(i, j int) {
				availableAuthorities[i], availableAuthorities[j] = availableAuthorities[j], availableAuthorities[i]
			})

			newAuthorities = availableAuthorities[:nAuthorities]

			// Map new authorities to GRANDPA raw authorities format
			nextAuthorities := []types.GrandpaAuthoritiesRaw{}
			for _, key := range newAuthorities {
				nextAuthorities = append(nextAuthorities,
					types.GrandpaAuthoritiesRaw{
						Key: [32]byte(key.Pair().Public().Bytes()),
						ID:  1,
					},
				)
			}

			// Create scheduled change
			scheduledChange := createGRANDPAConsensusDigest(t, types.GrandpaScheduledChange{
				Auths: nextAuthorities,
				Delay: 0,
			})
			digest.Add(scheduledChange)
		}

		// Create new block header
		header := &types.Header{
			ParentHash: lastBlockHeader.Hash(),
			Number:     lastBlockHeader.Number + 1,
			Digest:     digest,
		}

		headers = append(headers, header)

		// If we have an authority set change, create a justification
		if len(newAuthorities) > 0 {
			targetHash := hash.H256(string(header.Hash().ToBytes()))
			targetNumber := uint32(header.Number)

			// Create precommits for current voters
			precommits := []signedPrecommit{}
			for _, voter := range currentAuthorities {
				precommit := preCommit{
					TargetHash:   targetHash,
					TargetNumber: targetNumber,
				}

				msg := grandpa.NewMessage[hash.H256, uint32, preCommit](precommit)
				encoded := primitives.NewLocalizedPayload(1, currentSetId, msg)
				signature := voter.Sign(encoded)

				signedPreCommit := signedPrecommit{
					Precommit: preCommit{
						TargetHash:   targetHash,
						TargetNumber: targetNumber,
					},
					Signature: signature,
					ID:        voter.Pair().Public().(ced25519.Public),
				}

				precommits = append(precommits, signedPreCommit)
			}

			// Create justification
			justification := primitives.GrandpaJustification[hash.H256, uint32]{
				Round: 1,
				Commit: primitives.Commit[hash.H256, uint32]{
					TargetHash:   targetHash,
					TargetNumber: targetNumber,
					Precommits:   precommits,
				},
				VoteAncestries: genericHeadersList(t, headers),
			}

			encodedJustification, err := scale.Marshal(justification)
			require.NoError(t, err)

			blockStateMock.EXPECT().GetJustification(header.Hash()).Return(encodedJustification, nil).AnyTimes()
			blockStateMock.EXPECT().GetHighestFinalisedHeader().Return(header, nil).AnyTimes()

			// Update authorities and set id
			authoritySetChanges = append(authoritySetChanges, header.Number)
			currentAuthorities = slices.Clone(newAuthorities)
			currentSetId++
		}

	}

	// Return expected authority changes for each block
	authChanges := []uint{}
	for n := uint(1); n <= maxBlocks; n++ {
		for _, change := range authoritySetChanges {
			if n <= change {
				authChanges = append(authChanges, change)
			}
		}
		grandpaStateMock.EXPECT().GetAuthoritiesChangesFromBlock(n).Return(authChanges, nil).AnyTimes()
	}

	// Mock responses
	for _, header := range headers {
		blockStateMock.EXPECT().GetHeaderByNumber(header.Number).Return(header, nil).AnyTimes()
		blockStateMock.EXPECT().GetHeader(header.Hash()).Return(header, nil).AnyTimes()
	}

	// Initialize warp sync provider
	provider := NewWarpSyncProofProvider(blockStateMock, grandpaStateMock)

	// Generate proof
	proof, err := provider.Generate(headers[0].Hash())
	require.NoError(t, err)

	// Verify proof
	expectedAuthorities := primitives.AuthorityList{}
	for _, key := range currentAuthorities {
		expectedAuthorities = append(expectedAuthorities,
			primitives.AuthorityIDWeight{
				AuthorityID:     [32]byte(key.Pair().Public().Bytes()),
				AuthorityWeight: 1,
			},
		)
	}

	result, err := provider.Verify(proof, 0, genesisAuthorities)
	require.NoError(t, err)
	require.Equal(t, currentSetId, result.SetId)
	require.Equal(t, expectedAuthorities, result.AuthorityList)
}

func TestFindScheduledChange(t *testing.T) {
	t.Parallel()

	scheduledChange := createGRANDPAConsensusDigest(t, types.GrandpaScheduledChange{
		Auths: []types.GrandpaAuthoritiesRaw{},
		Delay: 2,
	})

	digest := types.NewDigest()
	digest.Add(scheduledChange)

	blockHeader := &types.Header{
		ParentHash: common.Hash{0x00},
		Number:     1,
		Digest:     digest,
	}

	// Find scheduled change in block header
	scheduledChangeDigest, err := findScheduledChange(*blockHeader)
	require.NoError(t, err)
	require.NotNil(t, scheduledChangeDigest)
}

func createGRANDPAConsensusDigest(t *testing.T, digestData any) types.ConsensusDigest {
	t.Helper()

	grandpaConsensusDigest := types.NewGrandpaConsensusDigest()
	require.NoError(t, grandpaConsensusDigest.SetValue(digestData))

	marshaledData, err := scale.Marshal(grandpaConsensusDigest)
	require.NoError(t, err)

	return types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              marshaledData,
	}
}

func genericHeadersList(t *testing.T, headers []*types.Header) []runtime.Header[uint32, hash.H256] {
	t.Helper()

	headerList := []runtime.Header[uint32, hash.H256]{}
	for _, header := range headers {
		if header == nil {
			continue
		}
		newHeader := generic.Header[uint32, hash.H256, runtime.BlakeTwo256]{}
		newHeader.SetParentHash(hash.H256(header.ParentHash.String()))
		newHeader.SetNumber(uint32(header.Number))
		newHeader.DigestMut().Push(header.Digest)
	}

	return headerList
}

func TestScheduledChange(t *testing.T) {
	hexData := "01d9017fc65343cc2305c1feff134cb225945a412d76aa1574f091b09b05116e147c3c010000000000000061db8a464dbaca0892ff7f849e37aa2113354cf4c735ec0eebc218f22c39a1100100000000000000e7ebf2f396adfa3f7ea8e7a6860e038818d786d0a56845f56921827c798788c80100000000000000a53405d4a76b242f85c6226a23a93e0ffb8dc965985250c8e0c218e91eea6cf80100000000000000554de9dd1889195d5f70d5473c69b5d97ed0267a806d9052a8358de3f29cd61e0100000000000000fbb28e290726411d017451209392891a92c8a777a43cff8a62da2b8350f5eac4010000000000000084beadb089cda5e0bc20bbc08665499c4d641bb189864f58e40a71a12b1bd77601000000000000009a006c1e11b7c923c43c20c50b3298610ec79aff08f7aed2e23a10f82ed2116e0100000000000000b036acb346f99b104c2f80ae6cb38ac12fbcf6935bbd2d61d9374cc010ef4195010000000000000098512c439944dec65a752c6c7f274dd16cdc0db79f534f8704e5ac85509d02db01000000000000003fbcbc5732ca3e3b5e5663ae213c6d6a8a71fe268eb0dfb3b6d8d4cbaaba38110100000000000000560719a8f30f0f258c22630a40e349f23bc23ad85bccff41108db5050e9f4e6c01000000000000002fd32fe7d146bb1003344e1fd4c00112a6bff758fc53a3bffe97e3122cbc09650100000000000000af69664425d84ef811ffae3cbe6ac6fbe2063c18ac74a655c2cbd6c4339987720100000000000000f5b183712646f29d40c2c0877305cb0d902c214f0a8d44c5b8e6c765384740b401000000000000002bf2597eda54f89e851d398be9c76ed7d8a43948ca60418c7361ec5bce6ab50e01000000000000005fc3b0ba5053e37c006ecd43b224c684cc194e1d222496a9535aaabc3a7e8f84010000000000000022d04ea680d28674d9809c2022b38e60911447657af9abe8eaedac409ae2deff0100000000000000c3af638f3ed2948314eb771e970f73eceac5a2e101ea327fa8fe43e9ee61dda80100000000000000264c9e93a6234365874d3817a54f2769799d647b04d10a64162cb58134277a8601000000000000006902cc7446affffa43ea696c8c0507a3c38ca0faf868544a7ab3d573019f3ca201000000000000001cdd699d681de6c6ef2405e25743a09b2d94548c840d0f1c9cdc5f4e0a856efb01000000000000005eec271b03fcf4277015ab182472fc55719511240c8bef8545195312d351f7750100000000000000d61483c2518c9c3fa34ea5877d6984a3a79a4029713ff31e74aaa7a43cf7dfb601000000000000007e4cf8ec7b00dadddcc10681ca9140ed9c4fb9683cc017f16ad967bb13a26a240100000000000000dbdb0b2b2e7b20de3f85dc61ab8fe9320c6d1e8c7323f60d08a909a31cf2ffed0100000000000000e4e71ed676259af3363516b55b604077015f662a84f3aabecf125fe8921b0ea70100000000000000afda316a0046180767d9ae5a65082971a994e881b5039a90e3b94fccd973782f0100000000000000c80773fbc6144ba0c3d5b35a6e46f6ca2998513d172a8818bce2445f3365b6e90100000000000000c6c9d24ef1f3bf0fa99aa5f0bafe680d8ef42e421398bcbe691f7bcb617bfa140100000000000000c5c61f9ac82e3f4e71ac57ed4731ef8517a9dfdc8b277b6b827d658e2fd74c0b0100000000000000b7fc0b2da60f9de62b40cbf1a280dbcb9f6e446afb2dcdb24b1e9968e3952bfc01000000000000008b4f995dbf0667e2c14342cbd141e3a87bd1c3dac56c4faaf3736a1e826ba696010000000000000058f2c2714d1197ec3ba358612d91b3057a3c9f8538242f4befeb94ecbd35d38c0100000000000000ca4a95c99f97c70ead7d0b99d1d305f1ff4d7ef633e49ca063d13df1ba9e9c4a01000000000000004d5e8f0e248bf0ebdf8c708b1d61e475162bcedded321222528402f10811eabd01000000000000007251da7147c58d25365bc6245897d11a6656be57fa3f9ad095746d3d2556e86201000000000000005652a8985d036a7e2ca61609f5a27d82ad77742397594475a57ad8c915108d5301000000000000003bc2c8a405f7acd25a5e6c1cee72ce8e2c247b05c6b37b9c2bf7654a624e0b4601000000000000004fbba25109fe9ee6e27322db51f40c56ad8940fa9388de42ca43bcc9f2c221a001000000000000006b8004a46f1dbac226955b2e96e7d86f6648837b1a896e48798d22669657aff0010000000000000024271b7d6fcac73456570d9c913a46f918818a693e157f2bcc6f7c0514dca78a0100000000000000bb621691b37e0a1e5006f09c9c43e7ba19a43edeaa383654c8576efdaa01245401000000000000003a960631477eaa37fbeea5016d0887240207e86a05537d39e61d6250e2238d340100000000000000e0d591aa1c2d5fe4ca6372e3f305c529095dac8beaf140296f9ccf4696223f5f01000000000000009485a6d171a367dae2b4645b69a4eb92684853cbb4a9f225a3c0b1d1133adfbb0100000000000000c26e1a1245b9d3becda90e6c0772dc4c8c4edfe678209ccacba87766a7ee12da01000000000000002f6f5a823eb5d9110d7ffbcc3d54518b1f4ac3a53de98df9eb881ec66dff70f30100000000000000b2e12c30ebc7eadcd2931fb54495a3a2a268e8daf9cb42fe039b634640052e990100000000000000be73d3cad71c400291d1f1ef1d3f25b5924f5efa2d509e62389106d5d28eab8b010000000000000078e0a781a72114b2ea5689cadf5401c8122b04f799985c3ea3c41c936981cdc401000000000000007f128131a242dbf8f88b48960252669df8100fe22a719ccaf31e466e9d0378cb010000000000000006081b1806b53c3dbc7a2fc3c5932f3dd3dbcde6e3cc865a39913f5e6352c4b90100000000000000a23c14809e06b77499b621c7528676377f2561a8ca43f5ec2ab86aba1929445101000000000000003468a2de1b18afd7978a2dedeef06d46651ab473181d0e063374d12e026dcf2c010000000000000060d1fe50e6f42402921500bcc8ef367e160c51d2f63fe6b1e81c703910320c1d0100000000000000c9da738b1fe72e00496e884864a0b9d521ea981cab204291809de08462a263cb0100000000000000fa93ba85b41840cfdb6e7fc74ae3a01210485205532688f1b906e2d7abe671340100000000000000b041195b7eccb4fd4f7bf4190f4196e5d1b07f14a2f09d708548592d387a8e4d01000000000000003b7ef18aa29b4afff98fe4af5ff9e4010c6441b63fe71e66d04bb19f22103e080100000000000000baad06b96ee4667aa069770d891802574b1f5ad24742e48f8f28d30164b707c4010000000000000040974dbb1eeb4a657bfd568795b0809f2b156c26d450eea2648794266af9b0830100000000000000705203fab555c0993842e562824ae52bee09dac6d79a47391588263eb2119b7d01000000000000006b65130f5d57f462fd3bb571406ea82f3851e95803e612d798aae10fbfb679dd01000000000000006cd9a2a780fd1600a86a28687562f57d01f7351ca4eef324a47cd694cb2c12ab010000000000000068757f0de0c83a652cf622798020f0e81860e44f93b0fb00e32523eb45ff02dc01000000000000000c0dece6d774fc11476ed423f3aff5e8013942e187acfbf3871414c478034b70010000000000000070a8f2c854fa3a17a68746cfe493beb9bff8f34bbcd83d4a1dac92a82a64392601000000000000001ae4bd150e14514065376d9a10a553b546de8052171561158345712d409f72a80100000000000000b6330a5333954bf601044454fb9bec3401ed59daad584bf62dd6141bdbe33bc30100000000000000727d484a95bb237b70c66aad2fdc6906595ae24784d4a4cbaa048e747cb5cfd00100000000000000d50d476423cec3f3e6227cc43d902397ea78cc0d7f4fb9d4edafc2176f95b9850100000000000000af732cc90dd5fff6d8f554af6a5eb13c0b300f4232ff516ab849f19c88d74ba501000000000000007d52fd6544e8ea0e282dc6699253ce676d9ba296991d2bee7b7da36ed999a67801000000000000009cb6f96ecc63671d4d40544cf69cbde1bf2f5d679ae1fcece0a256c7c9e8f17101000000000000004bec88182042dbb674335be57af1975c769532b36a292aa642b84d984307f48f0100000000000000b786b645f1c6db7c9031f439f49452df5863d4efdc9178dc98158408da8ed17c0100000000000000b60988157f67e9dd8dc6170e8d2001de3f10bad959589a9388936ed639c874170100000000000000eeb7717a1ee2ebd5854f3426d8f276651693403000fa939f4073e4474b75e0100100000000000000b19b35a50e5dfdc3eab30d2c79e5ab7680631ccdd1cadee24bb11a8610ffb5a60100000000000000f1e80cc588d39ad6464d5e890a704d5e9fc07920903b08f06b4b3a2ad218595e0100000000000000b41b864761edcfeca781411ac0163ca5aded0ac045c07431c0b012b1c9d815fd0100000000000000c0732b159411a4abdb0c29029c1ec2013fead26e03fe87b5f5ec4366435db85b010000000000000006fc73229bea563d9d98f48b4ef575dbb5570ef338798255fbb3da68bc4fff6401000000000000001ae4648be656f2d9050cead1a90191c954c96fe1953c21985c062f48c0f3fc780100000000000000bddf5cbdced272ebcfa5187c44ebbc22a614a840ca015a50f489ec373866d5f301000000000000001eda33b98265b1141eb06d7079c7cb51419dc2170dbc998cb9628fe26b340ce801000000000000009cc46f39c94d6dde4865a254f9c8eb3138c4b3576ffbf7e62f790b531038ef78010000000000000015061c9c0ad7cf2c833885ef8573ac0d727571373b372775189228e633343a290100000000000000f16d3c1a4c92fa0810afd33e57a8441903c86a32ef66b637862faacfbc9dc51d01000000000000003b443761d1055001e6b7778efacab8644557346fd36f7665063fb45902e9c9bb010000000000000099506d8a67e458bff8ceb93cb4d77f23957138399f11fc44996e61b1df1848ea010000000000000034acb964ffc586a04f38760e8106adeddc70ddd55958421ec92ae8b7d9f361850100000000000000bf47ea951bf20303e3ace58a8e95b79e1430c86b7ce97cc14b09f643e57192b20100000000000000c88c7938f90699566372f157528e56c14fc3402068e884dab3750c69e1b1f8db0100000000000000c859a8e75e32816122a9bb2c2e8fcc5a7df66dd791a83dd93eb2e68c7e39eade01000000000000004eaa2f286d28362d7bf043b6d32a5103bcff74d79b2260e6665d284b91a9a96d010000000000000014fa616d5b1d2f5403eee710709323ce2473f08d77b9e00d20ed517e52f4d9bc01000000000000005690eca4b54536fe07095867528709767a92b3e38852c2d051b842c5e9c3af1801000000000000005873f921ecc605ae9f4a98a5b59505a652fd643bb47ff66ab7cc096e957dea40010000000000000065d9a5d50ad7af8ed34f82a6a3cb1f84ca8271244c125d4d4349066aa456339101000000000000000ee3e9057a71d6c6501548b6e3785802de52b8a5e87d9b9e521eea2b05918b2f0100000000000000d2096c995e2bfdf52a7c60fe4db0d64bb64d9fc9e35df3bec5e7c6c837fe45a3010000000000000064fa44e74901bd0498d7f005e757c8269cbad75323d6c678fad732205c184e5c0100000000000000b45ea41759de4869aafd521bac1313a9e7a5e636fe864eb3c827e5f84fee4bc20100000000000000b33f15c24ddf0483bd1f374a2dc440c65391e8b54c9600694fb8f9dce0a85ef2010000000000000015dac847cdd7eb29e0ca6dc9e9586c5cfc6cda407692459413dd41de69589dc401000000000000007d5db285beb057c06c19de876d779b02a43a2b77747467fd3b4a946c90ec988a0100000000000000e5b4449610d8c33729d096216500ad634fe3654420f70b7fbaeb746ff8ba973a0100000000000000f5bf306dee98b6057c9cd931f5f1c9c9d36eb591ace063ec8bb3b1150a31f2e5010000000000000061d557a1396893c6f07979cbf2acefbc5494fd0c71c83d7066e1a7e62b0701c301000000000000002fb5594b7750ee0cafdde385912ea201ae53162aa66ee5588c0360391773f8880100000000000000905c647553c2f1eb6171e516416628b583d3d8871e40de09bd826ac4a4c4c8b30100000000000000f2881bc07407ab75cca9e3c1a93174b8a97f69362d39d35c755a1b426f131434010000000000000015a4d7b82bacbdbcec37aa687061702d65fd490d75562dda60db8da4696123140100000000000000e234848bc7615f26aa2cddc4a41a132aa41e2e6e8b53634b9228ef1b12760af50100000000000000ebb20ae018368ac9384a619d41cd04f3f622a710463f12eda65104ee48be4d4501000000000000008077d6e23edffc7b51ad360e560474a6581e728461a40e015813c96467263d9f010000000000000000000000"
	bytes, err := hex.DecodeString(hexData)
	require.NoError(t, err)

	consensusDigest := types.GrandpaConsensusDigest{}
	err = scale.Unmarshal(bytes, &consensusDigest)
	require.NoError(t, err)

	scheduledChange, err := consensusDigest.Value()
	require.NoError(t, err)

	parsedScheduledChange, ok := scheduledChange.(types.GrandpaScheduledChange)
	require.True(t, ok)
	require.Equal(t, 2, parsedScheduledChange.Auths)
}
