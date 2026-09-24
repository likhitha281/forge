package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	addr := os.Getenv("FORGE_ADDR")
	if addr == "" {
		addr = "localhost:50051"
	}

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	client := forgev1.NewForgeClient(conn)

	switch os.Args[1] {
	case "submit":
		submitCommand(client, os.Args[2:])

	case "status":
		statusCommand(client, os.Args[2:])

	case "jobs":
		jobsCommand(client, os.Args[2:])

	case "workers":
		workersCommand(client)

	case "cancel":
		cancelCommand(client, os.Args[2:])

	case "help", "--help", "-h":
		printUsage()

	default:
		fmt.Printf("unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func submitCommand(client forgev1.ForgeClient, args []string) {
	fs := flag.NewFlagSet("submit", flag.ExitOnError)

	cpu := fs.Float64(
		"cpu",
		0,
		"requested CPU cores",
	)

	memoryMB := fs.Int64(
		"memory-mb",
		0,
		"requested memory in MB",
	)

	// Legacy/fixed GPU allocation.
	gpus := fs.Int(
		"gpus",
		0,
		"requested fixed GPU count",
	)

	// Malleable GPU allocation envelope.
	gpuMin := fs.Int(
		"gpu-min",
		-1,
		"minimum GPU count for a malleable job",
	)

	gpuPreferred := fs.Int(
		"gpu-preferred",
		-1,
		"preferred GPU count for a malleable job",
	)

	gpuMax := fs.Int(
		"gpu-max",
		-1,
		"maximum GPU count for a malleable job",
	)

	priority := fs.Int(
		"priority",
		2,
		"job priority",
	)

	maxAttempts := fs.Int(
		"max-attempts",
		3,
		"maximum execution attempts",
	)

	idempotencyKey := fs.String(
		"idempotency-key",
		"",
		"optional idempotency key",
	)

	if len(args) == 0 {
		log.Fatal(`usage: forge submit "command" [options]`)
	}

	command := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		log.Fatal(err)
	}

	if *cpu < 0 {
		log.Fatal("--cpu cannot be negative")
	}

	if *memoryMB < 0 {
		log.Fatal("--memory-mb cannot be negative")
	}

	if *gpus < 0 {
		log.Fatal("--gpus cannot be negative")
	}

	malleableSpecified :=
		*gpuMin >= 0 ||
			*gpuPreferred >= 0 ||
			*gpuMax >= 0

	if malleableSpecified {
		if *gpuMin < 0 ||
			*gpuPreferred < 0 ||
			*gpuMax < 0 {
			log.Fatal(
				"--gpu-min, --gpu-preferred, and --gpu-max must be specified together",
			)
		}

		if *gpuMin > *gpuPreferred {
			log.Fatal(
				"--gpu-min cannot exceed --gpu-preferred",
			)
		}

		if *gpuPreferred > *gpuMax {
			log.Fatal(
				"--gpu-preferred cannot exceed --gpu-max",
			)
		}
	}

	request := &forgev1.SubmitJobRequest{
		Payload:        command,
		Priority:       int32(*priority),
		MaxAttempts:    int32(*maxAttempts),
		IdempotencyKey: *idempotencyKey,
	}

	if malleableSpecified {
		// New malleable-resource API.
		request.Requirements = &forgev1.JobResources{
			CpuCores: *cpu,
			MemoryMb: *memoryMB,
			Gpu: &forgev1.GPUEnvelope{
				Min:       int32(*gpuMin),
				Preferred: int32(*gpuPreferred),
				Max:       int32(*gpuMax),
			},
		}
	} else {
		// Backwards-compatible fixed-resource API.
		request.Resources = &forgev1.ResourceVector{
			CpuCores: *cpu,
			MemoryMb: *memoryMB,
			Gpus:     int32(*gpus),
		}
	}

	response, err := client.SubmitJob(
		context.Background(),
		request,
	)
	if err != nil {
		log.Fatalf("submit job: %v", err)
	}

	fmt.Println("Job submitted successfully")
	fmt.Printf("ID:       %s\n", response.JobId)
	fmt.Printf("Command:  %s\n", command)
	fmt.Printf("Priority: %d\n", *priority)
	fmt.Printf("CPU:      %.2f cores\n", *cpu)
	fmt.Printf("Memory:   %d MB\n", *memoryMB)

	if malleableSpecified {
		fmt.Printf(
			"GPUs:     min=%d preferred=%d max=%d\n",
			*gpuMin,
			*gpuPreferred,
			*gpuMax,
		)
	} else {
		fmt.Printf("GPUs:     %d (fixed)\n", *gpus)
	}
}

