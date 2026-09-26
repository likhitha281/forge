package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/likhitha281/forge/internal/costmodel"
	"github.com/likhitha281/forge/internal/transitionpolicy"
)

func main() {
	data := flag.String(
		"data",
		"results/transition_costs_combined.csv",
		"path to transition cost observations",
	)

	stateMB := flag.Int(
		"state-mb",
		0,
		"checkpoint state size in MiB",
	)

	sourceGPU := flag.Int(
		"source-gpu",
		0,
		"current GPU allocation",
	)

	targetGPU := flag.Int(
		"target-gpu",
		0,
		"proposed GPU allocation",
	)

	remaining := flag.Duration(
		"remaining",
		0,
		"estimated remaining runtime",
	)

	speedup := flag.Float64(
		"speedup",
		1.0,
		"expected speedup at target allocation",
	)

	minBenefit := flag.Duration(
		"min-benefit",
		5*time.Second,
		"minimum net benefit required to recommend transition",
	)

	safetyMultiplier := flag.Float64(
		"safety",
		2.0,
		"transition-cost safety multiplier",
	)

	minSamples := flag.Int(
		"min-samples",
		3,
		"minimum observations required for empirical estimation",
	)

	flag.Parse()

	if *stateMB <= 0 {
		log.Fatal("state-mb must be positive")
	}

	if *sourceGPU <= 0 {
		log.Fatal("source-gpu must be positive")
	}

	if *targetGPU <= 0 {
		log.Fatal("target-gpu must be positive")
	}

	if *sourceGPU == *targetGPU {
		log.Fatal("source-gpu and target-gpu must differ")
	}

	if *remaining <= 0 {
		log.Fatal("remaining must be positive")
	}

	if *speedup <= 0 {
		log.Fatal("speedup must be positive")
	}

	if *safetyMultiplier < 1 {
		log.Fatal("safety must be at least 1")
	}

	if *minSamples <= 0 {
		log.Fatal("min-samples must be positive")
	}

	observations, err :=
		costmodel.LoadCSV(*data)

	if err != nil {
		log.Fatalf(
			"load transition observations: %v",
			err,
		)
	}

	if len(observations) == 0 {
		log.Fatal(
			"transition observation dataset is empty",
		)
	}

	estimator :=
		costmodel.NewHybridEstimator(
			observations,
			*safetyMultiplier,
			*minSamples,
		)

	policy :=
		transitionpolicy.New(
			estimator,
			*minBenefit,
		)

	result :=
		policy.Evaluate(
			transitionpolicy.Request{
				StateMB: *stateMB,

				SourceGPU: *sourceGPU,

				TargetGPU: *targetGPU,

				RemainingRuntime: *remaining,

				Speedup: *speedup,
			},
		)

	fmt.Println("Forge Transition Recommendation")
	fmt.Println()

	fmt.Printf(
		"Decision:             %s\n",
		result.Decision,
	)

	fmt.Printf(
		"Model:                %s\n",
		result.Model,
	)

	fmt.Printf(
		"State size:           %d MiB\n",
		*stateMB,
	)

	fmt.Printf(
		"Allocation:           %d -> %d GPU\n",
		*sourceGPU,
		*targetGPU,
	)

	fmt.Printf(
		"Remaining runtime:    %s\n",
		remaining.String(),
	)

	fmt.Printf(
		"Expected speedup:     %.2fx\n",
		*speedup,
	)

	fmt.Printf(
		"Expected cost:        %s\n",
		result.ExpectedTransitionCost,
	)

	fmt.Printf(
		"Safe cost:            %s\n",
		result.SafeTransitionCost,
	)

	fmt.Printf(
		"Expected time saved:  %s\n",
		result.ExpectedTimeSaved,
	)

	fmt.Printf(
		"Net benefit:          %s\n",
		result.NetBenefit,
	)

	fmt.Printf(
		"Required net benefit: %s\n",
		minBenefit.String(),
	)
}
