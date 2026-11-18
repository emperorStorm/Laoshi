package suitime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	db "github.com/emperorStorm/Laoshi/server/db/sqlc"
	"github.com/jackc/pgx/v5"
)

const initLifetime = 7 * 24 * time.Hour
const maxInitBytes = 256 << 20

type initIdentity struct {
	SessionID string `json:"sessionId"`
	DeviceID  string `json:"deviceId"`
}
type initStartRequest struct {
	initIdentity
	BatchCount *int `json:"batchCount"`
}
type initBatchRequest struct {
	initIdentity
	Index   *int            `json:"index"`
	Changes json.RawMessage `json:"changes"`
}
type initSession struct {
	SessionID  string `json:"sessionId"`
	Status     string `json:"status"`
	BatchCount int    `json:"batchCount"`
	Received   []int  `json:"received"`
	ExpiresAt  int64  `json:"expiresAt"`
}

// 所有初始化及普通同步操作均在同一个账号行锁内检查会话状态。
func (s *Service) requireNoInit(w http.ResponseWriter, ctx context.Context, tx pgx.Tx, userID string) bool {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sui_time.sync_init_sessions WHERE user_id=$1 AND status='active')`, userID).Scan(&exists); err != nil {
		s.internal(w, err)
		return false
	}
	if exists {
		fail(w, 409, "init_in_progress", "初始化进行中或已过期，请先处理该会话")
		return false
	}
	return true
}

func (s *Service) initTx(w http.ResponseWriter, r *http.Request, id initIdentity) (pgx.Tx, string, bool) {
	if !idPattern.MatchString(id.SessionID) || !validDevice(id.DeviceID) {
		fail(w, 422, "validation_failed", "会话或设备标识无效")
		return nil, "", false
	}
	ctx := r.Context()
	userID := ctx.Value(userKey).(string)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.internal(w, err)
		return nil, "", false
	}
	if err = lockUser(ctx, tx, userID, false); err != nil {
		tx.Rollback(ctx)
		s.internal(w, err)
		return nil, "", false
	}
	var device string
	err = tx.QueryRow(ctx, `SELECT device_id FROM identity.sessions WHERE token_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>$3`, tokenDigest(readBearer(r.Header.Get("Authorization"))), userID, time.Now().UnixMilli()).Scan(&device)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && device != id.DeviceID {
		tx.Rollback(ctx)
		fail(w, 404, "init_not_found", "初始化会话不存在")
		return nil, "", false
	}
	if err != nil {
		tx.Rollback(ctx)
		s.internal(w, err)
		return nil, "", false
	}
	return tx, userID, true
}

func (s *Service) loadInit(w http.ResponseWriter, ctx context.Context, tx pgx.Tx, userID string, id initIdentity) (initSession, bool) {
	result := initSession{SessionID: id.SessionID, Received: []int{}}
	err := tx.QueryRow(ctx, `SELECT status,batch_count,expires_at FROM sui_time.sync_init_sessions WHERE user_id=$1 AND session_id=$2 AND device_id=$3`, userID, id.SessionID, id.DeviceID).Scan(&result.Status, &result.BatchCount, &result.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "init_not_found", "初始化会话不存在")
		return result, false
	}
	if err != nil {
		s.internal(w, err)
		return result, false
	}
	if result.Status == "active" && time.Now().UnixMilli() >= result.ExpiresAt {
		result.Status = "expired"
	}
	rows, err := tx.Query(ctx, `SELECT batch_index FROM sui_time.sync_init_batches WHERE user_id=$1 AND session_id=$2 ORDER BY batch_index`, userID, id.SessionID)
	if err != nil {
		s.internal(w, err)
		return result, false
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		if err = rows.Scan(&index); err != nil {
			s.internal(w, err)
			return result, false
		}
		result.Received = append(result.Received, index)
	}
	if err = rows.Err(); err != nil {
		s.internal(w, err)
		return result, false
	}
	return result, true
}

func (s *Service) finishInitResponse(w http.ResponseWriter, ctx context.Context, tx pgx.Tx, session initSession) {
	if err := tx.Commit(ctx); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, session)
}

func (s *Service) initStart(w http.ResponseWriter, r *http.Request) {
	var input initStartRequest
	if !decode(w, r, &input) {
		return
	}
	if input.BatchCount == nil || *input.BatchCount < 0 || *input.BatchCount > 1000 {
		fail(w, 422, "validation_failed", "批次数必须介于 0 和 1000 之间")
		return
	}
	tx, userID, ok := s.initTx(w, r, input.initIdentity)
	if !ok {
		return
	}
	ctx := r.Context()
	defer tx.Rollback(ctx)
	var device string
	var count int
	err := tx.QueryRow(ctx, `SELECT device_id,batch_count FROM sui_time.sync_init_sessions WHERE user_id=$1 AND session_id=$2`, userID, input.SessionID).Scan(&device, &count)
	if err == nil {
		if device != input.DeviceID {
			fail(w, 404, "init_not_found", "初始化会话不存在")
			return
		}
		if count != *input.BatchCount {
			fail(w, 409, "init_conflict", "会话批次数不一致")
			return
		}
		if session, ok := s.loadInit(w, ctx, tx, userID, input.initIdentity); ok {
			s.finishInitResponse(w, ctx, tx, session)
		}
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.internal(w, err)
		return
	}
	if !s.requireNoInit(w, ctx, tx, userID) {
		return
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sui_time.sync_entities WHERE user_id=$1)`, userID).Scan(&exists); err != nil {
		s.internal(w, err)
		return
	}
	if exists {
		fail(w, 409, "remote_not_empty", "云端已有数据，请选择合并或以云端为准")
		return
	}
	now := time.Now()
	_, err = tx.Exec(ctx, `INSERT INTO sui_time.sync_init_sessions(user_id,session_id,device_id,batch_count,status,created_at,expires_at) VALUES($1,$2,$3,$4,'active',$5,$6)`, userID, input.SessionID, input.DeviceID, *input.BatchCount, now.UnixMilli(), now.Add(initLifetime).UnixMilli())
	if err != nil {
		s.internal(w, err)
		return
	}
	if session, ok := s.loadInit(w, ctx, tx, userID, input.initIdentity); ok {
		s.finishInitResponse(w, ctx, tx, session)
	}
}

