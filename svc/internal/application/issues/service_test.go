package issues_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupIssues(t *testing.T, name string) (*sql.DB, *sqlite.Repository, *appissues.Service, *appprojects.Service, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
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
	repo := sqlite.NewRepository(db, 15*time.Minute)
	authSvc := auth.NewService(repo, repo, repo, nil, nil, []byte("abcdefghijklmnopqrstuvwxyz123456"), time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	issueSvc := &appissues.Service{
		Users: repo, Projects: repo, Issues: repo, Assignments: repo, Manuals: repo, Contacts: repo, Messages: repo,
	}
	userID, err := authSvc.Register(context.Background(), name+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: name + "@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return db, repo, issueSvc, projectSvc, userID, proj.ID, accountID
}

func TestCreateIssueDefaultsAssigneeToCaller(t *testing.T) {
	_, _, issueSvc, _, userID, projectID, _ := setupIssues(t, "issuecreate")
	view, err := issueSvc.Create(context.Background(), userID, projectID, appissues.CreateInput{Title: "Pump P-03"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Issue.AssigneeUserID == nil || *view.Issue.AssigneeUserID != userID {
		t.Fatalf("expected assignee caller, got %+v", view.Issue.AssigneeUserID)
	}
	if view.Issue.Status != "open" {
		t.Fatalf("status %s", view.Issue.Status)
	}
}

func TestRejectDualAssignee(t *testing.T) {
	_, repo, issueSvc, _, userID, projectID, _ := setupIssues(t, "issuedual")
	orgID, _ := repo.GetHomeOrganisationID(context.Background(), userID)
	contactID := uuid.New()
	now := time.Now().UTC()
	if err := repo.CreateContact(context.Background(), driven.ContactRow{
		ID: contactID, OrganisationID: orgID, DisplayName: "Alex", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	cid, uid := contactID, userID
	_, err := issueSvc.Create(context.Background(), userID, projectID, appissues.CreateInput{
		Title: "Bad", AssigneeUserID: &uid, AssigneeContactID: &cid,
	})
	if !errors.Is(err, appissues.ErrDualAssignee) {
		t.Fatalf("want dual assignee err, got %v", err)
	}
}

func TestAttachMailAndManualAndConflict(t *testing.T) {
	_, repo, issueSvc, projectSvc, userID, projectID, accountID := setupIssues(t, "issueattach")
	ctx := context.Background()
	msgID := uuid.New()
	conv := "conv-1"
	body := "pump sizing"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "Outlook pump", BodyText: &body,
		FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}
	manual, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "teams", OccurredAt: time.Now().UTC(), Title: "Teams note", BodyText: "90 kW",
		ProjectID: &projectID,
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := issueSvc.Create(ctx, userID, projectID, appissues.CreateInput{Title: "Pump P-03"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = issueSvc.AddItem(ctx, userID, view.Issue.ID, appissues.ItemRef{MessageID: &msgID})
	if err != nil {
		t.Fatal(err)
	}
	view, err = issueSvc.AddItem(ctx, userID, view.Issue.ID, appissues.ItemRef{ManualItemID: &manual.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(view.Items))
	}
	_, err = issueSvc.AddItem(ctx, userID, view.Issue.ID, appissues.ItemRef{MessageID: &msgID})
	if !errors.Is(err, appissues.ErrItemConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestDeleteMessageSurvivesIssue(t *testing.T) {
	db, repo, issueSvc, projectSvc, userID, projectID, accountID := setupIssues(t, "issuedel")
	ctx := context.Background()
	msgID := uuid.New()
	conv := "conv-del"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "pmdel",
		ReceivedAt: time.Now().UTC(), Subject: "gone", FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}
	view, err := issueSvc.Create(ctx, userID, projectID, appissues.CreateInput{
		Title: "Survives", ItemRefs: []appissues.ItemRef{{MessageID: &msgID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, msgID.String()); err != nil {
		t.Fatal(err)
	}
	got, err := issueSvc.Get(ctx, userID, view.Issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Issue.Title != "Survives" {
		t.Fatal("issue gone")
	}
	if len(got.Items) != 0 {
		t.Fatalf("link should cascade away, got %d", len(got.Items))
	}
}

func TestAwaitingMeAndPatchStatus(t *testing.T) {
	_, _, issueSvc, _, userID, projectID, _ := setupIssues(t, "issueawait")
	ctx := context.Background()
	view, err := issueSvc.Create(ctx, userID, projectID, appissues.CreateInput{Title: "Wait"})
	if err != nil {
		t.Fatal(err)
	}
	if view.AwaitingMe {
		t.Fatal("open should not be awaiting_me")
	}
	st := "awaiting_input"
	view, err = issueSvc.Update(ctx, userID, view.Issue.ID, appissues.UpdateInput{Status: &st})
	if err != nil {
		t.Fatal(err)
	}
	if !view.AwaitingMe {
		t.Fatal("expected awaiting_me")
	}
}

func TestTimelineUnassignedToIssue(t *testing.T) {
	_, repo, issueSvc, projectSvc, userID, projectID, _ := setupIssues(t, "issuetimeline")
	ctx := context.Background()
	manual, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "note", OccurredAt: time.Now().UTC(), Title: "A", BodyText: "b", ProjectID: &projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := issueSvc.Create(ctx, userID, projectID, appissues.CreateInput{Title: "I"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issueSvc.AddItem(ctx, userID, view.Issue.ID, appissues.ItemRef{ManualItemID: &manual.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "note", OccurredAt: time.Now().UTC(), Title: "Unlinked", BodyText: "x", ProjectID: &projectID,
	}); err != nil {
		t.Fatal(err)
	}
	orgID, _ := repo.GetHomeOrganisationID(ctx, userID)
	all, err := repo.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{Source: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 timeline items, got %d", len(all))
	}
	filt, err := repo.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{
		Source: "all", UnassignedToIssue: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filt) != 1 || filt[0].Title != "Unlinked" {
		t.Fatalf("filter got %+v", filt)
	}
	for _, it := range all {
		if it.ManualItemID != nil && *it.ManualItemID == manual.ID {
			if it.IssueID == nil || *it.IssueID != view.Issue.ID {
				t.Fatalf("missing issue_id on linked item")
			}
			return
		}
	}
	t.Fatal("linked item not found")
}

// An issue shows the caller's to-dos from mail on its trail, and resolving it
// can mark them done in the same step. It never touches to-dos elsewhere.
func TestIssueTodosFromTrailAndCompleteOnResolve(t *testing.T) {
	_, repo, issueSvc, projectSvc, userID, projectID, accountID := setupIssues(t, "issuetodos")
	issueSvc.Summaries = repo
	ctx := context.Background()
	now := time.Now().UTC()
	newMsg := func(subject, conv string) uuid.UUID {
		id := uuid.New()
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: id, AccountID: accountID, ProviderMessageID: id.String(),
			ReceivedAt: now, Subject: subject, FromJSON: `{}`, ConversationID: &conv,
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	onTrail := newMsg("Pump duty", "c-1")
	elsewhere := newMsg("Invoice", "c-2")
	if _, err := projectSvc.AssignMessage(ctx, userID, onTrail, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "summarize", "api", "success", now, now, nil, `{}`); err != nil {
		t.Fatal(err)
	}
	trailTodo, otherTodo := uuid.New(), uuid.New()
	if err := repo.InsertActionItems(ctx, []driven.ActionItemRow{
		{ID: trailTodo, UserID: userID, AccountID: accountID, MessageID: onTrail, RunID: runID, Text: "Reply to Jan", Status: "open", CreatedAt: now, UpdatedAt: now},
		{ID: otherTodo, UserID: userID, AccountID: accountID, MessageID: elsewhere, RunID: runID, Text: "Pay invoice", Status: "open", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatal(err)
	}

	view, err := issueSvc.Create(ctx, userID, projectID, appissues.CreateInput{Title: "Pump P-03"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = issueSvc.AddItem(ctx, userID, view.Issue.ID, appissues.ItemRef{MessageID: &onTrail})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Todos) != 1 || view.Todos[0].ID != trailTodo {
		t.Fatalf("want the trail's one to-do, got %+v", view.Todos)
	}

	openCount := func() int {
		items, err := repo.ListOpenActionItems(ctx, userID, nil)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	resolved, reopened := "resolved", "open"

	// Resolving without asking leaves the to-dos alone.
	if _, err := issueSvc.Update(ctx, userID, view.Issue.ID, appissues.UpdateInput{Status: &resolved}); err != nil {
		t.Fatal(err)
	}
	if n := openCount(); n != 2 {
		t.Fatalf("resolve alone closed to-dos: %d open, want 2", n)
	}
	// Asking to complete them on an update that does not resolve does nothing.
	if _, err := issueSvc.Update(ctx, userID, view.Issue.ID, appissues.UpdateInput{Status: &reopened, CompleteTodos: true}); err != nil {
		t.Fatal(err)
	}
	if n := openCount(); n != 2 {
		t.Fatalf("reopening closed to-dos: %d open, want 2", n)
	}

	view, err = issueSvc.Update(ctx, userID, view.Issue.ID, appissues.UpdateInput{Status: &resolved, CompleteTodos: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Todos) != 0 {
		t.Fatalf("resolved issue still lists to-dos: %+v", view.Todos)
	}
	left, err := repo.ListOpenActionItems(ctx, userID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ID != otherTodo {
		t.Fatalf("want only the unrelated to-do open, got %+v", left)
	}
}
