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

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	httphandler "github.com/Kapital-B/automata/svc/internal/adapters/inbound/http"
	googleoauth "github.com/Kapital-B/automata/svc/internal/adapters/outbound/google"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
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

	syncSvc := &appmessages.SyncService{
		Accounts: repo,
		Messages: repo,
		OAuth:    msMailOAuth,
		Graph:    graph,
		Vault:    vault,
		JobRuns:  repo,
	}

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
		AuthSvc:         authSvc,
		Accounts:        repo,
		Messages:        repo,
		OAuthStates:     repo,
		Users:           repo,
		Dashboard:       cfg.DashboardBaseURL,
		SuccessPath:     cfg.OAuthSuccessPath,
		ErrorPath:       cfg.OAuthErrorPath,
		AuthSuccessPath: cfg.AuthSuccessPath,
		AuthErrorPath:   cfg.AuthErrorPath,
		StateTTL:        cfg.OAuthStateTTL,
		JWTSecret:       cfg.JWTSecret,
		JWTTTL:          cfg.JWTTTL,
		DefaultUserID:   cfg.DefaultUserID,
	}

	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			cutoff := time.Now().UTC().Add(-cfg.OAuthStateTTL)
			_ = repo.DeleteExpiredStates(context.Background(), cutoff)
		}
	}()

	r := h.Routes()
	if len(cfg.CORSOrigins) > 0 {
		r = cors.New(cors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
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
