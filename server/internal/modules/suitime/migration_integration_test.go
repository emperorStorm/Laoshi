package suitime

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/emperorStorm/Laoshi/server/db/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// TestPostgresMigrationUpgradeLegacy 从真实的 00001 迁移状态升级，而不是模拟迁移记录。
func TestPostgresMigrationUpgradeLegacy(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; local PostgreSQL integration tests skipped")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || !localPostgresHost(parsed.Hostname()) {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL pointing at a local instance")
	}
	// 禁止 URL 参数或连接配置绕过主机检查，也不允许连接备用主机。
	query := parsed.Query()
	if query.Has("host") || query.Has("hostaddr") || query.Has("service") || query.Has("dbname") || query.Has("database") {
		t.Fatal("TEST_DATABASE_URL must not override the host or database or use a service")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !localPostgresHost(config.Host) {
		t.Fatal("TEST_DATABASE_URL must resolve to a local PostgreSQL host")
	}
	for _, fallback := range config.Fallbacks {
		if fallback.Host != config.Host || fallback.Port != config.Port {
			t.Fatal("TEST_DATABASE_URL must not specify alternative hosts")
		}
	}

	ctx := context.Background()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("laoshi_test_upgrade_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
			t.Errorf("drop disposable database: %v", err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Errorf("close database connection: %v", err)
		}
	})
	parsed.Path = "/" + name
	testDSN := parsed.String()
	testConfig, err := pgx.ParseConfig(testDSN)
	if err != nil || testConfig.Database != name || testConfig.Host != config.Host || testConfig.Port != config.Port {
		t.Fatal("disposable database connection does not match the created database")
	}
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// 使用同一套 goose SQL 文件，只应用第一个迁移以构造旧库。
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate migration files")
	}
	migrationDir := filepath.Join(filepath.Dir(source), "..", "..", "..", "db", "migrations")
	conn, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, os.DirFS(migrationDir))
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	_, err = provider.UpTo(ctx, 1)
	closeErr := conn.Close()
	if err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	assertMigrationVersions(t, ctx, pool, 1, 0)
	var initTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('sui_time.sync_init_sessions')::text`).Scan(&initTable); err != nil || initTable != nil {
		t.Fatalf("initial database unexpectedly has init sessions table: %v, %v", initTable, err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO identity.users (id, username, password_hash, created_at, updated_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'legacy-user', 'legacy-hash', 10, 20);
		INSERT INTO identity.sessions (token_hash, user_id, device_id, created_at, expires_at, revoked_at, last_used_at)
		VALUES ('legacy-token', '00000000-0000-0000-0000-000000000001', 'desktop', 11, 99, NULL, 12);
		INSERT INTO identity.devices (user_id, device_id, name, platform, last_seen_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'desktop', '旧设备', 'desktop', 13);
		INSERT INTO sui_time.sync_entities (user_id, entity, entity_id, updated_at, deleted_at, device_id, data)
		VALUES
		('00000000-0000-0000-0000-000000000001', 'task', 'live', 30, NULL, 'desktop', '{"title":"旧任务","status":"todo"}'),
		('00000000-0000-0000-0000-000000000001', 'task', 'deleted', 40, 40, 'desktop', '{"title":"已删除"}');
		INSERT INTO sui_time.sync_changelog (user_id, entity, entity_id, created_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'task', 'live', 30),
		       ('00000000-0000-0000-0000-000000000001', 'task', 'deleted', 40)`)
	if err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}
	legacy := migrationLegacySnapshot(t, ctx, pool)

	if err := migrations.Up(ctx, testDSN); err != nil {
		t.Fatalf("upgrade migrations: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 1, 1)
	if got := migrationLegacySnapshot(t, ctx, pool); got != legacy {
		t.Fatalf("legacy data changed after upgrade:\nbefore: %s\nafter: %s", legacy, got)
	}
	// 新表可用，且再次迁移也不能改变新表中的数据。
	_, err = pool.Exec(ctx, `
		INSERT INTO sui_time.sync_init_sessions (user_id, session_id, device_id, batch_count, status, created_at, expires_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'upgrade-session', 'desktop', 1, 'active', 50, 100);
		INSERT INTO sui_time.sync_init_batches (user_id, session_id, batch_index, digest, raw_changes, normalized)
		VALUES ('00000000-0000-0000-0000-000000000001', 'upgrade-session', 0, decode('0102', 'hex'), decode('0304', 'hex'), '[{"id":"legacy"}]')`)
	if err != nil {
		t.Fatalf("write upgraded tables: %v", err)
	}
	upgraded := migrationInitSnapshot(t, ctx, pool)
	history := migrationHistorySnapshot(t, ctx, pool)
	if err := migrations.Up(ctx, testDSN); err != nil {
		t.Fatalf("repeat migrations: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 1, 1)
	if got := migrationHistorySnapshot(t, ctx, pool); got != history {
		t.Fatalf("migration history changed after repeat migration:\nbefore: %s\nafter: %s", history, got)
	}
	if got := migrationLegacySnapshot(t, ctx, pool); got != legacy {
		t.Fatalf("legacy data changed after repeat migration:\nbefore: %s\nafter: %s", legacy, got)
	}
	if got := migrationInitSnapshot(t, ctx, pool); got != upgraded {
		t.Fatalf("new data changed after repeat migration:\nbefore: %s\nafter: %s", upgraded, got)
	}
}

func localPostgresHost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func assertMigrationVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, initial, init int) {
	t.Helper()
	var first, second, reverted int
	err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE version_id = 1 AND is_applied),
		count(*) FILTER (WHERE version_id = 2 AND is_applied),
		count(*) FILTER (WHERE version_id IN (1, 2) AND NOT is_applied)
		FROM goose_db_version`).Scan(&first, &second, &reverted)
	if err != nil || first != initial || second != init || reverted != 0 {
		t.Fatalf("unexpected migration history: 00001=%d 00002=%d reverted=%d: %v", first, second, reverted, err)
	}
}

func migrationHistorySnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(v) ORDER BY v.id), '[]'::jsonb)::text FROM goose_db_version v`).Scan(&snapshot)
	if err != nil {
		t.Fatalf("snapshot migration history: %v", err)
	}
	return snapshot
}

func migrationLegacySnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
		'users', (SELECT coalesce(jsonb_agg(to_jsonb(u) ORDER BY u.id), '[]'::jsonb) FROM identity.users u),
		'sessions', (SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY s.token_hash), '[]'::jsonb) FROM identity.sessions s),
		'devices', (SELECT coalesce(jsonb_agg(to_jsonb(d) ORDER BY d.user_id, d.device_id), '[]'::jsonb) FROM identity.devices d),
		'entities', (SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.user_id, e.entity, e.entity_id), '[]'::jsonb) FROM sui_time.sync_entities e),
		'changelog', (SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY c.seq), '[]'::jsonb) FROM sui_time.sync_changelog c)
	)::text`).Scan(&snapshot)
	if err != nil {
		t.Fatalf("snapshot legacy data: %v", err)
	}
	return snapshot
}

func migrationInitSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
		'sessions', (SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY s.user_id, s.session_id), '[]'::jsonb) FROM sui_time.sync_init_sessions s),
		'batches', (SELECT coalesce(jsonb_agg(to_jsonb(b) ORDER BY b.user_id, b.session_id, b.batch_index), '[]'::jsonb) FROM sui_time.sync_init_batches b)
	)::text`).Scan(&snapshot)
	if err != nil {
		t.Fatalf("snapshot init data: %v", err)
	}
	return snapshot
}
