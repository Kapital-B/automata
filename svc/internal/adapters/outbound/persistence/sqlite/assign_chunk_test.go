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

// TestAssignChunkKeysetVisitsEveryMessage walks an account in chunks while
// already-scanned messages are removed underneath the scan, and asserts that
// nothing unscanned is skipped.
//
// The job declared a keyset cursor but encoded an integer offset. An offset
// counts rows behind it, so when rows above the cursor disappear -- retention
// purge, account cleanup -- everything below shifts up and the next chunk steps
// straight over messages it never saw. A keyset names the last row it read, so
// changes above it cannot displace anything.
func TestAssignChunkKeysetVisitsEveryMessage(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	assign := &appprojects.AssignService{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)

	if _, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling Upgrade", Code: "DC01"}); err != nil {
		t.Fatal(err)
	}

	// Every message names the project code, so a message the scan actually
	// visits is always committed. Anything left unassigned at the end was
	// skipped.
	const total = 90
	base := time.Now().UTC().Add(-24 * time.Hour)
	for i := 0; i < total; i++ {
		insertMsgAt(t, repo, accountID, "Regarding DC01 works", uuid.NewString(), "body",
			base.Add(time.Duration(i)*time.Minute))
	}

	// thread_assignments.run_id is a FK, so the run must exist.
	var cursor *driven.JobCursor
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "assign_projects", "api", "running",
		time.Now().UTC(), time.Time{}, nil, `{}`); err != nil {
		t.Fatal(err)
	}

	scanned := map[uuid.UUID]bool{}
	for step := 0; step < 20; step++ {
		res, err := assign.AssignAccountChunk(ctx, driven.RunContext{
			UserID: userID, AccountID: &accountID, RunID: runID, Cursor: cursor,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Done {
			break
		}
		if res.NextCursor == nil {
			t.Fatal("expected a cursor while work remains")
		}
		if cursor != nil && res.NextCursor.Value == cursor.Value {
			t.Fatal("cursor did not advance")
		}
		cursor = res.NextCursor

		// Retention removes the newest already-scanned messages between chunks.
		deleteNewestMessages(t, db, accountID, 5)
	}

	// Whatever survived must have been visited and committed.
	msgs, err := repo.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	skipped := 0
	for _, m := range msgs {
		scanned[m.ID] = true
		eff, err := repo.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		if eff.ProjectID == nil || eff.Status != "committed" {
			skipped++
		}
	}
	if skipped != 0 {
		t.Fatalf("%d of %d surviving messages were never scored: the chunk cursor skipped them",
			skipped, len(msgs))
	}
}

// deleteNewestMessages simulates retention or account cleanup removing rows
// above the scan position.
func deleteNewestMessages(t *testing.T, db *sql.DB, accountID uuid.UUID, n int) {
	t.Helper()
	_, err := db.Exec(`
		DELETE FROM messages
		WHERE id IN (
			SELECT id FROM messages WHERE account_id = ?
			ORDER BY received_at DESC, id DESC LIMIT ?
		)`, accountID.String(), n)
	if err != nil {
		t.Fatal(err)
	}
}

func insertMsgAt(t *testing.T, repo *sqlite.Repository, accountID uuid.UUID, subject, conv, body string, at time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	c := conv
	b := body
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID: id, AccountID: accountID, ProviderMessageID: id.String(), Subject: subject,
		FromJSON: `{"address":"a@b.com"}`, ConversationID: &c, BodyText: &b,
		ReceivedAt: at, CreatedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
