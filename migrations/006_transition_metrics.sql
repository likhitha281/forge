ALTER TABLE transitions
    ADD COLUMN prepare_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN checkpoint_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN reconfigure_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN restore_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN resume_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN checkpoint_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN restore_bytes BIGINT NOT NULL DEFAULT 0;

ALTER TABLE transitions
    ADD CONSTRAINT transitions_prepare_ms_nonnegative
        CHECK (prepare_ms >= 0),
    ADD CONSTRAINT transitions_checkpoint_ms_nonnegative
        CHECK (checkpoint_ms >= 0),
    ADD CONSTRAINT transitions_reconfigure_ms_nonnegative
        CHECK (reconfigure_ms >= 0),
    ADD CONSTRAINT transitions_restore_ms_nonnegative
        CHECK (restore_ms >= 0),
    ADD CONSTRAINT transitions_resume_ms_nonnegative
        CHECK (resume_ms >= 0),
    ADD CONSTRAINT transitions_checkpoint_bytes_nonnegative
        CHECK (checkpoint_bytes >= 0),
    ADD CONSTRAINT transitions_restore_bytes_nonnegative
        CHECK (restore_bytes >= 0);