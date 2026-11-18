-- +goose Up
CREATE TABLE sui_time.sync_init_sessions (
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    session_id text NOT NULL,
    device_id text NOT NULL,
    batch_count integer NOT NULL CHECK (batch_count BETWEEN 0 AND 1000),
    status text NOT NULL CHECK (status IN ('active', 'completed', 'cancelled')),
    created_at bigint NOT NULL,
    expires_at bigint NOT NULL,
    PRIMARY KEY (user_id, session_id)
);
CREATE UNIQUE INDEX sync_init_one_active_per_user ON sui_time.sync_init_sessions (user_id) WHERE status = 'active';

CREATE TABLE sui_time.sync_init_batches (
    user_id uuid NOT NULL,
    session_id text NOT NULL,
    batch_index integer NOT NULL CHECK (batch_index >= 0),
    digest bytea NOT NULL,
    raw_changes bytea,
    normalized jsonb,
    PRIMARY KEY (user_id, session_id, batch_index),
    FOREIGN KEY (user_id, session_id) REFERENCES sui_time.sync_init_sessions(user_id, session_id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE sui_time.sync_init_batches;
DROP TABLE sui_time.sync_init_sessions;
