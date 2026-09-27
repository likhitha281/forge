package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type observation struct {
	Policy            string
	StateMB           int
	Work              int
	CompletionSeconds float64
	TransitionCount   int
	TransitionSeconds float64
}

type groupKey struct {
	StateMB int
	Work    int
	Policy  string
}

type summary struct {
	Key                   groupKey
	N                     int
	Mean                  float64
	Median                float64
	StdDev                float64
	Min                   float64
	Max                   float64
	Transitions           int
	MeanTransitionSeconds float64
}

func main() {
	input := flag.String(
		"input",
		"",
		"comma-separated policy-evaluation CSV files",
	)
	flag.Parse()

	if strings.TrimSpace(*input) == "" {
		log.Fatal("-input is required")
	}

	var observations []observation

	for _, rawPath := range strings.Split(*input, ",") {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}

		rows, err := loadCSV(path)
		if err != nil {
			log.Fatalf("load %s: %v", path, err)
		}

		observations = append(observations, rows...)
	}

	if len(observations) == 0 {
		log.Fatal("no observations loaded")
	}

	summaries := summarize(observations)

	fmt.Println("Forge Policy Evaluation")
	fmt.Printf("Observations: %d\n\n", len(observations))

	fmt.Printf(
		"%-9s %-10s %-12s %4s %11s %11s %11s %11s %11s %12s %15s\n",
		"STATE",
		"WORK",
		"POLICY",
		"N",
		"MEAN(s)",
		"MEDIAN(s)",
		"STDDEV(s)",
		"MIN(s)",
		"MAX(s)",
		"TRANSITIONS",
		"MEAN TRANS(s)",
	)

	for _, s := range summaries {
		fmt.Printf(
			"%-9s %-10d %-12s %4d %11.3f %11.3f %11.3f %11.3f %11.3f %12s %15.3f\n",
			fmt.Sprintf("%d MiB", s.Key.StateMB),
			s.Key.Work,
			s.Key.Policy,
			s.N,
			s.Mean,
			s.Median,
			s.StdDev,
			s.Min,
			s.Max,
			fmt.Sprintf("%d/%d", s.Transitions, s.N),
			s.MeanTransitionSeconds,
		)
	}

	fmt.Println()
	fmt.Println("Median completion-time comparison")

	printComparisons(summaries)
}

func loadCSV(path string) ([]observation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	header, err := reader.Read()
	if err != nil {
		return nil, err
	}

	index := make(map[string]int)

	for i, name := range header {
		index[strings.TrimSpace(name)] = i
	}

	required := []string{
		"policy",
		"state_mb",
		"work",
		"completion_seconds",
		"transition_count",
		"transition_seconds",
	}

	for _, name := range required {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf(
				"missing required column %q",
				name,
			)
		}
	}

	var observations []observation

	for {
		record, err := reader.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		stateMB, err := strconv.Atoi(
			record[index["state_mb"]],
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse state_mb: %w",
				err,
			)
		}

		work, err := strconv.Atoi(
			record[index["work"]],
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse work: %w",
				err,
			)
		}

		completion, err := strconv.ParseFloat(
			record[index["completion_seconds"]],
			64,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse completion_seconds: %w",
				err,
			)
		}

		transitionCount, err := strconv.Atoi(
			record[index["transition_count"]],
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse transition_count: %w",
				err,
			)
		}

		transitionSeconds, err := strconv.ParseFloat(
			record[index["transition_seconds"]],
			64,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse transition_seconds: %w",
				err,
			)
		}

		observations = append(
			observations,
			observation{
				Policy:            record[index["policy"]],
				StateMB:           stateMB,
				Work:              work,
				CompletionSeconds: completion,
				TransitionCount:   transitionCount,
				TransitionSeconds: transitionSeconds,
			},
		)
	}

	return observations, nil
}

