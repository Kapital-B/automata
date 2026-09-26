package attention_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestAttentionIncludesProvisionalDecision(t *testing.T) {
	db, err := sql.Open("sqlite", "file:attn1?mode=memory&cache=shared")
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
	decisionSvc := &appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	attn := &attention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo, Decisions: repo, Contradictions: repo,
	}
	userID, err := authSvc.Register(context.Background(), "attn@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = decisionSvc.Create(ctx, userID, proj.ID, appdecisions.CreateInput{
		Statement: "Approve vendor quote", Confirm: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}).Create(ctx, userID, proj.ID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty", Value: 90.0, Confirm: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := attn.ForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts.ProvisionalDecision < 1 || res.Counts.ProvisionalFact < 1 {
		t.Fatalf("want provisional decision+fact, got counts %+v items %+v", res.Counts, res.Items)
	}
}

func TestAttentionMergesMailActionItems(t *testing.T) {
	db, err := sql.Open("sqlite", "file:attn-mail?mode=memory&cache=shared")
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
	attn := &attention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo, Decisions: repo, Contradictions: repo,
		Summaries: repo,
	}
	ctx := context.Background()
	userID, err := authSvc.Register(ctx, "attn-mail@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}).Create(ctx, userID, proj.ID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty", Value: 90.0, Confirm: false,
	}); err != nil {
		t.Fatal(err)
	}

	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "attn-mail@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	body := "Please reply to the invoice"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "Invoice", BodyText: &body, FromJSON: `{}`,
	}); err != nil {
		t.Fatal(err)
	}
	actionID := uuid.New()
	runID := uuid.New()
	now := time.Now().UTC()
	if err := repo.InsertJobRun(ctx, runID, accountID, "summarize", "api", "success", now, now, nil, `{}`); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertActionItems(ctx, []driven.ActionItemRow{{
		ID: actionID, UserID: userID, AccountID: accountID, MessageID: msgID,
		RunID: runID, Text: "Reply to invoice", Status: "open",
		CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}

	res, err := attn.ForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts.MailActionItem != 1 {
		t.Fatalf("want mail_action_item=1, got counts %+v items %+v", res.Counts, res.Items)
	}
	if res.Counts.ProvisionalFact < 1 {
		t.Fatalf("want project fact still present, got counts %+v", res.Counts)
	}
	var mail *attention.Item
	for i := range res.Items {
		if res.Items[i].WhyMe == attention.WhyMailActionItem {
			mail = &res.Items[i]
			break
		}
	}
	if mail == nil || mail.AccountID != accountID.String() || mail.MessageID != msgID.String() {
		t.Fatalf("mail item missing account/message: %+v", mail)
	}
	if mail.RefID != actionID.String() {
		t.Fatalf("want ref_id %s, got %+v", actionID, mail)
	}
	ids, err := attn.ProjectIDsNeedingInput(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ids[proj.ID]; !ok {
		t.Fatalf("want DC01 in attention project ids, got %+v", ids)
	}
}

// A to-do from mail belongs where its message is filed, and to the issue
// whose trail carries the message, without either being stored on the to-do.
func TestAttentionPlacesMailToDosOnProjectAndIssue(t *testing.T) {
	db, err := sql.Open("sqlite", "file:attn-todo-place?mode=memory&cache=shared")
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
	attn := &attention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo, Decisions: repo, Contradictions: repo,
		Summaries: repo, Assignments: repo,
	}
	ctx := context.Background()
	userID, err := authSvc.Register(ctx, "attn-todo@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "attn-todo@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "summarize", "api", "success", now, now, nil, `{}`); err != nil {
		t.Fatal(err)
	}
	newMsg := func(subject string) uuid.UUID {
		id := uuid.New()
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: id, AccountID: accountID, ProviderMessageID: id.String(),
			ReceivedAt: now, Subject: subject, FromJSON: `{}`,
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	filed := newMsg("Pump duty")     // filed to DC01 and on the issue
	unfiled := newMsg("Pump follow") // not filed, but on the issue
	loose := newMsg("Invoice")       // neither

	if err := repo.UpsertMessageOverride(ctx, driven.AssignmentRow{
		ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, MessageID: &filed,
		ProjectID: &proj.ID, Status: "committed", Source: "user", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	issueID := uuid.New()
	if err := repo.CreateIssue(ctx, driven.IssueRow{
		ID: issueID, OrganisationID: orgID, ProjectID: proj.ID, Title: "Pump P-03 duty",
		Status: "open", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []uuid.UUID{filed, unfiled} {
		msg := m
		if err := repo.AddIssueItem(ctx, driven.IssueItemRow{ID: uuid.New(), IssueID: issueID, MessageID: &msg, AddedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	todoFor := map[uuid.UUID]uuid.UUID{}
	for _, m := range []uuid.UUID{filed, unfiled, loose} {
		id := uuid.New()
		todoFor[m] = id
		if err := repo.InsertActionItems(ctx, []driven.ActionItemRow{{
			ID: id, UserID: userID, AccountID: accountID, MessageID: m, RunID: runID,
			Text: "Do something", Status: "open", CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := attn.ForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]attention.Item{}
	for _, it := range res.Items {
		byRef[it.RefID] = it
	}
	for _, m := range []uuid.UUID{filed, unfiled} {
		it := byRef[todoFor[m].String()]
		if it.ProjectID != proj.ID.String() || it.ProjectName != "Cooling" {
			t.Errorf("to-do on %s: want project DC01, got %+v", m, it)
		}
		if it.IssueID != issueID.String() || it.IssueTitle != "Pump P-03 duty" {
			t.Errorf("to-do on %s: want the issue, got %+v", m, it)
		}
		if it.OccurredAt.IsZero() {
			t.Errorf("to-do on %s: want a date, got zero", m)
		}
	}
	if it := byRef[todoFor[loose].String()]; it.ProjectID != "" || it.IssueID != "" {
		t.Errorf("loose to-do should have no project or issue, got %+v", it)
	}

	// The project page's Needs you carries the two placed to-dos, not the loose one.
	projRes, err := attn.ForProject(ctx, userID, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projRes.Counts.MailActionItem != 2 {
		t.Errorf("project to-dos = %d, want 2: %+v", projRes.Counts.MailActionItem, projRes.Items)
	}
	total, byProject, err := attn.CountsForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || byProject[proj.ID] != 2 {
		t.Errorf("counts total=%d byProject=%d, want 3 and 2", total, byProject[proj.ID])
	}

	// Filing the message elsewhere moves the to-do with it: another project
	// wins over the issue's.
	other, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Roof", Code: "RF02"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertMessageOverride(ctx, driven.AssignmentRow{
		ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, MessageID: &filed,
		ProjectID: &other.ID, Status: "committed", Source: "user", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	res, err = attn.ForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.RefID == todoFor[filed].String() && (it.ProjectID != other.ID.String() || it.IssueID != "") {
			t.Errorf("refiled to-do should follow its message and leave the issue, got %+v", it)
		}
	}
}
