package costmodel

import (
	"math"
	"sort"
)

type Prediction struct {
	ActualUS   int64
	ExpectedUS int64
	SafeUS     int64
}

type Evaluation struct {
	Count int

	MAEUS  float64
	RMSEUS float64

	MedianRelativeError float64
	SafeCoverage        float64
}

func EvaluatePredictions(
	predictions []Prediction,
) Evaluation {
	if len(predictions) == 0 {
		return Evaluation{}
	}

	var (
		absoluteErrorSum float64
		squaredErrorSum  float64
		safeCount        int
	)

	relativeErrors :=
		make(
			[]float64,
			0,
			len(predictions),
		)

	for _, prediction := range predictions {
		errorUS :=
			float64(
				prediction.ExpectedUS -
					prediction.ActualUS,
			)

		absoluteError :=
			math.Abs(errorUS)

		absoluteErrorSum +=
			absoluteError

		squaredErrorSum +=
			errorUS * errorUS

		if prediction.ActualUS > 0 {
			relativeErrors = append(
				relativeErrors,
				absoluteError/
					float64(prediction.ActualUS),
			)
		}

		if prediction.SafeUS >=
			prediction.ActualUS {

			safeCount++
		}
	}

	sort.Float64s(
		relativeErrors,
	)

	return Evaluation{
		Count: len(predictions),

		MAEUS: absoluteErrorSum /
			float64(len(predictions)),

		RMSEUS: math.Sqrt(
			squaredErrorSum /
				float64(len(predictions)),
		),

		MedianRelativeError: medianFloat64(
			relativeErrors,
		),

		SafeCoverage: float64(safeCount) /
			float64(len(predictions)),
	}
}
