package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	TokenTTL      time.Duration
	AllowedOrigin map[string]struct{}
	BodyLimit     int64
	PhotoLimit    int64
	ClockSkew     time.Duration
}

func LoadConfig() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	ttlDays, err := readPositiveInt("TOKEN_TTL_DAYS", 30, 365)
	if err != nil {
		return Config{}, err
	}
	origins := make(map[string]struct{})
	for _, origin := range strings.Split(os.Getenv("CORS_ORIGINS"), ",") {
		if value := strings.TrimSpace(origin); value != "" {
			origins[value] = struct{}{}
		}
	}
	return Config{
		Addr:          addr,
		DatabaseURL:   databaseURL,
		TokenTTL:      time.Duration(ttlDays) * 24 * time.Hour,
		AllowedOrigin: origins,
		BodyLimit:     8 << 20,
		PhotoLimit:    6 << 20,
		ClockSkew:     5 * time.Minute,
	}, nil
}

func readPositiveInt(name string, fallback, max int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > max {
		return 0, fmt.Errorf("%s is outside its valid range", name)
	}
	return parsed, nil
}
