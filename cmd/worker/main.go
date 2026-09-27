package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/resource"
	"github.com/likhitha281/forge/internal/retry"
	workerruntime "github.com/likhitha281/forge/internal/worker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	// Use WORKER_ID when explicitly configured.
	// Otherwise use the hostname so Docker-scaled workers
	// automatically receive unique identities.
	addr := env(
		"FORGE_ADDR",
		"localhost:50051",
	)

	id := os.Getenv("WORKER_ID")

	if id == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf(
				"failed to determine worker hostname: %v",
				err,
			)
		}

		id = hostname
	}

	capN, err := strconv.Atoi(
		env("WORKER_CAPACITY", "4"),
	)
	if err != nil {
		log.Fatalf(
			"invalid WORKER_CAPACITY: %v",
			err,
		)
	}

	cpuCores, err := strconv.ParseFloat(
		env("WORKER_CPU_CORES", "0"),
		64,
	)
	if err != nil {
		log.Fatalf(
			"invalid WORKER_CPU_CORES: %v",
			err,
		)
	}

	memoryMB, err := strconv.ParseInt(
		env("WORKER_MEMORY_MB", "0"),
		10,
		64,
	)
	if err != nil {
		log.Fatalf(
			"invalid WORKER_MEMORY_MB: %v",
			err,
		)
	}

	gpus, err := strconv.ParseInt(
		env("WORKER_GPUS", "0"),
		10,
		32,
	)
	if err != nil {
		log.Fatalf(
			"invalid WORKER_GPUS: %v",
			err,
		)
	}

	log.Printf(
		"starting Forge worker id=%s capacity=%d cpu=%.2f memory=%dMB gpus=%d coordinator=%s",
		id,
		capN,
		cpuCores,
		memoryMB,
		gpus,
		addr,
	)

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := forgev1.NewForgeClient(conn)

	ctx := context.Background()

	// Register this worker with the coordinator.
	if _, err := client.RegisterWorker(
		ctx,
		&forgev1.RegisterWorkerRequest{
			WorkerId: id,
			Capacity: int32(capN),

			Resources: &forgev1.ResourceVector{
				CpuCores: cpuCores,
				MemoryMb: memoryMB,
				Gpus:     int32(gpus),
			},
		},
	); err != nil {
		log.Fatal(err)
	}

	log.Printf(
		"worker %s registered successfully",
		id,
	)

	var running atomic.Int32

	// Track all processes currently executing on this worker.
	runtimeManager :=
		workerruntime.NewRuntimeManager()

	performanceProfiles :=
		workerruntime.NewPerformanceProfiles(
			0.3,
		)

	// Run one transition-processing loop for the lifetime of the worker.
	go workerruntime.RunTransitionLoop(
		ctx,
		client,
		id,
		runtimeManager,
	)

	go workerruntime.RunPerformanceSampler(
		ctx,
		runtimeManager,
		performanceProfiles,
		time.Second,
	)

	go workerruntime.RunAutoscalerLoop(
		ctx,
		client,
		runtimeManager,
		performanceProfiles,
		workerruntime.AutoscalerConfig{
			Interval: 5 * time.Second,

			Cooldown: 30 * time.Second,

			MinimumNetBenefit: 5 * time.Second,

			SafetyMultiplier: 2.0,

			MinCostSamples: 3,

			CostDataPath: os.Getenv(
				"FORGE_TRANSITION_COST_DATA",
			),
		},
	)

	// Send periodic worker heartbeats.
	go func() {
		ticker := time.NewTicker(
			5 * time.Second,
		)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				_, err := client.Heartbeat(
					ctx,
					&forgev1.HeartbeatRequest{
						WorkerId: id,
						Running:  running.Load(),
					},
				)

				if err != nil {
					log.Printf(
						"worker=%s heartbeat failed: %v",
						id,
						err,
					)
				}
			}
		}
	}()

	// Semaphore limits the number of jobs this worker
	// can execute concurrently.
	sem := make(
		chan struct{},
		capN,
	)

	for {
		response, err := client.LeaseJob(
			ctx,
			&forgev1.LeaseJobRequest{
				WorkerId: id,
			},
		)

		if err != nil {
			if status.Code(err) ==
				codes.NotFound {
				time.Sleep(time.Second)
				continue
			}

			log.Printf(
				"worker=%s lease error: %v",
				id,
				err,
			)

			time.Sleep(time.Second)
			continue
		}

		sem <- struct{}{}
		running.Add(1)

		go func(job *forgev1.Job) {
			defer func() {
				<-sem
				running.Add(-1)
			}()

			log.Printf(
				"worker=%s executing job=%s attempt=%d",
				id,
				job.Id,
				job.Attempts,
			)

			// Give each executing job its own context.
			jobCtx, cancelJob :=
				context.WithCancel(ctx)
			defer cancelJob()

			// Signal used to stop lease renewal as soon as
			// execution terminates.
			renewDone := make(chan struct{})

			// Protect renewDone from accidental double close as
			// execution moves through the different error paths.
			var renewalStopped atomic.Bool

			stopRenewal := func() {
				if renewalStopped.CompareAndSwap(
					false,
					true,
				) {
					close(renewDone)
				}
			}

			// Renew the job lease every ten seconds.
			go func() {
				ticker := time.NewTicker(
					10 * time.Second,
				)
				defer ticker.Stop()

				for {
					select {
					case <-renewDone:
						return

					case <-jobCtx.Done():
						return

					case <-ticker.C:
						renewResponse, err :=
							client.RenewLease(
								jobCtx,
								&forgev1.RenewLeaseRequest{
									WorkerId: id,
									JobId:    job.Id,
								},
							)

						if err != nil {
							log.Printf(
								"worker=%s failed to renew lease job=%s: %v",
								id,
								job.Id,
								err,
							)
							continue
						}

						log.Printf(
							"worker=%s renewed lease job=%s lease=%ds",
							id,
							job.Id,
							renewResponse.LeaseSeconds,
						)
					}
				}
			}()

			if job.Allocation == nil {
				stopRenewal()

				log.Printf(
					"worker=%s job=%s has no allocation",
					id,
					job.Id,
				)

				_, completeErr :=
					client.CompleteJob(
						ctx,
						&forgev1.CompleteJobRequest{
							WorkerId: id,
							JobId:    job.Id,
							Success:  false,
							Error:    "leased job has no allocation",
						},
					)

				if completeErr != nil {
					log.Printf(
						"worker=%s complete invalid job=%s error=%v",
						id,
						job.Id,
						completeErr,
					)
				}

				return
			}

			// Construct the process but do not start it yet.
			cmd := exec.CommandContext(
				jobCtx,
				"/bin/sh",
				"-c",
				job.Payload,
			)

			controlDir :=
				"/tmp/forge/" + job.Id

			if err := os.MkdirAll(
				controlDir,
				0o755,
			); err != nil {
				stopRenewal()

				log.Printf(
					"worker=%s create control directory job=%s error=%v",
					id,
					job.Id,
					err,
				)

				_, completeErr :=
					client.CompleteJob(
						ctx,
						&forgev1.CompleteJobRequest{
							WorkerId: id,
							JobId:    job.Id,
							Success:  false,
							Error:    err.Error(),
						},
					)

				if completeErr != nil {
					log.Printf(
						"worker=%s complete control-directory failure job=%s error=%v",
						id,
						job.Id,
						completeErr,
					)
				}

				return
			}

			checkpointPath :=
				controlDir + "/checkpoint.bin"

			checkpointDone :=
				controlDir + "/checkpoint.done"

			restoreDone :=
				controlDir + "/restore.done"

			allocationPath :=
				filepath.Join(
					controlDir,
					"allocation",
				)

			progressPath :=
				filepath.Join(
					controlDir,
					"progress.json",
				)

			// Initialize the workload's logical GPU allocation
			// before starting the process.
			if err := os.WriteFile(
				allocationPath,
				[]byte(
					strconv.Itoa(
						int(job.Allocation.Gpus),
					)+"\n",
				),
				0o644,
			); err != nil {
				stopRenewal()

				log.Printf(
					"worker=%s initialize allocation file job=%s error=%v",
					id,
					job.Id,
					err,
				)

				return
			}

			cmd.Env = append(
				os.Environ(),

				"FORGE_JOB_ID="+job.Id,
				"FORGE_CONTROL_DIR="+controlDir,

				"FORGE_CHECKPOINT_PATH="+checkpointPath,
				"FORGE_CHECKPOINT_DONE="+checkpointDone,
				"FORGE_RESTORE_DONE="+restoreDone,
				"FORGE_ALLOCATION_FILE="+allocationPath,
				"FORGE_PROGRESS_FILE="+progressPath,
			)

			var output bytes.Buffer

			cmd.Stdout = &output
			cmd.Stderr = &output

			runtime := workerruntime.NewRuntime(
				job.Id,
				cmd,
				resource.Vector{
					CPUCores: job.Allocation.CpuCores,
					MemoryMB: job.Allocation.MemoryMb,
					GPUs:     job.Allocation.Gpus,
				},
				time.Now().UTC(),
			)

			var maxGPUs int32

			if job.Requirements != nil &&
				job.Requirements.Gpu != nil {

				maxGPUs =
					job.Requirements.Gpu.Max
			}

			runtime.SetWorkloadMetadata(
				job.Payload,
				maxGPUs,
			)

			runtime.SetWorkloadMetadata(
				job.Payload,
				job.Requirements.Gpu.Max,
			)

			runtime.SetControlPaths(
				controlDir,
				checkpointPath,
				checkpointDone,
				restoreDone,
				allocationPath,
				progressPath,
			)

			runtime.SetCheckpointable(
				strings.Contains(
					job.Payload,
					"checkpointable",
				) ||
					strings.Contains(
						job.Payload,
						"elastic",
					),
			)

			defer runtimeManager.Remove(job.Id)

			// Start the underlying process before publishing the runtime.
			// This prevents the transition loop from observing a runtime
			// whose process does not exist yet.

			// Start the underlying process before publishing the runtime.
			// This prevents the transition loop from observing a runtime
			// whose process does not exist yet.

			if err := cmd.Start(); err != nil {
				stopRenewal()

				log.Printf(
					"worker=%s start job=%s error=%v",
					id,
					job.Id,
					err,
				)

				_, completeErr :=
					client.CompleteJob(
						ctx,
						&forgev1.CompleteJobRequest{
							WorkerId: id,
							JobId:    job.Id,
							Success:  false,
							Error:    err.Error(),
						},
					)

				if completeErr != nil {
					log.Printf(
						"worker=%s complete failed-start job=%s error=%v",
						id,
						job.Id,
						completeErr,
					)
				}

				return
			}

			if err := runtimeManager.Add(
				runtime,
			); err != nil {
				stopRenewal()

				// The process already exists, so clean it up if registration fails.
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}

				log.Printf(
					"worker=%s register runtime job=%s error=%v",
					id,
					job.Id,
					err,
				)

				_, completeErr :=
					client.CompleteJob(
						ctx,
						&forgev1.CompleteJobRequest{
							WorkerId: id,
							JobId:    job.Id,
							Success:  false,
							Error:    err.Error(),
						},
					)

				if completeErr != nil {
					log.Printf(
						"worker=%s complete runtime-registration failure job=%s error=%v",
						id,
						job.Id,
						completeErr,
					)
				}

				return
			}

			defer runtimeManager.Remove(job.Id)

			log.Printf(
				"worker=%s registered runtime job=%s pid=%d checkpointable=%t",
				id,
				job.Id,
				runtime.PID(),
				runtime.Checkpointable(),
			)

			execErr := cmd.Wait()

			// Execution has finished, so lease renewal is no
			// longer necessary.
			stopRenewal()

			success := execErr == nil
			errorMessage := ""

			if execErr != nil {
				errorMessage =
					execErr.Error() +
						": " +
						output.String()

				log.Printf(
					"worker=%s job=%s failed: %s",
					id,
					job.Id,
					errorMessage,
				)
			} else {
				log.Printf(
					"worker=%s job=%s completed successfully",
					id,
					job.Id,
				)
			}

			_, completeErr :=
				client.CompleteJob(
					ctx,
					&forgev1.CompleteJobRequest{
						WorkerId: id,
						JobId:    job.Id,
						Success:  success,
						Error:    errorMessage,
					},
				)

			if completeErr != nil {
				log.Printf(
					"worker=%s complete job=%s error=%v; retry backoff=%s",
					id,
					job.Id,
					completeErr,
					retry.Backoff(
						int(job.Attempts),
					),
				)

				return
			}

			log.Printf(
				"worker=%s reported completion job=%s",
				id,
				job.Id,
			)
		}(response.Job)
	}
}

func env(
	key string,
	defaultValue string,
) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}
