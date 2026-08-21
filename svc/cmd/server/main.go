package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	httphandler "github.com/Kapital-B/automata/svc/internal/adapters/inbound/http"
	googleoauth "github.com/Kapital-B/automata/svc/internal/adapters/outbound/google"
	llmadapter "github.com/Kapital-B/automata/svc/internal/adapters/outbound/llm"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/go-chi/cors"
	_ "modernc.org/sqlite"
)

const msSignInScopes = "openid offline_access profile email"

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

	msMailOAuth := &microsoft.OAuth{
		ClientID:     cfg.MSClientID,
		ClientSecret: cfg.MSClientSecret,
		RedirectURI:  cfg.MSRedirectURI,
	}
	msAuthOAuth := &microsoft.OAuth{
		ClientID:     cfg.MSClientID,
		ClientSecret: cfg.MSClientSecret,
		RedirectURI:  cfg.MSAuthRedirectURI,
		Scopes:       msSignInScopes,
	}
	graph := &microsoft.GraphClient{}

	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts:    repo,
		OAuthState:  repo,
		JobRuns:     repo,
		OAuth:       msMailOAuth,
		Graph:       graph,
		Vault:       vault,
		Dashboard:   cfg.DashboardBaseURL,
		SuccessPath: cfg.OAuthSuccessPath,
		ErrorPath:   cfg.OAuthErrorPath,
		StateTTL:    cfg.OAuthStateTTL,
	})

	resolveSvc := &appcontacts.ResolveService{
		Users:    repo,
		Messages: repo,
		Contacts: repo,
	}
	contactSvc := &appcontacts.Service{
		Users:    repo,
		Contacts: repo,
		Messages: repo,
	}
	projectSvc := &appprojects.Service{
		Users:       repo,
		Projects:    repo,
		Assignments: repo,
		Contacts:    repo,
		Messages:    repo,
	}
	assignSvc := &appprojects.AssignService{
		Users:       repo,
		Projects:    repo,
		Assignments: repo,
		Contacts:    repo,
		Messages:    repo,
		JobRuns:     repo,
	}

	syncSvc := &appmessages.SyncService{
		Accounts: repo,
		Messages: repo,
		OAuth:    msMailOAuth,
		Graph:    graph,
		Vault:    vault,
		JobRuns:  repo,
		Resolve:  resolveSvc,
		Assign:   assignSvc,
	}
	var categorizeSvc *appmessages.CategorizeService
	var summarizeSvc *appmessages.SummarizeService
	var autoDraftSvc *appmessages.AutoDraftService
	var forwardRulesSvc *appmessages.ForwardRulesService
	draftsSvc := &appmessages.DraftLifecycleService{
		Summaries: repo,
		Messages:  repo,
		Accounts:  repo,
		OAuth:     msMailOAuth,
		Graph:     graph,
		Vault:     vault,
	}
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
			OAuth:     msMailOAuth,
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
			OAuth:    msMailOAuth,
			Graph:    graph,
			Vault:    vault,
			JobRuns:  repo,
		}
	}
	_ = autoDraftSvc
	var googleClient *googleoauth.OAuth
	if cfg.GoogleClientID != "" {
		googleClient = &googleoauth.OAuth{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURI:  cfg.GoogleRedirectURI,
		}
	}

	authSvc := auth.NewService(repo, repo, repo, msAuthOAuth, googleClient, cfg.JWTSecret, cfg.JWTTTL, cfg.RefreshTTL)

	h := &httphandler.Handlers{
		Log:             log,
		AccountSvc:      accountSvc,
		SyncSvc:         syncSvc,
		CategorizeSvc:   categorizeSvc,
		SummarizeSvc:    summarizeSvc,
		AutoDraftSvc:    autoDraftSvc,
		DraftsSvc:       draftsSvc,
		ForwardRulesSvc: forwardRulesSvc,
		AuthSvc:         authSvc,
		ContactSvc:      contactSvc,
		ProjectSvc:      projectSvc,
		Accounts:        repo,
		Messages:        repo,
		JobRuns:         repo,
		Summaries:       repo,
		Forwards:        repo,
		Schedules:       repo,
		OAuthStates:     repo,
		Users:           repo,
		Contacts:        repo,
		Projects:        repo,
		Assignments:     repo,
		Dashboard:       cfg.DashboardBaseURL,
		SuccessPath:     cfg.OAuthSuccessPath,
		ErrorPath:       cfg.OAuthErrorPath,
		AuthSuccessPath: cfg.AuthSuccessPath,
		AuthErrorPath:   cfg.AuthErrorPath,
		StateTTL:        cfg.OAuthStateTTL,
		JWTSecret:       cfg.JWTSecret,
		JWTTTL:          cfg.JWTTTL,
		DefaultUserID:   cfg.DefaultUserID,
		JobQueue:        queueClient,
	}

	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			cutoff := time.Now().UTC().Add(-cfg.OAuthStateTTL)
			_ = repo.DeleteExpiredStates(context.Background(), cutoff)
		}
	}()

	go func() {
		scheduler := &schedulerService{
			log:       log,
			schedules: repo,
			accounts:  repo,
			jobRuns:   repo,
			queue:     queueClient,
		}
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			_ = scheduler.Tick(context.Background(), time.Now().UTC())
		}
	}()

	r := h.Routes()
	if len(cfg.CORSOrigins) > 0 {
		r = cors.New(cors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Content-Type", "X-Request-ID", "Authorization"},
			AllowCredentials: false,
		}).Handler(r)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
