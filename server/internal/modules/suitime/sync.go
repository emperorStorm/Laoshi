package suitime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	db "github.com/emperorStorm/Laoshi/server/db/sqlc"
	"github.com/jackc/pgx/v5"
)

type pushRequest struct {
	DeviceID     string   `json:"deviceId"`
	RequireEmpty bool     `json:"requireEmpty"`
	Changes      []change `json:"changes"`
}
type pushResponse struct {
	Applied    []map[string]string `json:"applied"`
	Superseded []map[string]any    `json:"superseded"`
	Seq        int64               `json:"seq"`
	ServerTime int64               `json:"serverTime"`
}

func (s *Service) syncState(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userKey).(string)
	counts := map[string]int64{"category": 0, "task": 0, "anniversary": 0, "anniversary_record": 0, "setting": 0}
	rows, err := s.pool.Query(r.Context(), `SELECT entity,count(*) FROM sui_time.sync_entities WHERE user_id=$1 AND deleted_at IS NULL GROUP BY entity`, userID)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var entity string
		var count int64
		if err := rows.Scan(&entity, &count); err != nil {
			s.internal(w, err)
			return
		}
		counts[entity] = count
	}
	if err := rows.Err(); err != nil {
		s.internal(w, err)
		return
	}
	// Tombstones make a remote account nonempty too: upload must not resurrect
	// data from a previously used account after its last live row was deleted.
	var exists bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sui_time.sync_entities WHERE user_id=$1)`, userID).Scan(&exists); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remoteEmpty": !exists, "counts": counts, "serverTime": time.Now().UnixMilli()})
}

func (s *Service) push(w http.ResponseWriter, r *http.Request) {
	var input pushRequest
	if !decode(w, r, &input) {
		return
	}
	if !validDevice(input.DeviceID) || input.Changes == nil || len(input.Changes) > 200 {
		fail(w, 422, "validation_failed", "同步请求无效")
		return
	}
	now := time.Now().UnixMilli()
	normalized := make([]syncChange, len(input.Changes))
	skew := s.config.ClockSkew
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	for i, item := range input.Changes {
		if item.UpdatedAt > now+skew.Milliseconds() {
			writeError(w, apiError{Status: 422, Code: "clock_skew", Message: "请校准系统时间后重试", Details: map[string]any{"serverTime": now}})
			return
		}
		value, err := normalizeChange(item, input.DeviceID, now, s.config.PhotoLimit)
		if err != nil {
			fail(w, 422, "validation_failed", err.Error())
			return
		}
		normalized[i] = value
	}
	ctx := r.Context()
	userID := ctx.Value(userKey).(string)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err = lockUser(ctx, tx, userID, false); err != nil {
		s.internal(w, err)
		return
	}
	q := db.New(tx)
	if input.RequireEmpty {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sui_time.sync_entities WHERE user_id=$1)`, userID).Scan(&exists); err != nil {
			s.internal(w, err)
			return
		}
		if exists {
			fail(w, 409, "remote_not_empty", "云端已有数据，请选择合并或以云端为准")
			return
		}
	}
	response := pushResponse{Applied: []map[string]string{}, Superseded: []map[string]any{}, ServerTime: now}
	for _, item := range normalized {
		old, readErr := q.GetSyncEntity(ctx, db.GetSyncEntityParams{UserID: userID, Entity: item.Entity, EntityID: item.ID})
		if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
			s.internal(w, readErr)
			return
		}
		if readErr == nil && (item.UpdatedAt < old.UpdatedAt || item.UpdatedAt == old.UpdatedAt && item.DeviceID <= old.DeviceID) {
			stored, err := storedChange(old.Entity, old.EntityID, old.UpdatedAt, old.DeletedAt, old.DeviceID, old.Data)
			if err != nil {
				s.internal(w, err)
				return
			}
			response.Superseded = append(response.Superseded, map[string]any{"entity": item.Entity, "id": item.ID, "server": stored})
			continue
		}
		raw, err := json.Marshal(item.Data)
		if err != nil {
			s.internal(w, err)
			return
		}
		if err = q.UpsertSyncEntity(ctx, db.UpsertSyncEntityParams{UserID: userID, Entity: item.Entity, EntityID: item.ID, UpdatedAt: item.UpdatedAt, DeletedAt: item.DeletedAt, DeviceID: item.DeviceID, Data: raw}); err != nil {
			s.internal(w, err)
			return
		}
		if _, err = q.AppendSyncChange(ctx, db.AppendSyncChangeParams{UserID: userID, Entity: item.Entity, EntityID: item.ID, CreatedAt: now}); err != nil {
			s.internal(w, err)
			return
		}
		response.Applied = append(response.Applied, map[string]string{"entity": item.Entity, "id": item.ID})
	}
	response.Seq, err = q.GetSyncState(ctx, userID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, response)
}

