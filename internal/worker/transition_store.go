package worker

import (
	"context"
	"errors"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/resource"
	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

// TransitionStore adapts the coordinator's gRPC API to the persistence
// interface required by transition.Runner.
//
// Workers never access PostgreSQL directly.
type TransitionStore struct {
	client   forgev1.ForgeClient
	workerID string
}

func NewTransitionStore(
	client forgev1.ForgeClient,
	workerID string,
) *TransitionStore {
	return &TransitionStore{
		client:   client,
		workerID: workerID,
	}
}

func (s *TransitionStore) AdvanceTransition(
	ctx context.Context,
	id string,
	next transitionmodel.State,
) (transitionmodel.Transition, error) {
	state, err := transitionStatePB(next)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	response, err := s.client.AdvanceTransition(
		ctx,
		&forgev1.AdvanceTransitionRequest{
			WorkerId:     s.workerID,
			TransitionId: id,
			NextState:    state,
		},
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if response.Transition == nil {
		return transitionmodel.Transition{},
			errors.New("coordinator returned nil transition")
	}

	return transitionFromPB(
		response.Transition,
	)
}

func (s *TransitionStore) CompleteTransition(
	ctx context.Context,
	id string,
) (transitionmodel.Transition, error) {
	response, err := s.client.CompleteTransition(
		ctx,
		&forgev1.CompleteTransitionRequest{
			WorkerId:     s.workerID,
			TransitionId: id,
		},
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if response.Transition == nil {
		return transitionmodel.Transition{},
			errors.New("coordinator returned nil transition")
	}

	return transitionFromPB(
		response.Transition,
	)
}

func (s *TransitionStore) FailTransition(
	ctx context.Context,
	id string,
	reason string,
) (transitionmodel.Transition, error) {
	response, err := s.client.FailTransition(
		ctx,
		&forgev1.FailTransitionRequest{
			WorkerId:     s.workerID,
			TransitionId: id,
			Reason:       reason,
		},
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if response.Transition == nil {
		return transitionmodel.Transition{},
			errors.New("coordinator returned nil transition")
	}

	return transitionFromPB(
		response.Transition,
	)
}

func transitionStatePB(
	state transitionmodel.State,
) (forgev1.TransitionState, error) {
	switch state {
	case transitionmodel.StateRequested:
		return forgev1.TransitionState_TRANSITION_REQUESTED, nil

	case transitionmodel.StatePreparing:
		return forgev1.TransitionState_TRANSITION_PREPARING, nil

	case transitionmodel.StateCheckpointing:
		return forgev1.TransitionState_TRANSITION_CHECKPOINTING, nil

	case transitionmodel.StateReconfiguring:
		return forgev1.TransitionState_TRANSITION_RECONFIGURING, nil

	case transitionmodel.StateRestoring:
		return forgev1.TransitionState_TRANSITION_RESTORING, nil

	case transitionmodel.StateResuming:
		return forgev1.TransitionState_TRANSITION_RESUMING, nil

	case transitionmodel.StateCompleted:
		return forgev1.TransitionState_TRANSITION_COMPLETED, nil

	case transitionmodel.StateFailed:
		return forgev1.TransitionState_TRANSITION_FAILED, nil

	default:
		return forgev1.TransitionState_TRANSITION_STATE_UNSPECIFIED,
			errors.New("invalid transition state")
	}
}

func transitionStateFromPB(
	state forgev1.TransitionState,
) (transitionmodel.State, error) {
	switch state {
	case forgev1.TransitionState_TRANSITION_REQUESTED:
		return transitionmodel.StateRequested, nil

	case forgev1.TransitionState_TRANSITION_PREPARING:
		return transitionmodel.StatePreparing, nil

	case forgev1.TransitionState_TRANSITION_CHECKPOINTING:
		return transitionmodel.StateCheckpointing, nil

	case forgev1.TransitionState_TRANSITION_RECONFIGURING:
		return transitionmodel.StateReconfiguring, nil

	case forgev1.TransitionState_TRANSITION_RESTORING:
		return transitionmodel.StateRestoring, nil

	case forgev1.TransitionState_TRANSITION_RESUMING:
		return transitionmodel.StateResuming, nil

	case forgev1.TransitionState_TRANSITION_COMPLETED:
		return transitionmodel.StateCompleted, nil

	case forgev1.TransitionState_TRANSITION_FAILED:
		return transitionmodel.StateFailed, nil

	default:
		return "",
			errors.New("invalid transition state")
	}
}

func transitionFromPB(
	pb *forgev1.Transition,
) (transitionmodel.Transition, error) {
	state, err := transitionStateFromPB(
		pb.State,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr := transitionmodel.Transition{
		ID:    pb.Id,
		JobID: pb.JobId,
		State: state,

		Source: resource.Vector{},
		Target: resource.Vector{},

		FailureReason: pb.FailureReason,
		BytesMoved:    pb.BytesMoved,
	}

	if pb.Source != nil {
		tr.Source = resource.Vector{
			CPUCores: pb.Source.CpuCores,
			MemoryMB: pb.Source.MemoryMb,
			GPUs:     pb.Source.Gpus,
		}
	}

	if pb.Target != nil {
		tr.Target = resource.Vector{
			CPUCores: pb.Target.CpuCores,
			MemoryMB: pb.Target.MemoryMb,
			GPUs:     pb.Target.Gpus,
		}
	}

	return tr, nil
}

func (s *TransitionStore) RecordMetrics(
	ctx context.Context,
	id string,
	metrics transitionmodel.Metrics,
) error {
	_, err := s.client.RecordTransitionMetrics(
		ctx,
		&forgev1.RecordTransitionMetricsRequest{
			WorkerId:     s.workerID,
			TransitionId: id,

			Metrics: &forgev1.TransitionMetrics{
				PrepareMs: metrics.PrepareDuration.Milliseconds(),

				CheckpointMs: metrics.CheckpointDuration.Milliseconds(),

				ReconfigureMs: metrics.ReconfigureDuration.Milliseconds(),

				RestoreMs: metrics.RestoreDuration.Milliseconds(),

				ResumeMs: metrics.ResumeDuration.Milliseconds(),

				CheckpointBytes: metrics.CheckpointBytes,

				RestoreBytes: metrics.RestoreBytes,
			},
		},
	)

	return err
}
