package resource

import (
	"errors"
	"fmt"
)

var (
	ErrNegativeGPUMin       = errors.New("minimum GPU count cannot be negative")
	ErrInvalidGPUEnvelope   = errors.New("invalid GPU allocation envelope")
	ErrAllocationOutOfRange = errors.New("GPU allocation outside permitted envelope")
)

// GPUEnvelope describes the range of GPU allocations under which a
// malleable workload can execute.
//
// Min is the smallest viable allocation.
// Preferred is the allocation the workload would like under unconstrained
// cluster conditions.
// Max is the largest useful allocation.
//
// The invariant is:
//
//	Min <= Preferred <= Max
type GPUEnvelope struct {
	Min       int32
	Preferred int32
	Max       int32
}

func (e GPUEnvelope) Validate() error {
	if e.Min < 0 {
		return ErrNegativeGPUMin
	}

	if e.Preferred < 0 || e.Max < 0 {
		return ErrInvalidGPUEnvelope
	}

	if e.Min > e.Preferred {
		return fmt.Errorf(
			"%w: min %d exceeds preferred %d",
			ErrInvalidGPUEnvelope,
			e.Min,
			e.Preferred,
		)
	}

	if e.Preferred > e.Max {
		return fmt.Errorf(
			"%w: preferred %d exceeds max %d",
			ErrInvalidGPUEnvelope,
			e.Preferred,
			e.Max,
		)
	}

	return nil
}

func (e GPUEnvelope) Contains(gpus int32) bool {
	return gpus >= e.Min && gpus <= e.Max
}

func (e GPUEnvelope) IsFixed() bool {
	return e.Min == e.Preferred &&
		e.Preferred == e.Max
}

func (e GPUEnvelope) IsMalleable() bool {
	return !e.IsFixed()
}
