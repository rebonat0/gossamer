package prospectiveparachains

import (
	"bytes"
	"context"
	"testing"

	fragmentchain "github.com/ChainSafe/gossamer/dot/parachain/prospective-parachains/fragment-chain"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	inclusionemulator "github.com/ChainSafe/gossamer/dot/parachain/util/inclusion-emulator"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/assert"
)

const MaxPovSize = 1_000_000


func dummyPVD(parentHead parachaintypes.HeadData, relayParentNumber uint32) parachaintypes.PersistedValidationData {
	return parachaintypes.PersistedValidationData{
		ParentHead:             parentHead,
		RelayParentNumber:      relayParentNumber,
		RelayParentStorageRoot: common.EmptyHash,
		MaxPovSize:             MaxPovSize,
	}
}

func dummyCandidateReceiptBadSig(
	relayParentHash common.Hash,
	commitments *common.Hash,
) parachaintypes.CandidateReceipt {
	var commitmentsHash common.Hash

	if commitments != nil {
		commitmentsHash = *commitments
	} else {
		commitmentsHash = common.EmptyHash // TODO:
	}

	descriptor := parachaintypes.CandidateDescriptor{
		ParaID:                      parachaintypes.ParaID(0),
		RelayParent:                 relayParentHash,
		Collator:                    parachaintypes.CollatorID{},
		PovHash:                     common.EmptyHash,
		ErasureRoot:                 common.EmptyHash,
		Signature:                   parachaintypes.CollatorSignature{},
		ParaHead:                    common.EmptyHash,
		ValidationCodeHash:          parachaintypes.ValidationCodeHash{},
		PersistedValidationDataHash: common.EmptyHash,
	}

	return parachaintypes.CandidateReceipt{
		CommitmentsHash: commitmentsHash,
		Descriptor:      descriptor,
	}
}

func makeCandidate(

	relayParent common.Hash,
	relayParentNumber uint32,
	paraID parachaintypes.ParaID,
	parentHead parachaintypes.HeadData,
	headData parachaintypes.HeadData,
	validationCodeHash parachaintypes.ValidationCodeHash,
) parachaintypes.CommittedCandidateReceipt {
	pvd := dummyPVD(parentHead, relayParentNumber)

	commitments := parachaintypes.CandidateCommitments{
		HeadData:                  headData,
		HorizontalMessages:        []parachaintypes.OutboundHrmpMessage{},
		UpwardMessages:            []parachaintypes.UpwardMessage{},
		NewValidationCode:         nil,
		ProcessedDownwardMessages: 0,
		HrmpWatermark:             relayParentNumber,
	}

	commitmentsHash := commitments.Hash()

	candidate := dummyCandidateReceiptBadSig(relayParent, &commitmentsHash)
	candidate.CommitmentsHash = commitments.Hash()
	candidate.Descriptor.ParaID = paraID

	pvdh, err := pvd.Hash()

	if err != nil {
		panic(err)
	}

	candidate.Descriptor.PersistedValidationDataHash = pvdh
	candidate.Descriptor.ValidationCodeHash = validationCodeHash

	result := parachaintypes.CommittedCandidateReceipt{
		Descriptor:  candidate.Descriptor,
		Commitments: commitments,
	}

	return result
}

func introduceSecondedCandidate(
	t *testing.T,
	overseerToSubsystem chan any,
	candidate parachaintypes.CommittedCandidateReceipt,
	pvd parachaintypes.PersistedValidationData,
) {
	req := IntroduceSecondedCandidateRequest{
		CandidateParaID:         candidate.Descriptor.ParaID,
		CandidateReceipt:        candidate,
		PersistedValidationData: pvd,
	}

	response := make(chan bool)

	msg := IntroduceSecondedCandidate{
		IntroduceSecondedCandidateRequest: req,
		Response:                          response,
	}

	overseerToSubsystem <- msg

	assert.True(t, <-response)
}

func introduceSecondedCandidateFailed(
	t *testing.T,
	overseerToSubsystem chan any,
	candidate parachaintypes.CommittedCandidateReceipt,
	pvd parachaintypes.PersistedValidationData,
) {
	req := IntroduceSecondedCandidateRequest{
		CandidateParaID:         candidate.Descriptor.ParaID,
		CandidateReceipt:        candidate,
		PersistedValidationData: pvd,
	}

	response := make(chan bool)

	msg := IntroduceSecondedCandidate{
		IntroduceSecondedCandidateRequest: req,
		Response:                          response,
	}

	overseerToSubsystem <- msg

	assert.False(t, <-response)
}

