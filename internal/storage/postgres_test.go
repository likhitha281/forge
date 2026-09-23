package storage

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/likhitha281/forge/internal/resource"
)

func testStore(t *testing.T) *Store {
	t.Helper()

	url := os.Getenv("FORGE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FORGE_TEST_DATABASE_URL not configured")
	}

	ctx := context.Background()

	store, err := New(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	t.Cleanup(func() {
		store.DB.Close()
	})

	resetTestData(t, store)

	return store
}

func resetTestData(t *testing.T, store *Store) {
	t.Helper()

	_, err := store.DB.Exec(
		context.Background(),
		`
		TRUNCATE TABLE jobs, workers;
		`,
	)
	if err != nil {
		t.Fatalf("reset database: %v", err)
	}
}

func registerTestWorker(
	t *testing.T,
	store *Store,
	id string,
	capacity int,
	resources resource.Vector,
) {
	t.Helper()

	err := store.Register(
		context.Background(),
		id,
		capacity,
		resources,
	)
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}
}

func submitTestJob(
	t *testing.T,
	store *Store,
	resources resource.Vector,
	priority int,
) string {
	t.Helper()

	id := uuid.NewString()

	jobID, err := store.Submit(
		context.Background(),
		id,
		"sleep 10",
		"",
		priority,
		3,
		resources,
	)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	return jobID
}

func TestLeaseFitsResources(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		4,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     2,
		},
	)

	jobID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     1,
		},
		2,
	)

	job, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("Lease() error: %v", err)
	}

	if job.ID != jobID {
		t.Fatalf(
			"leased job %s, want %s",
			job.ID,
			jobID,
		)
	}

	if job.WorkerID != "worker-1" {
		t.Fatalf(
			"worker = %s, want worker-1",
			job.WorkerID,
		)
	}
}

func TestLeaseRejectsInsufficientGPU(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"cpu-worker",
		4,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     0,
		},
	)

	submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 1,
			MemoryMB: 512,
			GPUs:     1,
		},
		2,
	)

	_, err := store.Lease(
		context.Background(),
		"cpu-worker",
		30*time.Second,
	)

	if !errors.Is(err, ErrNoLeasableJob) {
		t.Fatalf(
			"Lease() error = %v, want ErrNoLeasableJob",
			err,
		)
	}
}

func TestLeaseBackfillsAroundNonFittingJob(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		4,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
		},
	)

	runningID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 6,
			MemoryMB: 8192,
		},
		1,
	)

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("initial Lease() error: %v", err)
	}

	if first.ID != runningID {
		t.Fatalf(
			"unexpected initial job: got %s want %s",
			first.ID,
			runningID,
		)
	}

	nonFittingID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 1024,
		},
		1,
	)

	fittingID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 1,
			MemoryMB: 1024,
		},
		2,
	)

	second, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("backfill Lease() error: %v", err)
	}

	if second.ID != fittingID {
		t.Fatalf(
			"leased %s, want fitting backfill job %s",
			second.ID,
			fittingID,
		)
	}

	blocked, err := store.Get(
		context.Background(),
		nonFittingID,
	)
	if err != nil {
		t.Fatalf("Get() blocked job: %v", err)
	}

	if blocked.Status != "QUEUED" {
		t.Fatalf(
			"non-fitting job status = %s, want QUEUED",
			blocked.Status,
		)
	}
}

func TestCompletionReleasesResources(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		2,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
		},
	)

	firstID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 4096,
		},
		1,
	)

	secondID := submitTestJob(
		t,
		store,
		resource.Vector{
			CPUCores: 4,
			MemoryMB: 4096,
		},
		2,
	)

	first, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("first Lease(): %v", err)
	}

	if first.ID != firstID {
		t.Fatalf(
			"first lease = %s, want %s",
			first.ID,
			firstID,
		)
	}

	_, err = store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)

	if !errors.Is(err, ErrNoLeasableJob) {
		t.Fatalf(
			"second lease before completion = %v, want ErrNoLeasableJob",
			err,
		)
	}

	if err := store.Complete(
		context.Background(),
		firstID,
		true,
		"",
	); err != nil {
		t.Fatalf("Complete(): %v", err)
	}

	second, err := store.Lease(
		context.Background(),
		"worker-1",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("lease after completion: %v", err)
	}

	if second.ID != secondID {
		t.Fatalf(
			"lease after completion = %s, want %s",
			second.ID,
			secondID,
		)
	}
}

func TestConcurrentLeaseDoesNotOversubscribeWorker(t *testing.T) {
	store := testStore(t)

	registerTestWorker(
		t,
		store,
		"worker-1",
		20,
		resource.Vector{
			CPUCores: 8,
			MemoryMB: 16384,
			GPUs:     4,
		},
	)

	const jobs = 20

	for i := 0; i < jobs; i++ {
		submitTestJob(
			t,
			store,
			resource.Vector{
				CPUCores: 2,
				MemoryMB: 1024,
				GPUs:     1,
			},
			2,
		)
	}

	var wg sync.WaitGroup

	for i := 0; i < jobs; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, _ = store.Lease(
				context.Background(),
				"worker-1",
				30*time.Second,
			)
		}()
	}

	wg.Wait()

	var running int
	var cpu float64
	var memory int64
	var gpus int32

	err := store.DB.QueryRow(
		context.Background(),
		`
		SELECT
			COUNT(*),
			COALESCE(SUM(cpu_cores), 0),
			COALESCE(SUM(memory_mb), 0),
			COALESCE(SUM(gpu_count), 0)
		FROM jobs
		WHERE status = 'RUNNING'
		  AND worker_id = 'worker-1'
		`,
	).Scan(
		&running,
		&cpu,
		&memory,
		&gpus,
	)
	if err != nil {
		t.Fatalf("query allocations: %v", err)
	}

	if cpu > 8 {
		t.Fatalf(
			"CPU oversubscribed: %.2f > 8",
			cpu,
		)
	}

	if memory > 16384 {
		t.Fatalf(
			"memory oversubscribed: %d > 16384",
			memory,
		)
	}

	if gpus > 4 {
		t.Fatalf(
			"GPUs oversubscribed: %d > 4",
			gpus,
		)
	}

	if running != 4 {
		t.Fatalf(
			"running jobs = %d, want exactly 4",
			running,
		)
	}
}
