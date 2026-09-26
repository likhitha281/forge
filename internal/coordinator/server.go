package coordinator

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/metrics"
	"github.com/likhitha281/forge/internal/resource"
	"github.com/likhitha281/forge/internal/storage"

	transitionmodel "github.com/likhitha281/forge/internal/transition"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	forgev1.UnimplementedForgeServer
	Store *storage.Store
}

func New(store *storage.Store) *Server {
	return &Server{Store: store}
}

func statusPB(value string) forgev1.JobStatus {
	switch value {
	case "QUEUED":
		return forgev1.JobStatus_QUEUED
	case "RUNNING":
		return forgev1.JobStatus_RUNNING
	case "COMPLETED":
		return forgev1.JobStatus_COMPLETED
	case "FAILED":
		return forgev1.JobStatus_FAILED
	case "CANCELLED":
		return forgev1.JobStatus_CANCELLED
	default:
		return forgev1.JobStatus_JOB_STATUS_UNSPECIFIED
	}
}

// resourcePB converts Forge's internal resource representation into the
// protobuf representation exposed through the gRPC API.
func resourcePB(v resource.Vector) *forgev1.ResourceVector {
	return &forgev1.ResourceVector{
		CpuCores: v.CPUCores,
		MemoryMb: v.MemoryMB,
		Gpus:     v.GPUs,
	}
}

// resourceFromPB converts the protobuf resource representation into the
// internal scheduler representation.
//
// A nil ResourceVector represents a zero-resource request. This preserves
// backwards compatibility with clients that do not yet specify resources.
func resourceFromPB(v *forgev1.ResourceVector) resource.Vector {
	if v == nil {
		return resource.Vector{}
	}

	return resource.Vector{
		CPUCores: v.CpuCores,
		MemoryMB: v.MemoryMb,
		GPUs:     v.Gpus,
	}
}

func jobResourcesPB(
	r resource.JobResources,
) *forgev1.JobResources {
	return &forgev1.JobResources{
		CpuCores: r.CPUCores,
		MemoryMb: r.MemoryMB,
		Gpu: &forgev1.GPUEnvelope{
			Min:       r.GPU.Min,
			Preferred: r.GPU.Preferred,
			Max:       r.GPU.Max,
		},
	}
}

func jobResourcesFromPB(
	r *forgev1.JobResources,
) resource.JobResources {
	if r == nil {
		return resource.JobResources{}
	}

	var gpu resource.GPUEnvelope

	if r.Gpu != nil {
		gpu = resource.GPUEnvelope{
			Min:       r.Gpu.Min,
			Preferred: r.Gpu.Preferred,
			Max:       r.Gpu.Max,
		}
	}

	return resource.JobResources{
		CPUCores: r.CpuCores,
		MemoryMB: r.MemoryMb,
		GPU:      gpu,
	}
}

// fixedJobResources converts the legacy fixed ResourceVector request into
// the new malleable representation.
//
// A fixed request for N GPUs is represented as:
//
//	min = preferred = max = N
func fixedJobResources(
	v resource.Vector,
) resource.JobResources {
	return resource.JobResources{
		CPUCores: v.CPUCores,
		MemoryMB: v.MemoryMB,
		GPU: resource.GPUEnvelope{
			Min:       v.GPUs,
			Preferred: v.GPUs,
			Max:       v.GPUs,
		},
	}
}

// pb converts the persistent storage representation of a job into the
// protobuf representation returned to clients and workers.
func pb(job storage.Job) *forgev1.Job {
	result := &forgev1.Job{
		Id:          job.ID,
		Payload:     job.Payload,
		Priority:    int32(job.Priority),
		Status:      statusPB(job.Status),
		Attempts:    int32(job.Attempts),
		Error:       job.Error,
		WorkerId:    job.WorkerID,
		MaxAttempts: int32(job.MaxAttempts),

		Requirements: jobResourcesPB(job.Requirements),
	}

	// Keep the legacy ResourceVector populated using the preferred
	// configuration so older clients still receive meaningful data.
	result.Resources = resourcePB(
		job.Requirements.Preferred(),
	)

	if job.Allocation != nil {
		result.Allocation = resourcePB(
			*job.Allocation,
		)
	}

	if !job.CreatedAt.IsZero() {
		result.CreatedAt = job.CreatedAt.Format(
			time.RFC3339,
		)
	}

	if !job.UpdatedAt.IsZero() {
		result.UpdatedAt = job.UpdatedAt.Format(
			time.RFC3339,
		)
	}

	return result
}

