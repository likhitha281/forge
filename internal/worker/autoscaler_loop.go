package worker

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/autoscaler"
	"github.com/likhitha281/forge/internal/costmodel"
	"github.com/likhitha281/forge/internal/performance"
	"github.com/likhitha281/forge/internal/resource"
	"github.com/likhitha281/forge/internal/transitionpolicy"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AutoscalerConfig struct {
	Interval time.Duration
	Cooldown time.Duration

	MinimumNetBenefit time.Duration

	SafetyMultiplier float64
	MinCostSamples   int

	CostDataPath string
}

type autoscalerState struct {
	mu sync.Mutex

	controllers map[string]*autoscaler.Controller
}

func newAutoscalerState() *autoscalerState {
	return &autoscalerState{
		controllers: make(
			map[string]*autoscaler.Controller,
		),
	}
}

func (s *autoscalerState) controller(
	jobID string,
	performanceModel *performance.Model,
	policy *transitionpolicy.Policy,
	cooldown time.Duration,
) *autoscaler.Controller {
	s.mu.Lock()
	defer s.mu.Unlock()

	controller, exists :=
		s.controllers[jobID]

	if exists {
		return controller
	}

	controller =
		autoscaler.New(
			performanceModel,
			policy,
			cooldown,
		)

	s.controllers[jobID] =
		controller

	return controller
}

func RunAutoscalerLoop(
	ctx context.Context,
	client forgev1.ForgeClient,
	runtimes *RuntimeManager,
	profiles *PerformanceProfiles,
	config AutoscalerConfig,
) {
	if config.CostDataPath == "" {
		log.Printf(
			"autoscaler disabled: no transition cost dataset configured",
		)
		return
	}

	observations, err :=
		costmodel.LoadCSV(
			config.CostDataPath,
		)

	if err != nil {
		log.Printf(
			"autoscaler disabled: load cost data: %v",
			err,
		)
		return
	}

	if len(observations) == 0 {
		log.Printf(
			"autoscaler disabled: transition cost dataset is empty",
		)
		return
	}

	if config.Interval <= 0 {
		config.Interval =
			5 * time.Second
	}

	if config.Cooldown <= 0 {
		config.Cooldown =
			30 * time.Second
	}

	if config.SafetyMultiplier <= 0 {
		config.SafetyMultiplier =
			2
	}

	if config.MinCostSamples <= 0 {
		config.MinCostSamples =
			3
	}

	estimator :=
		costmodel.NewHybridEstimator(
			observations,
			config.SafetyMultiplier,
			config.MinCostSamples,
		)

	policy :=
		transitionpolicy.New(
			estimator,
			config.MinimumNetBenefit,
		)

	state :=
		newAutoscalerState()

	ticker :=
		time.NewTicker(
			config.Interval,
		)

	defer ticker.Stop()

	log.Printf(
		"autoscaler enabled interval=%s cooldown=%s observations=%d",
		config.Interval,
		config.Cooldown,
		len(observations),
	)

	for {
		select {
		case <-ctx.Done():
			return

		case now := <-ticker.C:
			evaluateAutoscaling(
				ctx,
				client,
				runtimes,
				profiles,
				policy,
				state,
				config,
				now,
			)
		}
	}
}

func evaluateAutoscaling(
	ctx context.Context,
	client forgev1.ForgeClient,
	runtimes *RuntimeManager,
	profiles *PerformanceProfiles,
	policy *transitionpolicy.Policy,
	state *autoscalerState,
	config AutoscalerConfig,
	now time.Time,
) {
	for _, runtime := range runtimes.Snapshot() {

		if !runtime.Checkpointable() {
			continue
		}

		if !strings.Contains(
			runtime.Command(),
			"elastic",
		) {
			continue
		}

		progressPath :=
			runtime.ProgressPath()

		if progressPath == "" {
			continue
		}

		snapshot, err :=
			performance.ReadSnapshot(
				progressPath,
			)

		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf(
					"autoscaler job=%s telemetry error=%v",
					runtime.JobID,
					err,
				)
			}

			continue
		}

		if !snapshot.Timestamp.IsZero() &&
			now.Sub(snapshot.Timestamp) >
				10*time.Second {

			continue
		}

		model :=
			profiles.Model(
				"elastic",
			)

		// The sampler normally observes this first. Observing again
		// here is harmless and ensures the decision uses fresh data.

		current :=
			runtime.Allocation()

		targetGPU :=
			targetGPUFor(
				current.GPUs,
				runtime.MaxGPUs(),
			)

		if targetGPU <=
			int(current.GPUs) {

			continue
		}

		stateMB, ok :=
			stateMBFromCommand(
				runtime.Command(),
			)

		if !ok {
			continue
		}

		controller :=
			state.controller(
				runtime.JobID,
				model,
				policy,
				config.Cooldown,
			)

		decision, err :=
			controller.Evaluate(
				now,
				stateMB,
				int(current.GPUs),
				targetGPU,
			)

		if err != nil {
			continue
		}

		if decision.Policy.Decision !=
			transitionpolicy.DecisionTransition {

			continue
		}

		target :=
			current

		target.GPUs =
			int32(targetGPU)

		_, err =
			client.RequestTransition(
				ctx,
				&forgev1.RequestTransitionRequest{
					JobId: runtime.JobID,

					Target: resourcePBForAutoscaler(
						target,
					),
				},
			)

		if err != nil {
			code :=
				status.Code(err)

			if code != codes.ResourceExhausted &&
				code != codes.FailedPrecondition {

				log.Printf(
					"autoscaler job=%s request transition %d->%d error=%v",
					runtime.JobID,
					current.GPUs,
					targetGPU,
					err,
				)
			}

			continue
		}

		controller.RecordTransition(
			now,
		)

		log.Printf(
			"autoscaler job=%s decision=TRANSITION source_gpu=%d target_gpu=%d speedup=%.3f remaining=%s safe_cost=%s net_benefit=%s",
			runtime.JobID,
			current.GPUs,
			targetGPU,
			decision.Speedup,
			decision.RemainingRuntime,
			decision.Policy.SafeTransitionCost,
			decision.Policy.NetBenefit,
		)
	}
}

func targetGPUFor(
	current int32,
	max int32,
) int {
	if current >= max {
		return int(current)
	}

	target :=
		current * 2

	if target > max {
		target = max
	}

	return int(target)
}

func stateMBFromCommand(
	command string,
) (int, bool) {
	fields :=
		strings.Fields(command)

	for i := 0; i < len(fields); i++ {
		if fields[i] == "--state-mb" &&
			i+1 < len(fields) {

			value, err :=
				strconv.Atoi(
					fields[i+1],
				)

			if err != nil ||
				value <= 0 {

				return 0, false
			}

			return value, true
		}
	}

	return 0, false
}

func resourcePBForAutoscaler(
	value resource.Vector,
) *forgev1.ResourceVector {
	return &forgev1.ResourceVector{
		CpuCores: value.CPUCores,

		MemoryMb: value.MemoryMB,

		Gpus: value.GPUs,
	}
}
