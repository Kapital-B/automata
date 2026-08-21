package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

func seedUserAccount(t *testing.T, repo *sqlite.Repository) (userID, orgID, accountID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	userID = uuid.New()
	orgID, err := repo.CreateUserWithHomeOrg(ctx, userID, userID.String()+"@ex.com", nil, now, "password", userID.String(), userID.String()+"@ex.com")
	if err != nil {
		t.Fatal(err)
	}
	accountID = uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: userID.String() + "@ex.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return userID, orgID, accountID
}

func insertMsg(t *testing.T, repo *sqlite.Repository, accountID uuid.UUID, subject, conv string, body string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	convPtr := &conv
	bodyPtr := &body
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID: id, AccountID: accountID, ProviderMessageID: id.String(),
		Subject: subject, FromJSON: `{"address":"a@b.com"}`, BodyText: bodyPtr,
		ConversationID: convPtr, ReceivedAt: now, HasAttachments: false,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProjectCodeUniquePerOrg(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	u1, _, _ := seedUserAccount(t, repo)
	u2, _, _ := seedUserAccount(t, repo)

	if _, err := svc.Create(ctx, u1, appprojects.CreateProjectInput{Name: "A", Code: "DC01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, u1, appprojects.CreateProjectInput{Name: "B", Code: "dc01"}); err != appprojects.ErrCodeTaken {
		t.Fatalf("want ErrCodeTaken got %v", err)
	}
	// Other org can reuse code
	if _, err := svc.Create(ctx, u2, appprojects.CreateProjectInput{Name: "C", Code: "DC01"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectInsertsSingleMember(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, _, _ := seedUserAccount(t, repo)
	p, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01", MemberRole: "ME"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := repo.CountProjectMembers(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("members=%d", n)
	}
	m, err := repo.GetProjectMember(ctx, p.ID, userID)
	if err != nil || m == nil || m.Role != "ME" {
		t.Fatalf("member=%+v err=%v", m, err)
	}
}

func TestAssignThreadAppliesToSiblingWithoutOverride(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)
	p, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	m1 := insertMsg(t, repo, accountID, "First", "conv-1", "hi")
	m2 := insertMsg(t, repo, accountID, "Second", "conv-1", "hi again")

	if _, err := svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}
	eff, err := svc.EffectiveAssignment(ctx, userID, m2)
	if err != nil {
		t.Fatal(err)
	}
	if eff.ProjectID == nil || *eff.ProjectID != p.ID || eff.Scope != "thread" {
		t.Fatalf("eff=%+v", eff)
	}
}

func TestMessageOverrideSurvivesThreadReassign(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)
	p1, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "A", Code: "AA01"})
	p2, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "B", Code: "BB01"})
	m1 := insertMsg(t, repo, accountID, "One", "conv-x", "body")
	m2 := insertMsg(t, repo, accountID, "Two", "conv-x", "body")

	_, _ = svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p1.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	})
	_, _ = svc.AssignMessage(ctx, userID, m2, appprojects.AssignInput{
		ProjectID: &p2.ID, Scope: domainprojects.ScopeMessage, Status: domainprojects.StatusCommitted,
	})
	_, _ = svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p1.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	})

	eff2, err := svc.EffectiveAssignment(ctx, userID, m2)
	if err != nil {
		t.Fatal(err)
	}
	if eff2.ProjectID == nil || *eff2.ProjectID != p2.ID || eff2.Scope != "message" {
		t.Fatalf("override lost: %+v", eff2)
	}

	cleared, err := svc.ClearOverride(ctx, userID, m2)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ProjectID == nil || *cleared.ProjectID != p1.ID || cleared.Scope != "thread" {
		t.Fatalf("clear override: %+v", cleared)
	}
}

func TestAssignDoesNotCreateProjectMembers(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, orgID, accountID := seedUserAccount(t, repo)
	p, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "A", Code: "AA01"})
	m1 := insertMsg(t, repo, accountID, "Hello", "c1", "x")
	contactID, err := repo.ResolveEmailContact(ctx, orgID, "sarah@ex.com", "Sarah", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
		ID: uuid.New(), OrganisationID: orgID, ContactID: contactID, Role: "from", MessageID: &m1,
	})
	_, err = svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := repo.CountProjectMembers(ctx, p.ID)
	if n != 1 {
		t.Fatalf("members should stay 1, got %d", n)
	}
}

func TestAutoAssignSiblingCodeNameAmbiguous(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	assign := &appprojects.AssignService{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo, JobRuns: repo}
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)
	p, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling Upgrade", Code: "DC01", Keywords: []string{"chiller"}})
	p2, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Other", Code: "OT02"})

	// Sibling
	m1 := insertMsg(t, repo, accountID, "kickoff", "sib", "hello")
	m2 := insertMsg(t, repo, accountID, "followup", "sib", "hello")
	_, _ = svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	})
	// Clear thread so m2 needs assign, then sibling finder sees m1
	// Actually after assign, thread already covers m2. Delete thread and re-assign only via override on m1 to test sibling.
	_ = repo.DeleteThreadAssignment(ctx, accountID, "sib")
	_, _ = svc.AssignMessage(ctx, userID, m1, appprojects.AssignInput{
		ProjectID: &p.ID, Scope: domainprojects.ScopeMessage, Status: domainprojects.StatusCommitted,
	})
	if err := assign.AssignAfterSync(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	effSib, _ := svc.EffectiveAssignment(ctx, userID, m2)
	if effSib.ProjectID == nil || *effSib.ProjectID != p.ID {
		t.Fatalf("sibling assign failed: %+v", effSib)
	}

	// Code committed
	m3 := insertMsg(t, repo, accountID, "Regarding DC01 tomorrow", "code-conv", "please review")
	if err := assign.AssignAfterSync(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	effCode, _ := svc.EffectiveAssignment(ctx, userID, m3)
	if effCode.ProjectID == nil || *effCode.ProjectID != p.ID || effCode.Status != "committed" {
		t.Fatalf("code assign: %+v", effCode)
	}

	// Name provisional
	m4 := insertMsg(t, repo, accountID, "Cooling Upgrade update", "name-conv", "status")
	if err := assign.AssignAfterSync(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	effName, _ := svc.EffectiveAssignment(ctx, userID, m4)
	if effName.ProjectID == nil || *effName.ProjectID != p.ID || effName.Status != "provisional" {
		t.Fatalf("name assign: %+v", effName)
	}

	// Ambiguous codes → leave unassigned
	m5 := insertMsg(t, repo, accountID, "DC01 and OT02", "amb", "both")
	_ = p2 // silence
	before, _ := svc.EffectiveAssignment(ctx, userID, m5)
	if err := assign.AssignAfterSync(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.EffectiveAssignment(ctx, userID, m5)
	if after.ProjectID != nil {
		t.Fatalf("ambiguous should stay unassigned: before=%+v after=%+v", before, after)
	}
}

func TestUnassignedSummaryCounts(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	assign := &appprojects.AssignService{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo}
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)
	_, _ = svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling Upgrade", Code: "DC01"})
	_ = insertMsg(t, repo, accountID, "no match here", "u1", "body")
	_ = insertMsg(t, repo, accountID, "Cooling Upgrade please", "u2", "body")
	_ = assign.AssignAfterSync(ctx, userID, accountID)
	sum, err := svc.UnassignedSummary(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Unassigned < 1 || sum.Provisional < 1 {
		t.Fatalf("summary=%+v", sum)
	}
}