func (s *Server) SubmitJob(
	ctx context.Context,
	request *forgev1.SubmitJobRequest,
) (*forgev1.SubmitJobResponse, error) {
	if request.Payload == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"payload required",
		)
	}

	var requirements resource.JobResources

	// Prefer the new malleable requirements API.
	if request.Requirements != nil {
		requirements = jobResourcesFromPB(
			request.Requirements,
		)
	} else {
		// Backwards compatibility with Step-1 clients.
		legacy := resourceFromPB(
			request.Resources,
		)

		if err := legacy.Validate(); err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"invalid resources: %v",
				err,
			)
		}

		requirements = fixedJobResources(
			legacy,
		)
	}

	if err := requirements.Validate(); err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"invalid resource requirements: %v",
			err,
		)
	}

	id := uuid.NewString()

	jobID, err := s.Store.Submit(
		ctx,
		id,
		request.Payload,
		request.IdempotencyKey,
		int(request.Priority),
		int(request.MaxAttempts),
		requirements,
	)
	if err != nil {
		return nil, err
	}

	metrics.JobsSubmitted.Inc()

	return &forgev1.SubmitJobResponse{
		JobId: jobID,
	}, nil
}

func (s *Server) GetJob(
	ctx context.Context,
	request *forgev1.GetJobRequest,
) (*forgev1.Job, error) {
	job, err := s.Store.Get(ctx, request.JobId)
	if err != nil {
		return nil, err
	}

	return pb(job), nil
}

func (s *Server) ListJobs(
	ctx context.Context,
	request *forgev1.ListJobsRequest,
) (*forgev1.ListJobsResponse, error) {
	jobs, err := s.Store.ListJobs(
		ctx,
		int(request.Limit),
	)
	if err != nil {
		return nil, err
	}

	response := &forgev1.ListJobsResponse{}

	for _, job := range jobs {
		response.Jobs = append(
			response.Jobs,
			pb(job),
		)
	}

	return response, nil
}

func (s *Server) CancelJob(
	ctx context.Context,
	request *forgev1.CancelJobRequest,
) (*forgev1.CancelJobResponse, error) {
	if request.JobId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"job_id required",
		)
	}

	cancelled, err := s.Store.CancelJob(
		ctx,
		request.JobId,
	)
	if err != nil {
		return nil, err
	}

	if !cancelled {
		return nil, status.Error(
			codes.FailedPrecondition,
			"job is not queued or does not exist",
		)
	}

	return &forgev1.CancelJobResponse{
		Cancelled: true,
	}, nil
}

func (s *Server) RegisterWorker(
	ctx context.Context,
	request *forgev1.RegisterWorkerRequest,
) (*forgev1.RegisterWorkerResponse, error) {
	if request.WorkerId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id required",
		)
	}

	if request.Capacity <= 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker capacity must be greater than zero",
		)
	}

	resources := resourceFromPB(request.Resources)

	if err := resources.Validate(); err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"invalid worker resources: %v",
			err,
		)
	}

	if err := s.Store.Register(
		ctx,
		request.WorkerId,
		int(request.Capacity),
		resources,
	); err != nil {
		return nil, err
	}

	return &forgev1.RegisterWorkerResponse{
		Accepted: true,
	}, nil
}

func (s *Server) Heartbeat(
	ctx context.Context,
	request *forgev1.HeartbeatRequest,
) (*forgev1.HeartbeatResponse, error) {
	if request.WorkerId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id required",
		)
	}

	if err := s.Store.Heartbeat(
		ctx,
		request.WorkerId,
		int(request.Running),
	); err != nil {
		return nil, err
	}

	return &forgev1.HeartbeatResponse{
		Ok: true,
	}, nil
}

