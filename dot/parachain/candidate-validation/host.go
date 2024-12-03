package candidatevalidation

import (
	"fmt"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/internal/log"
	"github.com/ChainSafe/gossamer/pkg/scale"
)

var logger = log.NewFromGlobal(log.AddContext("pkg", "candidatevalidation"), log.SetLevel(log.Debug))

// host is the struct that holds the workerPool which is responsible for executing the validation tasks
type host struct {
	workerPool *workerPool
}

func newValidationHost() *host {
	return &host{
		workerPool: newWorkerPool(),
	}
}
func (v *host) precheck(data PvFPrepData) error {
	return v.workerPool.handlePrecheckPvF(data)
}

func (v *host) validate(msg *ValidationTask) (*ValidationResult, error) {
	validationCodeHash := msg.ValidationCode.Hash()
	validationErr, internalErr := performBasicChecks(&msg.CandidateReceipt.Descriptor,
		msg.PersistedValidationData.MaxPovSize,
		msg.PoV,
		validationCodeHash)

	if internalErr != nil {
		return nil, internalErr
	}
	if validationErr != nil {
		return &ValidationResult{Invalid: validationErr}, nil //nolint
	}

	// submit request
	return v.workerPool.executeRequest(msg)
}

// performBasicChecks does basic checks of a candidate. Provided the encoded PoV-block it returns ReasonForInvalidity
// and internal error if any.
func performBasicChecks(candidate *parachaintypes.CandidateDescriptor, maxPoVSize uint32,
	pov parachaintypes.PoV, validationCodeHash parachaintypes.ValidationCodeHash) (
	validationError *ReasonForInvalidity, internalError error) {
	povHash, err := pov.Hash()
	if err != nil {
		return nil, fmt.Errorf("hashing PoV: %w", err)
	}

	encodedPoV, err := scale.Marshal(pov)
	if err != nil {
		return nil, fmt.Errorf("encoding PoV: %w", err)
	}
	encodedPoVSize := uint32(len(encodedPoV))

	if encodedPoVSize > maxPoVSize {
		ci := ParamsTooLarge
		return &ci, nil
	}

	if povHash != candidate.PovHash {
		ci := PoVHashMismatch
		return &ci, nil
	}

	if validationCodeHash != candidate.ValidationCodeHash {
		ci := CodeHashMismatch
		return &ci, nil
	}

	err = candidate.CheckCollatorSignature()
	if err != nil {
		ci := BadSignature
		return &ci, nil
	}

	return nil, nil
}