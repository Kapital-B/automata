package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
)

// TestProjectTimelineOnSingleConnectionPool is the deadlock guard.
//
// ListProjectTimeline used to resolve participants and issue links per row
// while the outer result set was still streaming. That holds two connections
// at once, so on a single-connection pool -- which is exactly what the factory
// gives SQLite, and within the 1-3 it gives Postgres/DSQL -- the second query
// waits forever for a connection the first will not release until it finishes.
//
// The call is run with a deadline so a regression fails the suite instead of
// hanging CI.
func TestProjectTimelineOnSingleConnectionPool(t *testing.T) {
	db := openMigratedPool(t, 1)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Contacts: repo,
		Messages: repo, Manuals: repo, Timeline: repo,
	}
	ctx := context.Background()
	userID, orgID, accountID := seedUserAccount(t, repo)

	p, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	// Enough rows that a per-row lookup would be unmistakable.
	for i := 0; i < 25; i++ {
		conv := "conv-" + uuid.NewString()
		msgID := insertMsg(t, repo, accountID, "Regarding DC01", conv, "body")
		if err := repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
			ProjectID: &p.ID, Status: "committed", Reason: "seed", Source: "user",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
		_ = msgID
	}

	type result struct {
		items []driven.TimelineItem
		err   error
	}
	done := make(chan result, 1)
	go func() {
		items, err := svc.GetTimeline(ctx, userID, p.ID, driven.TimelineFilter{Limit: 100})
		done <- result{items, err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if len(res.items) != 25 {
			t.Fatalf("timeline returned %d items, want 25", len(res.items))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("ListProjectTimeline deadlocked on a single-connection pool")
	}
}

// openMigratedPool mirrors openMigrated but pins the connection pool size so a
// test can reproduce the production pool exactly.
func openMigratedPool(t *testing.T, maxConns int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+uuid.New().String()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
