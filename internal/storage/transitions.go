package storage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/likhitha281/forge/internal/resource"
	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

var (
	ErrTransitionNotFound = errors.New(
		"transition not found",
	)

	ErrInsufficientTransitionCapacity = errors.New(
		"insufficient capacity for transition",
	)
)

func (s *Store) AdmitTransition(
	ctx context.Context,
	jobID string,
	target resource.Vector,
) (transitionmodel.Transition, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return transitionmodel.Transition{}, err
	}
	defer tx.Rollback(ctx)

	// First discover which worker currently owns the job.
	//
	// We intentionally do NOT lock the job here. Both Lease() and
	// AdmitTransition() must acquire scheduler locks in the same order:
	//
	//	worker -> job
	//
	// Using a consistent lock order prevents the classic deadlock where
	// one transaction owns the job and waits for the worker while another
	// owns the worker and waits for the job.
	var workerID string

	err = tx.QueryRow(
		ctx,
		`
		SELECT worker_id
		FROM jobs
		WHERE id = $1
		  AND status = 'RUNNING'
		  AND worker_id IS NOT NULL
		`,
		jobID,
	).Scan(&workerID)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			errors.New("job is not running")
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// Lock the worker first.
	//
	// Lease() uses the same worker-row lock, so ordinary placement and
	// transition admission are serialized whenever they compete for
	// resources on the same worker.
	var capacity resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			cpu_capacity,
			memory_capacity_mb,
			gpu_capacity
		FROM workers
		WHERE id = $1
		FOR UPDATE
		`,
		workerID,
	).Scan(
		&capacity.CPUCores,
		&capacity.MemoryMB,
		&capacity.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// Now lock and re-read the job.
	//
	// The job may have changed between our initial worker lookup and
	// acquiring the worker lock, so we must revalidate all assumptions
	// under the lock.
	var lockedWorkerID string
	var source resource.Vector
	var requirements resource.JobResources

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			worker_id,

			allocated_cpu_cores,
			allocated_memory_mb,
			allocated_gpu_count,

			cpu_cores,
			memory_mb,
			gpu_min,
			gpu_preferred,
			gpu_max
		FROM jobs
		WHERE id = $1
		  AND status = 'RUNNING'
		  AND worker_id IS NOT NULL
		  AND allocated_cpu_cores IS NOT NULL
		  AND allocated_memory_mb IS NOT NULL
		  AND allocated_gpu_count IS NOT NULL
		FOR UPDATE
		`,
		jobID,
	).Scan(
		&lockedWorkerID,

		&source.CPUCores,
		&source.MemoryMB,
		&source.GPUs,

		&requirements.CPUCores,
		&requirements.MemoryMB,
		&requirements.GPU.Min,
		&requirements.GPU.Preferred,
		&requirements.GPU.Max,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			errors.New("job is no longer running")
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// If the worker changed between discovery and locking, abort rather
	// than making a scheduling decision against stale worker capacity.
	if lockedWorkerID != workerID {
		return transitionmodel.Transition{},
			errors.New(
				"job worker changed during transition admission",
			)
	}

	// Validate the requested target against the job's resource envelope.
	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		source,
		target,
		requirements,
		time.Now().UTC(),
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// A reservation contains only the additional resources needed to move
	// from the current allocation to the target allocation.
	//
	// Example:
	//
	//	2 GPU -> 4 GPU
	//	reservation = 2 GPU
	//
	//	4 GPU -> 2 GPU
	//	reservation = 0 GPU
	reservation := transitionmodel.RequiredReservation(
		source,
		target,
	)

	// Calculate all resources currently owned by RUNNING jobs on this
	// worker. This already includes the source allocation of the job being
	// transitioned.
	var running resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COALESCE(SUM(allocated_cpu_cores), 0),
			COALESCE(SUM(allocated_memory_mb), 0),
			COALESCE(SUM(allocated_gpu_count), 0)
		FROM jobs
		WHERE status = 'RUNNING'
		  AND worker_id = $1
		`,
		workerID,
	).Scan(
		&running.CPUCores,
		&running.MemoryMB,
		&running.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// Account for spare capacity already promised to other active
	// transitions on this worker.
	var reserved resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COALESCE(SUM(reserved_cpu_cores), 0),
			COALESCE(SUM(reserved_memory_mb), 0),
			COALESCE(SUM(reserved_gpu_count), 0)
		FROM transitions
		WHERE worker_id = $1
		  AND state NOT IN ('COMPLETED', 'FAILED')
		`,
		workerID,
	).Scan(
		&reserved.CPUCores,
		&reserved.MemoryMB,
		&reserved.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// Capacity invariant:
	//
	//	running allocations
	//	+ existing transition reservations
	//	+ this transition's reservation
	//	<= worker capacity
	used := running.Add(reserved)
	required := used.Add(reservation)

	if !capacity.Fits(required) {
		return transitionmodel.Transition{},
			ErrInsufficientTransitionCapacity
	}

	tr.WorkerID = workerID
	tr.Reserved = reservation

	// Persist the reservation in the same transaction as admission.
	//
	// Once this commits, Lease() will include these reserved resources in
	// its available-capacity calculation and therefore cannot steal them.
	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO transitions(
			id,
			job_id,
			worker_id,
			state,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			reserved_cpu_cores,
			reserved_memory_mb,
			reserved_gpu_count,

			requested_at,
			bytes_moved
		)
		VALUES(
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13,
			$14, 0
		)
		`,
		tr.ID,
		tr.JobID,
		tr.WorkerID,
		string(tr.State),

		tr.Source.CPUCores,
		tr.Source.MemoryMB,
		tr.Source.GPUs,

		tr.Target.CPUCores,
		tr.Target.MemoryMB,
		tr.Target.GPUs,

		tr.Reserved.CPUCores,
		tr.Reserved.MemoryMB,
		tr.Reserved.GPUs,

		tr.RequestedAt,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return transitionmodel.Transition{}, err
	}

	return tr, nil
}

func (s *Store) CreateTransition(
	ctx context.Context,
	tr transitionmodel.Transition,
) error {
	if !tr.State.Valid() {
		return errors.New("invalid transition state")
	}

	_, err := s.DB.Exec(
		ctx,
		`
		INSERT INTO transitions(
			id,
			job_id,
			state,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			requested_at,
			bytes_moved
		)
		VALUES(
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11
		)
		`,
		tr.ID,
		tr.JobID,
		string(tr.State),

		tr.Source.CPUCores,
		tr.Source.MemoryMB,
		tr.Source.GPUs,

		tr.Target.CPUCores,
		tr.Target.MemoryMB,
		tr.Target.GPUs,

		tr.RequestedAt,
		tr.BytesMoved,
	)

	return err
}

func (s *Store) GetTransition(
	ctx context.Context,
	id string,
) (transitionmodel.Transition, error) {
	var tr transitionmodel.Transition

	var state string
	var startedAt *time.Time
	var completedAt *time.Time

	err := s.DB.QueryRow(
		ctx,
		`
		SELECT
			id::text,
			job_id::text,
			state,
			COALESCE(worker_id, ''),

			reserved_cpu_cores,
			reserved_memory_mb,
			reserved_gpu_count,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			requested_at,
			started_at,
			completed_at,

			COALESCE(failure_reason, ''),
			bytes_moved
		FROM transitions
		WHERE id = $1
		`,
		id,
	).Scan(
		&tr.ID,
		&tr.JobID,
		&state,
		&tr.WorkerID,

		&tr.Reserved.CPUCores,
		&tr.Reserved.MemoryMB,
		&tr.Reserved.GPUs,

		&tr.Source.CPUCores,
		&tr.Source.MemoryMB,
		&tr.Source.GPUs,

		&tr.Target.CPUCores,
		&tr.Target.MemoryMB,
		&tr.Target.GPUs,

		&tr.RequestedAt,
		&startedAt,
		&completedAt,

		&tr.FailureReason,
		&tr.BytesMoved,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrTransitionNotFound
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.State(state)
	tr.StartedAt = startedAt
	tr.CompletedAt = completedAt

	return tr, nil
}

func (s *Store) AdvanceTransition(
	ctx context.Context,
	id string,
	next transitionmodel.State,
) (transitionmodel.Transition, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return transitionmodel.Transition{}, err
	}
	defer tx.Rollback(ctx)

	var tr transitionmodel.Transition
	var current string

	var startedAt *time.Time
	var completedAt *time.Time

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			id::text,
			job_id::text,
			state,
			COALESCE(worker_id, ''),

			reserved_cpu_cores,
			reserved_memory_mb,
			reserved_gpu_count,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			requested_at,
			started_at,
			completed_at,

			COALESCE(failure_reason, ''),
			bytes_moved
		FROM transitions
		WHERE id = $1
		FOR UPDATE
		`,
		id,
	).Scan(
		&tr.ID,
		&tr.JobID,
		&current,
		&tr.WorkerID,

		&tr.Reserved.CPUCores,
		&tr.Reserved.MemoryMB,
		&tr.Reserved.GPUs,

		&tr.Source.CPUCores,
		&tr.Source.MemoryMB,
		&tr.Source.GPUs,

		&tr.Target.CPUCores,
		&tr.Target.MemoryMB,
		&tr.Target.GPUs,

		&tr.RequestedAt,
		&startedAt,
		&completedAt,

		&tr.FailureReason,
		&tr.BytesMoved,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrTransitionNotFound
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.State(current)
	tr.StartedAt = startedAt
	tr.CompletedAt = completedAt

	if err := transitionmodel.ValidateTransition(
		tr.State,
		next,
	); err != nil {
		return transitionmodel.Transition{}, err
	}

	now := time.Now().UTC()

	switch {
	case tr.State == transitionmodel.StateRequested &&
		next == transitionmodel.StatePreparing:
		tr.StartedAt = &now

	case next == transitionmodel.StateCompleted:
		tr.CompletedAt = &now
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE transitions
		SET
			state = $2,
			started_at = $3,
			completed_at = $4
		WHERE id = $1
		`,
		id,
		string(next),
		tr.StartedAt,
		tr.CompletedAt,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = next

	if err := tx.Commit(ctx); err != nil {
		return transitionmodel.Transition{}, err
	}

	return tr, nil
}

func (s *Store) FailTransition(
	ctx context.Context,
	id string,
	reason string,
) (transitionmodel.Transition, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return transitionmodel.Transition{}, err
	}
	defer tx.Rollback(ctx)

	var current string

	err = tx.QueryRow(
		ctx,
		`
		SELECT state
		FROM transitions
		WHERE id = $1
		FOR UPDATE
		`,
		id,
	).Scan(&current)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrTransitionNotFound
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	from := transitionmodel.State(current)

	if err := transitionmodel.ValidateTransition(
		from,
		transitionmodel.StateFailed,
	); err != nil {
		return transitionmodel.Transition{}, err
	}

	now := time.Now().UTC()

	_, err = tx.Exec(
		ctx,
		`
		UPDATE transitions
		SET
			state = $2,
			failure_reason = $3,
			completed_at = $4
		WHERE id = $1
		`,
		id,
		string(transitionmodel.StateFailed),
		reason,
		now,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return transitionmodel.Transition{}, err
	}

	return s.GetTransition(ctx, id)
}

func (s *Store) SetTransitionBytesMoved(
	ctx context.Context,
	id string,
	bytes int64,
) error {
	if bytes < 0 {
		return errors.New("bytes moved cannot be negative")
	}

	result, err := s.DB.Exec(
		ctx,
		`
		UPDATE transitions
		SET bytes_moved = $2
		WHERE id = $1
		`,
		id,
		bytes,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrTransitionNotFound
	}

	return nil
}

func (s *Store) ListTransitionsForJob(
	ctx context.Context,
	jobID string,
) ([]transitionmodel.Transition, error) {
	rows, err := s.DB.Query(
		ctx,
		`
		SELECT
			id::text,
			job_id::text,
			state,
			COALESCE(worker_id, ''),

			reserved_cpu_cores,
			reserved_memory_mb,
			reserved_gpu_count,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			requested_at,
			started_at,
			completed_at,

			COALESCE(failure_reason, ''),
			bytes_moved
		FROM transitions
		WHERE job_id = $1
		ORDER BY requested_at ASC
		`,
		jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []transitionmodel.Transition

	for rows.Next() {
		var tr transitionmodel.Transition
		var state string

		if err := rows.Scan(
			&tr.ID,
			&tr.JobID,
			&state,
			&tr.WorkerID,

			&tr.Reserved.CPUCores,
			&tr.Reserved.MemoryMB,
			&tr.Reserved.GPUs,

			&tr.Source.CPUCores,
			&tr.Source.MemoryMB,
			&tr.Source.GPUs,

			&tr.Target.CPUCores,
			&tr.Target.MemoryMB,
			&tr.Target.GPUs,

			&tr.RequestedAt,
			&tr.StartedAt,
			&tr.CompletedAt,

			&tr.FailureReason,
			&tr.BytesMoved,
		); err != nil {
			return nil, err
		}

		tr.State = transitionmodel.State(state)

		transitions = append(
			transitions,
			tr,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return transitions, nil
}

// CurrentJobAllocation returns the concrete allocation currently held by
// a RUNNING job. It is useful when constructing a transition request.
func (s *Store) CurrentJobAllocation(
	ctx context.Context,
	jobID string,
) (resource.Vector, error) {
	var allocation resource.Vector

	err := s.DB.QueryRow(
		ctx,
		`
		SELECT
			allocated_cpu_cores,
			allocated_memory_mb,
			allocated_gpu_count
		FROM jobs
		WHERE id = $1
		  AND status = 'RUNNING'
		  AND allocated_cpu_cores IS NOT NULL
		  AND allocated_memory_mb IS NOT NULL
		  AND allocated_gpu_count IS NOT NULL
		`,
		jobID,
	).Scan(
		&allocation.CPUCores,
		&allocation.MemoryMB,
		&allocation.GPUs,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return resource.Vector{},
			errors.New("job has no active allocation")
	}

	return allocation, err
}

func failActiveTransitionsForJobTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID string,
	reason string,
) error {
	now := time.Now().UTC()

	_, err := tx.Exec(
		ctx,
		`
		UPDATE transitions
		SET
			state = 'FAILED',
			failure_reason = $2,
			completed_at = $3
		WHERE job_id = $1
		  AND state NOT IN ('COMPLETED', 'FAILED')
		`,
		jobID,
		reason,
		now,
	)

	return err
}

func (s *Store) ReconcileStaleTransitions(
	ctx context.Context,
) (int64, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	result, err := tx.Exec(
		ctx,
		`
		UPDATE transitions AS t
		SET
			state = 'FAILED',
			failure_reason = CASE
				WHEN j.status <> 'RUNNING'
					THEN 'reconciled stale transition: job is not running'

				WHEN j.worker_id IS NULL
					THEN 'reconciled stale transition: job has no worker'

				WHEN t.worker_id IS NULL
					THEN 'reconciled stale transition: transition has no worker'

				WHEN t.worker_id <> j.worker_id
					THEN 'reconciled stale transition: worker ownership mismatch'

				ELSE 'reconciled stale transition'
			END,
			completed_at = $1,
			reserved_cpu_cores = 0,
			reserved_memory_mb = 0,
			reserved_gpu_count = 0
		FROM jobs AS j
		WHERE t.job_id = j.id
		  AND t.state NOT IN ('COMPLETED', 'FAILED')
		  AND (
				j.status <> 'RUNNING'
				OR j.worker_id IS NULL
				OR t.worker_id IS NULL
				OR t.worker_id <> j.worker_id
		  )
		`,
		now,
	)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

func (s *Store) CompleteTransition(
	ctx context.Context,
	id string,
) (transitionmodel.Transition, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return transitionmodel.Transition{}, err
	}
	defer tx.Rollback(ctx)

	// Discover the worker first so we can preserve the global lock order:
	//
	//     worker -> job -> transition
	var workerID string
	var jobID string

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COALESCE(worker_id, ''),
			job_id::text
		FROM transitions
		WHERE id = $1
		`,
		id,
	).Scan(
		&workerID,
		&jobID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrTransitionNotFound
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if workerID == "" {
		return transitionmodel.Transition{},
			errors.New("transition has no worker")
	}

	// Lock worker first. Lease() and AdmitTransition() use the same
	// worker-level serialization boundary.
	var capacity resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			cpu_capacity,
			memory_capacity_mb,
			gpu_capacity
		FROM workers
		WHERE id = $1
		FOR UPDATE
		`,
		workerID,
	).Scan(
		&capacity.CPUCores,
		&capacity.MemoryMB,
		&capacity.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	// Lock the job second and verify that the execution we admitted is
	// still the execution we're about to modify.
	var currentAllocation resource.Vector
	var lockedWorkerID string
	var status string

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			status,
			worker_id,
			allocated_cpu_cores,
			allocated_memory_mb,
			allocated_gpu_count
		FROM jobs
		WHERE id = $1
		  AND worker_id IS NOT NULL
		  AND allocated_cpu_cores IS NOT NULL
		  AND allocated_memory_mb IS NOT NULL
		  AND allocated_gpu_count IS NOT NULL
		FOR UPDATE
		`,
		jobID,
	).Scan(
		&status,
		&lockedWorkerID,
		&currentAllocation.CPUCores,
		&currentAllocation.MemoryMB,
		&currentAllocation.GPUs,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			errors.New("transition job has no active allocation")
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if status != "RUNNING" {
		return transitionmodel.Transition{},
			errors.New("transition job is not running")
	}

	if lockedWorkerID != workerID {
		return transitionmodel.Transition{},
			errors.New("transition worker no longer owns job")
	}

	// Lock the transition last and load the authoritative source/target.
	var tr transitionmodel.Transition
	var state string

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			id::text,
			job_id::text,
			state,
			COALESCE(worker_id, ''),

			reserved_cpu_cores,
			reserved_memory_mb,
			reserved_gpu_count,

			source_cpu_cores,
			source_memory_mb,
			source_gpu_count,

			target_cpu_cores,
			target_memory_mb,
			target_gpu_count,

			requested_at,
			started_at,
			completed_at,

			COALESCE(failure_reason, ''),
			bytes_moved
		FROM transitions
		WHERE id = $1
		FOR UPDATE
		`,
		id,
	).Scan(
		&tr.ID,
		&tr.JobID,
		&state,
		&tr.WorkerID,

		&tr.Reserved.CPUCores,
		&tr.Reserved.MemoryMB,
		&tr.Reserved.GPUs,

		&tr.Source.CPUCores,
		&tr.Source.MemoryMB,
		&tr.Source.GPUs,

		&tr.Target.CPUCores,
		&tr.Target.MemoryMB,
		&tr.Target.GPUs,

		&tr.RequestedAt,
		&tr.StartedAt,
		&tr.CompletedAt,

		&tr.FailureReason,
		&tr.BytesMoved,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.State(state)

	// Only RESUMING may successfully commit the target allocation.
	if err := transitionmodel.ValidateTransition(
		tr.State,
		transitionmodel.StateCompleted,
	); err != nil {
		return transitionmodel.Transition{}, err
	}

	// The job must still have the exact allocation from which this
	// transition was admitted.
	if !currentAllocation.Equal(tr.Source) {
		return transitionmodel.Transition{},
			errors.New(
				"job allocation changed during transition",
			)
	}

	// Re-check the worker capacity invariant under the worker lock.
	//
	// We exclude this transition's own reservation because its target is
	// about to become a concrete RUNNING allocation.
	var otherRunning resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COALESCE(SUM(allocated_cpu_cores), 0),
			COALESCE(SUM(allocated_memory_mb), 0),
			COALESCE(SUM(allocated_gpu_count), 0)
		FROM jobs
		WHERE status = 'RUNNING'
		  AND worker_id = $1
		  AND id <> $2
		`,
		workerID,
		jobID,
	).Scan(
		&otherRunning.CPUCores,
		&otherRunning.MemoryMB,
		&otherRunning.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	var otherReserved resource.Vector

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			COALESCE(SUM(reserved_cpu_cores), 0),
			COALESCE(SUM(reserved_memory_mb), 0),
			COALESCE(SUM(reserved_gpu_count), 0)
		FROM transitions
		WHERE worker_id = $1
		  AND id <> $2
		  AND state NOT IN ('COMPLETED', 'FAILED')
		`,
		workerID,
		id,
	).Scan(
		&otherReserved.CPUCores,
		&otherReserved.MemoryMB,
		&otherReserved.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	required := otherRunning.
		Add(otherReserved).
		Add(tr.Target)

	if !capacity.Fits(required) {
		return transitionmodel.Transition{},
			ErrInsufficientTransitionCapacity
	}

	// Make the target the job's concrete allocation.
	_, err = tx.Exec(
		ctx,
		`
		UPDATE jobs
		SET
			allocated_cpu_cores = $2,
			allocated_memory_mb = $3,
			allocated_gpu_count = $4,
			updated_at = now()
		WHERE id = $1
		`,
		jobID,
		tr.Target.CPUCores,
		tr.Target.MemoryMB,
		tr.Target.GPUs,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	now := time.Now().UTC()

	_, err = tx.Exec(
		ctx,
		`
		UPDATE transitions
		SET
			state = 'COMPLETED',
			completed_at = $2
		WHERE id = $1
		`,
		id,
		now,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.StateCompleted
	tr.CompletedAt = &now

	if err := tx.Commit(ctx); err != nil {
		return transitionmodel.Transition{}, err
	}

	return tr, nil
}

var ErrNoLeasableTransition = errors.New(
	"no leasable transition",
)

func (s *Store) LeaseTransition(
	ctx context.Context,
	workerID string,
) (transitionmodel.Transition, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return transitionmodel.Transition{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize transition claiming with the worker's other scheduling
	// operations.
	var workerExists bool

	err = tx.QueryRow(
		ctx,
		`
		SELECT true
		FROM workers
		WHERE id = $1
		FOR UPDATE
		`,
		workerID,
	).Scan(&workerExists)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrNoLeasableTransition
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	var tr transitionmodel.Transition
	var state string

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			t.id::text,
			t.job_id::text,
			t.state,
			COALESCE(t.worker_id, ''),

			t.reserved_cpu_cores,
			t.reserved_memory_mb,
			t.reserved_gpu_count,

			t.source_cpu_cores,
			t.source_memory_mb,
			t.source_gpu_count,

			t.target_cpu_cores,
			t.target_memory_mb,
			t.target_gpu_count,

			t.requested_at,
			t.started_at,
			t.completed_at,

			COALESCE(t.failure_reason, ''),
			t.bytes_moved
		FROM transitions t
		JOIN jobs j
		  ON j.id = t.job_id
		WHERE t.worker_id = $1
		  AND t.state = 'REQUESTED'
		  AND j.status = 'RUNNING'
		  AND j.worker_id = $1
		ORDER BY t.requested_at ASC
		LIMIT 1
		FOR UPDATE OF t
		`,
		workerID,
	).Scan(
		&tr.ID,
		&tr.JobID,
		&state,
		&tr.WorkerID,

		&tr.Reserved.CPUCores,
		&tr.Reserved.MemoryMB,
		&tr.Reserved.GPUs,

		&tr.Source.CPUCores,
		&tr.Source.MemoryMB,
		&tr.Source.GPUs,

		&tr.Target.CPUCores,
		&tr.Target.MemoryMB,
		&tr.Target.GPUs,

		&tr.RequestedAt,
		&tr.StartedAt,
		&tr.CompletedAt,

		&tr.FailureReason,
		&tr.BytesMoved,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return transitionmodel.Transition{},
			ErrNoLeasableTransition
	}

	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.State(state)

	// Claiming is represented by REQUESTED -> PREPARING.
	if err := transitionmodel.ValidateTransition(
		tr.State,
		transitionmodel.StatePreparing,
	); err != nil {
		return transitionmodel.Transition{}, err
	}

	now := time.Now().UTC()

	_, err = tx.Exec(
		ctx,
		`
		UPDATE transitions
		SET
			state = 'PREPARING',
			started_at = $2
		WHERE id = $1
		`,
		tr.ID,
		now,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	tr.State = transitionmodel.StatePreparing
	tr.StartedAt = &now

	if err := tx.Commit(ctx); err != nil {
		return transitionmodel.Transition{}, err
	}

	return tr, nil
}

var ErrTransitionWorkerMismatch = errors.New(
	"transition is not owned by worker",
)

func (s *Store) TransitionOwnedByWorker(
	ctx context.Context,
	transitionID string,
	workerID string,
) (bool, error) {
	var exists bool

	err := s.DB.QueryRow(
		ctx,
		`
		SELECT EXISTS(
			SELECT 1
			FROM transitions t
			JOIN jobs j
			  ON j.id = t.job_id
			WHERE t.id = $1
			  AND t.worker_id = $2
			  AND j.worker_id = $2
			  AND j.status = 'RUNNING'
		)
		`,
		transitionID,
		workerID,
	).Scan(&exists)

	if err != nil {
		return false, err
	}

	return exists, nil
}

func (s *Store) RecordTransitionMetrics(
	ctx context.Context,
	id string,
	metrics transitionmodel.Metrics,
) (transitionmodel.Transition, error) {
	if metrics.PrepareDuration < 0 ||
		metrics.CheckpointDuration < 0 ||
		metrics.ReconfigureDuration < 0 ||
		metrics.RestoreDuration < 0 ||
		metrics.ResumeDuration < 0 ||
		metrics.CheckpointBytes < 0 ||
		metrics.RestoreBytes < 0 {
		return transitionmodel.Transition{},
			errors.New("transition metrics cannot be negative")
	}

	totalBytes := metrics.TotalBytesMoved()

	commandTag, err := s.DB.Exec(
		ctx,
		`
	UPDATE transitions
	SET
		prepare_ms = $2,
		checkpoint_ms = $3,
		reconfigure_ms = $4,
		restore_ms = $5,
		resume_ms = $6,

		prepare_us = $7,
		checkpoint_us = $8,
		reconfigure_us = $9,
		restore_us = $10,
		resume_us = $11,

		checkpoint_bytes = $12,
		restore_bytes = $13,
		bytes_moved = $14
	WHERE id = $1
	`,
		id,

		metrics.PrepareDuration.Milliseconds(),
		metrics.CheckpointDuration.Milliseconds(),
		metrics.ReconfigureDuration.Milliseconds(),
		metrics.RestoreDuration.Milliseconds(),
		metrics.ResumeDuration.Milliseconds(),

		metrics.PrepareDuration.Microseconds(),
		metrics.CheckpointDuration.Microseconds(),
		metrics.ReconfigureDuration.Microseconds(),
		metrics.RestoreDuration.Microseconds(),
		metrics.ResumeDuration.Microseconds(),

		metrics.CheckpointBytes,
		metrics.RestoreBytes,
		totalBytes,
	)
	if err != nil {
		return transitionmodel.Transition{}, err
	}

	if commandTag.RowsAffected() == 0 {
		return transitionmodel.Transition{},
			ErrTransitionNotFound
	}

	return s.GetTransition(
		ctx,
		id,
	)
}
