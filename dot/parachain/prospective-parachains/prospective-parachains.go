package prospectiveparachains

import (
	"context"
	"errors"

	"github.com/ChainSafe/gossamer/dot/parachain/backing"
	fragmentchain "github.com/ChainSafe/gossamer/dot/parachain/prospective-parachains/fragment-chain"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/internal/log"
	"github.com/ChainSafe/gossamer/lib/common
)

var logger = log.NewFromGlobal(log.AddContext("pkg", "prospective_parachains"), log.SetLevel(log.Debug))

type RelayBlockViewData struct {
	// The fragment chains for current and upcoming scheduled paras.
	FragmentChains map[parachaintypes.ParaID]fragmentchain.FragmentChain
}

type View struct {
	// Per relay parent fragment chains. These includes all relay parents under the implicit view.
	PerRelayParent map[common.Hash]RelayBlockViewData
	// The hashes of the currently active leaves. This is a subset of the keys in
	// `per_relay_parent`.
	ActiveLeaves map[common.Hash]struct{}
	// The backing implicit view.
	ImplicitView backing.ImplicitView
}

// Initialize with empty values.
func NewView() *View {
	return &View{
		PerRelayParent: make(map[common.Hash]RelayBlockViewData),
		ActiveLeaves:   make(map[common.Hash]struct{}),
		ImplicitView:   nil, // TODO: currently there's no implementation for ImplicitView, reference is: https://github.com/paritytech/polkadot-sdk/blob/028e61be43f05f6f6c88c5cca94160f8db075585/polkadot/node/subsystem-util/src/backing_implicit_view.rs#L40
	}
}

// Get the fragment chains of this leaf.
func (v *View) GetFragmentChains(leaf common.Hash) map[parachaintypes.ParaID]fragmentchain.FragmentChain {
	if viewData, ok := v.PerRelayParent[leaf]; ok {
		return viewData.FragmentChains
	}
	return nil
}

type ProspectiveParachains struct {
	SubsystemToOverseer chan<- any
	View                *view
}

type view struct {
	activeLeaves   map[common.Hash]bool
	perRelayParent map[common.Hash]*relayParentData
}

type relayParentData struct {
	fragmentChains map[parachaintypes.ParaID]*fragmentChain
}

// Name returns the name of the subsystem
func (*ProspectiveParachains) Name() parachaintypes.SubSystemName {
	return parachaintypes.ProspectiveParachains
}

// NewProspectiveParachains creates a new ProspectiveParachain subsystem
func NewProspectiveParachains(overseerChan chan<- any) *ProspectiveParachains {
	prospectiveParachain := ProspectiveParachains{
		SubsystemToOverseer: overseerChan,
		View:                NewView(),
	}
	return &prospectiveParachain
}

// Run starts the ProspectiveParachains subsystem
func (pp *ProspectiveParachains) Run(ctx context.Context, overseerToSubsystem <-chan any) {
	for {
		select {
		case msg := <-overseerToSubsystem:
			pp.processMessage(msg)
		case <-ctx.Done():
			if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
				logger.Errorf("ctx error: %s\n", err)
			}
			return
		}
	}
}

func (*ProspectiveParachains) Stop() {}

func (pp *ProspectiveParachains) processMessage(msg any) {
	switch msg := msg.(type) {
	case parachaintypes.Conclude:
		pp.Stop()
	case parachaintypes.ActiveLeavesUpdateSignal:
		_ = pp.ProcessActiveLeavesUpdateSignal(msg)
	case parachaintypes.BlockFinalizedSignal:
		_ = pp.ProcessBlockFinalizedSignal(msg)
	case IntroduceSecondedCandidate:
		pp.introduceSecondedCandidate(
			pp.View,
			msg.IntroduceSecondedCandidateRequest,
			msg.Response,
		)
	case CandidateBacked:
		panic("not implemented yet: see issue #4309")
	case GetBackableCandidates:
		pp.getBackableCandidates(msg)
	case GetHypotheticalMembership:
		panic("not implemented yet: see issue #4311")
	case GetMinimumRelayParents:
		// Directly use the msg since it's already of type GetMinimumRelayParents
		pp.getMinimumRelayParents(msg.RelayChainBlockHash, msg.Sender)
	case GetProspectiveValidationData:
		panic("not implemented yet: see issue #4313")
	default:
		logger.Errorf("%w: %T", parachaintypes.ErrUnknownOverseerMessage, msg)
	}

}