func (s *Server) ListWorkers(
	ctx context.Context,
	_ *forgev1.ListWorkersRequest,
) (*forgev1.ListWorkersResponse, error) {
	workers, err := s.Store.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}

	response := &forgev1.ListWorkersResponse{}

	for _, worker := range workers {
		response.Workers = append(
			response.Workers,
			&forgev1.Worker{
				Id:            worker.ID,
				Capacity:      int32(worker.Capacity),
				Running:       int32(worker.Running),
				LastHeartbeat: worker.LastHeartbeat.Format(time.RFC3339),
				Resources:     resourcePB(worker.Resources),
			},
		)
	}

	return response, nil
}

func (s *Server) LeaseJob(
	ctx context.Context,
	request *forgev1.LeaseJobRequest,
) (*forgev1.LeaseJobResponse, error) {
	if request.WorkerId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id required",
		)
	}

	const leaseDuration = 30 * time.Second

	job, err := s.Store.Lease(
		ctx,
		request.WorkerId,
		leaseDuration,
	)
	if err != nil {
		if errors.Is(err, storage.ErrNoLeasableJob) {
			return nil, status.Error(
				codes.NotFound,
				"no queued job fits worker resources",
			)
		}

		return nil, status.Error(
			codes.Internal,
			"failed to lease job",
		)
	}

	metrics.QueueLeases.Inc()

	return &forgev1.LeaseJobResponse{
		Job:          pb(job),
		LeaseSeconds: int32(leaseDuration.Seconds()),
	}, nil
}

func (s *Server) CompleteJob(
	ctx context.Context,
	request *forgev1.CompleteJobRequest,
) (*forgev1.CompleteJobResponse, error) {
	if request.JobId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"job_id required",
		)
	}

	if err := s.Store.Complete(
		ctx,
		request.JobId,
		request.Success,
		request.Error,
	); err != nil {
		return nil, err
	}

	result := "success"

	if !request.Success {
		result = "failure"
	}

	metrics.JobsCompleted.
		WithLabelValues(result).
		Inc()

	return &forgev1.CompleteJobResponse{
		Ok: true,
	}, nil
}

func (s *Server) RenewLease(
	ctx context.Context,
	request *forgev1.RenewLeaseRequest,
) (*forgev1.RenewLeaseResponse, error) {
	if request.JobId == "" || request.WorkerId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"job_id and worker_id are required",
		)
	}

	const leaseDuration = 30 * time.Second

	renewed, err := s.Store.RenewLease(
		ctx,
		request.JobId,
		request.WorkerId,
		leaseDuration,
	)
	if err != nil {
		return nil, err
	}

	if !renewed {
		return nil, status.Error(
			codes.FailedPrecondition,
			"worker does not own this running job",
		)
	}

	return &forgev1.RenewLeaseResponse{
		Renewed:      true,
		LeaseSeconds: int32(leaseDuration.Seconds()),
	}, nil
}

func transitionStatePB(
	state transitionmodel.State,
) forgev1.TransitionState {
	switch state {
	case transitionmodel.StateRequested:
		return forgev1.TransitionState_TRANSITION_REQUESTED

	case transitionmodel.StatePreparing:
		return forgev1.TransitionState_TRANSITION_PREPARING

	case transitionmodel.StateCheckpointing:
		return forgev1.TransitionState_TRANSITION_CHECKPOINTING

	case transitionmodel.StateReconfiguring:
		return forgev1.TransitionState_TRANSITION_RECONFIGURING

	case transitionmodel.StateRestoring:
		return forgev1.TransitionState_TRANSITION_RESTORING

	case transitionmodel.StateResuming:
		return forgev1.TransitionState_TRANSITION_RESUMING

	case transitionmodel.StateCompleted:
		return forgev1.TransitionState_TRANSITION_COMPLETED

	case transitionmodel.StateFailed:
		return forgev1.TransitionState_TRANSITION_FAILED

	default:
		return forgev1.TransitionState_TRANSITION_STATE_UNSPECIFIED
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
		return "", errors.New(
			"invalid transition state",
		)
	}
}

