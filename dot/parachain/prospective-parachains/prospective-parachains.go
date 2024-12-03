package prospectiveparachains

import (
	"context"
	"errors"

	"github.com/ChainSafe/gossamer/dot/parachain/backing"
	fragmentchain "github.com/ChainSafe/gossamer/dot/parachain/prospective-parachains/fragment-chain"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/internal/log"
	common "github.com/ChainSafe/gossamer/lib/common"
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

func HandleIntroduceSecondedCandidate(
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
		logger.Debugf("Added seconded candidate to %d relay parents: %s", len(added), candidateHash)
	}

	response <- len(added) > 0
}

type ProspectiveParachains struct {
	SubsystemToOverseer chan<- any
	View                *View
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
		HandleIntroduceSecondedCandidate(
			pp.View,
			msg.IntroduceSecondedCandidateRequest,
			msg.Response,
		)
	case CandidateBacked:
		panic("not implemented yet: see issue #4309")
	case GetBackableCandidates:
		panic("not implemented yet: see issue #4310")
	case GetHypotheticalMembership:
		panic("not implemented yet: see issue #4311")
	case GetMinimumRelayParents:
		panic("not implemented yet: see issue #4312")
	case GetProspectiveValidationData:
		panic("not implemented yet: see issue #4313")
	default:
		logger.Errorf("%w: %T", parachaintypes.ErrUnknownOverseerMessage, msg)
	}

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
