package configuration

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds environment-backed settings (spec §9).
type Config struct {
	ListenAddr         string
	DatabaseURL        string
	CORSOrigins        []string
	DashboardBaseURL   string
	OAuthSuccessPath   string
	OAuthErrorPath     string
	MSClientID         string
	MSClientSecret     string
	MSRedirectURI      string
	EncryptionKey      []byte
	OAuthStateTTL      time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	keyStr := os.Getenv("ENCRYPTION_KEY")
	if keyStr == "" {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY is required (32-byte base64 or raw string; use 32 ascii chars for dev)")
	}
	key := []byte(keyStr)
	if len(key) != 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (got %d)", len(key))
	}

	ttl := 15 * time.Minute
	if v := os.Getenv("OAUTH_STATE_TTL_SECONDS"); v != "" {
		var sec int
		if _, err := fmt.Sscanf(v, "%d", &sec); err == nil && sec > 0 {
			ttl = time.Duration(sec) * time.Second
		}
	}

	cfg := Config{
		ListenAddr:       getenv("LISTEN_ADDR", ":8080"),
		DatabaseURL:      getenv("DATABASE_URL", "file:./data.db?_foreign_keys=on"),
		CORSOrigins:      splitComma(os.Getenv("CORS_ORIGINS")),
		DashboardBaseURL: strings.TrimRight(getenv("DASHBOARD_BASE_URL", "http://localhost:5173"), "/"),
		OAuthSuccessPath: getenv("OAUTH_SUCCESS_PATH", "/accounts/connected"),
		OAuthErrorPath:   getenv("OAUTH_ERROR_PATH", "/accounts/error"),
		MSClientID:       os.Getenv("MS_CLIENT_ID"),
		MSClientSecret:   os.Getenv("MS_CLIENT_SECRET"),
		MSRedirectURI:    os.Getenv("MS_REDIRECT_URI"),
		EncryptionKey:    key,
		OAuthStateTTL:    ttl,
	}
	if cfg.MSClientID == "" || cfg.MSClientSecret == "" || cfg.MSRedirectURI == "" {
		return Config{}, fmt.Errorf("MS_CLIENT_ID, MS_CLIENT_SECRET, and MS_REDIRECT_URI are required for Phase 1")
	}
	return cfg, nil
}
