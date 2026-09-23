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
		Resources:   resourcePB(job.Resources),
	}

	if !job.CreatedAt.IsZero() {
		result.CreatedAt = job.CreatedAt.Format(time.RFC3339)
	}

	if !job.UpdatedAt.IsZero() {
		result.UpdatedAt = job.UpdatedAt.Format(time.RFC3339)
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

	resources := resourceFromPB(request.Resources)

	if err := resources.Validate(); err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"invalid resources: %v",
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
		resources,
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
