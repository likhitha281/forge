//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	stateMB := flag.Int(
		"state-mb",
		64,
		"mutable state size in MiB",
	)

	duration := flag.Duration(
		"duration",
		0,
		"maximum workload runtime; zero runs indefinitely",
	)

	checkpointPath := flag.String(
		"checkpoint",
		envOrDefault(
			"FORGE_CHECKPOINT_PATH",
			"/tmp/forge-checkpoint.bin",
		),
		"checkpoint file",
	)

	checkpointDone := flag.String(
		"checkpoint-done",
		envOrDefault(
			"FORGE_CHECKPOINT_DONE",
			"/tmp/checkpoint.done",
		),
		"checkpoint completion acknowledgement file",
	)

	restoreDone := flag.String(
		"restore-done",
		envOrDefault(
			"FORGE_RESTORE_DONE",
			"/tmp/restore.done",
		),
		"restore completion acknowledgement file",
	)

	flag.Parse()

	if *stateMB <= 0 {
		log.Fatal("state-mb must be positive")
	}

	if *duration < 0 {
		log.Fatal("duration cannot be negative")
	}

	var deadline time.Time

	if *duration > 0 {
		deadline = time.Now().Add(*duration)
	}

	state := make(
		[]byte,
		*stateMB*1024*1024,
	)

	// Give the state deterministic non-zero contents.
	for i := range state {
		state[i] = byte(i % 251)
	}

	var iteration atomic.Uint64

	var stateMu sync.Mutex

	signals := make(
		chan os.Signal,
		4,
	)

	signal.Notify(
		signals,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
		syscall.SIGTERM,
		syscall.SIGINT,
	)

	defer signal.Stop(signals)

	log.Printf(
		"checkpointable workload started pid=%d state=%dMiB checkpoint=%s duration=%s",
		os.Getpid(),
		*stateMB,
		*checkpointPath,
		*duration,
	)

	go func() {
		for sig := range signals {
			switch sig {
			case syscall.SIGUSR1:
				start := time.Now()

				stateMu.Lock()

				err := checkpoint(
					*checkpointPath,
					iteration.Load(),
					state,
				)

				stateMu.Unlock()

				if err != nil {
					log.Printf(
						"checkpoint failed: %v",
						err,
					)
					continue
				}

				if err := writeDoneFile(
					*checkpointDone,
				); err != nil {
					log.Printf(
						"checkpoint acknowledgement failed: %v",
						err,
					)
					continue
				}

				log.Printf(
					"checkpoint completed bytes=%d duration=%s",
					len(state)+8,
					time.Since(start),
				)

			case syscall.SIGUSR2:
				start := time.Now()

				stateMu.Lock()

				restoredIteration, err :=
					restore(
						*checkpointPath,
						state,
					)

				if err == nil {
					iteration.Store(
						restoredIteration,
					)
				}

				stateMu.Unlock()

				if err != nil {
					log.Printf(
						"restore failed: %v",
						err,
					)
					continue
				}

				if err := writeDoneFile(
					*restoreDone,
				); err != nil {
					log.Printf(
						"restore acknowledgement failed: %v",
						err,
					)
					continue
				}

				log.Printf(
					"restore completed bytes=%d duration=%s",
					len(state)+8,
					time.Since(start),
				)

			case syscall.SIGTERM,
				syscall.SIGINT:
				log.Printf(
					"terminating iteration=%d",
					iteration.Load(),
				)

				os.Exit(0)
			}
		}
	}()

	for {
		if !deadline.IsZero() &&
			time.Now().After(deadline) {

			log.Printf(
				"workload duration reached iteration=%d",
				iteration.Load(),
			)

			return
		}

		// Real CPU work rather than sleep.
		// Real CPU work rather than sleep.
		//
		// Hold stateMu for one logical compute iteration so checkpoint
		// and restore observe a consistent state/progress pair.
		stateMu.Lock()

		sum := sha256.Sum256(state)

		index :=
			int(
				iteration.Load() %
					uint64(len(state)),
			)

		state[index] ^= sum[0]

		current :=
			iteration.Add(1)

		stateMu.Unlock()

		if current%1000 == 0 {
			fmt.Printf(
				"iteration=%d\n",
				current,
			)
		}
	}
}

func checkpoint(
	path string,
	iteration uint64,
	state []byte,
) error {
	if err := os.MkdirAll(
		filepath.Dir(path),
		0o755,
	); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := binary.Write(
		file,
		binary.LittleEndian,
		iteration,
	); err != nil {
		return err
	}

	if _, err := file.Write(state); err != nil {
		return err
	}

	return file.Sync()
}

func restore(
	path string,
	state []byte,
) (uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var iteration uint64

	if err := binary.Read(
		file,
		binary.LittleEndian,
		&iteration,
	); err != nil {
		return 0, err
	}

	if _, err := io.ReadFull(
		file,
		state,
	); err != nil {
		return 0, err
	}

	return iteration, nil
}

func writeDoneFile(
	path string,
) error {
	tmp := path + ".tmp"

	if err := os.WriteFile(
		tmp,
		[]byte("done\n"),
		0o644,
	); err != nil {
		return err
	}

	return os.Rename(
		tmp,
		path,
	)
}

func envOrDefault(
	key string,
	fallback string,
) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
