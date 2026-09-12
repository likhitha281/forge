# Forge

**A distributed task execution engine built in Go with lease-based failure recovery.**

Forge is a distributed job execution system that schedules shell workloads across concurrent worker nodes using gRPC, Protocol Buffers, PostgreSQL, and Docker.

It was built to explore the engineering problems behind distributed task systems: scheduling, worker coordination, leases, retries, failure detection, concurrency, observability, and at-least-once execution.

---

## Features

- Distributed coordinator/worker architecture
- gRPC + Protocol Buffers communication
- PostgreSQL-backed persistent job queue
- Priority-based scheduling
- Concurrent worker execution
- Dynamic worker registration
- Worker heartbeats and health classification
- Lease-based job ownership
- Automatic lease renewal for long-running jobs
- Recovery and reassignment after worker failure
- Retry limits
- Idempotency keys
- Queued-job cancellation
- Prometheus metrics
- Grafana monitoring
- Docker Compose development environment
- Command-line client
- Race-detector and unit testing
- GitHub Actions CI

---

## Architecture

```text
                         Forge CLI
                            |
                            | gRPC
                            v
                    +---------------+
                    |  Coordinator  |
                    |      Go       |
                    +-------+-------+
                            |
                         PostgreSQL
                            |
                    Persistent Queue
                            |
             +--------------+--------------+
             |              |              |
             v              v              v
         +--------+     +--------+     +--------+
         |Worker 1|     |Worker 2|     |Worker 3|
         +--------+     +--------+     +--------+
             |              |              |
             +--------------+--------------+
                            |
                         Heartbeats
                            |
                      Lease Renewal

                    +---------------+
                    |  Prometheus   |
                    +-------+-------+
                            |
                    +-------v-------+
                    |    Grafana    |
                    +---------------+