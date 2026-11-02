-- +goose Up
CREATE SCHEMA identity;
CREATE SCHEMA sui_time;

CREATE TABLE identity.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL,
    password_hash text NOT NULL,
    created_at bigint NOT NULL,
    updated_at bigint NOT NULL
);
CREATE UNIQUE INDEX identity_users_username_lower_key ON identity.users (lower(username));

CREATE TABLE identity.sessions (
    token_hash text PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    device_id text NOT NULL,
    created_at bigint NOT NULL,
    expires_at bigint NOT NULL,
    revoked_at bigint,
    last_used_at bigint NOT NULL
);
CREATE INDEX sessions_user_id_idx ON identity.sessions (user_id);

CREATE TABLE identity.devices (
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    device_id text NOT NULL,
    name text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT '',
    last_seen_at bigint NOT NULL,
    PRIMARY KEY (user_id, device_id)
);

CREATE TABLE sui_time.sync_entities (
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    entity text NOT NULL CHECK (entity IN ('category', 'task', 'anniversary', 'anniversary_record', 'setting')),
    entity_id text NOT NULL,
    updated_at bigint NOT NULL,
    deleted_at bigint,
    device_id text NOT NULL,
    data jsonb NOT NULL,
    PRIMARY KEY (user_id, entity, entity_id)
);
CREATE INDEX sync_entities_live_idx ON sui_time.sync_entities (user_id, entity) WHERE deleted_at IS NULL;

CREATE TABLE sui_time.sync_changelog (
    seq bigserial PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    entity text NOT NULL,
    entity_id text NOT NULL,
    created_at bigint NOT NULL
);
CREATE INDEX sync_changelog_user_seq_idx ON sui_time.sync_changelog (user_id, seq);

-- +goose Down
DROP SCHEMA sui_time CASCADE;
DROP SCHEMA identity CASCADE;
