package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistencetest"
)

func TestRepositoryContract(t *testing.T) {
	persistencetest.Run(t, func(t *testing.T) persistencetest.Handle {
		db, counter := openMigratedCounting(t)
		return persistencetest.Handle{
			DB:      db,
			Repo:    sqlite.NewRepository(db, 15*time.Minute),
			Counter: counter,
		}
	})
}

// openMigratedCounting mirrors openMigrated but routes the connection through a
// statement-counting driver so the contract suite can assert that hot read
// paths issue a bounded number of queries.
func openMigratedCounting(t *testing.T) (*sql.DB, *persistencetest.QueryCounter) {
	t.Helper()
	db, counter, err := persistencetest.OpenCounting("sqlite", "file:"+uuid.New().String()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db, counter
}
