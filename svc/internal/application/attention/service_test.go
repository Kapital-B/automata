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
