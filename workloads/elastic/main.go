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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	stateMB := flag.Int(
		"state-mb",
		64,
		"checkpoint state size in MiB",
	)

	totalWork := flag.Uint64(
		"work",
		100000,
		"total work units",
	)

	allocationPath := flag.String(
		"allocation-file",
		envOrDefault(
			"FORGE_ALLOCATION_FILE",
			"/tmp/forge-allocation",
		),
		"path containing current logical GPU allocation",
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

	if *totalWork == 0 {
		log.Fatal("work must be positive")
	}

	state := make(
		[]byte,
		*stateMB*1024*1024,
	)

	// Give the checkpoint state deterministic, non-zero contents.
	for i := range state {
		state[i] = byte(i % 251)
	}

	var (
		completed     atomic.Uint64
		progressReset atomic.Bool
		stateMu       sync.Mutex
	)

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
		"elastic workload started pid=%d state=%dMiB work=%d allocation_file=%s",
		os.Getpid(),
		*stateMB,
		*totalWork,
		*allocationPath,
	)

	// Handle checkpoint, restore, and termination signals.
	go func() {
		for sig := range signals {
			switch sig {
			case syscall.SIGUSR1:
				stateMu.Lock()

				err := checkpoint(
					*checkpointPath,
					completed.Load(),
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
					"checkpoint completed work=%d bytes=%d",
					completed.Load(),
					len(state)+8,
				)

			case syscall.SIGUSR2:
				stateMu.Lock()

				restored, err :=
					restore(
						*checkpointPath,
						state,
					)

				if err == nil {
					completed.Store(restored)

					// Tell the progress reporter that work moved
					// backwards so its rate baseline must reset.
					progressReset.Store(true)
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
					"restore completed work=%d bytes=%d",
					restored,
					len(state)+8,
				)

			case syscall.SIGTERM,
				syscall.SIGINT:

				log.Printf(
					"terminating work=%d/%d",
					completed.Load(),
					*totalWork,
				)

				return
			}
		}
	}()

	started := time.Now()

	var lastReported uint64
	lastReportTime := started

	for {
		current :=
			completed.Load()

		if current >= *totalWork {
			break
		}

		parallelism, err :=
			readAllocation(
				*allocationPath,
			)

		if err != nil {
			log.Printf(
				"read allocation: %v",
				err,
			)

			// Keep the workload alive even if the control file
			// temporarily cannot be read.
			parallelism = 1
		}

		remaining :=
			*totalWork - current

		batch :=
			uint64(parallelism)

		if batch > remaining {
			batch = remaining
		}

		var workers sync.WaitGroup

		for i := uint64(0); i < batch; i++ {
			workers.Add(1)

			go func(offset uint64) {
				defer workers.Done()

				index :=
					int(
						(current + offset) %
							uint64(len(state)),
					)

				const chunkSize = 64 * 1024

				start :=
					(index / chunkSize) *
						chunkSize

				end :=
					start + chunkSize

				if end > len(state) {
					end = len(state)
				}

				// Copy shared state while holding the lock.
				// The expensive computation happens after releasing it.
				stateMu.Lock()

				chunk := make(
					[]byte,
					end-start,
				)

				copy(
					chunk,
					state[start:end],
				)

				stateMu.Unlock()

				sum :=
					sha256.Sum256(chunk)

				// Make each unit sufficiently CPU-heavy that
				// increased parallelism has a measurable effect.
				for round := 0; round < 64; round++ {
					sum =
						sha256.Sum256(
							sum[:],
						)
				}

				// Only the mutation itself needs synchronization.
				stateMu.Lock()

				state[index] ^=
					sum[0]

				stateMu.Unlock()
			}(i)
		}

		workers.Wait()

		// Progress advances under the same lock used by
		// checkpoint and restore. This gives checkpointing
		// a coherent state/progress boundary.
		stateMu.Lock()

		newCompleted :=
			completed.Add(batch)

		stateMu.Unlock()

		now := time.Now()

		// Restore can rewind the completed counter. Do not calculate
		// throughput against a pre-restore progress baseline.
		if progressReset.Swap(false) {
			lastReported =
				newCompleted

			lastReportTime =
				now

			fmt.Printf(
				"progress completed=%d total=%d allocation=%d rate=reset elapsed=%s\n",
				newCompleted,
				*totalWork,
				parallelism,
				now.Sub(started),
			)

			continue
		}

		// Defensive fallback in case progress moved backwards before
		// the reset flag was observed by this loop.
		if newCompleted < lastReported {
			lastReported =
				newCompleted

			lastReportTime =
				now

			continue
		}

		if newCompleted-lastReported >= 100 ||
			now.Sub(lastReportTime) >= time.Second {

			elapsed :=
				now.Sub(started)

			interval :=
				now.Sub(lastReportTime)

			delta :=
				newCompleted -
					lastReported

			rate :=
				float64(delta) /
					interval.Seconds()

			fmt.Printf(
				"progress completed=%d total=%d allocation=%d rate=%.2f elapsed=%s\n",
				newCompleted,
				*totalWork,
				parallelism,
				rate,
				elapsed,
			)

			lastReported =
				newCompleted

			lastReportTime =
				now
		}
	}

	elapsed :=
		time.Since(started)

	log.Printf(
		"elastic workload completed work=%d duration=%s",
		completed.Load(),
		elapsed,
	)
}

func readAllocation(
	path string,
) (int, error) {
	data, err :=
		os.ReadFile(path)

	if err != nil {
		return 0, err
	}

	value :=
		strings.TrimSpace(
			string(data),
		)

	allocation, err :=
		strconv.Atoi(value)

	if err != nil {
		return 0, err
	}

	if allocation <= 0 {
		return 0,
			fmt.Errorf(
				"allocation must be positive",
			)
	}

	return allocation, nil
}

func checkpoint(
	path string,
	completed uint64,
	state []byte,
) error {
	if err := os.MkdirAll(
		filepath.Dir(path),
		0o755,
	); err != nil {
		return err
	}

	file, err :=
		os.Create(path)

	if err != nil {
		return err
	}

	defer file.Close()

	if err := binary.Write(
		file,
		binary.LittleEndian,
		completed,
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
	file, err :=
		os.Open(path)

	if err != nil {
		return 0, err
	}

	defer file.Close()

	var completed uint64

	if err := binary.Read(
		file,
		binary.LittleEndian,
		&completed,
	); err != nil {
		return 0, err
	}

	if _, err := io.ReadFull(
		file,
		state,
	); err != nil {
		return 0, err
	}

	return completed, nil
}

func writeDoneFile(
	path string,
) error {
	if err := os.MkdirAll(
		filepath.Dir(path),
		0o755,
	); err != nil {
		return err
	}

	tmp :=
		path + ".tmp"

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
	if value :=
		os.Getenv(key); value != "" {

		return value
	}

	return fallback
}
