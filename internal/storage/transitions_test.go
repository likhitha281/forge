package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/likhitha281/forge/internal/resource"
	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

func createTransitionTestJob(
	t *testing.T,
	store *Store,
) string {
	t.Helper()

	id := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		id,
		"sleep 60",
		"",
		2,
		3,
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit transition test job: %v", err)
	}

	return id
}

func TestCreateAndGetTransition(t *testing.T) {
	store := testStore(t)

	jobID := createTransitionTestJob(t, store)

	now := time.Now().UTC()

	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     2,
		},
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
		now,
	)
	if err != nil {
		t.Fatalf("transition.New(): %v", err)
	}

	if err := store.CreateTransition(
		context.Background(),
		tr,
	); err != nil {
		t.Fatalf("CreateTransition(): %v", err)
	}

	got, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf("GetTransition(): %v", err)
	}

	if got.State != transitionmodel.StateRequested {
		t.Fatalf(
			"state = %s, want REQUESTED",
			got.State,
		)
	}

	if !got.Source.Equal(tr.Source) {
		t.Fatalf(
			"source = %v, want %v",
			got.Source,
			tr.Source,
		)
	}

	if !got.Target.Equal(tr.Target) {
		t.Fatalf(
			"target = %v, want %v",
			got.Target,
			tr.Target,
		)
	}
}

func TestAdvanceTransitionHappyPath(t *testing.T) {
	store := testStore(t)

	jobID := createTransitionTestJob(t, store)

	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     2,
		},
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("transition.New(): %v", err)
	}

	if err := store.CreateTransition(
		context.Background(),
		tr,
	); err != nil {
		t.Fatalf("CreateTransition(): %v", err)
	}

	path := []transitionmodel.State{
		transitionmodel.StatePreparing,
		transitionmodel.StateCheckpointing,
		transitionmodel.StateReconfiguring,
		transitionmodel.StateRestoring,
		transitionmodel.StateResuming,
		transitionmodel.StateCompleted,
	}

	for _, state := range path {
		tr, err = store.AdvanceTransition(
			context.Background(),
			tr.ID,
			state,
		)
		if err != nil {
			t.Fatalf(
				"AdvanceTransition(%s): %v",
				state,
				err,
			)
		}

		if tr.State != state {
			t.Fatalf(
				"state = %s, want %s",
				tr.State,
				state,
			)
		}
	}

	if tr.StartedAt == nil {
		t.Fatal("expected StartedAt")
	}

	if tr.CompletedAt == nil {
		t.Fatal("expected CompletedAt")
	}

	if _, ok := tr.Duration(); !ok {
		t.Fatal("expected completed duration")
	}
}

func TestAdvanceTransitionRejectsSkippedState(t *testing.T) {
	store := testStore(t)

	jobID := createTransitionTestJob(t, store)

	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     2,
		},
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("transition.New(): %v", err)
	}

	if err := store.CreateTransition(
		context.Background(),
		tr,
	); err != nil {
		t.Fatalf("CreateTransition(): %v", err)
	}

	_, err = store.AdvanceTransition(
		context.Background(),
		tr.ID,
		transitionmodel.StateReconfiguring,
	)

	if !errors.Is(
		err,
		transitionmodel.ErrInvalidTransition,
	) {
		t.Fatalf(
			"error = %v, want ErrInvalidTransition",
			err,
		)
	}
}

