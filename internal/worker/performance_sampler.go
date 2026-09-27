package worker

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/likhitha281/forge/internal/performance"
)

func RunPerformanceSampler(
	ctx context.Context,
	runtimes *RuntimeManager,
	profiles *PerformanceProfiles,
	interval time.Duration,
) {
	if interval <= 0 {
		interval =
			time.Second
	}

	ticker :=
		time.NewTicker(
			interval,
		)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			sampleRuntimes(
				runtimes,
				profiles,
			)
		}
	}
}

func sampleRuntimes(
	runtimes *RuntimeManager,
	profiles *PerformanceProfiles,
) {
	for _, runtime := range runtimes.Snapshot() {

		progressPath :=
			runtime.ProgressPath()

		if progressPath == "" {
			continue
		}

		snapshot, err :=
			performance.ReadSnapshot(
				progressPath,
			)

		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf(
					"job=%s read performance telemetry error=%v",
					runtime.JobID,
					err,
				)
			}

			continue
		}

		// Ignore stale telemetry. A stopped or wedged workload
		// must not keep influencing autoscaling decisions.
		if !snapshot.Timestamp.IsZero() &&
			time.Since(
				snapshot.Timestamp,
			) > 10*time.Second {

			continue
		}

		model :=
			profiles.Model(
				"elastic",
			)

		if err := model.Observe(
			snapshot,
		); err != nil {
			log.Printf(
				"job=%s observe performance telemetry error=%v",
				runtime.JobID,
				err,
			)
		}
	}
}