func (s *Service) pull(w http.ResponseWriter, r *http.Request) {
	cursor, err := readCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		fail(w, 422, "validation_failed", "cursor 必须是非负整数")
		return
	}
	limit, err := readLimit(r.URL.Query().Get("limit"))
	if err != nil {
		fail(w, 422, "validation_failed", "limit 必须是正整数")
		return
	}
	ctx := r.Context()
	userID := ctx.Value(userKey).(string)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	// Shared user lock keeps head and joined entity snapshots consistent with
	// pushes; exclusive per-user push locks also order committed sequence numbers.
	if err = lockUser(ctx, tx, userID, true); err != nil {
		s.internal(w, err)
		return
	}
	q := db.New(tx)
	head, err := q.GetSyncState(ctx, userID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if cursor > head {
		fail(w, 422, "validation_failed", "cursor 超出当前账号的同步范围")
		return
	}
	entries, err := q.ListSyncChanges(ctx, db.ListSyncChangesParams{UserID: userID, AfterSeq: cursor, PageSize: int32(limit + 1)})
	if err != nil {
		s.internal(w, err)
		return
	}
	hasMore := int64(len(entries)) > limit
	if hasMore {
		entries = entries[:limit]
	}
	// Deduplicate only after slicing: the lookahead row cannot suppress an
	// entity from this page. Keep each key at its last position within the page.
	type key struct{ entity, id string }
	latest := make(map[key]int)
	for i, item := range entries {
		latest[key{item.Entity, item.EntityID}] = i
	}
	changes := make([]syncChange, 0, len(latest))
	nextCursor := cursor
	for i, item := range entries {
		nextCursor = item.Seq
		if latest[key{item.Entity, item.EntityID}] != i {
			continue
		}
		value, err := storedChange(item.Entity, item.EntityID, item.UpdatedAt, item.DeletedAt, item.DeviceID, item.Data)
		if err != nil {
			s.internal(w, err)
			return
		}
		changes = append(changes, value)
	}
	if !hasMore {
		nextCursor = head
	}
	if err = tx.Commit(ctx); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"changes": changes, "nextCursor": nextCursor, "hasMore": hasMore, "serverTime": time.Now().UnixMilli()})
}

func lockUser(ctx context.Context, tx pgx.Tx, userID string, shared bool) error {
	query := `SELECT id FROM identity.users WHERE id=$1 FOR UPDATE`
	if shared {
		query = `SELECT id FROM identity.users WHERE id=$1 FOR SHARE`
	}
	var id string
	return tx.QueryRow(ctx, query, userID).Scan(&id)
}
func storedChange(entity, id string, updated int64, deleted *int64, device string, raw []byte) (syncChange, error) {
	result := syncChange{Entity: entity, ID: id, UpdatedAt: updated, DeletedAt: deleted, DeviceID: device}
	value, err := parseJSONValue(raw)
	if err != nil {
		return result, err
	}
	var ok bool
	result.Data, ok = value.(map[string]any)
	if !ok {
		return result, fmt.Errorf("stored entity is not an object")
	}
	return result, nil
}
func readCursor(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || n > 9007199254740991 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return n, nil
}
func readLimit(value string) (int64, error) {
	if value == "" {
		return 200, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid limit")
	}
	if n > 500 {
		n = 500
	}
	return n, nil
}
