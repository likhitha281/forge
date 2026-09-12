CREATE TABLE IF NOT EXISTS jobs (
 id UUID PRIMARY KEY,
 payload TEXT NOT NULL,
 priority INT NOT NULL DEFAULT 2,
 status TEXT NOT NULL DEFAULT 'QUEUED',
 attempts INT NOT NULL DEFAULT 0,
 max_attempts INT NOT NULL DEFAULT 3,
 idempotency_key TEXT UNIQUE,
 worker_id TEXT,
 lease_until TIMESTAMPTZ,
 error TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS jobs_sched_idx ON jobs(status, priority, created_at);
CREATE TABLE IF NOT EXISTS workers (
 id TEXT PRIMARY KEY,
 capacity INT NOT NULL,
 running INT NOT NULL DEFAULT 0,
 last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now()
);
