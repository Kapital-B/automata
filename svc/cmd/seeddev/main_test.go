package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestSeedCreatesThreadedMailAndIsRepeatable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 0)
	if _, err := repo.CreateUserWithHomeOrg(ctx, uuid.New(), "seed@example.test", nil, time.Now(), "", "", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := run(ctx, "seed@example.test", path, "sqlite", 42); err != nil {
			t.Fatal(err)
		}
	}
	for query, want := range map[string]int{
		"SELECT COUNT(*) FROM projects":                                                 5,
		"SELECT COUNT(*) FROM messages":                                                 200,
		"SELECT COUNT(*) FROM thread_assignments":                                       45,
		"SELECT COUNT(DISTINCT conversation_id) FROM messages":                          45,
		"SELECT MIN(n) FROM (SELECT COUNT(*) n FROM messages GROUP BY conversation_id)": 1,
		"SELECT MAX(n) FROM (SELECT COUNT(*) n FROM messages GROUP BY conversation_id)": 8,
	} {
		var got int
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestSeedRejectsRemotePostgres(t *testing.T) {
	err := run(context.Background(), "seed@example.test", "postgres://user:pass@db.example.com:5432/automata", "postgres", 42)
	if err == nil {
		t.Fatal("expected remote Postgres to be rejected")
	}
}
