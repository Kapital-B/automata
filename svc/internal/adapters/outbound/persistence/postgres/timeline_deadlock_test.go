package postgres_test

import (
	"context"
	"database/sql"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
	pgmigrate "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/migrate"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/postgres"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// TestProjectTimelineOnSingleConnectionPool guards the deadlock on the hosted
// path.
//
// ListProjectTimeline used to resolve participants and issue links per row
// while the outer result set was still streaming, holding two connections at
// once. The factory gives Postgres/DSQL a pool of 3 (factory.go:77-83), so
// production survived on headroom rather than by design and could still starve
// under concurrency. At a pool of 1 the old code hangs outright.
func TestProjectTimelineOnSingleConnectionPool(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AUTOMATA_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set AUTOMATA_TEST_POSTGRES_DSN to run postgres tests")
	}
	ctx := context.Background()
	schema := "deadlock_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
	})

	scopedDSN, err := withSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The whole point of the test: one connection, as the tightest production
	// pool would give.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := pgmigrate.Apply(ctx, db, factory.EnginePostgres); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewRepository(db, 15*time.Minute)

	now := time.Now().UTC()
	userID := uuid.New()
	orgID, err := repo.CreateUserWithHomeOrg(ctx, userID, "tl@example.com", nil, now, "password", userID.String(), "tl@example.com")
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, domainaccounts.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "tl@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	if err := repo.CreateProject(ctx, domainprojects.ProjectRow{
		ID: projectID, OrganisationID: orgID, Name: "Timeline", Code: "DC06", CreatedAt: now, UpdatedAt: now,
	}, domainprojects.ProjectMemberRow{
		ID: uuid.New(), ProjectID: projectID, UserID: userID, Role: "owner", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	body := "body"
	for i := 0; i < 25; i++ {
		conv := "conv-" + uuid.NewString()
		msgID := uuid.New()
		if err := repo.UpsertMessage(ctx, domainmessages.MessageRow{
			ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(), Subject: "mail",
			FromJSON: `{"address":"a@b.com"}`, ConversationID: &conv, BodyText: &body,
			ReceivedAt: now.Add(-time.Duration(i) * time.Minute), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpsertThreadAssignment(ctx, domainprojects.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
			ProjectID: &projectID, Status: "committed", Reason: "seed", Source: "user",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	type result struct {
		items []domainprojects.TimelineItem
		err   error
	}
	done := make(chan result, 1)
	go func() {
		items, err := repo.ListProjectTimeline(ctx, userID, orgID, projectID, domainprojects.TimelineFilter{Limit: 100})
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
	case <-time.After(20 * time.Second):
		t.Fatal("ListProjectTimeline deadlocked on a single-connection pool")
	}
}
