package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/likhitha281/forge/internal/resource"
)

var ErrNoLeasableJob = errors.New("no leasable job")

type Job struct {
	ID          string
	Payload     string
	Status      string
	Error       string
	WorkerID    string
	Priority    int
	Attempts    int
	MaxAttempts int
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Requirements resource.JobResources
	Allocation   *resource.Vector
}

type nullableAllocation struct {
	CPU    sql.NullFloat64
	Memory sql.NullInt64
	GPU    sql.NullInt32
}

func (a nullableAllocation) Vector() *resource.Vector {
	if !a.CPU.Valid && !a.Memory.Valid && !a.GPU.Valid {
		return nil
	}

	// Allocations are written and cleared atomically as a group.
	// A partially NULL allocation represents corrupt/inconsistent state.
	if !a.CPU.Valid || !a.Memory.Valid || !a.GPU.Valid {
		return nil
	}

	return &resource.Vector{
		CPUCores: a.CPU.Float64,
		MemoryMB: a.Memory.Int64,
		GPUs:     a.GPU.Int32,
	}
}

type Worker struct {
	ID            string
	Capacity      int
	Running       int
	LastHeartbeat time.Time

	Resources resource.Vector
}

type Store struct {
	DB *pgxpool.Pool
}

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}

	return &Store{DB: pool}, pool.Ping(ctx)
}

func (s *Store) Submit(
	ctx context.Context,
	id string,
	payload string,
	key string,
	priority int,
	maxAttempts int,
	requirements resource.JobResources,
) (string, error) {
	if err := requirements.Validate(); err != nil {
		return "", err
	}

	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var jobID string

	err := s.DB.QueryRow(
		ctx,
		`
		INSERT INTO jobs(
			id,
			payload,
			priority,
			idempotency_key,
			max_attempts,
			cpu_cores,
			memory_mb,
			gpu_count,
			gpu_min,
			gpu_preferred,
			gpu_max
		)
		VALUES(
			$1, $2, $3, NULLIF($4, ''), $5,
			$6, $7, $8, $9, $10, $11
		)
		ON CONFLICT(idempotency_key)
		DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id::text
		`,
		id,
		payload,
		priority,
		key,
		maxAttempts,
		requirements.CPUCores,
		requirements.MemoryMB,

		// gpu_count is retained temporarily for compatibility with the
		// Step-1 schema. Preferred is the closest representation of the
		// old fixed GPU request.
		requirements.GPU.Preferred,

		requirements.GPU.Min,
		requirements.GPU.Preferred,
		requirements.GPU.Max,
	).Scan(&jobID)

	return jobID, err
}

func (s *Store) Get(
	ctx context.Context,
	id string,
) (Job, error) {
	var job Job
	var allocation nullableAllocation

	err := s.DB.QueryRow(
		ctx,
		`
		SELECT
			id::text,
			payload,
			priority,
			status,
			attempts,
			max_attempts,
			COALESCE(error, ''),
			COALESCE(worker_id, ''),
			created_at,
			updated_at,

			cpu_cores,
			memory_mb,
			gpu_min,
			gpu_preferred,
			gpu_max,

			allocated_cpu_cores,
			allocated_memory_mb,
			allocated_gpu_count
		FROM jobs
		WHERE id = $1
		`,
		id,
	).Scan(
		&job.ID,
		&job.Payload,
		&job.Priority,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.Error,
		&job.WorkerID,
		&job.CreatedAt,
		&job.UpdatedAt,

		&job.Requirements.CPUCores,
		&job.Requirements.MemoryMB,
		&job.Requirements.GPU.Min,
		&job.Requirements.GPU.Preferred,
		&job.Requirements.GPU.Max,

		&allocation.CPU,
		&allocation.Memory,
		&allocation.GPU,
	)
	if err != nil {
		return Job{}, err
	}

	job.Allocation = allocation.Vector()

	return job, nil
}