func TestFailTransition(t *testing.T) {
	store := testStore(t)

	jobID := createTransitionTestJob(t, store)

	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     2,
		},
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("transition.New(): %v", err)
	}

	if err := store.CreateTransition(
		context.Background(),
		tr,
	); err != nil {
		t.Fatalf("CreateTransition(): %v", err)
	}

	tr, err = store.AdvanceTransition(
		context.Background(),
		tr.ID,
		transitionmodel.StatePreparing,
	)
	if err != nil {
		t.Fatalf("advance to PREPARING: %v", err)
	}

	tr, err = store.FailTransition(
		context.Background(),
		tr.ID,
		"checkpoint failed",
	)
	if err != nil {
		t.Fatalf("FailTransition(): %v", err)
	}

	if tr.State != transitionmodel.StateFailed {
		t.Fatalf(
			"state = %s, want FAILED",
			tr.State,
		)
	}

	if tr.FailureReason != "checkpoint failed" {
		t.Fatalf(
			"failure reason = %q",
			tr.FailureReason,
		)
	}

	if tr.CompletedAt == nil {
		t.Fatal("failed transition should have CompletedAt")
	}
}
func createRunningTransitionTestJob(
	t *testing.T,
	store *Store,
	workerID string,
	workerResources resource.Vector,
	jobRequirements resource.JobResources,
) Job {
	t.Helper()

	registerTestWorker(
		t,
		store,
		workerID,
		10,
		workerResources,
	)

	id := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		id,
		"sleep 60",
		"",
		2,
		3,
		jobRequirements,
	)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	job, err := store.Lease(
		context.Background(),
		workerID,
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease job: %v", err)
	}

	return job
}

func TestAdmitTransitionReservesGrowth(t *testing.T) {
	store := testStore(t)

	job := createRunningTransitionTestJob(
		t,
		store,
		"worker-1",
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 8192,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)

	if job.Allocation == nil {
		t.Fatal("expected initial allocation")
	}

	if job.Allocation.GPUs != 2 {
		t.Fatalf(
			"initial GPUs = %d, want 2",
			job.Allocation.GPUs,
		)
	}

	target := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     4,
	}

	tr, err := store.AdmitTransition(
		context.Background(),
		job.ID,
		target,
	)
	if err != nil {
		t.Fatalf("AdmitTransition(): %v", err)
	}

	if tr.Reserved.GPUs != 2 {
		t.Fatalf(
			"reserved GPUs = %d, want 2",
			tr.Reserved.GPUs,
		)
	}

	if tr.WorkerID != "worker-1" {
		t.Fatalf(
			"worker = %s, want worker-1",
			tr.WorkerID,
		)
	}

	if tr.State != transitionmodel.StateRequested {
		t.Fatalf(
			"state = %s, want REQUESTED",
			tr.State,
		)
	}

	stored, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf("GetTransition(): %v", err)
	}

	if stored.Reserved.GPUs != 2 {
		t.Fatalf(
			"stored reserved GPUs = %d, want 2",
			stored.Reserved.GPUs,
		)
	}

	if stored.WorkerID != "worker-1" {
		t.Fatalf(
			"stored worker = %s, want worker-1",
			stored.WorkerID,
		)
	}
}

func TestAdmitTransitionRejectsInsufficientCapacity(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		10,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	requirements := resource.JobResources{
		CPUCores: 2,
		MemoryMB: 2048,
		GPU: resource.GPUEnvelope{
			Min:       2,
			Preferred: 2,
			Max:       4,
		},
	}

	firstID := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		firstID,
		"sleep 60",
		"",
		1,
		3,
		requirements,
	)
	if err != nil {
		t.Fatalf("submit first job: %v", err)
	}

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}

	secondID := uuid.NewString()

	_, err = store.Submit(
		context.Background(),
		secondID,
		"sleep 60",
		"",
		2,
		3,
		requirements,
	)
	if err != nil {
		t.Fatalf("submit second job: %v", err)
	}

	second, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease second job: %v", err)
	}

	if first.Allocation == nil || second.Allocation == nil {
		t.Fatal("expected both jobs to have allocations")
	}

	if first.Allocation.GPUs != 2 ||
		second.Allocation.GPUs != 2 {
		t.Fatalf(
			"expected 2 GPU allocations, got %d and %d",
			first.Allocation.GPUs,
			second.Allocation.GPUs,
		)
	}

	target := resource.Vector{
		CPUCores: 2,
		MemoryMB: 2048,
		GPUs:     4,
	}

	_, err = store.AdmitTransition(
		context.Background(),
		first.ID,
		target,
	)

	if !errors.Is(
		err,
		ErrInsufficientTransitionCapacity,
	) {
		t.Fatalf(
			"error = %v, want ErrInsufficientTransitionCapacity",
			err,
		)
	}
}

