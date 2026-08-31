package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/composition"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	if runningInLambda() {
		lambda.Start(handle)
		return
	}
	if err := runCLI(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func handle(ctx context.Context, _ map[string]any) (map[string]string, error) {
	cfg, err := loadConfigFromFlags()
	if err != nil {
		return nil, err
	}
	log.Printf("migrate lambda start engine=%s", cfg.DatabaseEngine)
	if err := composition.ApplyMigrations(ctx, cfg); err != nil {
		log.Printf("migrate lambda failed: %v", err)
		return nil, err
	}
	log.Printf("migrate lambda ok engine=%s", cfg.DatabaseEngine)
	return map[string]string{
		"status":          "ok",
		"database_engine": cfg.DatabaseEngine,
	}, nil
}

func runCLI(ctx context.Context) error {
	cfg, err := loadConfigFromFlags()
	if err != nil {
		return err
	}
	if err := composition.ApplyMigrations(ctx, cfg); err != nil {
		return err
	}
	log.Printf("migrations applied for %s", cfg.DatabaseEngine)
	return nil
}

func loadConfigFromFlags() (configuration.Config, error) {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	engineFlag := fs.String("engine", envOr("DATABASE_ENGINE", "sqlite"), "database engine: sqlite|postgres|dsql")
	dsnFlag := fs.String("database-url", os.Getenv("DATABASE_URL"), "database connection string")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return configuration.Config{}, err
	}

	cfg := configuration.Config{
		DatabaseEngine: strings.TrimSpace(*engineFlag),
		DatabaseURL:    strings.TrimSpace(*dsnFlag),
	}
	if cfg.DatabaseURL == "" {
		return configuration.Config{}, fmt.Errorf("DATABASE_URL or -database-url is required")
	}
	return cfg, nil
}

func runningInLambda() bool {
	return strings.TrimSpace(os.Getenv("AWS_LAMBDA_RUNTIME_API")) != ""
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
