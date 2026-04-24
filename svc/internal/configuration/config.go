package configuration

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
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
	JWTSecret          []byte
	JWTTTL             time.Duration
	RefreshTTL         time.Duration
	MSAuthRedirectURI  string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	AuthSuccessPath    string
	AuthErrorPath      string
	DefaultUserID      uuid.UUID
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

	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	jwtTTL := 24 * time.Hour
	if v := os.Getenv("JWT_TTL_HOURS"); v != "" {
		var h int
		if _, err := fmt.Sscanf(v, "%d", &h); err == nil && h > 0 {
			jwtTTL = time.Duration(h) * time.Hour
		}
	}

	refreshTTL := 30 * 24 * time.Hour
	if v := os.Getenv("REFRESH_TTL_DAYS"); v != "" {
		var d int
		if _, err := fmt.Sscanf(v, "%d", &d); err == nil && d > 0 {
			refreshTTL = time.Duration(d) * 24 * time.Hour
		}
	}

	defaultUID, err := uuid.Parse(getenv("AUTH_DEFAULT_USER_ID", "a0000001-0000-4000-8000-000000000001"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_DEFAULT_USER_ID: %w", err)
	}

	publicAPI := strings.TrimRight(getenv("APP_PUBLIC_URL", "http://localhost:8080"), "/")

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
		JWTSecret:        jwtSecret,
		JWTTTL:           jwtTTL,
		RefreshTTL:       refreshTTL,
		MSAuthRedirectURI: getenv("MS_AUTH_REDIRECT_URI", publicAPI+"/api/auth/microsoft/callback"),
		GoogleClientID:    os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI: getenv("GOOGLE_REDIRECT_URI", publicAPI+"/api/auth/google/callback"),
		AuthSuccessPath:   getenv("AUTH_SUCCESS_PATH", "/auth/callback"),
		AuthErrorPath:     getenv("AUTH_ERROR_PATH", "/auth/error"),
		DefaultUserID:     defaultUID,
	}
	if cfg.MSClientID == "" || cfg.MSClientSecret == "" || cfg.MSRedirectURI == "" {
		return Config{}, fmt.Errorf("MS_CLIENT_ID, MS_CLIENT_SECRET, and MS_REDIRECT_URI are required for mail connect")
	}
	if cfg.GoogleClientID != "" && (cfg.GoogleClientSecret == "" || cfg.GoogleRedirectURI == "") {
		return Config{}, fmt.Errorf("GOOGLE_CLIENT_ID set: also require GOOGLE_CLIENT_SECRET and GOOGLE_REDIRECT_URI")
	}
	return cfg, nil
}