func transitionPB(
	tr transitionmodel.Transition,
) *forgev1.Transition {
	result := &forgev1.Transition{
		Id:            tr.ID,
		JobId:         tr.JobID,
		State:         transitionStatePB(tr.State),
		Source:        resourcePB(tr.Source),
		Target:        resourcePB(tr.Target),
		FailureReason: tr.FailureReason,
		BytesMoved:    tr.BytesMoved,
	}

	if !tr.RequestedAt.IsZero() {
		result.RequestedAt =
			tr.RequestedAt.Format(time.RFC3339Nano)
	}

	if tr.StartedAt != nil {
		result.StartedAt =
			tr.StartedAt.Format(time.RFC3339Nano)
	}

	if tr.CompletedAt != nil {
		result.CompletedAt =
			tr.CompletedAt.Format(time.RFC3339Nano)
	}

	return result
}

func (s *Server) RequestTransition(
	ctx context.Context,
	req *forgev1.RequestTransitionRequest,
) (*forgev1.RequestTransitionResponse, error) {
	if req.JobId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"job_id required",
		)
	}

	if req.Target == nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"target allocation required",
		)
	}

	target := resourceFromPB(req.Target)

	if err := target.Validate(); err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"invalid target allocation: %v",
			err,
		)
	}

	tr, err := s.Store.AdmitTransition(
		ctx,
		req.JobId,
		target,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			storage.ErrInsufficientTransitionCapacity,
		):
			return nil, status.Error(
				codes.ResourceExhausted,
				"insufficient capacity for transition",
			)

		case errors.Is(
			err,
			transitionmodel.ErrSameAllocation,
		):
			return nil, status.Error(
				codes.InvalidArgument,
				"target equals current allocation",
			)

		case errors.Is(
			err,
			transitionmodel.ErrInvalidTarget,
		):
			return nil, status.Error(
				codes.InvalidArgument,
				"target is outside job resource requirements",
			)

		default:
			return nil, err
		}
	}

	return &forgev1.RequestTransitionResponse{
		Transition: transitionPB(tr),
	}, nil
}

func (s *Server) GetTransition(
	ctx context.Context,
	req *forgev1.GetTransitionRequest,
) (*forgev1.Transition, error) {
	if req.TransitionId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"transition_id required",
		)
	}

	tr, err := s.Store.GetTransition(
		ctx,
		req.TransitionId,
	)
	if err != nil {
		if errors.Is(
			err,
			storage.ErrTransitionNotFound,
		) {
			return nil, status.Error(
				codes.NotFound,
				"transition not found",
			)
		}

		return nil, err
	}

	return transitionPB(tr), nil
}
func (s *Server) ListJobTransitions(
	ctx context.Context,
	req *forgev1.ListJobTransitionsRequest,
) (*forgev1.ListJobTransitionsResponse, error) {
	if req.JobId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"job_id required",
		)
	}

	transitions, err :=
		s.Store.ListTransitionsForJob(
			ctx,
			req.JobId,
		)
	if err != nil {
		return nil, err
	}

	response :=
		&forgev1.ListJobTransitionsResponse{
			Transitions: make(
				[]*forgev1.Transition,
				0,
				len(transitions),
			),
		}

	for _, tr := range transitions {
		response.Transitions = append(
			response.Transitions,
			transitionPB(tr),
		)
	}

	return response, nil
}

func (s *Server) LeaseTransition(
	ctx context.Context,
	req *forgev1.LeaseTransitionRequest,
) (*forgev1.LeaseTransitionResponse, error) {
	if req.WorkerId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id required",
		)
	}

	tr, err := s.Store.LeaseTransition(
		ctx,
		req.WorkerId,
	)
	if err != nil {
		if errors.Is(
			err,
			storage.ErrNoLeasableTransition,
		) {
			return nil, status.Error(
				codes.NotFound,
				"no requested transition for worker",
			)
		}

		return nil, status.Error(
			codes.Internal,
			"failed to lease transition",
		)
	}

	return &forgev1.LeaseTransitionResponse{
		Transition: transitionPB(tr),
	}, nil
}