func TestTransitionReservationBlocksLease(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		10,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	firstID := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		firstID,
		"sleep 60",
		"",
		1,
		3,
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit first job: %v", err)
	}

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}

	tr, err := store.AdmitTransition(
		context.Background(),
		first.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf("AdmitTransition(): %v", err)
	}

	if tr.Reserved.GPUs != 2 {
		t.Fatalf(
			"reserved GPUs = %d, want 2",
			tr.Reserved.GPUs,
		)
	}

	secondID := uuid.NewString()

	_, err = store.Submit(
		context.Background(),
		secondID,
		"sleep 60",
		"",
		2,
		3,
		resource.JobResources{
			CPUCores: 1,
			MemoryMB: 1024,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 1,
				Max:       1,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit second job: %v", err)
	}

	_, err = store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)

	if !errors.Is(err, ErrNoLeasableJob) {
		t.Fatalf(
			"Lease() error = %v, want ErrNoLeasableJob",
			err,
		)
	}

	queued, err := store.Get(
		context.Background(),
		secondID,
	)
	if err != nil {
		t.Fatalf("Get(second job): %v", err)
	}

	if queued.Status != "QUEUED" {
		t.Fatalf(
			"second job status = %s, want QUEUED",
			queued.Status,
		)
	}
}

func TestConcurrentLeaseAndTransitionAdmissionDoNotOversubscribe(
	t *testing.T,
) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		10,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	// Job A starts with 2 GPUs but is allowed to grow to 4.
	firstID := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		firstID,
		"sleep 60",
		"",
		1,
		3,
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit first job: %v", err)
	}

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}

	if first.Allocation == nil {
		t.Fatal("expected first job allocation")
	}

	if first.Allocation.GPUs != 2 {
		t.Fatalf(
			"first allocation = %d GPUs, want 2",
			first.Allocation.GPUs,
		)
	}

	// Job B also needs exactly 2 GPUs.
	//
	// The worker therefore has exactly enough spare capacity for either:
	//
	//	A's 2 -> 4 transition
	//
	// OR
	//
	//	B's initial allocation
	//
	// but never both.
	secondID := uuid.NewString()

	_, err = store.Submit(
		context.Background(),
		secondID,
		"sleep 60",
		"",
		2,
		3,
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       2,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit second job: %v", err)
	}

	type transitionResult struct {
		tr  transitionmodel.Transition
		err error
	}

	type leaseResult struct {
		job Job
		err error
	}

	start := make(chan struct{})

	transitionResults := make(
		chan transitionResult,
		1,
	)

	leaseResults := make(
		chan leaseResult,
		1,
	)

	go func() {
		<-start

		tr, err := store.AdmitTransition(
			context.Background(),
			first.ID,
			resource.Vector{
				CPUCores: 2,
				MemoryMB: 2048,
				GPUs:     4,
			},
		)

		transitionResults <- transitionResult{
			tr:  tr,
			err: err,
		}
	}()

	go func() {
		<-start

		job, err := store.Lease(
			context.Background(),
			"worker-1",
			30*time.Second,
		)

		leaseResults <- leaseResult{
			job: job,
			err: err,
		}
	}()

	// Release both goroutines at approximately the same time.
	close(start)

	transitionResultValue := <-transitionResults
	leaseResultValue := <-leaseResults

	transitionSucceeded :=
		transitionResultValue.err == nil

	leaseSucceeded :=
		leaseResultValue.err == nil

	// Exactly one operation must acquire the remaining capacity.
	if transitionSucceeded == leaseSucceeded {
		t.Fatalf(
			"expected exactly one operation to succeed; "+
				"transition err=%v lease err=%v",
			transitionResultValue.err,
			leaseResultValue.err,
		)
	}

	if !transitionSucceeded &&
		!errors.Is(
			transitionResultValue.err,
			ErrInsufficientTransitionCapacity,
		) {
		t.Fatalf(
			"unexpected transition error: %v",
			transitionResultValue.err,
		)
	}

	if !leaseSucceeded &&
		!errors.Is(
			leaseResultValue.err,
			ErrNoLeasableJob,
		) {
		t.Fatalf(
			"unexpected lease error: %v",
			leaseResultValue.err,
		)
	}

	// Verify the persisted state rather than trusting only the return
	// values from the two concurrent calls.
	var runningGPUs int32

	err = store.DB.QueryRow(
		context.Background(),
		`
		SELECT
			COALESCE(SUM(allocated_gpu_count), 0)
		FROM jobs
		WHERE status = 'RUNNING'
		  AND worker_id = $1
		`,
		"worker-1",
	).Scan(&runningGPUs)
	if err != nil {
		t.Fatalf(
			"query running GPUs: %v",
			err,
		)
	}

	var reservedGPUs int32

	err = store.DB.QueryRow(
		context.Background(),
		`
		SELECT
			COALESCE(SUM(reserved_gpu_count), 0)
		FROM transitions
		WHERE worker_id = $1
		  AND state NOT IN ('COMPLETED', 'FAILED')
		`,
		"worker-1",
	).Scan(&reservedGPUs)
	if err != nil {
		t.Fatalf(
			"query reserved GPUs: %v",
			err,
		)
	}

	effectiveUsage :=
		runningGPUs + reservedGPUs

	if effectiveUsage > 4 {
		t.Fatalf(
			"worker oversubscribed: running=%d reserved=%d total=%d capacity=4",
			runningGPUs,
			reservedGPUs,
			effectiveUsage,
		)
	}

	if effectiveUsage != 4 {
		t.Fatalf(
			"expected all 4 GPUs to be claimed, got running=%d reserved=%d total=%d",
			runningGPUs,
			reservedGPUs,
			effectiveUsage,
		)
	}

	// Validate the expected persisted state for whichever operation won.
	second, err := store.Get(
		context.Background(),
		secondID,
	)
	if err != nil {
		t.Fatalf(
			"get second job: %v",
			err,
		)
	}

	if transitionSucceeded {
		if second.Status != "QUEUED" {
			t.Fatalf(
				"transition won but second job status = %s, want QUEUED",
				second.Status,
			)
		}

		if transitionResultValue.tr.Reserved.GPUs != 2 {
			t.Fatalf(
				"transition reserved %d GPUs, want 2",
				transitionResultValue.tr.Reserved.GPUs,
			)
		}
	} else {
		if second.Status != "RUNNING" {
			t.Fatalf(
				"lease won but second job status = %s, want RUNNING",
				second.Status,
			)
		}

		if second.Allocation == nil ||
			second.Allocation.GPUs != 2 {
			t.Fatalf(
				"lease won but second job allocation = %v, want 2 GPUs",
				second.Allocation,
			)
		}
	}
}