func (s *Store) ListJobs(
	ctx context.Context,
	limit int,
) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}

	if limit > 500 {
		limit = 500
	}

	rows, err := s.DB.Query(
		ctx,
		`
		SELECT
			id::text,
			payload,
			priority,
			status,
			attempts,
			max_attempts,
			COALESCE(error, ''),
			COALESCE(worker_id, ''),
			created_at,
			updated_at,

			cpu_cores,
			memory_mb,
			gpu_min,
			gpu_preferred,
			gpu_max,

			allocated_cpu_cores,
			allocated_memory_mb,
			allocated_gpu_count
		FROM jobs
		ORDER BY created_at DESC
		LIMIT $1
		`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var job Job
		var allocation nullableAllocation

		if err := rows.Scan(
			&job.ID,
			&job.Payload,
			&job.Priority,
			&job.Status,
			&job.Attempts,
			&job.MaxAttempts,
			&job.Error,
			&job.WorkerID,
			&job.CreatedAt,
			&job.UpdatedAt,

			&job.Requirements.CPUCores,
			&job.Requirements.MemoryMB,
			&job.Requirements.GPU.Min,
			&job.Requirements.GPU.Preferred,
			&job.Requirements.GPU.Max,

			&allocation.CPU,
			&allocation.Memory,
			&allocation.GPU,
		); err != nil {
			return nil, err
		}

		job.Allocation = allocation.Vector()
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (s *Store) CancelJob(
	ctx context.Context,
	id string,
) (bool, error) {
	result, err := s.DB.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = 'CANCELLED',
			worker_id = NULL,
			lease_until = NULL,
			allocated_cpu_cores = NULL,
			allocated_memory_mb = NULL,
			allocated_gpu_count = NULL,
			updated_at = now()
		WHERE id = $1
		  AND status = 'QUEUED'
		`,
		id,
	)
	if err != nil {
		return false, err
	}

	return result.RowsAffected() == 1, nil
}

func (s *Store) Register(
	ctx context.Context,
	id string,
	capacity int,
	resources resource.Vector,
) error {
	if err := resources.Validate(); err != nil {
		return err
	}

	_, err := s.DB.Exec(
		ctx,
		`
		INSERT INTO workers(
			id,
			capacity,
			cpu_capacity,
			memory_capacity_mb,
			gpu_capacity
		)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT(id)
		DO UPDATE SET
			capacity = EXCLUDED.capacity,
			cpu_capacity = EXCLUDED.cpu_capacity,
			memory_capacity_mb = EXCLUDED.memory_capacity_mb,
			gpu_capacity = EXCLUDED.gpu_capacity,
			last_heartbeat = now()
		`,
		id,
		capacity,
		resources.CPUCores,
		resources.MemoryMB,
		resources.GPUs,
	)

	return err
}

func (s *Store) Heartbeat(
	ctx context.Context,
	id string,
	running int,
) error {
	result, err := s.DB.Exec(
		ctx,
		`
		UPDATE workers
		SET
			running = $2,
			last_heartbeat = now()
		WHERE id = $1
		`,
		id,
		running,
	)

	if err == nil && result.RowsAffected() == 0 {
		return errors.New("unknown worker")
	}

	return err
}

func (s *Store) ListWorkers(
	ctx context.Context,
) ([]Worker, error) {
	rows, err := s.DB.Query(
		ctx,
		`
		SELECT
			id,
			capacity,
			running,
			last_heartbeat,
			cpu_capacity,
			memory_capacity_mb,
			gpu_capacity
		FROM workers
		ORDER BY id
		`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []Worker

	for rows.Next() {
		var worker Worker

		if err := rows.Scan(
			&worker.ID,
			&worker.Capacity,
			&worker.Running,
			&worker.LastHeartbeat,
			&worker.Resources.CPUCores,
			&worker.Resources.MemoryMB,
			&worker.Resources.GPUs,
		); err != nil {
			return nil, err
		}

		workers = append(workers, worker)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return workers, nil
}

func (s *Store) Lease(
	ctx context.Context,
	worker string,
	lease time.Duration,
) (Job, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize scheduling decisions for this worker.
	//
	// Without this worker-row lock, concurrent Lease calls could observe
	// the same available resources and collectively oversubscribe them.
	var capacity resource.Vector
	var maxConcurrent int

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			capacity,
			cpu_capacity,
			memory_capacity_mb,
			gpu_capacity
		FROM workers
		WHERE id = $1
		FOR UPDATE
		`,
		worker,
	).Scan(
		&maxConcurrent,
		&capacity.CPUCores,
		&capacity.MemoryMB,
		&capacity.GPUs,
	)
	if err != nil {
		return Job{}, err
	}

	// Capacity accounting is based on concrete allocations, not preferred
	// resource requirements. This distinction is what allows malleable jobs
	// to execute at degraded GPU allocations.
	var allocated resource.Vector
	var runningJobs int

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COUNT(*),
			COALESCE(SUM(allocated_cpu_cores), 0),
			COALESCE(SUM(allocated_memory_mb), 0),
			COALESCE(SUM(allocated_gpu_count), 0)
		FROM jobs
		WHERE status = 'RUNNING'
		  AND worker_id = $1
		`,
		worker,
	).Scan(
		&runningJobs,
		&allocated.CPUCores,
		&allocated.MemoryMB,
		&allocated.GPUs,
	)
	if err != nil {
		return Job{}, err
	}

	if runningJobs >= maxConcurrent {
		return Job{}, ErrNoLeasableJob
	}

	available, err := capacity.Subtract(allocated)
	if err != nil {
		return Job{}, errors.New(
			"worker resource invariant violated: allocated resources exceed capacity",
		)
	}

	var job Job

	// A malleable job is eligible if its fixed CPU/memory requirements and
	// minimum GPU requirement fit. Preferred/max GPU counts do not determine
	// eligibility.
	err = tx.QueryRow(
		ctx,
		`
		SELECT
			id::text,
			payload,
			priority,
			status,
			attempts,
			max_attempts,
			COALESCE(error, ''),
			cpu_cores,
			memory_mb,
			gpu_min,
			gpu_preferred,
			gpu_max
		FROM jobs
		WHERE status = 'QUEUED'
		  AND cpu_cores <= $1
		  AND memory_mb <= $2
		  AND gpu_min <= $3
		ORDER BY priority ASC, created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
		`,
		available.CPUCores,
		available.MemoryMB,
		available.GPUs,
	).Scan(
		&job.ID,
		&job.Payload,
		&job.Priority,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.Error,
		&job.Requirements.CPUCores,
		&job.Requirements.MemoryMB,
		&job.Requirements.GPU.Min,
		&job.Requirements.GPU.Preferred,
		&job.Requirements.GPU.Max,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNoLeasableJob
	}

	if err != nil {
		return Job{}, err
	}

	allocation, ok := job.Requirements.InitialAllocation(available)
	if !ok {
		return Job{}, errors.New(
			"scheduler invariant violated: selected job cannot fit available resources",
		)
	}

	// Assignment and concrete allocation are persisted atomically.
	_, err = tx.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = 'RUNNING',
			worker_id = $2,
			lease_until = now() + $3::interval,
			attempts = attempts + 1,
			allocated_cpu_cores = $4,
			allocated_memory_mb = $5,
			allocated_gpu_count = $6,
			updated_at = now()
		WHERE id = $1
		`,
		job.ID,
		worker,
		lease.String(),
		allocation.CPUCores,
		allocation.MemoryMB,
		allocation.GPUs,
	)
	if err != nil {
		return Job{}, err
	}

	job.Status = "RUNNING"
	job.WorkerID = worker
	job.Attempts++
	job.Allocation = &allocation

	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}

	return job, nil
}

