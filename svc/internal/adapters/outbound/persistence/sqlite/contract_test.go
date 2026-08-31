package sqlite_test

import (
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistencetest"
)

func TestRepositoryContract(t *testing.T) {
	persistencetest.Run(t, func(t *testing.T) persistencetest.Handle {
		db := openMigrated(t)
		return persistencetest.Handle{
			DB:   db,
			Repo: sqlite.NewRepository(db, 15*time.Minute),
		}
	})
}
