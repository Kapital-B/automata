package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
	pgmigrate "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/migrate"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/postgres"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistencetest"
	"github.com/google/uuid"
)

func TestRepositoryContract(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AUTOMATA_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set AUTOMATA_TEST_POSTGRES_DSN to run postgres contract tests")
	}
	persistencetest.Run(t, func(t *testing.T) persistencetest.Handle {
		schema := "contract_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		ctx := context.Background()
		db, err := factory.Open(ctx, factory.Config{
			Engine:          factory.EnginePostgres,
			DatabaseURL:     dsn,
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.ExecContext(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
			_ = db.Close()
		})
		if _, err := db.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `SET search_path TO "`+schema+`"`); err != nil {
			t.Fatal(err)
		}
		if err := pgmigrate.Apply(ctx, db, factory.EnginePostgres); err != nil {
			t.Fatal(err)
		}
		return persistencetest.Handle{
			DB:   db,
			Repo: postgres.NewRepository(db, 15*time.Minute),
		}
	})
}
