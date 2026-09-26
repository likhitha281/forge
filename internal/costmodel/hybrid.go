package costmodel

type HybridEstimator struct {
	stateCounts map[int]int

	stateEstimator      *StateMedianEstimator
	throughputEstimator *ThroughputEstimator

	minSamples int
}

func NewHybridEstimator(
	observations []Observation,
	safetyMultiplier float64,
	minSamples int,
) *HybridEstimator {
	counts :=
		make(
			map[int]int,
		)

	for _, observation := range observations {

		counts[observation.StateMB]++
	}

	return &HybridEstimator{
		stateCounts: counts,

		stateEstimator: NewStateMedianEstimator(
			observations,
			safetyMultiplier,
		),

		throughputEstimator: NewThroughputEstimator(
			observations,
			safetyMultiplier,
		),

		minSamples: minSamples,
	}
}

func (e *HybridEstimator) Estimate(
	request Request,
) Estimate {
	if e.stateCounts[request.StateMB] >=
		e.minSamples {

		estimate :=
			e.stateEstimator.Estimate(
				request,
			)

		estimate.Model =
			"hybrid-empirical"

		return estimate
	}

	estimate :=
		e.throughputEstimator.Estimate(
			request,
		)

	estimate.Model =
		"hybrid-throughput"

	return estimate
}
