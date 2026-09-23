-- Forge resource model.
--
-- Existing worker.capacity remains the maximum number of concurrently
-- executing jobs. The new columns represent actual compute resources.

ALTER TABLE jobs
    ADD COLUMN cpu_cores DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN memory_mb BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN gpu_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE workers
    ADD COLUMN cpu_capacity DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN memory_capacity_mb BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN gpu_capacity INTEGER NOT NULL DEFAULT 0;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_cpu_cores_nonnegative
        CHECK (cpu_cores >= 0),
    ADD CONSTRAINT jobs_memory_mb_nonnegative
        CHECK (memory_mb >= 0),
    ADD CONSTRAINT jobs_gpu_count_nonnegative
        CHECK (gpu_count >= 0);

ALTER TABLE workers
    ADD CONSTRAINT workers_cpu_capacity_nonnegative
        CHECK (cpu_capacity >= 0),
    ADD CONSTRAINT workers_memory_capacity_nonnegative
        CHECK (memory_capacity_mb >= 0),
    ADD CONSTRAINT workers_gpu_capacity_nonnegative
        CHECK (gpu_capacity >= 0);