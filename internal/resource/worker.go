package resource

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidCapacity = errors.New("invalid worker capacity")
	ErrInvalidRequest  = errors.New("invalid resource request")
)

// WorkerResources tracks the capacity and current allocation of one worker.
//
// The mutex protects in-process accounting. This does NOT replace the
// PostgreSQL transaction that will later make scheduling + reservation
// atomic across coordinator operations.
type WorkerResources struct {
	mu sync.RWMutex

	capacity  Vector
	allocated Vector
}

func NewWorkerResources(capacity Vector) (*WorkerResources, error) {
	if err := capacity.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCapacity, err)
	}

	return &WorkerResources{
		capacity: capacity,
	}, nil
}

func (r *WorkerResources) Capacity() Vector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.capacity
}

func (r *WorkerResources) Allocated() Vector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.allocated
}

func (r *WorkerResources) Available() Vector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	available, err := r.capacity.Subtract(r.allocated)
	if err != nil {
		// This represents an internal invariant violation.
		panic("resource invariant violated: allocated exceeds capacity")
	}

	return available
}

// CanReserve reports whether request can currently fit on this worker.
func (r *WorkerResources) CanReserve(request Vector) bool {
	if request.Validate() != nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	available, err := r.capacity.Subtract(r.allocated)
	if err != nil {
		return false
	}

	return available.Fits(request)
}

// Reserve atomically reserves resources within this WorkerResources object.
//
// Database-level atomicity between job assignment and resource reservation
// will be added when this abstraction is integrated with Forge's scheduler.
func (r *WorkerResources) Reserve(request Vector) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	available, err := r.capacity.Subtract(r.allocated)
	if err != nil {
		return fmt.Errorf(
			"resource invariant violated: allocated exceeds capacity: %w",
			err,
		)
	}

	if !available.Fits(request) {
		return fmt.Errorf(
			"%w: requested %s, available %s",
			ErrInsufficient,
			request,
			available,
		)
	}

	r.allocated = r.allocated.Add(request)

	return nil
}

// Release returns resources previously reserved by a job.
func (r *WorkerResources) Release(request Vector) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	next, err := r.allocated.Subtract(request)
	if err != nil {
		return fmt.Errorf(
			"cannot release unallocated resources: allocated %s, release %s: %w",
			r.allocated,
			request,
			err,
		)
	}

	r.allocated = next

	return nil
}
