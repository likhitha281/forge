package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/likhitha281/forge/internal/costmodel"
)

type result struct {
	name       string
	evaluation costmodel.Evaluation
}

func main() {
	input := flag.String(
		"input",
		"results/transition_costs_combined.csv",
		"path to transition-cost CSV",
	)

	flag.Parse()

	observations, err :=
		costmodel.LoadCSV(*input)

	if err != nil {
		log.Fatalf(
			"load observations: %v",
			err,
		)
	}

	if len(observations) < 2 {
		log.Fatal(
			"at least two observations are required",
		)
	}

	const (
		safetyMultiplier = 2.0
		minHybridSamples = 3
	)

	looResults := []result{
		{
			name: "global-median",
			evaluation: costmodel.LeaveOneOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewMedianEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "state-median",
			evaluation: costmodel.LeaveOneOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewStateMedianEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "throughput",
			evaluation: costmodel.LeaveOneOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewThroughputEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "linear-state-size",
			evaluation: costmodel.LeaveOneOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewLinearEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "hybrid",
			evaluation: costmodel.LeaveOneOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewHybridEstimator(
						training,
						safetyMultiplier,
						minHybridSamples,
					)
				},
			),
		},
	}

	stateOutResults := []result{
		{
			name: "global-median",
			evaluation: costmodel.LeaveOneStateSizeOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewMedianEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "state-median",
			evaluation: costmodel.LeaveOneStateSizeOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewStateMedianEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "throughput",
			evaluation: costmodel.LeaveOneStateSizeOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewThroughputEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "linear-state-size",
			evaluation: costmodel.LeaveOneStateSizeOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewLinearEstimator(
						training,
						safetyMultiplier,
					)
				},
			),
		},
		{
			name: "hybrid",
			evaluation: costmodel.LeaveOneStateSizeOut(
				observations,
				func(
					training []costmodel.Observation,
				) costmodel.Estimator {
					return costmodel.NewHybridEstimator(
						training,
						safetyMultiplier,
						minHybridSamples,
					)
				},
			),
		},
	}

	fmt.Println(
		"Forge Cost Model Evaluation",
	)

	fmt.Printf(
		"Observations: %d\n\n",
		len(observations),
	)

	printResults(
		"Leave-one-observation-out",
		looResults,
	)

	printResults(
		"Leave-one-state-size-out",
		stateOutResults,
	)
}

func printResults(
	title string,
	results []result,
) {
	fmt.Println(title)

	fmt.Printf(
		"%-20s %12s %12s %18s %16s\n",
		"MODEL",
		"MAE(ms)",
		"RMSE(ms)",
		"MEDIAN REL ERROR",
		"SAFE COVERAGE",
	)

	for _, result := range results {
		evaluation :=
			result.evaluation

		fmt.Printf(
			"%-20s %12.2f %12.2f %17.2f%% %15.2f%%\n",
			result.name,
			evaluation.MAEUS/1000,
			evaluation.RMSEUS/1000,
			evaluation.MedianRelativeError*100,
			evaluation.SafeCoverage*100,
		)
	}

	fmt.Println()
}