func (s *Store) Complete(
	ctx context.Context,
	id string,
	success bool,
	message string,
) error {
	jobStatus := "COMPLETED"

	if !success {
		jobStatus = "FAILED"
	}

	_, err := s.DB.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = $2,
			error = NULLIF($3, ''),
			lease_until = NULL,
			allocated_cpu_cores = NULL,
			allocated_memory_mb = NULL,
			allocated_gpu_count = NULL,
			updated_at = now()
		WHERE id = $1
		`,
		id,
		jobStatus,
		message,
	)

	return err
}

func (s *Store) Requeue(
	ctx context.Context,
) error {
	_, err := s.DB.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = CASE
				WHEN attempts < max_attempts THEN 'QUEUED'
				ELSE 'FAILED'
			END,
			worker_id = NULL,
			lease_until = NULL,
			allocated_cpu_cores = NULL,
			allocated_memory_mb = NULL,
			allocated_gpu_count = NULL,
			error = CASE
				WHEN attempts < max_attempts THEN error
				ELSE COALESCE(error, 'lease expired')
			END,
			updated_at = now()
		WHERE status = 'RUNNING'
		  AND lease_until < now()
		`,
	)

	return err
}

func (s *Store) RenewLease(
	ctx context.Context,
	jobID string,
	workerID string,
	lease time.Duration,
) (bool, error) {
	result, err := s.DB.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			lease_until = now() + $3::interval,
			updated_at = now()
		WHERE id = $1
		  AND worker_id = $2
		  AND status = 'RUNNING'
		`,
		jobID,
		workerID,
		lease.String(),
	)
	if err != nil {
		return false, err
	}

	return result.RowsAffected() == 1, nil
}
