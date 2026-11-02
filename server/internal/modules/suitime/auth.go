package suitime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"
)

type cloudUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type credentials struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
}

func validUsername(value string) bool {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 3 || len(runes) > 32 {
		return false
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '.' && r != '-' {
			return false
		}
	}
	return true
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(strings.TrimPrefix(encoded, "$"), "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" || parts[2] != "m=19456,t=2,p=1" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func tokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func readBearer(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (s *Service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := readBearer(r.Header.Get("Authorization"))
		if token == "" {
			fail(w, http.StatusUnauthorized, "auth_required", "请先登录")
			return
		}
		var userID string
		now := time.Now().UnixMilli()
		err := s.pool.QueryRow(r.Context(), `
			UPDATE identity.sessions
			SET last_used_at = $2
			WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2
			RETURNING user_id::text`, tokenDigest(token), now).Scan(&userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				fail(w, http.StatusUnauthorized, "token_expired", "登录已过期，请重新登录")
			} else {
				s.internal(w, err)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, userID)))
	})
}

func (s *Service) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decode(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if !validUsername(input.Username) || len(input.Password) < 8 || len(input.Password) > 72 || !validDevice(input.DeviceID) {
		fail(w, http.StatusUnprocessableEntity, "validation_failed", "用户名、密码或设备信息无效")
		return
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		s.internal(w, err)
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	now := time.Now().UnixMilli()
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.users(username, password_hash, created_at, updated_at)
		VALUES($1, $2, $3, $3) RETURNING id::text`, input.Username, passwordHash, now).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			fail(w, http.StatusConflict, "username_taken", "该用户名已被注册")
			return
		}
		s.internal(w, err)
		return
	}
	user := cloudUser{ID: userID, Username: input.Username}
	response, err := s.createSession(ctx, tx, user, input)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decode(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if !validUsername(input.Username) || len(input.Password) < 8 || len(input.Password) > 72 || !validDevice(input.DeviceID) {
		fail(w, http.StatusUnprocessableEntity, "validation_failed", "用户名、密码或设备信息无效")
		return
	}
	var user cloudUser
	var passwordHash string
	err := s.pool.QueryRow(r.Context(), `
		SELECT id::text, username, password_hash
		FROM identity.users WHERE lower(username) = lower($1)`, input.Username).Scan(&user.ID, &user.Username, &passwordHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.internal(w, err)
		return
	}
	if err != nil || !verifyPassword(passwordHash, input.Password) {
		fail(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码不正确")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	response, err := s.createSession(r.Context(), tx, user, input)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, user cloudUser, input credentials) (map[string]any, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	nowMS := now.UnixMilli()
	expiresAt := now.Add(s.config.TokenTTL).UnixMilli()
	if _, err = tx.Exec(ctx, `
		INSERT INTO identity.sessions(token_hash, user_id, device_id, created_at, expires_at, last_used_at)
		VALUES($1, $2, $3, $4, $5, $4)`, tokenDigest(token), user.ID, input.DeviceID, nowMS, expiresAt); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO identity.devices(user_id, device_id, name, platform, last_seen_at)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT(user_id, device_id) DO UPDATE SET name = EXCLUDED.name,
		platform = EXCLUDED.platform, last_seen_at = EXCLUDED.last_seen_at`,
		user.ID, input.DeviceID, trimLabel(input.DeviceName), trimLabel(input.Platform), nowMS); err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "expiresAt": expiresAt, "user": user, "serverTime": nowMS}, nil
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	token := readBearer(r.Header.Get("Authorization"))
	_, err := s.pool.Exec(r.Context(), `
		UPDATE identity.sessions SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenDigest(token), time.Now().UnixMilli())
	if err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userKey).(string)
	var user cloudUser
	if err := s.pool.QueryRow(r.Context(), `
		SELECT id::text, username FROM identity.users WHERE id = $1`, userID).Scan(&user.ID, &user.Username); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "serverTime": time.Now().UnixMilli()})
}

func validDevice(value string) bool {
	return len(value) > 0 && len(value) <= 128 && idPattern.MatchString(value)
}

func trimLabel(value string) string {
	runes := []rune(value)
	if len(runes) > 64 {
		return string(runes[:64])
	}
	return value
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