func (s *Service) initStatus(w http.ResponseWriter, r *http.Request) {
	id := initIdentity{SessionID: r.URL.Query().Get("sessionId"), DeviceID: r.URL.Query().Get("deviceId")}
	tx, userID, ok := s.initTx(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	defer tx.Rollback(ctx)
	if session, ok := s.loadInit(w, ctx, tx, userID, id); ok {
		s.finishInitResponse(w, ctx, tx, session)
	}
}

func (s *Service) initBatch(w http.ResponseWriter, r *http.Request) {
	var input initBatchRequest
	if !decode(w, r, &input) {
		return
	}
	var changes []change
	decoder := json.NewDecoder(bytes.NewReader(input.Changes))
	decoder.DisallowUnknownFields()
	if input.Index == nil || decoder.Decode(&changes) != nil || changes == nil || len(changes) > 200 {
		fail(w, 422, "validation_failed", "批次格式无效或超过 200 条")
		return
	}
	tx, userID, ok := s.initTx(w, r, input.initIdentity)
	if !ok {
		return
	}
	ctx := r.Context()
	defer tx.Rollback(ctx)
	session, ok := s.loadInit(w, ctx, tx, userID, input.initIdentity)
	if !ok {
		return
	}
	if *input.Index < 0 || *input.Index >= session.BatchCount {
		fail(w, 422, "validation_failed", "批次序号无效")
		return
	}
	digest := sha256.Sum256(input.Changes)
	var oldDigest []byte
	err := tx.QueryRow(ctx, `SELECT digest FROM sui_time.sync_init_batches WHERE user_id=$1 AND session_id=$2 AND batch_index=$3`, userID, input.SessionID, *input.Index).Scan(&oldDigest)
	if err == nil {
		if !bytes.Equal(oldDigest, digest[:]) {
			fail(w, 409, "batch_conflict", "批次内容与已接收内容不同")
			return
		}
		if session.Status == "expired" || session.Status == "cancelled" {
			fail(w, 409, "init_not_active", "初始化会话不可继续上传")
			return
		}
		s.finishInitResponse(w, ctx, tx, session)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.internal(w, err)
		return
	}
	if session.Status != "active" {
		fail(w, 409, "init_not_active", "初始化会话不可继续上传")
		return
	}
	var size int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(raw_changes)),0) FROM sui_time.sync_init_batches WHERE user_id=$1 AND session_id=$2`, userID, input.SessionID).Scan(&size); err != nil {
		s.internal(w, err)
		return
	}
	if size+int64(len(input.Changes)) > maxInitBytes {
		fail(w, 413, "init_too_large", "初始化暂存数据超过 256MiB")
		return
	}
	now := time.Now().UnixMilli()
	skew := s.config.ClockSkew
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	normalized := make([]syncChange, len(changes))
	for i, item := range changes {
		if item.UpdatedAt > now+skew.Milliseconds() {
			writeError(w, apiError{Status: 422, Code: "clock_skew", Message: "请校准系统时间后重试", Details: map[string]any{"serverTime": now}})
			return
		}
		normalized[i], err = normalizeChange(item, input.DeviceID, now, s.config.PhotoLimit)
		if err != nil {
			fail(w, 422, "validation_failed", err.Error())
			return
		}
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		s.internal(w, err)
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO sui_time.sync_init_batches(user_id,session_id,batch_index,digest,raw_changes,normalized) VALUES($1,$2,$3,$4,$5,$6)`, userID, input.SessionID, *input.Index, digest[:], []byte(input.Changes), raw)
	if err != nil {
		s.internal(w, err)
		return
	}
	if session, ok = s.loadInit(w, ctx, tx, userID, input.initIdentity); ok {
		s.finishInitResponse(w, ctx, tx, session)
	}
}

