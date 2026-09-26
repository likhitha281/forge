package costmodel

import (
	"math"
	"sort"
)

type Summary struct {
	Count int

	MeanUS   float64
	MedianUS float64
	StdDevUS float64
	P95US    float64

	MinUS int64
	MaxUS int64

	MeanCheckpointThroughputMBps float64
	MeanRestoreThroughputMBps    float64
}

func Summarize(
	observations []Observation,
) Summary {
	if len(observations) == 0 {
		return Summary{}
	}

	values := make(
		[]int64,
		0,
		len(observations),
	)

	var (
		sumUS                float64
		checkpointThroughput float64
		restoreThroughput    float64
	)

	for _, observation := range observations {
		values = append(
			values,
			observation.TotalUS,
		)

		sumUS += float64(
			observation.TotalUS,
		)

		if observation.CheckpointUS > 0 {
			checkpointSeconds :=
				float64(observation.CheckpointUS) /
					1_000_000

			checkpointMB :=
				float64(observation.CheckpointBytes) /
					(1024 * 1024)

			checkpointThroughput +=
				checkpointMB /
					checkpointSeconds
		}

		if observation.RestoreUS > 0 {
			restoreSeconds :=
				float64(observation.RestoreUS) /
					1_000_000

			restoreMB :=
				float64(observation.RestoreBytes) /
					(1024 * 1024)

			restoreThroughput +=
				restoreMB /
					restoreSeconds
		}
	}

	sort.Slice(
		values,
		func(i, j int) bool {
			return values[i] < values[j]
		},
	)

	mean :=
		sumUS /
			float64(len(values))

	var variance float64

	for _, value := range values {
		difference :=
			float64(value) - mean

		variance +=
			difference * difference
	}

	variance /=
		float64(len(values))

	return Summary{
		Count: len(values),

		MeanUS:   mean,
		MedianUS: median(values),
		StdDevUS: math.Sqrt(variance),
		P95US:    percentile(values, 0.95),

		MinUS: values[0],
		MaxUS: values[len(values)-1],

		MeanCheckpointThroughputMBps: checkpointThroughput /
			float64(len(values)),

		MeanRestoreThroughputMBps: restoreThroughput /
			float64(len(values)),
	}
}

func median(
	values []int64,
) float64 {
	n := len(values)

	if n == 0 {
		return 0
	}

	if n%2 == 1 {
		return float64(
			values[n/2],
		)
	}

	return (float64(values[n/2-1]) + float64(values[n/2])) / 2
}

func percentile(
	values []int64,
	p float64,
) float64 {
	if len(values) == 0 {
		return 0
	}

	if p <= 0 {
		return float64(values[0])
	}

	if p >= 1 {
		return float64(values[len(values)-1])
	}

	index := int(math.Ceil(
		p*float64(len(values)),
	)) - 1

	return float64(values[index])
}
