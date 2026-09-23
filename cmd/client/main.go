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

	gpus := fs.Int(
		"gpus",
		0,
		"requested GPU count",
	)

	priority := fs.Int("priority", 2, "job priority")
	maxAttempts := fs.Int("max-attempts", 3, "maximum execution attempts")
	idempotencyKey := fs.String(
		"idempotency-key",
		"",
		"optional idempotency key",
	)

	if len(args) == 0 {
		log.Fatal(`usage: forge submit "command" --priority 2`)
	}

	command := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		log.Fatal(err)
	}

	response, err := client.SubmitJob(
		context.Background(),
		&forgev1.SubmitJobRequest{
			Payload:        command,
			Priority:       int32(*priority),
			MaxAttempts:    int32(*maxAttempts),
			IdempotencyKey: *idempotencyKey,
			Resources: &forgev1.ResourceVector{
				CpuCores: *cpu,
				MemoryMb: *memoryMB,
				Gpus:     int32(*gpus),
			},
		},
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
	fmt.Printf("GPUs:     %d\n", *gpus)
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
	fmt.Printf("Attempts:     %d/%d\n", job.Attempts, job.MaxAttempts)

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

	if job.Resources != nil {
		fmt.Printf(
			"Resources:    CPU %.2f | RAM %d MB | GPU %d\n",
			job.Resources.CpuCores,
			job.Resources.MemoryMb,
			job.Resources.Gpus,
		)
	}
}

func jobsCommand(client forgev1.ForgeClient, args []string) {
	fs := flag.NewFlagSet("jobs", flag.ExitOnError)

	limit := fs.Int("limit", 20, "maximum jobs to return")

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
		"%-10s  %-11s %-8s %-10s %-14s %s\n",
		"JOB",
		"STATUS",
		"PRIORITY",
		"ATTEMPTS",
		"WORKER",
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

		fmt.Printf(
			"%-10s  %-11s %-8d %-10s %-14s %s\n",
			shortID(job.Id),
			job.Status.String(),
			job.Priority,
			fmt.Sprintf("%d/%d", job.Attempts, job.MaxAttempts),
			worker,
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
		"%-14s %-10s %-10s %-10s %s\n",
		"WORKER",
		"CAPACITY",
		"RUNNING",
		"STATUS",
		"HEARTBEAT",
	)

	for _, worker := range response.Workers {
		age, health := workerHealth(worker.LastHeartbeat)

		fmt.Printf(
			"%-14s %-10d %-10d %-10s %s\n",
			shortID(worker.Id),
			worker.Capacity,
			worker.Running,
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
		fmt.Printf("Job %s cancelled.\n", args[0])
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
Forge - Distributed Task Execution Engine

Usage:
  forge submit "COMMAND" [options]
  forge status JOB_ID
  forge jobs [--limit N]
  forge workers
  forge cancel JOB_ID

Examples:
  forge submit "sleep 10"
  forge submit "sleep 10" --priority 1
  forge submit "echo hello" --priority 2 --max-attempts 5

  forge status JOB_ID
  forge jobs
  forge jobs --limit 50
  forge workers
  forge cancel JOB_ID

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
		ageText = fmt.Sprintf("%ds ago", int(age.Seconds()))

	case age < time.Hour:
		ageText = fmt.Sprintf("%dm ago", int(age.Minutes()))

	default:
		ageText = fmt.Sprintf("%dh ago", int(age.Hours()))
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
