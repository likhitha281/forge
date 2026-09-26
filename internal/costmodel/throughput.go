package costmodel

import (
	"math"
	"sort"
)

type ThroughputEstimator struct {
	checkpointMBps float64
	restoreMBps    float64

	safetyMultiplier float64
}

func NewThroughputEstimator(
	observations []Observation,
	safetyMultiplier float64,
) *ThroughputEstimator {
	checkpointRates :=
		make([]float64, 0, len(observations))

	restoreRates :=
		make([]float64, 0, len(observations))

	for _, observation := range observations {
		if observation.CheckpointUS > 0 &&
			observation.CheckpointBytes > 0 {

			seconds :=
				float64(observation.CheckpointUS) /
					1_000_000

			mb :=
				float64(observation.CheckpointBytes) /
					(1024 * 1024)

			checkpointRates =
				append(
					checkpointRates,
					mb/seconds,
				)
		}

		if observation.RestoreUS > 0 &&
			observation.RestoreBytes > 0 {

			seconds :=
				float64(observation.RestoreUS) /
					1_000_000

			mb :=
				float64(observation.RestoreBytes) /
					(1024 * 1024)

			restoreRates =
				append(
					restoreRates,
					mb/seconds,
				)
		}
	}

	return &ThroughputEstimator{
		checkpointMBps: medianFloat64(
			checkpointRates,
		),

		restoreMBps: medianFloat64(
			restoreRates,
		),

		safetyMultiplier: safetyMultiplier,
	}
}

func (e *ThroughputEstimator) Estimate(
	request Request,
) Estimate {
	if request.StateMB <= 0 ||
		e.checkpointMBps <= 0 ||
		e.restoreMBps <= 0 {

		return Estimate{
			Model: "throughput",
		}
	}

	stateMB :=
		float64(request.StateMB)

	expectedSeconds :=
		stateMB/e.checkpointMBps +
			stateMB/e.restoreMBps

	expectedUS :=
		int64(
			math.Round(
				expectedSeconds *
					1_000_000,
			),
		)

	safeUS :=
		int64(
			math.Round(
				float64(expectedUS) *
					e.safetyMultiplier,
			),
		)

	return Estimate{
		ExpectedUS: expectedUS,
		SafeUS:     safeUS,
		Model:      "throughput",
	}
}

func medianFloat64(
	values []float64,
) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted :=
		append(
			[]float64(nil),
			values...,
		)

	sort.Float64s(sorted)

	n := len(sorted)

	if n%2 == 1 {
		return sorted[n/2]
	}

	return (sorted[n/2-1] + sorted[n/2]) / 2
}
