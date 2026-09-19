package postgres_test

import (
	"context"
	"database/sql"
	"net/url"
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

// withSearchPath pins the schema on the connection string rather than issuing
// `SET search_path` after connecting. The pool opens more than one connection,
// and a session-level SET only binds the connection that ran it — the rest
// would silently fall back to `public` and defeat per-test isolation.
func withSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func TestRepositoryContract(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AUTOMATA_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set AUTOMATA_TEST_POSTGRES_DSN to run postgres contract tests")
	}
	persistencetest.Run(t, func(t *testing.T) persistencetest.Handle {
		schema := "contract_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		ctx := context.Background()

		admin, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer admin.Close()
		if _, err := admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanup, err := sql.Open("pgx", dsn)
			if err != nil {
				return
			}
			defer cleanup.Close()
			_, _ = cleanup.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		})

		scopedDSN, err := withSearchPath(dsn, schema)
		if err != nil {
			t.Fatal(err)
		}
		db, counter, err := persistencetest.OpenCounting("pgx", scopedDSN)
		if err != nil {
			t.Fatal(err)
		}
		// Match the production pool (the factory defaults to 3 for
		// postgres/DSQL). A pool of 1 deadlocks any repository call that opens a
		// nested query while a result set is still streaming.
		db.SetMaxOpenConns(3)
		db.SetMaxIdleConns(3)
		db.SetConnMaxLifetime(time.Minute)
		t.Cleanup(func() { _ = db.Close() })

		if err := pgmigrate.Apply(ctx, db, factory.EnginePostgres); err != nil {
			t.Fatal(err)
		}
		return persistencetest.Handle{
			DB:      db,
			Repo:    postgres.NewRepository(db, 15*time.Minute),
			Counter: counter,
		}
	})
}
