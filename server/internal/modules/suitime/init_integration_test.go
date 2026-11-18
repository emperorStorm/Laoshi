package suitime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emperorStorm/Laoshi/server/internal/platform"
	"log/slog"
)

const initPath = "/api/v1/sui-time/sync/init/"

func assertInit(t *testing.T, body map[string]json.RawMessage, status string, received ...int) {
	t.Helper()
	var result initSession
	raw, _ := json.Marshal(body)
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != status || len(result.Received) != len(received) {
		t.Fatalf("unexpected init response: %s", raw)
	}
	for i := range received {
		if result.Received[i] != received[i] {
			t.Fatalf("unexpected init response: %s", raw)
		}
	}
}

func initBody(session, device string) map[string]any {
	return map[string]any{"sessionId": session, "deviceId": device}
}
func initStartBody(session, device string, count int) map[string]any {
	return map[string]any{"sessionId": session, "deviceId": device, "batchCount": count}
}
func initBatchBody(session, device string, index int, changes []change) map[string]any {
	return map[string]any{"sessionId": session, "deviceId": device, "index": index, "changes": changes}
}
func assertErrorCode(t *testing.T, body map[string]json.RawMessage, code string) {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body["error"], &e); err != nil || e.Code != code {
		t.Fatalf("expected %s: %s", code, body["error"])
	}
}

func TestPostgresInitResumeAndIsolation(t *testing.T) {
	server, pool := integrationServer(t)
	base := server.URL
	a := registerTest(t, base, "init_user_a")
	b := registerTest(t, base, "init_user_b")
	id := initBody("snapshot-1", "desktop")
	start := initStartBody("snapshot-1", "desktop", 2)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"start", start, 200), "active")
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"start", start, 200), "active")
	callAPI(t, base, a.Token, "POST", initPath+"start", initStartBody("another", "desktop", 0), 409)
	callAPI(t, base, b.Token, "GET", initPath+"status?sessionId=snapshot-1&deviceId=desktop", nil, 404)
	callAPI(t, base, a.Token, "GET", initPath+"status?sessionId=snapshot-1&deviceId=other", nil, 404)
	login := callAPI(t, base, "", "POST", "/api/v1/auth/login", credentials{Username: "init_user_a", Password: "password123", DeviceID: "mobile"}, 200)
	var mobileToken string
	if err := json.Unmarshal(login["token"], &mobileToken); err != nil {
		t.Fatal(err)
	}
	callAPI(t, base, mobileToken, "GET", initPath+"status?sessionId=snapshot-1&deviceId=desktop", nil, 404)
	callAPI(t, base, mobileToken, "POST", initPath+"cancel", id, 404)
	callAPI(t, base, "", "POST", initPath+"start", start, 401)
	first := candidate("category", "cat", `{"name":"分类","color":"#fff"}`)
	second := candidate("task", "task", `{"title":"待办","status":"todo"}`)
	batch := initBatchBody("snapshot-1", "desktop", 1, []change{second})
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"batch", batch, 200), "active", 1)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"batch", batch, 200), "active", 1)
	altered := second
	altered.Data = json.RawMessage(`{"title":"待办","status":"todo","unused":true}`)
	assertErrorCode(t, callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("snapshot-1", "desktop", 1, []change{altered}), 409), "batch_conflict")
	assertErrorCode(t, callAPI(t, base, a.Token, "POST", initPath+"complete", id, 409), "batches_missing")
	for _, path := range []string{"state", "pull"} {
		assertErrorCode(t, callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/"+path, nil, 409), "init_in_progress")
	}
	assertErrorCode(t, pushTest(t, base, a.Token, []change{first}, false, 409), "init_in_progress")
	assertErrorCode(t, pushTest(t, base, a.Token, nil, false, 409), "init_in_progress")
	assertErrorCode(t, callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/pull?cursor=invalid", nil, 409), "init_in_progress")
	callAPI(t, base, b.Token, "GET", "/api/v1/sui-time/sync/state", nil, 200)
	var entities, changes int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1),(SELECT count(*) FROM sui_time.sync_changelog WHERE user_id=$1)`, a.User.ID).Scan(&entities, &changes); err != nil || entities != 0 || changes != 0 {
		t.Fatalf("partial snapshot leaked: %d %d %v", entities, changes, err)
	}
	// 新服务实例只复用数据库，模拟进程重启和上传响应丢失。
	cfg := platform.Config{TokenTTL: time.Hour, BodyLimit: 8 << 20, PhotoLimit: 6 << 20, ClockSkew: 5 * time.Minute}
	restarted := httptest.NewServer(platform.Middleware(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), New(pool, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()))
	defer restarted.Close()
	base = restarted.URL
	assertInit(t, callAPI(t, base, a.Token, "GET", initPath+"status?sessionId=snapshot-1&deviceId=desktop", nil, 200), "active", 1)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("snapshot-1", "desktop", 0, []change{first}), 200), "active", 0, 1)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"complete", id, 200), "completed", 0, 1)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"complete", id, 200), "completed", 0, 1)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"start", start, 200), "completed", 0, 1)
	callAPI(t, base, a.Token, "POST", initPath+"cancel", id, 409)
	var retained int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_init_batches WHERE user_id=$1 AND (raw_changes IS NOT NULL OR normalized IS NOT NULL)`, a.User.ID).Scan(&retained); err != nil || retained != 0 {
		t.Fatalf("completed payload retained: %d %v", retained, err)
	}
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"batch", batch, 200), "completed", 0, 1)
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1),(SELECT count(*) FROM sui_time.sync_changelog WHERE user_id=$1)`, a.User.ID).Scan(&entities, &changes); err != nil || entities != 2 || changes != 2 {
		t.Fatalf("publication not exactly once: %d %d %v", entities, changes, err)
	}
	callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/pull", nil, 200)
	assertErrorCode(t, callAPI(t, base, a.Token, "POST", initPath+"start", initStartBody("new", "desktop", 0), 409), "remote_not_empty")
}

func TestPostgresInitCancelExpiryAndEmpty(t *testing.T) {
	server, pool := integrationServer(t)
	base := server.URL
	a := registerTest(t, base, "init_expiry")
	id := initBody("expired", "desktop")
	callAPI(t, base, a.Token, "POST", initPath+"start", initStartBody("expired", "desktop", 1), 200)
	first := candidate("category", "cat", `{"name":"分类","color":"#fff"}`)
	tooMany := make([]change, 201)
	for i := range tooMany {
		tooMany[i] = first
	}
	callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("expired", "desktop", 0, tooMany), 422)
	oversized := first
	oversized.Data = json.RawMessage(`{"name":"` + strings.Repeat("x", 8<<20) + `","color":"#fff"}`)
	callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("expired", "desktop", 0, []change{oversized}), 413)
	callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("expired", "desktop", 1, []change{first}), 422)
	future := first
	future.UpdatedAt = time.Now().Add(6 * time.Minute).UnixMilli()
	assertErrorCode(t, callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("expired", "desktop", 0, []change{future}), 422), "clock_skew")
	assertInit(t, callAPI(t, base, a.Token, "GET", initPath+"status?sessionId=expired&deviceId=desktop", nil, 200), "active")
	callAPI(t, base, a.Token, "POST", initPath+"batch", initBatchBody("expired", "desktop", 0, []change{first}), 200)
	if _, err := pool.Exec(context.Background(), `UPDATE sui_time.sync_init_sessions SET expires_at=$1 WHERE user_id=$2`, time.Now().Add(-time.Second).UnixMilli(), a.User.ID); err != nil {
		t.Fatal(err)
	}
	assertInit(t, callAPI(t, base, a.Token, "GET", initPath+"status?sessionId=expired&deviceId=desktop", nil, 200), "expired", 0)
	callAPI(t, base, a.Token, "POST", initPath+"complete", id, 409)
	assertErrorCode(t, callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/state", nil, 409), "init_in_progress")
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"cancel", id, 200), "cancelled")
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"cancel", id, 200), "cancelled")
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_init_batches WHERE user_id=$1`, a.User.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cancel left staged data: %d %v", n, err)
	}
	callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/state", nil, 200)
	callAPI(t, base, a.Token, "POST", initPath+"start", initStartBody("empty", "desktop", 0), 200)
	assertInit(t, callAPI(t, base, a.Token, "POST", initPath+"complete", initBody("empty", "desktop"), 200), "completed")
	deleted := first
	deleted.DeletedAt = &deleted.UpdatedAt
	pushTest(t, base, a.Token, []change{deleted}, false, 200)
	assertErrorCode(t, callAPI(t, base, a.Token, "POST", initPath+"start", initStartBody("tombstone", "desktop", 0), 409), "remote_not_empty")
}

