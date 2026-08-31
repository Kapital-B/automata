package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kapital-B/automata/svc/internal/composition"
	"github.com/Kapital-B/automata/svc/internal/configuration"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := configuration.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	runtime, err := composition.Build(context.Background(), log, cfg, composition.Options{
		AutoMigrate: true,
		EnableAsynq: !cfg.JobsInline,
	})
	if err != nil {
		log.Error("compose", "err", err)
		os.Exit(1)
	}
	defer runtime.Close()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           runtime.Router,
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