func TestJobCompletionFailsActiveTransitionAndReleasesReservation(
	t *testing.T,
) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		10,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	firstID := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		firstID,
		"sleep 60",
		"",
		1,
		3,
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"submit first job: %v",
			err,
		)
	}

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf(
			"lease first job: %v",
			err,
		)
	}

	if first.Allocation == nil ||
		first.Allocation.GPUs != 2 {
		t.Fatalf(
			"initial allocation = %v, want 2 GPUs",
			first.Allocation,
		)
	}

	tr, err := store.AdmitTransition(
		context.Background(),
		first.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf(
			"AdmitTransition(): %v",
			err,
		)
	}

	if tr.Reserved.GPUs != 2 {
		t.Fatalf(
			"reserved GPUs = %d, want 2",
			tr.Reserved.GPUs,
		)
	}

	// Completing the job must atomically terminate its active transition.
	if err := store.Complete(
		context.Background(),
		first.ID,
		true,
		"",
	); err != nil {
		t.Fatalf(
			"Complete(): %v",
			err,
		)
	}

	completed, err := store.Get(
		context.Background(),
		first.ID,
	)
	if err != nil {
		t.Fatalf(
			"Get(completed job): %v",
			err,
		)
	}

	if completed.Status != "COMPLETED" {
		t.Fatalf(
			"job status = %s, want COMPLETED",
			completed.Status,
		)
	}

	if completed.Allocation != nil {
		t.Fatalf(
			"completed job still has allocation: %v",
			completed.Allocation,
		)
	}

	storedTransition, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf(
			"GetTransition(): %v",
			err,
		)
	}

	if storedTransition.State !=
		transitionmodel.StateFailed {
		t.Fatalf(
			"transition state = %s, want FAILED",
			storedTransition.State,
		)
	}

	if storedTransition.CompletedAt == nil {
		t.Fatal(
			"failed transition should have completed_at",
		)
	}

	if storedTransition.FailureReason !=
		"job completed during transition" {
		t.Fatalf(
			"failure reason = %q",
			storedTransition.FailureReason,
		)
	}

	// The important scheduler-level assertion:
	// the old reservation must no longer consume capacity.
	secondID := uuid.NewString()

	_, err = store.Submit(
		context.Background(),
		secondID,
		"sleep 60",
		"",
		2,
		3,
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 4096,
			GPU: resource.GPUEnvelope{
				Min:       4,
				Preferred: 4,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"submit second job: %v",
			err,
		)
	}

	second, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf(
			"lease second job after completion: %v",
			err,
		)
	}

	if second.ID != secondID {
		t.Fatalf(
			"leased job = %s, want %s",
			second.ID,
			secondID,
		)
	}

	if second.Allocation == nil ||
		second.Allocation.GPUs != 4 {
		t.Fatalf(
			"second allocation = %v, want 4 GPUs",
			second.Allocation,
		)
	}
}