func TestFailedIntroduceSecondedCandidateWhenMissingViewPerRelayParent(
	t *testing.T,
) {
	candidateRelayParent := common.Hash{0x01}
	paraId := parachaintypes.ParaID(1)
	parentHead := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x01}, 32),
	}
	headData := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x02}, 32),
	}
	validationCodeHash := parachaintypes.ValidationCodeHash{0x01}
	candidateRelayParentNumber := uint32(1)

	candidate := makeCandidate(
		candidateRelayParent,
		candidateRelayParentNumber,
		paraId,
		parentHead,
		headData,
		validationCodeHash,
	)

	pvd := dummyPVD(parentHead, candidateRelayParentNumber)

	subsystemToOverseer := make(chan any)
	overseerToSubsystem := make(chan any)

	prospectiveParachains := NewProspectiveParachains(subsystemToOverseer)

	go prospectiveParachains.Run(context.Background(), overseerToSubsystem)

	introduceSecondedCandidateFailed(t, overseerToSubsystem, candidate, pvd)
}

func TestFailedIntroduceSecondedCandidateWhenParentHeadAndHeadDataEquals(
	t *testing.T,
) {
	candidateRelayParent := common.Hash{0x01}
	paraId := parachaintypes.ParaID(1)
	parentHead := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x01}, 32),
	}
	headData := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x01}, 32),
	}
	validationCodeHash := parachaintypes.ValidationCodeHash{0x01}
	candidateRelayParentNumber := uint32(1)

	candidate := makeCandidate(
		candidateRelayParent,
		candidateRelayParentNumber,
		paraId,
		parentHead,
		headData,
		validationCodeHash,
	)

	pvd := dummyPVD(parentHead, candidateRelayParentNumber)

	subsystemToOverseer := make(chan any)
	overseerToSubsystem := make(chan any)

	prospectiveParachains := NewProspectiveParachains(subsystemToOverseer)

	relayParent := inclusionemulator.RelayChainBlockInfo{
		Hash:        candidateRelayParent,
		Number:      0,
		StorageRoot: common.Hash{0x00},
	}

	baseConstraints := &inclusionemulator.Constraints{
		RequiredParent:       parachaintypes.HeadData{Data: []byte{byte(0)}},
		MinRelayParentNumber: 0,
		ValidationCodeHash:   parachaintypes.ValidationCodeHash(common.Hash{0x03}),
	}

	scope, err := fragmentchain.NewScopeWithAncestors(relayParent, baseConstraints, nil, 10, nil)
	assert.NoError(t, err)

	prospectiveParachains.View.PerRelayParent[candidateRelayParent] = RelayBlockViewData{
		FragmentChains: map[parachaintypes.ParaID]fragmentchain.FragmentChain{
			paraId: *fragmentchain.NewFragmentChain(scope, fragmentchain.NewCandidateStorage()),
		},
	}
	go prospectiveParachains.Run(context.Background(), overseerToSubsystem)

	introduceSecondedCandidateFailed(t, overseerToSubsystem, candidate, pvd)
}

func TestHandleIntroduceSecondedCandidate(
	t *testing.T,
) {
	candidateRelayParent := common.Hash{0x01}
	paraId := parachaintypes.ParaID(1)
	parentHead := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x01}, 32),
	}
	headData := parachaintypes.HeadData{
		Data: bytes.Repeat([]byte{0x02}, 32),
	}
	validationCodeHash := parachaintypes.ValidationCodeHash{0x01}
	candidateRelayParentNumber := uint32(1)

	candidate := makeCandidate(
		candidateRelayParent,
		candidateRelayParentNumber,
		paraId,
		parentHead,
		headData,
		validationCodeHash,
	)

	pvd := dummyPVD(parentHead, candidateRelayParentNumber)

	subsystemToOverseer := make(chan any)
	overseerToSubsystem := make(chan any)

	prospectiveParachains := NewProspectiveParachains(subsystemToOverseer)

	relayParent := inclusionemulator.RelayChainBlockInfo{
		Hash:        candidateRelayParent,
		Number:      0,
		StorageRoot: common.Hash{0x00},
	}

	baseConstraints := &inclusionemulator.Constraints{
		RequiredParent:       parachaintypes.HeadData{Data: []byte{byte(0)}},
		MinRelayParentNumber: 0,
		ValidationCodeHash:   parachaintypes.ValidationCodeHash(common.Hash{0x03}),
	}

	scope, err := fragmentchain.NewScopeWithAncestors(relayParent, baseConstraints, nil, 10, nil)
	assert.NoError(t, err)

	prospectiveParachains.View.PerRelayParent[candidateRelayParent] = RelayBlockViewData{
		FragmentChains: map[parachaintypes.ParaID]fragmentchain.FragmentChain{
			paraId: *fragmentchain.NewFragmentChain(scope, fragmentchain.NewCandidateStorage()),
		},
	}
	go prospectiveParachains.Run(context.Background(), overseerToSubsystem)

	introduceSecondedCandidate(t, overseerToSubsystem, candidate, pvd)
}
