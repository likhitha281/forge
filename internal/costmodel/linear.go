package costmodel

import "math"

type LinearEstimator struct {
	intercept float64
	slope     float64

	safetyMultiplier float64
}

func NewLinearEstimator(
	observations []Observation,
	safetyMultiplier float64,
) *LinearEstimator {
	if len(observations) == 0 {
		return &LinearEstimator{
			safetyMultiplier: safetyMultiplier,
		}
	}

	var (
		sumX float64
		sumY float64
	)

	for _, observation := range observations {
		sumX += float64(
			observation.StateMB,
		)

		sumY += float64(
			observation.TotalUS,
		)
	}

	meanX :=
		sumX /
			float64(len(observations))

	meanY :=
		sumY /
			float64(len(observations))

	var (
		numerator   float64
		denominator float64
	)

	for _, observation := range observations {
		x :=
			float64(
				observation.StateMB,
			)

		y :=
			float64(
				observation.TotalUS,
			)

		dx := x - meanX

		numerator +=
			dx * (y - meanY)

		denominator +=
			dx * dx
	}

	var slope float64

	if denominator > 0 {
		slope =
			numerator /
				denominator
	}

	intercept :=
		meanY -
			slope*meanX

	return &LinearEstimator{
		intercept: intercept,
		slope:     slope,

		safetyMultiplier: safetyMultiplier,
	}
}

func (e *LinearEstimator) Estimate(
	request Request,
) Estimate {
	expected :=
		e.intercept +
			e.slope*
				float64(request.StateMB)

	if expected < 0 {
		expected = 0
	}

	expectedUS :=
		int64(
			math.Round(expected),
		)

	safeUS :=
		int64(
			math.Round(
				expected *
					e.safetyMultiplier,
			),
		)

	return Estimate{
		ExpectedUS: expectedUS,
		SafeUS:     safeUS,
		Model:      "linear-state-size",
	}
}
