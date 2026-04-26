package main

import (
	"database/sql"
	"log/slog"
	"os"

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	llmadapter "github.com/Kapital-B/automata/svc/internal/adapters/outbound/llm"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/hibiken/asynq"
	_ "modernc.org/sqlite"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := configuration.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	db, err := sql.Open("sqlite", cfg.DatabaseURL)
	if err != nil {
		log.Error("db open", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	vault, err := security.NewAESGCMVault(cfg.EncryptionKey)
	if err != nil {
		log.Error("vault", "err", err)
		os.Exit(1)
	}
	repo := sqlite.NewRepository(db, cfg.OAuthStateTTL)
	graph := &microsoft.GraphClient{}
	oauth := &microsoft.OAuth{
		ClientID:     cfg.MSClientID,
		ClientSecret: cfg.MSClientSecret,
		RedirectURI:  cfg.MSRedirectURI,
	}
	syncSvc := &appmessages.SyncService{
		Accounts: repo,
		Messages: repo,
		OAuth:    oauth,
		Graph:    graph,
		Vault:    vault,
		JobRuns:  repo,
	}
	var categorizeSvc *appmessages.CategorizeService
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		categorizeSvc = &appmessages.CategorizeService{
			Messages: repo,
			LLM: &llmadapter.OpenAIClient{
				BaseURL: cfg.LLMBaseURL,
				Model:   cfg.LLMModel,
				APIKey:  cfg.LLMAPIKey,
			},
			JobRuns: repo,
		}
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: cfg.GlobalMaxConcurrentJobs,
			Queues: map[string]int{
				asynqadapter.QueueSync:       cfg.QueueSyncConcurrency,
				asynqadapter.QueueCategorize: cfg.QueueCategorizeConcurrency,
			},
		},
	)
	sem := make(chan struct{}, cfg.GlobalMaxConcurrentJobs)
	mux := asynqadapter.NewWorkerMux(asynqadapter.WorkerDeps{
		Log:             log,
		SyncSvc:         syncSvc,
		CategorizeSvc:   categorizeSvc,
		JobRuns:         repo,
		GlobalSemaphore: sem,
	})
	log.Info("worker listening", "redis", cfg.RedisAddr)
	if err := srv.Run(mux); err != nil {
		log.Error("worker", "err", err)
		os.Exit(1)
	}
}
