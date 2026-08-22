package facts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupFacts(t *testing.T, name string) (*sql.DB, *sqlite.Repository, *appfacts.Service, *appprojects.Service, uuid.UUID, uuid.UUID, uuid.UUID) {
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
	factSvc := &appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
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
	return db, repo, factSvc, projectSvc, userID, proj.ID, accountID
}

func TestInvalidSubjectKey(t *testing.T) {
	_, _, factSvc, _, userID, projectID, _ := setupFacts(t, "factbadkey")
	_, err := factSvc.Create(context.Background(), userID, projectID, appfacts.CreateInput{
		SubjectKey: "duty", Label: "Duty", Value: 75, Confirm: true,
	})
	if !errors.Is(err, appfacts.ErrInvalidSubjectKey) {
		t.Fatalf("want invalid subject, got %v", err)
	}
}

func TestCreateConfirmAndSupersede(t *testing.T) {
	_, _, factSvc, _, userID, projectID, _ := setupFacts(t, "factsupersede")
	ctx := context.Background()
	first, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Pump P-03 duty", Value: 75.0, Unit: strPtr("kW"), Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Versions) != 1 || first.Versions[0].Version.Status != "active" {
		t.Fatalf("expected one active version, got %+v", first.Versions)
	}
	activeID := first.Versions[0].Version.ID

	proposed, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Pump P-03 duty", Value: 90.0, Unit: strPtr("kW"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var proposedID uuid.UUID
	for _, v := range proposed.Versions {
		if v.Version.Status == "proposed" {
			proposedID = v.Version.ID
		}
	}
	if proposedID == uuid.Nil {
		t.Fatal("expected proposed version")
	}

	_, err = factSvc.Confirm(ctx, userID, proposedID, appfacts.ConfirmInput{})
	if !errors.Is(err, appfacts.ErrSupersedeRequired) {
		t.Fatalf("want supersede required, got %v", err)
	}

	confirmed, err := factSvc.Confirm(ctx, userID, proposedID, appfacts.ConfirmInput{SupersedesVersionID: &activeID})
	if err != nil {
		t.Fatal(err)
	}
	var activeCount, supersededCount int
	for _, v := range confirmed.Versions {
		switch v.Version.Status {
		case "active":
			activeCount++
			if v.Version.ValueText != "90" {
				t.Fatalf("active value %q", v.Version.ValueText)
			}
		case "superseded":
			supersededCount++
			if v.Version.SupersededByVersionID == nil || *v.Version.SupersededByVersionID != proposedID {
				t.Fatalf("superseded pointers %+v", v.Version)
			}
		}
	}
	if activeCount != 1 || supersededCount != 1 {
		t.Fatalf("active=%d superseded=%d", activeCount, supersededCount)
	}

	pos, err := factSvc.CurrentPosition(ctx, userID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pos.Facts) != 1 || pos.Facts[0].ValueText != "90" {
		t.Fatalf("current position %+v", pos.Facts)
	}
}

func TestRejectProposed(t *testing.T) {
	_, _, factSvc, _, userID, projectID, _ := setupFacts(t, "factreject")
	ctx := context.Background()
	view, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty", Value: 75, Confirm: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	vid := view.Versions[0].Version.ID
	rejected, err := factSvc.Reject(ctx, userID, vid)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Versions[0].Version.Status != "rejected" {
		t.Fatalf("status %s", rejected.Versions[0].Version.Status)
	}
}

func TestEvidenceCascadeLinkOnly(t *testing.T) {
	db, repo, factSvc, projectSvc, userID, projectID, accountID := setupFacts(t, "factevidence")
	ctx := context.Background()
	msgID := uuid.New()
	conv := "conv-fact"
	body := "75 kW"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "duty", BodyText: &body,
		FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}

	view, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty", Value: 75, Confirm: true,
		Evidence: []appfacts.EvidenceRef{{MessageID: &msgID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Versions[0].Evidence) != 1 {
		t.Fatalf("expected evidence, got %d", len(view.Versions[0].Evidence))
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, msgID.String()); err != nil {
		t.Fatal(err)
	}
	got, err := factSvc.Get(ctx, userID, view.Fact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fact.SubjectKey != "pump.p03.duty_kw" {
		t.Fatal("fact should remain after message delete")
	}
	if len(got.Versions) != 1 || got.Versions[0].Version.Status != "active" {
		t.Fatalf("version should remain %+v", got.Versions)
	}
	if len(got.Versions[0].Evidence) != 0 {
		t.Fatalf("evidence link should be gone, got %d", len(got.Versions[0].Evidence))
	}
}

func TestUniqueSubjectPerProject(t *testing.T) {
	_, _, factSvc, _, userID, projectID, _ := setupFacts(t, "factunique")
	ctx := context.Background()
	first, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty", Value: 75, Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Duty updated", Value: 80, Confirm: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Fact.ID != first.Fact.ID {
		t.Fatal("same subject should append version on existing fact")
	}
	if len(second.Versions) != 2 {
		t.Fatalf("want 2 versions, got %d", len(second.Versions))
	}
}

func strPtr(s string) *string { return &s }