func summarize(observations []observation) []summary {
	grouped := make(map[groupKey][]observation)

	for _, obs := range observations {
		k := groupKey{
			StateMB: obs.StateMB,
			Work:    obs.Work,
			Policy:  obs.Policy,
		}

		grouped[k] = append(grouped[k], obs)
	}

	result := make([]summary, 0, len(grouped))

	for k, rows := range grouped {
		values := make([]float64, 0, len(rows))

		transitionCount := 0
		transitionTotal := 0.0

		for _, row := range rows {
			values = append(
				values,
				row.CompletionSeconds,
			)

			if row.TransitionCount > 0 {
				transitionCount++
			}

			transitionTotal += row.TransitionSeconds
		}

		sort.Float64s(values)

		avg := mean(values)

		result = append(
			result,
			summary{
				Key:         k,
				N:           len(values),
				Mean:        avg,
				Median:      median(values),
				StdDev:      sampleStdDev(values, avg),
				Min:         values[0],
				Max:         values[len(values)-1],
				Transitions: transitionCount,
				MeanTransitionSeconds: transitionTotal /
					float64(len(rows)),
			},
		)
	}

	sort.Slice(
		result,
		func(i, j int) bool {
			if result[i].Key.StateMB != result[j].Key.StateMB {
				return result[i].Key.StateMB <
					result[j].Key.StateMB
			}

			if result[i].Key.Work != result[j].Key.Work {
				return result[i].Key.Work <
					result[j].Key.Work
			}

			return policyOrder(result[i].Key.Policy) <
				policyOrder(result[j].Key.Policy)
		},
	)

	return result
}

func printComparisons(summaries []summary) {
	type experiment struct {
		StateMB int
		Work    int
	}

	grouped := make(
		map[experiment]map[string]summary,
	)

	for _, s := range summaries {
		e := experiment{
			StateMB: s.Key.StateMB,
			Work:    s.Key.Work,
		}

		if grouped[e] == nil {
			grouped[e] = make(
				map[string]summary,
			)
		}

		grouped[e][s.Key.Policy] = s
	}

	experiments := make(
		[]experiment,
		0,
		len(grouped),
	)

	for e := range grouped {
		experiments = append(
			experiments,
			e,
		)
	}

	sort.Slice(
		experiments,
		func(i, j int) bool {
			if experiments[i].StateMB !=
				experiments[j].StateMB {
				return experiments[i].StateMB <
					experiments[j].StateMB
			}

			return experiments[i].Work <
				experiments[j].Work
		},
	)

	fmt.Printf(
		"%-9s %-10s %12s %12s %14s %15s %17s %16s\n",
		"STATE",
		"WORK",
		"STATIC(s)",
		"NAIVE(s)",
		"COST-AWARE(s)",
		"NAIVE vs STATIC",
		"AWARE vs STATIC",
		"AWARE vs NAIVE",
	)

	for _, e := range experiments {
		rows := grouped[e]

		static, staticOK := rows["static"]
		naive, naiveOK := rows["naive"]
		aware, awareOK := rows["cost-aware"]

		if !staticOK || !naiveOK || !awareOK {
			continue
		}

		fmt.Printf(
			"%-9s %-10d %12.3f %12.3f %14.3f %14.2f%% %16.2f%% %15.2f%%\n",
			fmt.Sprintf("%d MiB", e.StateMB),
			e.Work,
			static.Median,
			naive.Median,
			aware.Median,
			improvement(
				static.Median,
				naive.Median,
			),
			improvement(
				static.Median,
				aware.Median,
			),
			improvement(
				naive.Median,
				aware.Median,
			),
		)
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func median(values []float64) float64 {
	n := len(values)

	if n == 0 {
		return 0
	}

	if n%2 == 1 {
		return values[n/2]
	}

	return (values[n/2-1] + values[n/2]) / 2
}

func sampleStdDev(
	values []float64,
	avg float64,
) float64 {
	if len(values) < 2 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		delta := value - avg
		sum += delta * delta
	}

	return math.Sqrt(
		sum / float64(len(values)-1),
	)
}

func improvement(
	baseline float64,
	candidate float64,
) float64 {
	if baseline == 0 {
		return 0
	}

	return ((baseline - candidate) / baseline) * 100
}

func policyOrder(policy string) int {
	switch policy {
	case "static":
		return 0
	case "naive":
		return 1
	case "cost-aware":
		return 2
	default:
		return 3
	}
}
