package main

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

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
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Hour)
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
	queueClient := asynqadapter.NewQueueClient(cfg.RedisAddr, cfg.AsynqPrefix)
	defer queueClient.Close()
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
	var summarizeSvc *appmessages.SummarizeService
	var autoDraftSvc *appmessages.AutoDraftService
	var forwardRulesSvc *appmessages.ForwardRulesService
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		llm := &llmadapter.OpenAIClient{
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
			APIKey:  cfg.LLMAPIKey,
		}
		categorizeSvc = &appmessages.CategorizeService{
			Messages: repo,
			LLM:      llm,
			JobRuns:  repo,
		}
		summarizeSvc = &appmessages.SummarizeService{
			Messages:  repo,
			Summaries: repo,
			LLM:       llm,
			JobRuns:   repo,
		}
		autoDraftSvc = &appmessages.AutoDraftService{
			Messages:   repo,
			Summaries:  repo,
			LLM:        llm,
			JobRuns:    repo,
			ModelLabel: cfg.LLMModel,
		}
		forwardRulesSvc = &appmessages.ForwardRulesService{
			Messages:  repo,
			Forwards:  repo,
			Accounts:  repo,
			OAuth:     oauth,
			Graph:     graph,
			Vault:     vault,
			LLM:       llm,
			JobRuns:   repo,
			ModelName: cfg.LLMModel,
		}
	} else {
		forwardRulesSvc = &appmessages.ForwardRulesService{
			Messages: repo,
			Forwards: repo,
			Accounts: repo,
			OAuth:    oauth,
			Graph:    graph,
			Vault:    vault,
			JobRuns:  repo,
		}
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: cfg.GlobalMaxConcurrentJobs,
			Queues: map[string]int{
				asynqadapter.QueueSync:       cfg.QueueSyncConcurrency,
				asynqadapter.QueueCategorize: cfg.QueueCategorizeConcurrency,
				asynqadapter.QueueSummarize:  cfg.QueueSummarizeConcurrency,
				asynqadapter.QueueDraft:      cfg.QueueDraftConcurrency,
				asynqadapter.QueueForward:    cfg.QueueForwardConcurrency,
			},
		},
	)
	sem := make(chan struct{}, cfg.GlobalMaxConcurrentJobs)
	mux := asynqadapter.NewWorkerMux(asynqadapter.WorkerDeps{
		Log:             log,
		SyncSvc:         syncSvc,
		CategorizeSvc:   categorizeSvc,
		SummarizeSvc:    summarizeSvc,
		AutoDraftSvc:    autoDraftSvc,
		ForwardRulesSvc: forwardRulesSvc,
		JobRuns:         repo,
		GlobalSemaphore: sem,
		Queue:           queueClient,
	})
	log.Info("worker listening", "redis", cfg.RedisAddr)
	if err := srv.Run(mux); err != nil {
		log.Error("worker", "err", err)
		os.Exit(1)
	}
}
