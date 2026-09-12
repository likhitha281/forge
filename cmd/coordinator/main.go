package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	forgev1 "github.com/likhitha281/forge/gen/forge/v1"
	"github.com/likhitha281/forge/internal/coordinator"
	"github.com/likhitha281/forge/internal/metrics"
	"github.com/likhitha281/forge/internal/storage"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

const (
	maxDatabaseAttempts = 10
	initialBackoff      = 1 * time.Second
	maxBackoff          = 10 * time.Second
)

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://forge:forge@localhost:5432/forge?sslmode=disable"
	}

	store := connectToDatabase(ctx, databaseURL)
	defer store.DB.Close()

	metrics.Register()

	// Periodically recover jobs whose worker lease expired.
	go requeueExpiredJobs(ctx, store)

	// Expose Prometheus metrics.
	go startMetricsServer()

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen on :50051: %v", err)
	}

	grpcServer := grpc.NewServer()

	forgev1.RegisterForgeServer(
		grpcServer,
		coordinator.New(store),
	)

	log.Println("Forge coordinator ready")
	log.Println("gRPC:    :50051")
	log.Println("Metrics: :9090")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}

func connectToDatabase(
	ctx context.Context,
	databaseURL string,
) *storage.Store {
	backoff := initialBackoff

	for attempt := 1; attempt <= maxDatabaseAttempts; attempt++ {
		log.Printf(
			"connecting to PostgreSQL attempt=%d/%d",
			attempt,
			maxDatabaseAttempts,
		)

		store, err := storage.New(ctx, databaseURL)
		if err == nil {
			log.Printf(
				"connected to PostgreSQL after %d attempt(s)",
				attempt,
			)

			return store
		}

		log.Printf(
			"PostgreSQL unavailable attempt=%d/%d error=%v",
			attempt,
			maxDatabaseAttempts,
			err,
		)

		if attempt == maxDatabaseAttempts {
			log.Fatalf(
				"unable to connect to PostgreSQL after %d attempts",
				maxDatabaseAttempts,
			)
		}

		log.Printf(
			"retrying PostgreSQL connection in %s",
			backoff,
		)

		select {
		case <-ctx.Done():
			log.Fatalf(
				"database connection cancelled: %v",
				ctx.Err(),
			)

		case <-time.After(backoff):
		}

		backoff *= 2

		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// The loop always returns or terminates the process.
	panic("unreachable")
}

func requeueExpiredJobs(
	ctx context.Context,
	store *storage.Store,
) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := store.Requeue(ctx); err != nil {
				log.Printf(
					"failed to requeue expired jobs: %v",
					err,
				)
			}
		}
	}
}

func startMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":9090",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("starting Prometheus metrics server on :9090")

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Printf(
			"metrics server failed: %v",
			err,
		)
	}
}
