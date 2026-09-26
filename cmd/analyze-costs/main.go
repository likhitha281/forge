package main

import (
	"flag"
	"fmt"
	"log"
	"sort"

	"github.com/likhitha281/forge/internal/costmodel"
)

type groupKey struct {
	StateMB   int
	SourceGPU int
	TargetGPU int
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

	if len(observations) == 0 {
		log.Fatal("no observations found")
	}

	groups :=
		make(
			map[groupKey][]costmodel.Observation,
		)

	for _, observation := range observations {
		key := groupKey{
			StateMB:   observation.StateMB,
			SourceGPU: observation.SourceGPU,
			TargetGPU: observation.TargetGPU,
		}

		groups[key] = append(
			groups[key],
			observation,
		)
	}

	keys := make(
		[]groupKey,
		0,
		len(groups),
	)

	for key := range groups {
		keys = append(
			keys,
			key,
		)
	}

	sort.Slice(
		keys,
		func(i, j int) bool {
			if keys[i].StateMB != keys[j].StateMB {
				return keys[i].StateMB <
					keys[j].StateMB
			}

			if keys[i].SourceGPU != keys[j].SourceGPU {
				return keys[i].SourceGPU <
					keys[j].SourceGPU
			}

			return keys[i].TargetGPU <
				keys[j].TargetGPU
		},
	)

	fmt.Println(
		"Forge Transition Cost Analysis",
	)

	fmt.Printf(
		"Observations: %d\n\n",
		len(observations),
	)

	fmt.Printf(
		"%-8s %-12s %4s %12s %12s %12s %12s %12s %12s\n",
		"STATE",
		"TRANSITION",
		"N",
		"MEDIAN(ms)",
		"MEAN(ms)",
		"STDDEV(ms)",
		"P95(ms)",
		"MIN(ms)",
		"MAX(ms)",
	)

	for _, key := range keys {
		summary :=
			costmodel.Summarize(
				groups[key],
			)

		fmt.Printf(
			"%-8s %-12s %4d %12.2f %12.2f %12.2f %12.2f %12.2f %12.2f\n",
			fmt.Sprintf(
				"%d MiB",
				key.StateMB,
			),
			fmt.Sprintf(
				"%d -> %d",
				key.SourceGPU,
				key.TargetGPU,
			),
			summary.Count,
			summary.MedianUS/1000,
			summary.MeanUS/1000,
			summary.StdDevUS/1000,
			summary.P95US/1000,
			float64(summary.MinUS)/1000,
			float64(summary.MaxUS)/1000,
		)
	}

	fmt.Println()
	fmt.Println(
		"Mean throughput by configuration",
	)

	fmt.Printf(
		"%-8s %-12s %4s %18s %18s\n",
		"STATE",
		"TRANSITION",
		"N",
		"CHECKPOINT(MB/s)",
		"RESTORE(MB/s)",
	)

	for _, key := range keys {
		summary :=
			costmodel.Summarize(
				groups[key],
			)

		fmt.Printf(
			"%-8s %-12s %4d %18.2f %18.2f\n",
			fmt.Sprintf(
				"%d MiB",
				key.StateMB,
			),
			fmt.Sprintf(
				"%d -> %d",
				key.SourceGPU,
				key.TargetGPU,
			),
			summary.Count,
			summary.MeanCheckpointThroughputMBps,
			summary.MeanRestoreThroughputMBps,
		)
	}

	overall :=
		costmodel.Summarize(
			observations,
		)

	fmt.Println()
	fmt.Println("Overall")

	fmt.Printf(
		"Median transition: %.2f ms\n",
		overall.MedianUS/1000,
	)

	fmt.Printf(
		"Mean transition:   %.2f ms\n",
		overall.MeanUS/1000,
	)

	fmt.Printf(
		"P95 transition:    %.2f ms\n",
		overall.P95US/1000,
	)

	fmt.Printf(
		"Mean checkpoint throughput: %.2f MB/s\n",
		overall.MeanCheckpointThroughputMBps,
	)

	fmt.Printf(
		"Mean restore throughput:    %.2f MB/s\n",
		overall.MeanRestoreThroughputMBps,
	)
}
