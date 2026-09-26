ALTER TABLE transitions
    ADD COLUMN worker_id TEXT REFERENCES workers(id),

    ADD COLUMN reserved_cpu_cores DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN reserved_memory_mb BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN reserved_gpu_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE transitions
    ADD CONSTRAINT transitions_reserved_cpu_nonnegative
        CHECK (reserved_cpu_cores >= 0),

    ADD CONSTRAINT transitions_reserved_memory_nonnegative
        CHECK (reserved_memory_mb >= 0),

    ADD CONSTRAINT transitions_reserved_gpu_nonnegative
        CHECK (reserved_gpu_count >= 0);

CREATE INDEX transitions_active_worker_idx
    ON transitions(worker_id, state)
    WHERE state NOT IN ('COMPLETED', 'FAILED');

CREATE UNIQUE INDEX transitions_one_active_per_job
    ON transitions(job_id)
    WHERE state NOT IN ('COMPLETED', 'FAILED');