-- name: GetSyncState :one
SELECT COALESCE(max(seq), 0)::bigint AS seq
FROM sui_time.sync_changelog
WHERE user_id = sqlc.arg(user_id)::uuid;

-- name: GetSyncEntity :one
SELECT entity, entity_id, updated_at, device_id, deleted_at, data
FROM sui_time.sync_entities
WHERE user_id = sqlc.arg(user_id)::uuid
  AND entity = sqlc.arg(entity)
  AND entity_id = sqlc.arg(entity_id);

-- name: ListSyncChanges :many
SELECT c.seq, e.entity, e.entity_id, e.updated_at, e.deleted_at, e.device_id, e.data
FROM sui_time.sync_changelog AS c
JOIN sui_time.sync_entities AS e
  ON e.user_id = c.user_id AND e.entity = c.entity AND e.entity_id = c.entity_id
WHERE c.user_id = sqlc.arg(user_id)::uuid AND c.seq > sqlc.arg(after_seq)
ORDER BY c.seq
LIMIT sqlc.arg(page_size);

-- name: UpsertSyncEntity :exec
INSERT INTO sui_time.sync_entities(user_id, entity, entity_id, updated_at, deleted_at, device_id, data)
VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(entity), sqlc.arg(entity_id), sqlc.arg(updated_at), sqlc.narg(deleted_at), sqlc.arg(device_id), sqlc.arg(data))
ON CONFLICT (user_id, entity, entity_id) DO UPDATE
SET updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at,
    device_id = EXCLUDED.device_id, data = EXCLUDED.data;

-- name: AppendSyncChange :one
INSERT INTO sui_time.sync_changelog(user_id, entity, entity_id, created_at)
VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(entity), sqlc.arg(entity_id), sqlc.arg(created_at))
RETURNING seq;
