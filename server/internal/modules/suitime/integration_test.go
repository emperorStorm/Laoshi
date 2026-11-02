package suitime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/emperorStorm/Laoshi/server/db/migrations"
	"github.com/emperorStorm/Laoshi/server/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each run gets a disposable database; never migrate the supplied database.
func integrationServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; local PostgreSQL integration tests skipped")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") {
		t.Fatal("TEST_DATABASE_URL must point at a local PostgreSQL instance")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("laoshi_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		admin.Close(ctx)
	})
	parsed.Path = "/" + name
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Up(ctx, parsed.String()); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Up(ctx, parsed.String()); err != nil {
		t.Fatalf("repeat migrations: %v", err)
	}
	cfg := platform.Config{TokenTTL: time.Hour, BodyLimit: 8 << 20, PhotoLimit: 6 << 20, ClockSkew: 5 * time.Minute}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(platform.Middleware(cfg, logger, New(pool, cfg, logger).Routes()))
	t.Cleanup(server.Close)
	return server, pool
}

type testSession struct {
	Token string    `json:"token"`
	User  cloudUser `json:"user"`
}

func callAPI(t *testing.T, base, token, method, path string, body any, status int) map[string]json.RawMessage {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, res.StatusCode, status, raw)
	}
	result := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
func registerTest(t *testing.T, base, name string) testSession {
	body := callAPI(t, base, "", "POST", "/api/v1/auth/register", credentials{Username: name, Password: "password123", DeviceID: "desktop"}, 201)
	raw, _ := json.Marshal(body)
	var session testSession
	json.Unmarshal(raw, &session)
	if session.Token == "" || session.User.ID == "" {
		t.Fatal("invalid session contract")
	}
	return session
}
func pushTest(t *testing.T, base, token string, items []change, empty bool, status int) map[string]json.RawMessage {
	return callAPI(t, base, token, "POST", "/api/v1/sui-time/sync/push", pushRequest{DeviceID: "batch-device", RequireEmpty: empty, Changes: items}, status)
}

func TestPostgresSyncAndAuth(t *testing.T) {
	server, pool := integrationServer(t)
	base := server.URL
	a := registerTest(t, base, "用户甲")
	b := registerTest(t, base, "用户乙")
	callAPI(t, base, "", "POST", "/api/v1/auth/register", credentials{Username: "用户甲", Password: "password123", DeviceID: "mobile"}, 409)
	callAPI(t, base, "", "POST", "/api/v1/auth/login", credentials{Username: "用户甲", Password: "wrongpass", DeviceID: "mobile"}, 401)
	callAPI(t, base, a.Token, "GET", "/api/v1/auth/me", nil, 200)
	callAPI(t, base, "", "GET", "/api/v1/sui-time/sync/state", nil, 401)
	first := candidate("task", "t", `{"title":"desktop","status":"todo","notes":"","createdAt":10}`)
	pushTest(t, base, a.Token, []change{first}, true, 200)
	// The lookahead is the same key: first page must still include it.
	second := first
	second.UpdatedAt = 200
	second.DeviceID = "z-mobile"
	pushTest(t, base, a.Token, []change{second}, false, 200)
	page := callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/pull?limit=1", nil, 200)
	var changes []syncChange
	json.Unmarshal(page["changes"], &changes)
	var more bool
	json.Unmarshal(page["hasMore"], &more)
	var cursor int64
	json.Unmarshal(page["nextCursor"], &cursor)
	if len(changes) != 1 || changes[0].UpdatedAt != 200 || !more || cursor <= 0 {
		t.Fatalf("lookahead dropped page: %s", page)
	}
	next := callAPI(t, base, a.Token, "GET", fmt.Sprintf("/api/v1/sui-time/sync/pull?cursor=%d&limit=1", cursor), nil, 200)
	json.Unmarshal(next["hasMore"], &more)
	if more {
		t.Fatal("final page hasMore")
	}
	result := pushTest(t, base, a.Token, []change{first}, false, 200)
	var superseded []struct{ Server syncChange }
	json.Unmarshal(result["superseded"], &superseded)
	if len(superseded) != 1 || superseded[0].Server.DeviceID != "z-mobile" {
		t.Fatal("missing conflict winner")
	}
	var seqBefore int64
	json.Unmarshal(result["seq"], &seqBefore)
	retry := pushTest(t, base, a.Token, []change{second}, false, 200)
	var seqAfter int64
	json.Unmarshal(retry["seq"], &seqAfter)
	if seqBefore != seqAfter {
		t.Fatal("idempotent retry appended change")
	}
	pushTest(t, base, a.Token, []change{}, true, 409)
	other := callAPI(t, base, b.Token, "GET", "/api/v1/sui-time/sync/pull", nil, 200)
	json.Unmarshal(other["changes"], &changes)
	if len(changes) != 0 {
		t.Fatal("cross-account data leak")
	}
	pushTest(t, base, b.Token, []change{first}, true, 200)
	deleted := second
	deleted.UpdatedAt = 300
	deleted.DeletedAt = &deleted.UpdatedAt
	pushTest(t, base, a.Token, []change{deleted}, false, 200)
	state := callAPI(t, base, a.Token, "GET", "/api/v1/sui-time/sync/state", nil, 200)
	var empty bool
	json.Unmarshal(state["remoteEmpty"], &empty)
	if empty {
		t.Fatal("tombstone account reported empty")
	}
	// An invalid batch is atomic, even when its first change is valid.
	good := candidate("category", "cat", `{"name":"分类","color":"#ffffff"}`)
	bad := candidate("task", "invalid", `{"title":"bad","status":"invalid"}`)
	pushTest(t, base, a.Token, []change{good, bad}, false, 422)
	var count int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM sui_time.sync_entities WHERE user_id=$1 AND entity_id='cat'`, a.User.ID).Scan(&count)
	if count != 0 {
		t.Fatal("invalid batch was partially committed")
	}
	future := first
	future.UpdatedAt = time.Now().Add(time.Hour).UnixMilli()
	skew := pushTest(t, base, a.Token, []change{future}, false, 422)
	if !bytes.Contains(skew["error"], []byte("clock_skew")) {
		t.Fatal("clock skew error contract")
	}
	callAPI(t, base, a.Token, "POST", "/api/v1/auth/logout", nil, 204)
	callAPI(t, base, a.Token, "GET", "/api/v1/auth/me", nil, 401)
}

func TestRealMobileClientContract(t *testing.T) {
	root := os.Getenv("SUI_TIME_CLIENT_ROOT")
	if root == "" {
		t.Skip("set SUI_TIME_CLIENT_ROOT to the real uni-client/src directory")
	}
	server, _ := integrationServer(t)
	_, file, _, _ := runtime.Caller(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--experimental-vm-modules", filepath.Join(filepath.Dir(file), "client_contract.mjs"), server.URL, root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real client failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}

func TestPostgresConcurrentRequireEmpty(t *testing.T) {
	server, _ := integrationServer(t)
	session := registerTest(t, server.URL, "concurrent")
	body, _ := json.Marshal(pushRequest{DeviceID: "desktop", RequireEmpty: true, Changes: []change{candidate("category", "cat", `{"name":"cat","color":"#fff"}`)}})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", server.URL+"/api/v1/sui-time/sync/push", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+session.Token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			defer res.Body.Close()
			io.Copy(io.Discard, res.Body)
			statuses <- res.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("requireEmpty race: %v", counts)
	}
}
