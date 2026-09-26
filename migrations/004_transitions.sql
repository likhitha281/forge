CREATE TABLE transitions (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,

    state TEXT NOT NULL,

    source_cpu_cores DOUBLE PRECISION NOT NULL,
    source_memory_mb BIGINT NOT NULL,
    source_gpu_count INTEGER NOT NULL,

    target_cpu_cores DOUBLE PRECISION NOT NULL,
    target_memory_mb BIGINT NOT NULL,
    target_gpu_count INTEGER NOT NULL,

    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    failure_reason TEXT,
    bytes_moved BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT transitions_state_valid CHECK (
        state IN (
            'REQUESTED',
            'PREPARING',
            'CHECKPOINTING',
            'RECONFIGURING',
            'RESTORING',
            'RESUMING',
            'COMPLETED',
            'FAILED'
        )
    ),

    CONSTRAINT transitions_source_cpu_nonnegative
        CHECK (source_cpu_cores >= 0),

    CONSTRAINT transitions_source_memory_nonnegative
        CHECK (source_memory_mb >= 0),

    CONSTRAINT transitions_source_gpu_nonnegative
        CHECK (source_gpu_count >= 0),

    CONSTRAINT transitions_target_cpu_nonnegative
        CHECK (target_cpu_cores >= 0),

    CONSTRAINT transitions_target_memory_nonnegative
        CHECK (target_memory_mb >= 0),

    CONSTRAINT transitions_target_gpu_nonnegative
        CHECK (target_gpu_count >= 0),

    CONSTRAINT transitions_bytes_moved_nonnegative
        CHECK (bytes_moved >= 0)
);

CREATE INDEX transitions_job_idx
    ON transitions(job_id, requested_at DESC);

CREATE INDEX transitions_state_idx
    ON transitions(state);