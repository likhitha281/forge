package storage

import (
	"context"
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

	Resources resource.Vector
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
	resources resource.Vector,
) (string, error) {
	if err := resources.Validate(); err != nil {
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
			gpu_count
		)
		VALUES($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
		ON CONFLICT(idempotency_key)
		DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id::text
		`,
		id,
		payload,
		priority,
		key,
		maxAttempts,
		resources.CPUCores,
		resources.MemoryMB,
		resources.GPUs,
	).Scan(&jobID)

	return jobID, err
}

func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	var job Job

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
			gpu_count
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
		&job.Resources.CPUCores,
		&job.Resources.MemoryMB,
		&job.Resources.GPUs,
	)

	return job, err
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
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
			gpu_count
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
			&job.Resources.CPUCores,
			&job.Resources.MemoryMB,
			&job.Resources.GPUs,
		); err != nil {
			return nil, err
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (s *Store) CancelJob(ctx context.Context, id string) (bool, error) {
	result, err := s.DB.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = 'CANCELLED',
			worker_id = NULL,
			lease_until = NULL,
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

func (s *Store) ListWorkers(ctx context.Context) ([]Worker, error) {
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

	// Lock the worker row for the entire scheduling decision.
	//
	// This serializes concurrent Lease calls for the same worker. Without
	// this lock, multiple goroutines could observe the same available
	// resources and collectively oversubscribe the worker.
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

	// Derive current resource allocation from RUNNING jobs.
	//
	// RUNNING jobs are the authoritative source of allocation state. This
	// avoids maintaining a separate mutable allocation counter that could
	// drift out of sync after failures or coordinator restarts.
	var allocated resource.Vector
	var runningJobs int

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COUNT(*),
			COALESCE(SUM(cpu_cores), 0),
			COALESCE(SUM(memory_mb), 0),
			COALESCE(SUM(gpu_count), 0)
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

	// Preserve the existing worker concurrency-slot limit in addition to
	// resource-based scheduling.
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

	// Select the highest-priority queued job that fits the worker's
	// currently available resources.
	//
	// SKIP LOCKED allows different workers to lease different jobs
	// concurrently while avoiding duplicate ownership.
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
			gpu_count
		FROM jobs
		WHERE status = 'QUEUED'
		  AND cpu_cores <= $1
		  AND memory_mb <= $2
		  AND gpu_count <= $3
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
		&job.Resources.CPUCores,
		&job.Resources.MemoryMB,
		&job.Resources.GPUs,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNoLeasableJob
	}

	if err != nil {
		return Job{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			status = 'RUNNING',
			worker_id = $2,
			lease_until = now() + $3::interval,
			attempts = attempts + 1,
			updated_at = now()
		WHERE id = $1
		`,
		job.ID,
		worker,
		lease.String(),
	)
	if err != nil {
		return Job{}, err
	}

	job.Status = "RUNNING"
	job.WorkerID = worker
	job.Attempts++

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
			updated_at = now()
		WHERE id = $1
		`,
		id,
		jobStatus,
		message,
	)

	return err
}

func (s *Store) Requeue(ctx context.Context) error {
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