func TestLeaseExpiryFailsActiveTransitionAndReleasesReservation(
	t *testing.T,
) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		10,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	firstID := uuid.NewString()

	_, err := store.Submit(
		context.Background(),
		firstID,
		"sleep 60",
		"",
		1,
		3,
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit first job: %v", err)
	}

	// Use a short lease. We will explicitly expire it in PostgreSQL below
	// so the test remains deterministic rather than depending on sleep.
	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}

	if first.Allocation == nil ||
		first.Allocation.GPUs != 2 {
		t.Fatalf(
			"initial allocation = %v, want 2 GPUs",
			first.Allocation,
		)
	}

	tr, err := store.AdmitTransition(
		context.Background(),
		first.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf("AdmitTransition(): %v", err)
	}

	if tr.Reserved.GPUs != 2 {
		t.Fatalf(
			"reserved GPUs = %d, want 2",
			tr.Reserved.GPUs,
		)
	}

	// Deterministically expire the lease.
	_, err = store.DB.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET lease_until = now() - interval '1 second'
		WHERE id = $1
		`,
		first.ID,
	)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	if err := store.Requeue(
		context.Background(),
	); err != nil {
		t.Fatalf("Requeue(): %v", err)
	}

	requeued, err := store.Get(
		context.Background(),
		first.ID,
	)
	if err != nil {
		t.Fatalf("Get(requeued job): %v", err)
	}

	// attempts is 1 and max_attempts is 3, so existing Forge semantics
	// require this execution to return to QUEUED.
	if requeued.Status != "QUEUED" {
		t.Fatalf(
			"job status = %s, want QUEUED",
			requeued.Status,
		)
	}

	if requeued.Allocation != nil {
		t.Fatalf(
			"requeued job still has allocation: %v",
			requeued.Allocation,
		)
	}

	storedTransition, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf("GetTransition(): %v", err)
	}

	if storedTransition.State != transitionmodel.StateFailed {
		t.Fatalf(
			"transition state = %s, want FAILED",
			storedTransition.State,
		)
	}

	if storedTransition.CompletedAt == nil {
		t.Fatal(
			"failed transition should have completed_at",
		)
	}

	if storedTransition.FailureReason !=
		"job lease expired during transition" {
		t.Fatalf(
			"failure reason = %q",
			storedTransition.FailureReason,
		)
	}

	// Give the requeued job lower scheduling priority than the new job.
	// Job B requires all four GPUs. If the failed transition still counts
	// as a reservation, this lease will fail.
	secondID := uuid.NewString()

	_, err = store.Submit(
		context.Background(),
		secondID,
		"sleep 60",
		"",
		0,
		3,
		resource.JobResources{
			CPUCores: 4,
			MemoryMB: 4096,
			GPU: resource.GPUEnvelope{
				Min:       4,
				Preferred: 4,
				Max:       4,
			},
		},
	)
	if err != nil {
		t.Fatalf("submit second job: %v", err)
	}

	second, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf(
			"lease after transition cleanup: %v",
			err,
		)
	}

	if second.ID != secondID {
		t.Fatalf(
			"leased job = %s, want %s",
			second.ID,
			secondID,
		)
	}

	if second.Allocation == nil ||
		second.Allocation.GPUs != 4 {
		t.Fatalf(
			"second allocation = %v, want 4 GPUs",
			second.Allocation,
		)
	}
}

func TestCompleteTransitionAppliesTargetAllocation(
	t *testing.T,
) {
	store := testStore(t)

	job := createRunningTransitionTestJob(
		t,
		store,
		"worker-1",
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)

	tr, err := store.AdmitTransition(
		context.Background(),
		job.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf(
			"AdmitTransition(): %v",
			err,
		)
	}

	path := []transitionmodel.State{
		transitionmodel.StatePreparing,
		transitionmodel.StateCheckpointing,
		transitionmodel.StateReconfiguring,
		transitionmodel.StateRestoring,
		transitionmodel.StateResuming,
	}

	for _, state := range path {
		tr, err = store.AdvanceTransition(
			context.Background(),
			tr.ID,
			state,
		)
		if err != nil {
			t.Fatalf(
				"AdvanceTransition(%s): %v",
				state,
				err,
			)
		}
	}

	tr, err = store.CompleteTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf(
			"CompleteTransition(): %v",
			err,
		)
	}

	if tr.State != transitionmodel.StateCompleted {
		t.Fatalf(
			"transition state = %s, want COMPLETED",
			tr.State,
		)
	}

	updated, err := store.Get(
		context.Background(),
		job.ID,
	)
	if err != nil {
		t.Fatalf("Get(job): %v", err)
	}

	if updated.Status != "RUNNING" {
		t.Fatalf(
			"job status = %s, want RUNNING",
			updated.Status,
		)
	}

	if updated.Allocation == nil {
		t.Fatal("expected updated allocation")
	}

	if updated.Allocation.GPUs != 4 {
		t.Fatalf(
			"job GPU allocation = %d, want 4",
			updated.Allocation.GPUs,
		)
	}

	stored, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf(
			"GetTransition(): %v",
			err,
		)
	}

	if stored.State != transitionmodel.StateCompleted {
		t.Fatalf(
			"stored transition state = %s, want COMPLETED",
			stored.State,
		)
	}

	if stored.CompletedAt == nil {
		t.Fatal(
			"completed transition missing completed_at",
		)
	}
}

func TestLeaseTransitionClaimsOnlyOwningWorkersTransition(
	t *testing.T,
) {
	store := testStore(t)

	job := createRunningTransitionTestJob(
		t,
		store,
		"worker-1",
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)

	tr, err := store.AdmitTransition(
		context.Background(),
		job.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf("AdmitTransition(): %v", err)
	}

	claimed, err := store.LeaseTransition(
		context.Background(),
		"worker-1",
	)
	if err != nil {
		t.Fatalf("LeaseTransition(): %v", err)
	}

	if claimed.ID != tr.ID {
		t.Fatalf(
			"claimed transition = %s, want %s",
			claimed.ID,
			tr.ID,
		)
	}

	if claimed.State != transitionmodel.StatePreparing {
		t.Fatalf(
			"state = %s, want PREPARING",
			claimed.State,
		)
	}

	if claimed.StartedAt == nil {
		t.Fatal("claimed transition missing started_at")
	}

	// It must not be claimable twice.
	_, err = store.LeaseTransition(
		context.Background(),
		"worker-1",
	)

	if !errors.Is(
		err,
		ErrNoLeasableTransition,
	) {
		t.Fatalf(
			"second LeaseTransition() error = %v, want ErrNoLeasableTransition",
			err,
		)
	}
}

func TestRecordTransitionMetrics(
	t *testing.T,
) {

	var (
		prepareUS     int64
		checkpointUS  int64
		reconfigureUS int64
		restoreUS     int64
		resumeUS      int64
	)

	store := testStore(t)

	jobID := createTransitionTestJob(
		t,
		store,
	)

	tr, err := transitionmodel.New(
		uuid.NewString(),
		jobID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     2,
		},
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       1,
				Preferred: 2,
				Max:       4,
			},
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("transition.New(): %v", err)
	}

	if err := store.CreateTransition(
		context.Background(),
		tr,
	); err != nil {
		t.Fatalf("CreateTransition(): %v", err)
	}

	metrics := transitionmodel.Metrics{
		PrepareDuration:     100 * time.Millisecond,
		CheckpointDuration:  300 * time.Millisecond,
		ReconfigureDuration: 200 * time.Millisecond,
		RestoreDuration:     300 * time.Millisecond,
		ResumeDuration:      100 * time.Millisecond,

		CheckpointBytes: 64 * 1024 * 1024,
		RestoreBytes:    64 * 1024 * 1024,
	}

	_, err = store.RecordTransitionMetrics(
		context.Background(),
		tr.ID,
		metrics,
	)
	if err != nil {
		t.Fatalf(
			"RecordTransitionMetrics(): %v",
			err,
		)
	}

	var (
		prepareMS       int64
		checkpointMS    int64
		reconfigureMS   int64
		restoreMS       int64
		resumeMS        int64
		checkpointBytes int64
		restoreBytes    int64
		totalBytesMoved int64
	)

	err = store.DB.QueryRow(
		context.Background(),
		`
	SELECT
		prepare_ms,
		checkpoint_ms,
		reconfigure_ms,
		restore_ms,
		resume_ms,

		prepare_us,
		checkpoint_us,
		reconfigure_us,
		restore_us,
		resume_us,

		checkpoint_bytes,
		restore_bytes,
		bytes_moved
	FROM transitions
	WHERE id = $1
	`,
		tr.ID,
	).Scan(
		&prepareMS,
		&checkpointMS,
		&reconfigureMS,
		&restoreMS,
		&resumeMS,

		&prepareUS,
		&checkpointUS,
		&reconfigureUS,
		&restoreUS,
		&resumeUS,

		&checkpointBytes,
		&restoreBytes,
		&totalBytesMoved,
	)
	if err != nil {
		t.Fatalf(
			"query metrics: %v",
			err,
		)
	}

	if prepareUS != 100000 {
		t.Fatalf(
			"prepare_us = %d, want 100000",
			prepareUS,
		)
	}

	if checkpointUS != 300000 {
		t.Fatalf(
			"checkpoint_us = %d, want 300000",
			checkpointUS,
		)
	}

	if reconfigureUS != 200000 {
		t.Fatalf(
			"reconfigure_us = %d, want 200000",
			reconfigureUS,
		)
	}

	if restoreUS != 300000 {
		t.Fatalf(
			"restore_us = %d, want 300000",
			restoreUS,
		)
	}

	if resumeUS != 100000 {
		t.Fatalf(
			"resume_us = %d, want 100000",
			resumeUS,
		)
	}

	if prepareMS != 100 ||
		checkpointMS != 300 ||
		reconfigureMS != 200 ||
		restoreMS != 300 ||
		resumeMS != 100 {
		t.Fatalf(
			"unexpected durations: %d %d %d %d %d",
			prepareMS,
			checkpointMS,
			reconfigureMS,
			restoreMS,
			resumeMS,
		)
	}

	wantBytes := int64(
		64 * 1024 * 1024,
	)

	if checkpointBytes != wantBytes ||
		restoreBytes != wantBytes {
		t.Fatalf(
			"bytes = checkpoint:%d restore:%d, want %d each",
			checkpointBytes,
			restoreBytes,
			wantBytes,
		)
	}

	if totalBytesMoved != 2*wantBytes {
		t.Fatalf(
			"bytes_moved = %d, want %d",
			totalBytesMoved,
			2*wantBytes,
		)
	}
}

func TestReconcileStaleTransitionsReleasesReservation(
	t *testing.T,
) {
	store := testStore(t)

	job := createRunningTransitionTestJob(
		t,
		store,
		"worker-1",
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
		resource.JobResources{
			CPUCores: 2,
			MemoryMB: 2048,
			GPU: resource.GPUEnvelope{
				Min:       2,
				Preferred: 2,
				Max:       4,
			},
		},
	)

	tr, err := store.AdmitTransition(
		context.Background(),
		job.ID,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)
	if err != nil {
		t.Fatalf(
			"AdmitTransition(): %v",
			err,
		)
	}

	if tr.Reserved.GPUs != 2 {
		t.Fatalf(
			"reserved GPUs = %d, want 2",
			tr.Reserved.GPUs,
		)
	}

	// Deliberately create persisted state that violates Forge's invariant.
	//
	// We bypass Store.Complete because Complete correctly cleans transitions.
	_, err = store.DB.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = 'COMPLETED',
			worker_id = NULL,
			lease_until = NULL,
			allocated_cpu_cores = NULL,
			allocated_memory_mb = NULL,
			allocated_gpu_count = NULL
		WHERE id = $1
		`,
		job.ID,
	)
	if err != nil {
		t.Fatalf(
			"corrupt test job state: %v",
			err,
		)
	}

	reconciled, err :=
		store.ReconcileStaleTransitions(
			context.Background(),
		)
	if err != nil {
		t.Fatalf(
			"ReconcileStaleTransitions(): %v",
			err,
		)
	}

	if reconciled < 1 {
		t.Fatalf(
			"reconciled = %d, want at least 1",
			reconciled,
		)
	}

	got, err := store.GetTransition(
		context.Background(),
		tr.ID,
	)
	if err != nil {
		t.Fatalf(
			"GetTransition(): %v",
			err,
		)
	}

	if got.State != transitionmodel.StateFailed {
		t.Fatalf(
			"state = %s, want FAILED",
			got.State,
		)
	}

	if got.Reserved.CPUCores != 0 ||
		got.Reserved.MemoryMB != 0 ||
		got.Reserved.GPUs != 0 {

		t.Fatalf(
			"reservation = %+v, want zero",
			got.Reserved,
		)
	}

	if got.FailureReason == "" {
		t.Fatal(
			"expected reconciliation failure reason",
		)
	}
}