func statusCommand(client forgev1.ForgeClient, args []string) {
	if len(args) != 1 {
		log.Fatal("usage: forge status JOB_ID")
	}

	job, err := client.GetJob(
		context.Background(),
		&forgev1.GetJobRequest{
			JobId: args[0],
		},
	)
	if err != nil {
		log.Fatalf("get job: %v", err)
	}

	fmt.Printf("ID:           %s\n", job.Id)
	fmt.Printf("Command:      %s\n", job.Payload)
	fmt.Printf("Priority:     %d\n", job.Priority)
	fmt.Printf("Status:       %s\n", job.Status.String())
	fmt.Printf(
		"Attempts:     %d/%d\n",
		job.Attempts,
		job.MaxAttempts,
	)

	if job.WorkerId != "" {
		fmt.Printf("Worker:       %s\n", job.WorkerId)
	}

	if job.CreatedAt != "" {
		fmt.Printf("Created:      %s\n", job.CreatedAt)
	}

	if job.UpdatedAt != "" {
		fmt.Printf("Updated:      %s\n", job.UpdatedAt)
	}

	if job.Error != "" {
		fmt.Printf("Error:        %s\n", job.Error)
	}

	if job.Requirements != nil {
		fmt.Printf(
			"Requirements: CPU %.2f | RAM %d MB\n",
			job.Requirements.CpuCores,
			job.Requirements.MemoryMb,
		)

		if job.Requirements.Gpu != nil {
			fmt.Printf(
				"GPU envelope: min=%d preferred=%d max=%d\n",
				job.Requirements.Gpu.Min,
				job.Requirements.Gpu.Preferred,
				job.Requirements.Gpu.Max,
			)
		}
	} else if job.Resources != nil {
		// Compatibility with older coordinator responses.
		fmt.Printf(
			"Resources:    CPU %.2f | RAM %d MB | GPU %d\n",
			job.Resources.CpuCores,
			job.Resources.MemoryMb,
			job.Resources.Gpus,
		)
	}

	if job.Allocation != nil {
		fmt.Printf(
			"Allocation:   CPU %.2f | RAM %d MB | GPU %d\n",
			job.Allocation.CpuCores,
			job.Allocation.MemoryMb,
			job.Allocation.Gpus,
		)
	} else {
		fmt.Println("Allocation:   none")
	}
}

func jobsCommand(client forgev1.ForgeClient, args []string) {
	fs := flag.NewFlagSet("jobs", flag.ExitOnError)

	limit := fs.Int(
		"limit",
		20,
		"maximum jobs to return",
	)

	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	response, err := client.ListJobs(
		context.Background(),
		&forgev1.ListJobsRequest{
			Limit: int32(*limit),
		},
	)
	if err != nil {
		log.Fatalf("list jobs: %v", err)
	}

	if len(response.Jobs) == 0 {
		fmt.Println("No jobs found.")
		return
	}

	fmt.Printf(
		"%-12s %-11s %-8s %-10s %-14s %-10s %s\n",
		"JOB",
		"STATUS",
		"PRIORITY",
		"ATTEMPTS",
		"WORKER",
		"GPU",
		"COMMAND",
	)

	for _, job := range response.Jobs {
		worker := shortID(job.WorkerId)
		if worker == "" {
			worker = "-"
		}

		command := job.Payload
		if len(command) > 35 {
			command = command[:32] + "..."
		}

		gpuText := "-"

		if job.Allocation != nil {
			gpuText = fmt.Sprintf(
				"%d",
				job.Allocation.Gpus,
			)
		} else if job.Requirements != nil &&
			job.Requirements.Gpu != nil {
			gpuText = fmt.Sprintf(
				"%d/%d/%d",
				job.Requirements.Gpu.Min,
				job.Requirements.Gpu.Preferred,
				job.Requirements.Gpu.Max,
			)
		}

		fmt.Printf(
			"%-12s %-11s %-8d %-10s %-14s %-10s %s\n",
			shortID(job.Id),
			job.Status.String(),
			job.Priority,
			fmt.Sprintf(
				"%d/%d",
				job.Attempts,
				job.MaxAttempts,
			),
			worker,
			gpuText,
			command,
		)
	}
}

