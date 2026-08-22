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
