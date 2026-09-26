package transition

import (
	"errors"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

var (
	ErrSameAllocation = errors.New(
		"source and target allocations are identical",
	)

	ErrInvalidTarget = errors.New(
		"target allocation is outside job requirements",
	)
)

type Transition struct {
	ID    string
	JobID string

	WorkerID string

	State State

	Source   resource.Vector
	Target   resource.Vector
	Reserved resource.Vector

	RequestedAt time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	FailureReason string

	BytesMoved int64
}

func New(
	id string,
	jobID string,
	source resource.Vector,
	target resource.Vector,
	requirements resource.JobResources,
	now time.Time,
) (Transition, error) {
	if source.Equal(target) {
		return Transition{}, ErrSameAllocation
	}

	if !requirements.Accepts(target) {
		return Transition{}, ErrInvalidTarget
	}

	return Transition{
		ID:          id,
		JobID:       jobID,
		State:       StateRequested,
		Source:      source,
		Target:      target,
		RequestedAt: now,
	}, nil
}

// RequiredReservation returns only the additional resources that must be
// protected while moving from source to target.
//
// Growing 2 -> 4 GPUs reserves 2 GPUs.
// Shrinking 4 -> 2 GPUs reserves nothing.
func RequiredReservation(
	source resource.Vector,
	target resource.Vector,
) resource.Vector {
	reservation := resource.Vector{}

	if target.CPUCores > source.CPUCores {
		reservation.CPUCores =
			target.CPUCores - source.CPUCores
	}

	if target.MemoryMB > source.MemoryMB {
		reservation.MemoryMB =
			target.MemoryMB - source.MemoryMB
	}

	if target.GPUs > source.GPUs {
		reservation.GPUs =
			target.GPUs - source.GPUs
	}

	return reservation
}

func (t Transition) Duration() (time.Duration, bool) {
	if t.StartedAt == nil || t.CompletedAt == nil {
		return 0, false
	}

	return t.CompletedAt.Sub(*t.StartedAt), true
}
