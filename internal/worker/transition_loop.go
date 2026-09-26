package worker

import (
	"context"
	"log"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	transitionmodel "github.com/likhitha281/forge/internal/transition"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RunTransitionLoop continuously claims transitions belonging to this worker
// and executes them against the corresponding local runtime.
func RunTransitionLoop(
	ctx context.Context,
	client forgev1.ForgeClient,
	workerID string,
	runtimes *RuntimeManager,
) {
	store := NewTransitionStore(
		client,
		workerID,
	)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		response, err := client.LeaseTransition(
			ctx,
			&forgev1.LeaseTransitionRequest{
				WorkerId: workerID,
			},
		)

		if err != nil {
			if status.Code(err) == codes.NotFound {
				select {
				case <-ctx.Done():
					return

				case <-time.After(time.Second):
					continue
				}
			}

			log.Printf(
				"worker=%s transition lease error: %v",
				workerID,
				err,
			)

			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
				continue
			}
		}

		if response.Transition == nil {
			log.Printf(
				"worker=%s coordinator returned nil leased transition",
				workerID,
			)

			continue
		}

		tr, err := transitionFromPB(
			response.Transition,
		)
		if err != nil {
			log.Printf(
				"worker=%s decode transition=%s error=%v",
				workerID,
				response.Transition.Id,
				err,
			)

			continue
		}

		log.Printf(
			"worker=%s claimed transition=%s job=%s source_gpu=%d target_gpu=%d",
			workerID,
			tr.ID,
			tr.JobID,
			tr.Source.GPUs,
			tr.Target.GPUs,
		)

		runtime, ok := runtimes.Get(
			tr.JobID,
		)

		if !ok {
			reason :=
				"worker runtime not found for transition"

			log.Printf(
				"worker=%s transition=%s job=%s failed: %s",
				workerID,
				tr.ID,
				tr.JobID,
				reason,
			)

			_, failErr := store.FailTransition(
				ctx,
				tr.ID,
				reason,
			)

			if failErr != nil {
				log.Printf(
					"worker=%s transition=%s failed to persist failure: %v",
					workerID,
					tr.ID,
					failErr,
				)
			}

			continue
		}

		// The worker's local allocation must still match the source
		// allocation from which the coordinator admitted the transition.
		current := runtime.Allocation()

		if !current.Equal(tr.Source) {
			reason :=
				"local runtime allocation does not match transition source"

			log.Printf(
				"worker=%s transition=%s job=%s failed: %s local=%v source=%v",
				workerID,
				tr.ID,
				tr.JobID,
				reason,
				current,
				tr.Source,
			)

			_, failErr := store.FailTransition(
				ctx,
				tr.ID,
				reason,
			)

			if failErr != nil {
				log.Printf(
					"worker=%s transition=%s failed to persist failure: %v",
					workerID,
					tr.ID,
					failErr,
				)
			}

			continue
		}

		var executor transitionmodel.Executor

		if runtime.Checkpointable() {
			executor = &ProcessExecutor{
				Runtime: runtime,

				CheckpointPath: runtime.CheckpointPath(),

				CheckpointDone: runtime.CheckpointDonePath(),

				RestoreDone: runtime.RestoreDonePath(),

				Timeout:      30 * time.Second,
				PollInterval: 250 * time.Microsecond,
			}

			log.Printf(
				"worker=%s transition=%s using process executor",
				workerID,
				tr.ID,
			)
		} else {
			executor = transitionmodel.SimulatedExecutor{
				PrepareDuration: 100 * time.Millisecond,

				CheckpointDuration: 300 * time.Millisecond,

				ReconfigureDuration: 200 * time.Millisecond,

				RestoreDuration: 300 * time.Millisecond,

				ResumeDuration: 100 * time.Millisecond,

				CheckpointBytes: 64 * 1024 * 1024,

				RestoreBytes: 64 * 1024 * 1024,
			}

			log.Printf(
				"worker=%s transition=%s using simulated executor",
				workerID,
				tr.ID,
			)
		}

		runner := transitionmodel.Runner{
			Store:    store,
			Executor: executor,
		}

		result, err := runner.Run(
			ctx,
			tr,
		)

		if err != nil {
			log.Printf(
				"worker=%s transition=%s job=%s execution failed: %v",
				workerID,
				tr.ID,
				tr.JobID,
				err,
			)

			continue
		}

		metrics :=
			transitionmodel.MetricsFromExecutionResult(
				result,
			)

		if err := store.RecordMetrics(
			ctx,
			tr.ID,
			metrics,
		); err != nil {
			log.Printf(
				"worker=%s transition=%s failed to record metrics: %v",
				workerID,
				tr.ID,
				err,
			)
		}

		// CompleteTransition has already committed Target as the persisted
		// allocation. Keep the worker's local view consistent with it.
		runtime.SetAllocation(
			tr.Target,
		)

		log.Printf(
			"worker=%s transition=%s job=%s completed source_gpu=%d target_gpu=%d duration=%s bytes_moved=%d",
			workerID,
			tr.ID,
			tr.JobID,
			tr.Source.GPUs,
			tr.Target.GPUs,
			result.TotalDuration(),
			result.TotalBytesMoved(),
		)
	}
}
