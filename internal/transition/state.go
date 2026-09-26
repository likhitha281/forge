package transition

import (
	"errors"
	"fmt"
)

type State string

const (
	StateRequested     State = "REQUESTED"
	StatePreparing     State = "PREPARING"
	StateCheckpointing State = "CHECKPOINTING"
	StateReconfiguring State = "RECONFIGURING"
	StateRestoring     State = "RESTORING"
	StateResuming      State = "RESUMING"
	StateCompleted     State = "COMPLETED"
	StateFailed        State = "FAILED"
)

var ErrInvalidTransition = errors.New("invalid transition state change")

func (s State) Valid() bool {
	switch s {
	case StateRequested,
		StatePreparing,
		StateCheckpointing,
		StateReconfiguring,
		StateRestoring,
		StateResuming,
		StateCompleted,
		StateFailed:
		return true
	default:
		return false
	}
}

func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed
}

func CanTransition(from, to State) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}

	if from.Terminal() {
		return false
	}

	// Failure may occur from any non-terminal state.
	if to == StateFailed {
		return true
	}

	switch from {
	case StateRequested:
		return to == StatePreparing

	case StatePreparing:
		return to == StateCheckpointing

	case StateCheckpointing:
		return to == StateReconfiguring

	case StateReconfiguring:
		return to == StateRestoring

	case StateRestoring:
		return to == StateResuming

	case StateResuming:
		return to == StateCompleted

	default:
		return false
	}
}

func ValidateTransition(from, to State) error {
	if !CanTransition(from, to) {
		return fmt.Errorf(
			"%w: %s -> %s",
			ErrInvalidTransition,
			from,
			to,
		)
	}

	return nil
}