func TestPostgresInitRejectsConcurrentLegacyPublication(t *testing.T) {
	server, pool := integrationServer(t)
	a := registerTest(t, server.URL, "init_legacy_write")
	id := initBody("legacy-race", "desktop")
	callAPI(t, server.URL, a.Token, "POST", initPath+"start", initStartBody("legacy-race", "desktop", 1), 200)
	item := candidate("category", "staged", `{"name":"暂存","color":"#fff"}`)
	callAPI(t, server.URL, a.Token, "POST", initPath+"batch", initBatchBody("legacy-race", "desktop", 0, []change{item}), 200)
	_, err := pool.Exec(context.Background(), `INSERT INTO sui_time.sync_entities(user_id,entity,entity_id,updated_at,device_id,data) VALUES($1,'category','legacy',1,'old','{"name":"旧版数据"}')`, a.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertErrorCode(t, callAPI(t, server.URL, a.Token, "POST", initPath+"complete", id, 409), "remote_not_empty")
	var count int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1`, a.User.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("initialization changed existing data: %d %v", count, err)
	}
	assertInit(t, callAPI(t, server.URL, a.Token, "POST", initPath+"cancel", id, 200), "cancelled")
}

func TestPostgresInitPublicationRollback(t *testing.T) {
	server, pool := integrationServer(t)
	a := registerTest(t, server.URL, "init_rollback")
	id := initBody("rollback", "desktop")
	callAPI(t, server.URL, a.Token, "POST", initPath+"start", initStartBody("rollback", "desktop", 1), 200)
	items := []change{candidate("category", "ok", `{"name":"a","color":"#fff"}`), candidate("category", "fail", `{"name":"b","color":"#fff"}`)}
	callAPI(t, server.URL, a.Token, "POST", initPath+"batch", initBatchBody("rollback", "desktop", 0, items), 200)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE sui_time.sync_entities ADD CONSTRAINT test_reject CHECK (entity_id <> 'fail')`); err != nil {
		t.Fatal(err)
	}
	callAPI(t, server.URL, a.Token, "POST", initPath+"complete", id, 500)
	var entities, changes int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1),(SELECT count(*) FROM sui_time.sync_changelog WHERE user_id=$1)`, a.User.ID).Scan(&entities, &changes); err != nil || entities != 0 || changes != 0 {
		t.Fatalf("failed publication leaked: %d %d %v", entities, changes, err)
	}
	assertInit(t, callAPI(t, server.URL, a.Token, "GET", initPath+"status?sessionId=rollback&deviceId=desktop", nil, 200), "active", 0)
	if _, err := pool.Exec(ctx, `ALTER TABLE sui_time.sync_entities DROP CONSTRAINT test_reject`); err != nil {
		t.Fatal(err)
	}
	assertInit(t, callAPI(t, server.URL, a.Token, "POST", initPath+"complete", id, 200), "completed", 0)
}

func concurrentRequests(base, token string, paths []string, bodies []any) []int {
	var wg sync.WaitGroup
	statuses := make([]int, len(paths))
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body, _ := json.Marshal(bodies[i])
			req, _ := http.NewRequest("POST", base+paths[i], bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer res.Body.Close()
			io.Copy(io.Discard, res.Body)
			statuses[i] = res.StatusCode
		}(i)
	}
	wg.Wait()
	return statuses
}

func TestPostgresInitConcurrentRaces(t *testing.T) {
	server, pool := integrationServer(t)
	base := server.URL
	a := registerTest(t, base, "init_races")
	paths := []string{initPath + "start", initPath + "start"}
	statuses := concurrentRequests(base, a.Token, paths, []any{initStartBody("a", "desktop", 1), initStartBody("b", "desktop", 1)})
	if !((statuses[0] == 200 && statuses[1] == 409) || (statuses[0] == 409 && statuses[1] == 200)) {
		t.Fatalf("start race: %v", statuses)
	}
	winner := "a"
	if statuses[1] == 200 {
		winner = "b"
	}
	batch := initBatchBody(winner, "desktop", 0, []change{candidate("category", "cat", `{"name":"分类","color":"#fff"}`)})
	statuses = concurrentRequests(base, a.Token, []string{initPath + "batch", initPath + "batch"}, []any{batch, batch})
	if statuses[0] != 200 || statuses[1] != 200 {
		t.Fatalf("batch race: %v", statuses)
	}
	id := initBody(winner, "desktop")
	statuses = concurrentRequests(base, a.Token, []string{initPath + "complete", initPath + "complete"}, []any{id, id})
	if statuses[0] != 200 || statuses[1] != 200 {
		t.Fatalf("complete race: %v", statuses)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_changelog WHERE user_id=$1`, a.User.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate publication: %d %v", count, err)
	}
	b := registerTest(t, base, "init_old_race")
	push := pushRequest{DeviceID: "desktop", Changes: []change{candidate("category", "x", `{"name":"x","color":"#fff"}`)}}
	statuses = concurrentRequests(base, b.Token, []string{initPath + "start", "/api/v1/sui-time/sync/push"}, []any{initStartBody("race", "desktop", 0), push})
	if !((statuses[0] == 200 && statuses[1] == 409) || (statuses[0] == 409 && statuses[1] == 200)) {
		t.Fatalf("old client race: %v", statuses)
	}
	if statuses[0] == 200 {
		callAPI(t, base, b.Token, "POST", initPath+"cancel", initBody("race", "desktop"), 200)
	}
	callAPI(t, base, b.Token, "GET", fmt.Sprintf("%sstatus?sessionId=%s&deviceId=desktop", initPath, url.QueryEscape("not-found")), nil, 404)
	c := registerTest(t, base, "init_cancel_race")
	callAPI(t, base, c.Token, "POST", initPath+"start", initStartBody("cancel-race", "desktop", 1), 200)
	callAPI(t, base, c.Token, "POST", initPath+"batch", initBatchBody("cancel-race", "desktop", 0, []change{candidate("category", "race", `{"name":"race","color":"#fff"}`)}), 200)
	raceID := initBody("cancel-race", "desktop")
	statuses = concurrentRequests(base, c.Token, []string{initPath + "complete", initPath + "cancel"}, []any{raceID, raceID})
	if !((statuses[0] == 200 && statuses[1] == 409) || (statuses[0] == 409 && statuses[1] == 200)) {
		t.Fatalf("cancel race: %v", statuses)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1`, c.User.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if (statuses[0] == 200 && count != 1) || (statuses[1] == 200 && count != 0) {
		t.Fatalf("cancel race leaked data: %v, %d", statuses, count)
	}
}