func (s *Service) initComplete(w http.ResponseWriter, r *http.Request) {
	var id initIdentity
	if !decode(w, r, &id) {
		return
	}
	tx, userID, ok := s.initTx(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	defer tx.Rollback(ctx)
	session, ok := s.loadInit(w, ctx, tx, userID, id)
	if !ok {
		return
	}
	if session.Status == "completed" {
		s.finishInitResponse(w, ctx, tx, session)
		return
	}
	if session.Status != "active" {
		fail(w, 409, "init_not_active", "初始化会话不可完成")
		return
	}
	if len(session.Received) != session.BatchCount {
		fail(w, 409, "batches_missing", "尚未接收全部批次")
		return
	}
	for i, index := range session.Received {
		if i != index {
			fail(w, 409, "batches_missing", "尚未接收全部批次")
			return
		}
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sui_time.sync_entities WHERE user_id=$1)`, userID).Scan(&exists); err != nil {
		s.internal(w, err)
		return
	}
	if exists {
		fail(w, 409, "remote_not_empty", "初始化期间云端数据发生变化，请取消初始化后选择合并")
		return
	}
	if err := publishInit(ctx, tx, userID, id.SessionID, session.BatchCount); err != nil {
		s.internal(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE sui_time.sync_init_batches SET raw_changes=NULL, normalized=NULL WHERE user_id=$1 AND session_id=$2`, userID, id.SessionID); err != nil {
		s.internal(w, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE sui_time.sync_init_sessions SET status='completed' WHERE user_id=$1 AND session_id=$2`, userID, id.SessionID); err != nil {
		s.internal(w, err)
		return
	}
	session.Status = "completed"
	s.finishInitResponse(w, ctx, tx, session)
}

func publishInit(ctx context.Context, tx pgx.Tx, userID, sessionID string, count int) error {
	q := db.New(tx)
	now := time.Now().UnixMilli()
	for i := 0; i < count; i++ {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT normalized FROM sui_time.sync_init_batches WHERE user_id=$1 AND session_id=$2 AND batch_index=$3`, userID, sessionID, i).Scan(&raw); err != nil {
			return err
		}
		var items []syncChange
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&items); err != nil {
			return err
		}
		for _, item := range items {
			data, err := json.Marshal(item.Data)
			if err != nil {
				return err
			}
			var entity string
			err = tx.QueryRow(ctx, `INSERT INTO sui_time.sync_entities(user_id,entity,entity_id,updated_at,deleted_at,device_id,data) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id,entity,entity_id) DO UPDATE SET updated_at=EXCLUDED.updated_at,deleted_at=EXCLUDED.deleted_at,device_id=EXCLUDED.device_id,data=EXCLUDED.data WHERE (sui_time.sync_entities.updated_at,sui_time.sync_entities.device_id COLLATE "C")<(EXCLUDED.updated_at,EXCLUDED.device_id COLLATE "C") RETURNING entity`, userID, item.Entity, item.ID, item.UpdatedAt, item.DeletedAt, item.DeviceID, data).Scan(&entity)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err = q.AppendSyncChange(ctx, db.AppendSyncChangeParams{UserID: userID, Entity: item.Entity, EntityID: item.ID, CreatedAt: now}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) initCancel(w http.ResponseWriter, r *http.Request) {
	var id initIdentity
	if !decode(w, r, &id) {
		return
	}
	tx, userID, ok := s.initTx(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	defer tx.Rollback(ctx)
	session, ok := s.loadInit(w, ctx, tx, userID, id)
	if !ok {
		return
	}
	if session.Status == "completed" {
		fail(w, 409, "init_completed", "已完成的初始化不能取消")
		return
	}
	if session.Status != "cancelled" {
		if _, err := tx.Exec(ctx, `DELETE FROM sui_time.sync_init_batches WHERE user_id=$1 AND session_id=$2`, userID, id.SessionID); err != nil {
			s.internal(w, err)
			return
		}
		if _, err := tx.Exec(ctx, `UPDATE sui_time.sync_init_sessions SET status='cancelled' WHERE user_id=$1 AND session_id=$2`, userID, id.SessionID); err != nil {
			s.internal(w, err)
			return
		}
	}
	session.Status = "cancelled"
	session.Received = []int{}
	s.finishInitResponse(w, ctx, tx, session)
}
