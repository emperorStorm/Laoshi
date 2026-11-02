package suitime

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/emperorStorm/Laoshi/server/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	config  platform.Config
	logger  *slog.Logger
	limitMu sync.Mutex
	limits  map[string]rateWindow
}
type rateWindow struct {
	start time.Time
	count int
}
type apiError struct {
	Status        int
	Code, Message string
	Details       any
}
type contextKey string

const userKey contextKey = "user_id"

func New(pool *pgxpool.Pool, config platform.Config, logger *slog.Logger) *Service {
	return &Service{pool: pool, config: config, logger: logger, limits: make(map[string]rateWindow)}
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Body == nil {
		fail(w, http.StatusBadRequest, "invalid_json", "请求体不能为空")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			fail(w, http.StatusRequestEntityTooLarge, "payload_too_large", "请求体超过 8MiB")
		} else {
			fail(w, http.StatusBadRequest, "invalid_json", "请求体不是有效的 JSON")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		fail(w, http.StatusBadRequest, "invalid_json", "请求体只能包含一个 JSON 对象")
		return false
	}
	return true
}

func (s *Service) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.pool.Ping(r.Context()); err != nil {
			fail(w, http.StatusServiceUnavailable, "not_ready", "服务暂不可用")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/register", s.limit("auth", 10, time.Minute, s.register))
		api.Post("/auth/login", s.limit("auth", 10, time.Minute, s.login))
		api.Group(func(a chi.Router) {
			a.Use(s.authenticate)
			a.Post("/auth/logout", s.logout)
			a.Get("/auth/me", s.me)
			a.Get("/sui-time/sync/state", s.syncState)
			a.Post("/sui-time/sync/push", s.limit("sync", 120, time.Minute, s.push))
			a.Get("/sui-time/sync/pull", s.limit("sync", 120, time.Minute, s.pull))
		})
	})
	return r
}

func (s *Service) limit(scope string, max int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := scope + ":" + platform.ClientIP(r)
		now := time.Now()
		s.limitMu.Lock()
		for k, v := range s.limits {
			if now.Sub(v.start) >= window {
				delete(s.limits, k)
			}
		}
		b := s.limits[key]
		if b.start.IsZero() {
			b = rateWindow{start: now}
		}
		b.count++
		s.limits[key] = b
		s.limitMu.Unlock()
		if b.count > max {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "rate_limited", "请求过于频繁，请稍后重试")
			return
		}
		next(w, r)
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err apiError) {
	body := map[string]any{"code": err.Code, "message": err.Message}
	if err.Details != nil {
		body["details"] = err.Details
	}
	writeJSON(w, err.Status, map[string]any{"error": body})
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	writeError(w, apiError{Status: status, Code: code, Message: msg})
}
func (s *Service) internal(w http.ResponseWriter, err error) {
	s.logger.Error("database operation failed", "error", err)
	fail(w, http.StatusInternalServerError, "internal_error", "服务器内部错误")
}
