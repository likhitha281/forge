ALTER TABLE transitions
    ADD COLUMN prepare_us BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN checkpoint_us BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN reconfigure_us BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN restore_us BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN resume_us BIGINT NOT NULL DEFAULT 0;

ALTER TABLE transitions
    ADD CONSTRAINT transitions_prepare_us_nonnegative
        CHECK (prepare_us >= 0),
    ADD CONSTRAINT transitions_checkpoint_us_nonnegative
        CHECK (checkpoint_us >= 0),
    ADD CONSTRAINT transitions_reconfigure_us_nonnegative
        CHECK (reconfigure_us >= 0),
    ADD CONSTRAINT transitions_restore_us_nonnegative
        CHECK (restore_us >= 0),
    ADD CONSTRAINT transitions_resume_us_nonnegative
        CHECK (resume_us >= 0);