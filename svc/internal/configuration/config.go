package configuration

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config holds environment-backed settings (spec §9).
type Config struct {
	ListenAddr                 string
	DatabaseEngine             string
	DatabaseURL                string
	CORSOrigins                []string
	DashboardBaseURL           string
	OAuthSuccessPath           string
	OAuthErrorPath             string
	MSClientID                 string
	MSClientSecret             string
	MSRedirectURI              string
	EncryptionKey              []byte
	OAuthStateTTL              time.Duration
	JWTSecret                  []byte
	JWTTTL                     time.Duration
	RefreshTTL                 time.Duration
	MSAuthRedirectURI          string
	GoogleClientID             string
	GoogleClientSecret         string
	GoogleRedirectURI          string
	SlackClientID              string
	SlackClientSecret          string
	SlackRedirectURI           string
	SlackMode                  string
	SlackSuccessPath           string
	AuthSuccessPath            string
	AuthErrorPath              string
	DefaultUserID              uuid.UUID
	JobsInline                 bool
	JobTerminalRetention       time.Duration
	JobLeaseDuration           time.Duration
	JobPendingWakeAfter        time.Duration
	JobsTableName              string
	JobCursorSecret            []byte
	AWSRegion                  string
	AWSEndpoint                string
	BedrockModelID             string
	BedrockRuntimeEndpoint     string
	LLMBaseURL                 string
	LLMModel                   string
	LLMAPIKey                  string
	RedisAddr                  string
	AsynqPrefix                string
	QueueSyncConcurrency       int
	QueueCategorizeConcurrency int
	QueueSummarizeConcurrency  int
	QueueDraftConcurrency      int
	QueueForwardConcurrency    int
	GlobalMaxConcurrentJobs    int
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getenvInt(key string, def int) int {
	var out int
	if _, err := fmt.Sscanf(strings.TrimSpace(os.Getenv(key)), "%d", &out); err == nil && out > 0 {
		return out
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

// Load reads configuration from the environment (and Secrets Manager when *_SECRET_ID is set).
func Load() (Config, error) {
	keyStr, err := resolveSecret("ENCRYPTION_KEY", "ENCRYPTION_KEY_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY: %w", err)
	}
	if keyStr == "" {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY is required (32-byte base64 or raw string; use 32 ascii chars for dev), or set ENCRYPTION_KEY_SECRET_ID")
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

	jwtSecretStr, err := resolveSecret("JWT_SECRET", "JWT_SECRET_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("JWT_SECRET: %w", err)
	}
	jwtSecret := []byte(jwtSecretStr)
	if len(jwtSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes, or set JWT_SECRET_SECRET_ID")
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
	queueSyncConcurrency := 2
	if v := os.Getenv("JOB_QUEUE_SYNC_CONCURRENCY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			queueSyncConcurrency = n
		}
	}
	queueCategorizeConcurrency := 1
	if v := os.Getenv("JOB_QUEUE_CATEGORIZE_CONCURRENCY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			queueCategorizeConcurrency = n
		}
	}
	queueSummarizeConcurrency := 1
	if v := os.Getenv("JOB_QUEUE_SUMMARIZE_CONCURRENCY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			queueSummarizeConcurrency = n
		}
	}
	queueDraftConcurrency := 1
	if v := os.Getenv("JOB_QUEUE_DRAFT_CONCURRENCY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			queueDraftConcurrency = n
		}
	}
	queueForwardConcurrency := 1
	if v := os.Getenv("JOB_QUEUE_FORWARD_CONCURRENCY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			queueForwardConcurrency = n
		}
	}
	globalMaxConcurrentJobs := 2
	if v := os.Getenv("GLOBAL_MAX_CONCURRENT_JOBS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			globalMaxConcurrentJobs = n
		}
	}

	defaultUID, err := uuid.Parse(getenv("AUTH_DEFAULT_USER_ID", "a0000001-0000-4000-8000-000000000001"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_DEFAULT_USER_ID: %w", err)
	}

	jobLeaseDuration := time.Duration(getenvInt("JOB_LEASE_SECONDS", 960)) * time.Second
	jobPendingWakeAfter := time.Duration(getenvInt("JOB_PENDING_WAKE_AFTER_SECONDS", 120)) * time.Second
	jobTerminalRetention := time.Duration(getenvInt("JOB_TERMINAL_RETENTION_DAYS", 30)) * 24 * time.Hour

	publicAPI := strings.TrimRight(getenv("APP_PUBLIC_URL", "http://localhost:8080"), "/")
	jobCursorSecretStr, err := resolveSecret("JOB_CURSOR_SECRET", "JOB_CURSOR_SECRET_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("JOB_CURSOR_SECRET: %w", err)
	}
	if jobCursorSecretStr == "" {
		jobCursorSecretStr = string(jwtSecret)
	}
	jobCursorSecret := []byte(jobCursorSecretStr)

	msClientSecret, err := resolveSecret("MS_CLIENT_SECRET", "MS_CLIENT_SECRET_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("MS_CLIENT_SECRET: %w", err)
	}
	googleClientSecret, err := resolveOptionalSecret("GOOGLE_CLIENT_SECRET", "GOOGLE_CLIENT_SECRET_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("GOOGLE_CLIENT_SECRET: %w", err)
	}
	slackClientSecret, err := resolveOptionalSecret("SLACK_CLIENT_SECRET", "SLACK_CLIENT_SECRET_SECRET_ID")
	if err != nil {
		return Config{}, fmt.Errorf("SLACK_CLIENT_SECRET: %w", err)
	}

	cfg := Config{
		ListenAddr:                 getenv("LISTEN_ADDR", ":8080"),
		DatabaseEngine:             getenv("DATABASE_ENGINE", "sqlite"),
		DatabaseURL:                getenv("DATABASE_URL", "file:./data.db?_foreign_keys=on"),
		CORSOrigins:                splitComma(os.Getenv("CORS_ORIGINS")),
		DashboardBaseURL:           strings.TrimRight(getenv("DASHBOARD_BASE_URL", "http://localhost:5173"), "/"),
		OAuthSuccessPath:           getenv("OAUTH_SUCCESS_PATH", "/accounts/connected"),
		OAuthErrorPath:             getenv("OAUTH_ERROR_PATH", "/accounts/error"),
		MSClientID:                 os.Getenv("MS_CLIENT_ID"),
		MSClientSecret:             msClientSecret,
		MSRedirectURI:              os.Getenv("MS_REDIRECT_URI"),
		EncryptionKey:              key,
		OAuthStateTTL:              ttl,
		JWTSecret:                  jwtSecret,
		JWTTTL:                     jwtTTL,
		RefreshTTL:                 refreshTTL,
		MSAuthRedirectURI:          getenv("MS_AUTH_REDIRECT_URI", publicAPI+"/api/auth/microsoft/callback"),
		GoogleClientID:             os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:         googleClientSecret,
		GoogleRedirectURI:          getenv("GOOGLE_REDIRECT_URI", publicAPI+"/api/auth/google/callback"),
		SlackClientID:              os.Getenv("SLACK_CLIENT_ID"),
		SlackClientSecret:          slackClientSecret,
		SlackRedirectURI:           getenv("SLACK_REDIRECT_URI", publicAPI+"/api/connectors/callback"),
		SlackMode:                  os.Getenv("SLACK_MODE"),
		SlackSuccessPath:           getenv("SLACK_SUCCESS_PATH", "/accounts?connector=slack"),
		AuthSuccessPath:            getenv("AUTH_SUCCESS_PATH", "/auth/callback"),
		AuthErrorPath:              getenv("AUTH_ERROR_PATH", "/auth/error"),
		DefaultUserID:              defaultUID,
		JobsInline:                 getenvBool("JOBS_INLINE", false),
		JobTerminalRetention:       jobTerminalRetention,
		JobLeaseDuration:           jobLeaseDuration,
		JobPendingWakeAfter:        jobPendingWakeAfter,
		JobsTableName:              strings.TrimSpace(os.Getenv("JOBS_TABLE")),
		JobCursorSecret:            jobCursorSecret,
		AWSRegion:                  getenv("AWS_REGION", "us-east-1"),
		AWSEndpoint:                strings.TrimSpace(os.Getenv("AWS_ENDPOINT")),
		BedrockModelID:             getenv("BEDROCK_MODEL_ID", "eu.amazon.nova-2-lite-v1:0"),
		BedrockRuntimeEndpoint:     strings.TrimSpace(getenv("BEDROCK_RUNTIME_ENDPOINT", os.Getenv("AWS_ENDPOINT"))),
		LLMBaseURL:                 os.Getenv("LLM_BASE_URL"),
		LLMModel:                   os.Getenv("LLM_MODEL"),
		LLMAPIKey:                  os.Getenv("LLM_API_KEY"),
		RedisAddr:                  getenv("REDIS_ADDR", "localhost:6379"),
		AsynqPrefix:                getenv("ASYNQ_PREFIX", "automata"),
		QueueSyncConcurrency:       queueSyncConcurrency,
		QueueCategorizeConcurrency: queueCategorizeConcurrency,
		QueueSummarizeConcurrency:  queueSummarizeConcurrency,
		QueueDraftConcurrency:      queueDraftConcurrency,
		QueueForwardConcurrency:    queueForwardConcurrency,
		GlobalMaxConcurrentJobs:    globalMaxConcurrentJobs,
	}
	if len(cfg.JobCursorSecret) < 32 {
		return Config{}, fmt.Errorf("JOB_CURSOR_SECRET must be at least 32 bytes")
	}
	if cfg.MSClientID == "" || cfg.MSClientSecret == "" || cfg.MSRedirectURI == "" {
		return Config{}, fmt.Errorf("MS_CLIENT_ID, MS_CLIENT_SECRET, and MS_REDIRECT_URI are required for mail connect")
	}
	if cfg.GoogleClientID != "" && (cfg.GoogleClientSecret == "" || cfg.GoogleRedirectURI == "") {
		slog.Warn("GOOGLE_CLIENT_ID set without secret/redirect; disabling Google auth")
		cfg.GoogleClientID = ""
		cfg.GoogleClientSecret = ""
	}
	if cfg.SlackClientID != "" && !strings.EqualFold(cfg.SlackMode, "fake") &&
		(cfg.SlackClientSecret == "" || cfg.SlackRedirectURI == "") {
		slog.Warn("SLACK_CLIENT_ID set without secret/redirect; disabling Slack OAuth")
		cfg.SlackClientID = ""
		cfg.SlackClientSecret = ""
	}
	if cfg.LLMBaseURL != "" && cfg.LLMModel == "" {
		return Config{}, fmt.Errorf("LLM_BASE_URL set: also require LLM_MODEL")
	}
	return cfg, nil
}
