-- Malleable workload support.
--
-- Resource requirements describe what configurations a job permits.
-- Allocated resources describe the concrete configuration currently
-- assigned to a RUNNING job.

ALTER TABLE jobs
    ADD COLUMN gpu_min INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN gpu_preferred INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN gpu_max INTEGER NOT NULL DEFAULT 0;

ALTER TABLE jobs
    ADD COLUMN allocated_cpu_cores DOUBLE PRECISION,
    ADD COLUMN allocated_memory_mb BIGINT,
    ADD COLUMN allocated_gpu_count INTEGER;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_gpu_min_nonnegative
        CHECK (gpu_min >= 0),
    ADD CONSTRAINT jobs_gpu_preferred_nonnegative
        CHECK (gpu_preferred >= 0),
    ADD CONSTRAINT jobs_gpu_max_nonnegative
        CHECK (gpu_max >= 0),
    ADD CONSTRAINT jobs_gpu_envelope_order
        CHECK (
            gpu_min <= gpu_preferred
            AND gpu_preferred <= gpu_max
        );

ALTER TABLE jobs
    ADD CONSTRAINT jobs_allocated_cpu_nonnegative
        CHECK (
            allocated_cpu_cores IS NULL
            OR allocated_cpu_cores >= 0
        ),
    ADD CONSTRAINT jobs_allocated_memory_nonnegative
        CHECK (
            allocated_memory_mb IS NULL
            OR allocated_memory_mb >= 0
        ),
    ADD CONSTRAINT jobs_allocated_gpu_nonnegative
        CHECK (
            allocated_gpu_count IS NULL
            OR allocated_gpu_count >= 0
        );

UPDATE jobs
SET
    gpu_min = gpu_count,
    gpu_preferred = gpu_count,
    gpu_max = gpu_count;