func (s *Server) AdvanceTransition(
	ctx context.Context,
	req *forgev1.AdvanceTransitionRequest,
) (*forgev1.AdvanceTransitionResponse, error) {
	if req.WorkerId == "" ||
		req.TransitionId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id and transition_id required",
		)
	}

	next, err := transitionStateFromPB(
		req.NextState,
	)
	if err != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			err.Error(),
		)
	}

	owned, err := s.Store.TransitionOwnedByWorker(
		ctx,
		req.TransitionId,
		req.WorkerId,
	)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to verify transition ownership",
		)
	}

	if !owned {
		return nil, status.Error(
			codes.PermissionDenied,
			"worker does not own transition",
		)
	}

	tr, err := s.Store.AdvanceTransition(
		ctx,
		req.TransitionId,
		next,
	)
	if err != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"advance transition: %v",
			err,
		)
	}

	return &forgev1.AdvanceTransitionResponse{
		Transition: transitionPB(tr),
	}, nil
}

func (s *Server) CompleteTransition(
	ctx context.Context,
	req *forgev1.CompleteTransitionRequest,
) (*forgev1.CompleteTransitionResponse, error) {
	if req.WorkerId == "" ||
		req.TransitionId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id and transition_id required",
		)
	}

	owned, err := s.Store.TransitionOwnedByWorker(
		ctx,
		req.TransitionId,
		req.WorkerId,
	)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to verify transition ownership",
		)
	}

	if !owned {
		return nil, status.Error(
			codes.PermissionDenied,
			"worker does not own transition",
		)
	}

	tr, err := s.Store.CompleteTransition(
		ctx,
		req.TransitionId,
	)
	if err != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"complete transition: %v",
			err,
		)
	}

	return &forgev1.CompleteTransitionResponse{
		Transition: transitionPB(tr),
	}, nil
}

func (s *Server) FailTransition(
	ctx context.Context,
	req *forgev1.FailTransitionRequest,
) (*forgev1.FailTransitionResponse, error) {
	if req.WorkerId == "" ||
		req.TransitionId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id and transition_id required",
		)
	}

	if req.Reason == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"failure reason required",
		)
	}

	owned, err := s.Store.TransitionOwnedByWorker(
		ctx,
		req.TransitionId,
		req.WorkerId,
	)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to verify transition ownership",
		)
	}

	if !owned {
		return nil, status.Error(
			codes.PermissionDenied,
			"worker does not own transition",
		)
	}

	tr, err := s.Store.FailTransition(
		ctx,
		req.TransitionId,
		req.Reason,
	)
	if err != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"fail transition: %v",
			err,
		)
	}

	return &forgev1.FailTransitionResponse{
		Transition: transitionPB(tr),
	}, nil
}

func (s *Server) RecordTransitionMetrics(
	ctx context.Context,
	req *forgev1.RecordTransitionMetricsRequest,
) (*forgev1.RecordTransitionMetricsResponse, error) {
	if req.WorkerId == "" ||
		req.TransitionId == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"worker_id and transition_id required",
		)
	}

	if req.Metrics == nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"metrics required",
		)
	}

	owned, err := s.Store.TransitionOwnedByWorker(
		ctx,
		req.TransitionId,
		req.WorkerId,
	)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to verify transition ownership",
		)
	}

	if !owned {
		return nil, status.Error(
			codes.PermissionDenied,
			"worker does not own transition",
		)
	}

	m := req.Metrics

	if m.PrepareMs < 0 ||
		m.CheckpointMs < 0 ||
		m.ReconfigureMs < 0 ||
		m.RestoreMs < 0 ||
		m.ResumeMs < 0 ||
		m.CheckpointBytes < 0 ||
		m.RestoreBytes < 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			"transition metrics cannot be negative",
		)
	}

	tr, err := s.Store.RecordTransitionMetrics(
		ctx,
		req.TransitionId,
		transitionmodel.Metrics{
			PrepareDuration: time.Duration(m.PrepareMs) * time.Millisecond,

			CheckpointDuration: time.Duration(m.CheckpointMs) * time.Millisecond,

			ReconfigureDuration: time.Duration(m.ReconfigureMs) * time.Millisecond,

			RestoreDuration: time.Duration(m.RestoreMs) * time.Millisecond,

			ResumeDuration: time.Duration(m.ResumeMs) * time.Millisecond,

			CheckpointBytes: m.CheckpointBytes,

			RestoreBytes: m.RestoreBytes,
		},
	)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"record transition metrics: %v",
			err,
		)
	}

	return &forgev1.RecordTransitionMetricsResponse{
		Transition: transitionPB(tr),
	}, nil
}