func workersCommand(client forgev1.ForgeClient) {
	response, err := client.ListWorkers(
		context.Background(),
		&forgev1.ListWorkersRequest{},
	)
	if err != nil {
		log.Fatalf("list workers: %v", err)
	}

	if len(response.Workers) == 0 {
		fmt.Println("No workers registered.")
		return
	}

	fmt.Printf(
		"%-14s %-10s %-10s %-8s %-12s %-8s %-10s %s\n",
		"WORKER",
		"CAPACITY",
		"RUNNING",
		"CPU",
		"MEMORY_MB",
		"GPU",
		"STATUS",
		"HEARTBEAT",
	)

	for _, worker := range response.Workers {
		age, health := workerHealth(
			worker.LastHeartbeat,
		)

		cpu := "-"
		memory := "-"
		gpu := "-"

		if worker.Resources != nil {
			cpu = fmt.Sprintf(
				"%.2f",
				worker.Resources.CpuCores,
			)

			memory = fmt.Sprintf(
				"%d",
				worker.Resources.MemoryMb,
			)

			gpu = fmt.Sprintf(
				"%d",
				worker.Resources.Gpus,
			)
		}

		fmt.Printf(
			"%-14s %-10d %-10d %-8s %-12s %-8s %-10s %s\n",
			shortID(worker.Id),
			worker.Capacity,
			worker.Running,
			cpu,
			memory,
			gpu,
			health,
			age,
		)
	}
}

func cancelCommand(client forgev1.ForgeClient, args []string) {
	if len(args) != 1 {
		log.Fatal("usage: forge cancel JOB_ID")
	}

	response, err := client.CancelJob(
		context.Background(),
		&forgev1.CancelJobRequest{
			JobId: args[0],
		},
	)
	if err != nil {
		log.Fatalf("cancel job: %v", err)
	}

	if response.Cancelled {
		fmt.Printf(
			"Job %s cancelled.\n",
			args[0],
		)
	}
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}

	return id[:12]
}

func printUsage() {
	fmt.Println(strings.TrimSpace(`
Forge - Transition-Aware Distributed Execution Engine

Usage:
  forge submit "COMMAND" [options]
  forge status JOB_ID
  forge jobs [--limit N]
  forge workers
  forge cancel JOB_ID

Fixed-resource job:
  forge submit "sleep 30" \
    --cpu 2 \
    --memory-mb 2048 \
    --gpus 1

Malleable GPU job:
  forge submit "sleep 30" \
    --cpu 4 \
    --memory-mb 8192 \
    --gpu-min 1 \
    --gpu-preferred 4 \
    --gpu-max 8

Submit options:
  --cpu N              Requested CPU cores
  --memory-mb N        Requested memory in MB
  --gpus N             Fixed GPU count

  --gpu-min N          Minimum GPU allocation
  --gpu-preferred N    Preferred GPU allocation
  --gpu-max N          Maximum GPU allocation

  --priority N         Job priority (default 2)
  --max-attempts N     Maximum execution attempts (default 3)
  --idempotency-key K  Optional idempotency key

Notes:
  --gpu-min, --gpu-preferred, and --gpu-max must be
  specified together.

  Use --gpus for fixed jobs OR the GPU envelope flags
  for malleable jobs.

Environment:
  FORGE_ADDR    Coordinator address (default localhost:50051)
`))
}

func workerHealth(value string) (string, string) {
	if value == "" {
		return "-", "UNKNOWN"
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value, "UNKNOWN"
	}

	age := time.Since(t)

	if age < 0 {
		age = 0
	}

	var ageText string

	switch {
	case age < time.Minute:
		ageText = fmt.Sprintf(
			"%ds ago",
			int(age.Seconds()),
		)

	case age < time.Hour:
		ageText = fmt.Sprintf(
			"%dm ago",
			int(age.Minutes()),
		)

	default:
		ageText = fmt.Sprintf(
			"%dh ago",
			int(age.Hours()),
		)
	}

	var health string

	switch {
	case age <= 15*time.Second:
		health = "ACTIVE"

	case age <= 60*time.Second:
		health = "STALE"

	default:
		health = "DEAD"
	}

	return ageText, health
}
