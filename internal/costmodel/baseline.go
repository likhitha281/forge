package costmodel

import (
	"math"
	"sort"
)

type MedianEstimator struct {
	medianUS         int64
	safetyMultiplier float64
	model            string
}

func NewMedianEstimator(
	observations []Observation,
	safetyMultiplier float64,
) *MedianEstimator {
	values := make(
		[]int64,
		0,
		len(observations),
	)

	for _, observation := range observations {
		values = append(
			values,
			observation.TotalUS,
		)
	}

	sort.Slice(
		values,
		func(i, j int) bool {
			return values[i] < values[j]
		},
	)

	var medianUS int64

	if len(values) > 0 {
		medianUS = int64(
			math.Round(
				median(values),
			),
		)
	}

	return &MedianEstimator{
		medianUS:         medianUS,
		safetyMultiplier: safetyMultiplier,
		model:            "global-median",
	}
}

func (e *MedianEstimator) Estimate(
	request Request,
) Estimate {
	_ = request

	return Estimate{
		ExpectedUS: e.medianUS,
		SafeUS: int64(
			math.Round(
				float64(e.medianUS) *
					e.safetyMultiplier,
			),
		),
		Model: e.model,
	}
}

type StateMedianEstimator struct {
	medians map[int]int64

	fallback *MedianEstimator

	safetyMultiplier float64
}

func NewStateMedianEstimator(
	observations []Observation,
	safetyMultiplier float64,
) *StateMedianEstimator {
	groups :=
		make(
			map[int][]int64,
		)

	for _, observation := range observations {
		groups[observation.StateMB] =
			append(
				groups[observation.StateMB],
				observation.TotalUS,
			)
	}

	medians :=
		make(
			map[int]int64,
			len(groups),
		)

	for stateMB, values := range groups {
		sort.Slice(
			values,
			func(i, j int) bool {
				return values[i] < values[j]
			},
		)

		medians[stateMB] =
			int64(
				math.Round(
					median(values),
				),
			)
	}

	return &StateMedianEstimator{
		medians: medians,

		fallback: NewMedianEstimator(
			observations,
			safetyMultiplier,
		),

		safetyMultiplier: safetyMultiplier,
	}
}

func (e *StateMedianEstimator) Estimate(
	request Request,
) Estimate {
	value, ok :=
		e.medians[request.StateMB]

	if !ok {
		fallback :=
			e.fallback.Estimate(
				request,
			)

		fallback.Model =
			"state-median-fallback"

		return fallback
	}

	return Estimate{
		ExpectedUS: value,

		SafeUS: int64(
			math.Round(
				float64(value) *
					e.safetyMultiplier,
			),
		),

		Model: "state-median",
	}
}
