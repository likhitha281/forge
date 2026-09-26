package transition

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeTransitionStore struct {
	transition Transition
	states     []State
}

func (s *fakeTransitionStore) AdvanceTransition(
	ctx context.Context,
	id string,
	next State,
) (Transition, error) {
	if id != s.transition.ID {
		return Transition{}, errors.New(
			"unknown transition",
		)
	}

	if err := ValidateTransition(
		s.transition.State,
		next,
	); err != nil {
		return Transition{}, err
	}

	s.transition.State = next
	s.states = append(
		s.states,
		next,
	)

	return s.transition, nil
}

func (s *fakeTransitionStore) FailTransition(
	ctx context.Context,
	id string,
	reason string,
) (Transition, error) {
	if id != s.transition.ID {
		return Transition{}, errors.New(
			"unknown transition",
		)
	}

	if err := ValidateTransition(
		s.transition.State,
		StateFailed,
	); err != nil {
		return Transition{}, err
	}

	s.transition.State = StateFailed
	s.transition.FailureReason = reason

	s.states = append(
		s.states,
		StateFailed,
	)

	return s.transition, nil
}

func TestRunnerHappyPath(t *testing.T) {
	tr := Transition{
		ID:       "transition-1",
		JobID:    "job-1",
		WorkerID: "worker-1",

		State: StatePreparing,

		Source: testExecution().Source,
		Target: testExecution().Target,
	}

	store := &fakeTransitionStore{
		transition: tr,
	}

	executor := SimulatedExecutor{
		PrepareDuration:     time.Millisecond,
		CheckpointDuration:  time.Millisecond,
		ReconfigureDuration: time.Millisecond,
		RestoreDuration:     time.Millisecond,
		ResumeDuration:      time.Millisecond,

		CheckpointBytes: 1024,
		RestoreBytes:    2048,
	}

	runner := Runner{
		Store:    store,
		Executor: executor,
	}

	result, err := runner.Run(
		context.Background(),
		tr,
	)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}

	if store.transition.State != StateCompleted {
		t.Fatalf(
			"final state = %s, want COMPLETED",
			store.transition.State,
		)
	}

	wantStates := []State{
		StateCheckpointing,
		StateReconfiguring,
		StateRestoring,
		StateResuming,
		StateCompleted,
	}

	if len(store.states) != len(wantStates) {
		t.Fatalf(
			"state count = %d, want %d",
			len(store.states),
			len(wantStates),
		)
	}

	for i := range wantStates {
		if store.states[i] != wantStates[i] {
			t.Fatalf(
				"state[%d] = %s, want %s",
				i,
				store.states[i],
				wantStates[i],
			)
		}
	}

	if result.TotalDuration() !=
		5*time.Millisecond {
		t.Fatalf(
			"total duration = %v, want 5ms",
			result.TotalDuration(),
		)
	}

	if result.TotalBytesMoved() != 3072 {
		t.Fatalf(
			"bytes moved = %d, want 3072",
			result.TotalBytesMoved(),
		)
	}
}

type failingExecutor struct {
	failStage State
}

var errInjectedFailure = errors.New(
	"injected executor failure",
)

func (e failingExecutor) Prepare(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	if e.failStage == StatePreparing {
		return StageResult{}, errInjectedFailure
	}

	return StageResult{}, nil
}

func (e failingExecutor) Checkpoint(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	if e.failStage == StateCheckpointing {
		return StageResult{}, errInjectedFailure
	}

	return StageResult{}, nil
}

func (e failingExecutor) Reconfigure(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	if e.failStage == StateReconfiguring {
		return StageResult{}, errInjectedFailure
	}

	return StageResult{}, nil
}

func (e failingExecutor) Restore(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	if e.failStage == StateRestoring {
		return StageResult{}, errInjectedFailure
	}

	return StageResult{}, nil
}

func (e failingExecutor) Resume(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	if e.failStage == StateResuming {
		return StageResult{}, errInjectedFailure
	}

	return StageResult{}, nil
}

func TestRunnerFailureMarksTransitionFailed(
	t *testing.T,
) {
	tr := Transition{
		ID:       "transition-1",
		JobID:    "job-1",
		WorkerID: "worker-1",

		State: StatePreparing,

		Source: testExecution().Source,
		Target: testExecution().Target,
	}

	store := &fakeTransitionStore{
		transition: tr,
	}

	runner := Runner{
		Store: store,
		Executor: failingExecutor{
			failStage: StateCheckpointing,
		},
	}

	_, err := runner.Run(
		context.Background(),
		tr,
	)

	if !errors.Is(
		err,
		errInjectedFailure,
	) {
		t.Fatalf(
			"error = %v, want injected failure",
			err,
		)
	}

	if store.transition.State != StateFailed {
		t.Fatalf(
			"final state = %s, want FAILED",
			store.transition.State,
		)
	}

	wantStates := []State{
		StateCheckpointing,
		StateFailed,
	}

	if len(store.states) != len(wantStates) {
		t.Fatalf(
			"state count = %d, want %d",
			len(store.states),
			len(wantStates),
		)
	}

	for i := range wantStates {
		if store.states[i] != wantStates[i] {
			t.Fatalf(
				"state[%d] = %s, want %s",
				i,
				store.states[i],
				wantStates[i],
			)
		}
	}

	if store.transition.FailureReason == "" {
		t.Fatal(
			"expected failure reason",
		)
	}
}

func TestRunnerRejectsNonPreparingTransition(
	t *testing.T,
) {
	tr := Transition{
		ID:       "transition-1",
		JobID:    "job-1",
		WorkerID: "worker-1",

		// REQUESTED has not yet been claimed by LeaseTransition.
		// Runner may execute only an already-claimed PREPARING
		// transition.
		State: StateRequested,

		Source: testExecution().Source,
		Target: testExecution().Target,
	}

	store := &fakeTransitionStore{
		transition: tr,
	}

	runner := Runner{
		Store:    store,
		Executor: SimulatedExecutor{},
	}

	_, err := runner.Run(
		context.Background(),
		tr,
	)

	if err == nil {
		t.Fatal(
			"expected non-PREPARING transition to be rejected",
		)
	}

	if store.transition.State != StateRequested {
		t.Fatalf(
			"transition state changed to %s, want REQUESTED",
			store.transition.State,
		)
	}

	if len(store.states) != 0 {
		t.Fatalf(
			"runner persisted %d state changes, want 0",
			len(store.states),
		)
	}
}

func (s *fakeTransitionStore) CompleteTransition(
	ctx context.Context,
	id string,
) (Transition, error) {
	if id != s.transition.ID {
		return Transition{},
			errors.New("unknown transition")
	}

	if err := ValidateTransition(
		s.transition.State,
		StateCompleted,
	); err != nil {
		return Transition{}, err
	}

	s.transition.State = StateCompleted
	s.states = append(
		s.states,
		StateCompleted,
	)

	return s.transition, nil
}
