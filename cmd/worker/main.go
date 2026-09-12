package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync/atomic"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/retry"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	addr := env("FORGE_ADDR", "localhost:50051")

	// Use WORKER_ID when explicitly configured.
	// Otherwise use the hostname so Docker-scaled workers
	// automatically receive unique identities.
	id := os.Getenv("WORKER_ID")
	if id == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("failed to determine worker hostname: %v", err)
		}
		id = hostname
	}

	capN, err := strconv.Atoi(env("WORKER_CAPACITY", "4"))
	if err != nil {
		log.Fatalf("invalid WORKER_CAPACITY: %v", err)
	}

	log.Printf(
		"starting Forge worker id=%s capacity=%d coordinator=%s",
		id,
		capN,
		addr,
	)

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
		},
	); err != nil {
		log.Fatal(err)
	}

	log.Printf("worker %s registered successfully", id)

	var running atomic.Int32

	// Send periodic worker heartbeats.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
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
	sem := make(chan struct{}, capN)

	for {
		response, err := client.LeaseJob(
			ctx,
			&forgev1.LeaseJobRequest{
				WorkerId: id,
			},
		)

		if err != nil {
			if status.Code(err) == codes.NotFound {
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
			jobCtx, cancelJob := context.WithCancel(ctx)
			defer cancelJob()

			// Signal used to stop the lease-renewal goroutine
			// immediately after execution finishes.
			renewDone := make(chan struct{})

			// Renew the lease every 10 seconds while the job runs.
			//
			// The coordinator currently gives jobs a 30-second
			// lease, so renewing every 10 seconds gives us enough
			// safety margin for temporary delays.
			go func() {
				ticker := time.NewTicker(10 * time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-renewDone:
						return

					case <-jobCtx.Done():
						return

					case <-ticker.C:
						renewResponse, err := client.RenewLease(
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

			// Execute the job inside the worker's Linux container.
			cmd := exec.CommandContext(
				jobCtx,
				"/bin/sh",
				"-c",
				job.Payload,
			)

			output, execErr := cmd.CombinedOutput()

			// Execution has finished, so lease renewal is no longer
			// necessary.
			close(renewDone)

			success := execErr == nil
			errorMessage := ""

			if execErr != nil {
				errorMessage = execErr.Error() + ": " + string(output)

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

			_, completeErr := client.CompleteJob(
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
					retry.Backoff(int(job.Attempts)),
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

func env(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}
