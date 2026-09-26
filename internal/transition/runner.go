package transition

import (
	"context"
	"fmt"
)

// Store is the persistence interface required by Runner.
//
// internal/storage.Store satisfies this interface without transition
// depending directly on the storage package.
type Store interface {
	AdvanceTransition(
		ctx context.Context,
		id string,
		next State,
	) (Transition, error)

	CompleteTransition(
		ctx context.Context,
		id string,
	) (Transition, error)

	FailTransition(
		ctx context.Context,
		id string,
		reason string,
	) (Transition, error)
}

// Runner drives a persisted transition through its execution lifecycle.
//
// State progression belongs here. Executors perform workload-specific
// mechanics but do not mutate transition state themselves.
type Runner struct {
	Store    Store
	Executor Executor
}

func (r Runner) Run(
	ctx context.Context,
	tr Transition,
) (ExecutionResult, error) {
	var result ExecutionResult

	if r.Store == nil {
		return result, fmt.Errorf(
			"transition runner store is nil",
		)
	}

	if r.Executor == nil {
		return result, fmt.Errorf(
			"transition runner executor is nil",
		)
	}

	if tr.State != StatePreparing {
		return result, fmt.Errorf(
			"transition must be PREPARING, got %s",
			tr.State,
		)
	}

	execution := Execution{
		TransitionID: tr.ID,
		JobID:        tr.JobID,
		WorkerID:     tr.WorkerID,
		Source:       tr.Source,
		Target:       tr.Target,
	}

	var err error

	result.Prepare, err = r.Executor.Prepare(
		ctx,
		execution,
	)
	if err != nil {
		return result, r.fail(
			ctx,
			tr.ID,
			StatePreparing,
			err,
		)
	}

	// PREPARING -> CHECKPOINTING
	tr, err = r.Store.AdvanceTransition(
		ctx,
		tr.ID,
		StateCheckpointing,
	)
	if err != nil {
		return result, err
	}

	result.Checkpoint, err = r.Executor.Checkpoint(
		ctx,
		execution,
	)
	if err != nil {
		return result, r.fail(
			ctx,
			tr.ID,
			StateCheckpointing,
			err,
		)
	}

	// CHECKPOINTING -> RECONFIGURING
	tr, err = r.Store.AdvanceTransition(
		ctx,
		tr.ID,
		StateReconfiguring,
	)
	if err != nil {
		return result, err
	}

	result.Reconfigure, err = r.Executor.Reconfigure(
		ctx,
		execution,
	)
	if err != nil {
		return result, r.fail(
			ctx,
			tr.ID,
			StateReconfiguring,
			err,
		)
	}

	// RECONFIGURING -> RESTORING
	tr, err = r.Store.AdvanceTransition(
		ctx,
		tr.ID,
		StateRestoring,
	)
	if err != nil {
		return result, err
	}

	result.Restore, err = r.Executor.Restore(
		ctx,
		execution,
	)
	if err != nil {
		return result, r.fail(
			ctx,
			tr.ID,
			StateRestoring,
			err,
		)
	}

	// RESTORING -> RESUMING
	tr, err = r.Store.AdvanceTransition(
		ctx,
		tr.ID,
		StateResuming,
	)
	if err != nil {
		return result, err
	}

	result.Resume, err = r.Executor.Resume(
		ctx,
		execution,
	)
	if err != nil {
		return result, r.fail(
			ctx,
			tr.ID,
			StateResuming,
			err,
		)
	}

	// RESUMING -> COMPLETED
	_, err = r.Store.CompleteTransition(
		ctx,
		tr.ID,
	)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (r Runner) fail(
	ctx context.Context,
	transitionID string,
	stage State,
	cause error,
) error {
	reason := fmt.Sprintf(
		"%s failed: %v",
		stage,
		cause,
	)

	_, failErr := r.Store.FailTransition(
		ctx,
		transitionID,
		reason,
	)

	if failErr != nil {
		return fmt.Errorf(
			"transition stage failed: %v; "+
				"also failed to persist FAILED state: %w",
			cause,
			failErr,
		)
	}

	return fmt.Errorf(
		"transition %s: %w",
		stage,
		cause,
	)
}