func (pp *ProspectiveParachains) introduceSecondedCandidate(
	view *View,
	request IntroduceSecondedCandidateRequest,
	response chan bool,
) {
	para := request.CandidateParaID
	candidate := request.CandidateReceipt
	pvd := request.PersistedValidationData

	hash, err := candidate.Hash()

	if err != nil {
		logger.Tracef("hashing candidate: %s", err.Error())
		response <- false
		return
	}

	candidateHash := parachaintypes.CandidateHash{Value: hash}

	entry, err := fragmentchain.NewCandidateEntry(
		candidateHash,
		candidate,
		pvd,
		fragmentchain.Seconded,
	)

	if err != nil {
		logger.Tracef("adding seconded candidate error: %s para: %v", err.Error(), para)
		response <- false
		return
	}

	added := make([]common.Hash, 0, len(view.PerRelayParent))
	paraScheduled := false

	for relayParent, rpData := range view.PerRelayParent {
		chain, exists := rpData.FragmentChains[para]

		if !exists {
			continue
		}

		_, isActiveLeaf := view.ActiveLeaves[relayParent]

		paraScheduled = true

		err = chain.TryAddingSecondedCandidate(entry)

		if err != nil {
			if errors.Is(err, fragmentchain.ErrCandidateAlradyKnown) {
				logger.Tracef(
					"attempting to introduce an already known candidate with hash: %s, para: %v relayParent: %v isActiveLeaf: %v",
					candidateHash,
					para,
					relayParent,
					isActiveLeaf,
				)
				added = append(added, relayParent)
			} else {
				logger.Tracef(
					"adding seconded candidate with hash: %s error: %s para: %v relayParent: %v isActiveLeaf: %v",
					candidateHash,
					err.Error(),
					para,
					relayParent,
					isActiveLeaf,
				)
			}
		} else {
			added = append(added, relayParent)
		}
	}

	if !paraScheduled {
		logger.Warnf(
			"received seconded candidate with hash: %s for inactive para: %v",
			candidateHash,
			para,
		)
	}

	if len(added) == 0 {
		logger.Debugf("newly-seconded candidate cannot be kept under any relay parent: %s", candidateHash)
	} else {
		logger.Tracef("added seconded candidate to %d relay parents: %s", len(added), candidateHash)
	}

	response <- len(added) > 0
}

// ProcessActiveLeavesUpdateSignal processes active leaves update signal
func (pp *ProspectiveParachains) ProcessActiveLeavesUpdateSignal(parachaintypes.ActiveLeavesUpdateSignal) error {
	panic("not implemented yet: see issue #4305")
}

// ProcessBlockFinalizedSignal processes block finalized signal
func (*ProspectiveParachains) ProcessBlockFinalizedSignal(parachaintypes.BlockFinalizedSignal) error {
	// NOTE: this subsystem does not process block finalized signal
	return nil
}

func (pp *ProspectiveParachains) getMinimumRelayParents(
	relayChainBlockHash common.Hash,
	sender chan []ParaIDBlockNumber,
) {
	var result []ParaIDBlockNumber

	// Check if the relayChainBlockHash exists in active_leaves
	if exists := pp.View.activeLeaves[relayChainBlockHash]; exists {
		// Retrieve data associated with the relayChainBlockHash
		if leafData, found := pp.View.perRelayParent[relayChainBlockHash]; found {
			// Iterate over fragment_chains and collect the data
			for paraID, fragmentChain := range leafData.fragmentChains {
				result = append(result, ParaIDBlockNumber{
					ParaId:      paraID,
					BlockNumber: fragmentChain.scope.relayParent.Number,
				})
			}
		}
	}

	// Send the result through the sender channel
	sender <- result
}

func (pp *ProspectiveParachains) getBackableCandidates(
	msg GetBackableCandidates,
) {
	// Extract details from the message
	relayParentHash := msg.RelayParentHash
	paraId := msg.ParaId
	requestedQty := msg.RequestedQty
	ancestors := msg.Ancestors
	responseChan := msg.Response

	// Check if the relay parent is active
	if _, exists := pp.View.activeLeaves[relayParentHash]; !exists {
		logger.Debugf(
			"Requested backable candidates for inactive relay-parent. "+
				"RelayParentHash: %v, ParaId: %v",
			relayParentHash, paraId,
		)
		responseChan <- []parachaintypes.CandidateHashAndRelayParent{}
		return
	}

	// Retrieve data for the relay parent
	data, ok := pp.View.perRelayParent[relayParentHash]
	if !ok {
		logger.Debugf(
			"Requested backable candidates for nonexistent relay-parent. "+
				"RelayParentHash: %v, ParaId: %v",
			relayParentHash, paraId,
		)
		responseChan <- []parachaintypes.CandidateHashAndRelayParent{}
		return
	}

	// Retrieve the fragment chain for the ParaID
	chain, ok := data.fragmentChains[paraId]
	if !ok {
		logger.Debugf(
			"Requested backable candidates for inactive ParaID. "+
				"RelayParentHash: %v, ParaId: %v",
			relayParentHash, paraId,
		)
		responseChan <- []parachaintypes.CandidateHashAndRelayParent{}
		return
	}

	// Retrieve backable candidates from the fragment chain
	backableCandidates := chain.findBackableChain(ancestors, requestedQty)
	if len(backableCandidates) == 0 {
		logger.Debugf(
			"No backable candidates found. RelayParentHash: %v, ParaId: %v, Ancestors: %v",
			relayParentHash, paraId, ancestors,
		)
		responseChan <- []parachaintypes.CandidateHashAndRelayParent{}
		return
	}

	logger.Debugf(
		"Found backable candidates: %v. RelayParentHash: %v, ParaId: %v, Ancestors: %v",
		backableCandidates, relayParentHash, paraId, ancestors,
	)

	// Convert backable candidates to the expected response format
	candidateHashes := make([]parachaintypes.CandidateHashAndRelayParent, len(backableCandidates))
	for i, candidate := range backableCandidates {
		candidateHashes[i] = parachaintypes.CandidateHashAndRelayParent{
			CandidateHash:        candidate.candidateHash,
			CandidateRelayParent: candidate.realyParentHash,
		}
	}

	// Send the result through the response channel
	responseChan <- candidateHashes
}
