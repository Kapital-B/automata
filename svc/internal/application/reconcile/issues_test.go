package reconcile_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupIssueReconcile(t *testing.T, name string) (*sqlite.Repository, *appissues.Service, *appreconcile.Service, uuid.UUID, uuid.UUID, uuid.UUID) {
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
		Users: repo, Projects: repo, Issues: repo, Contacts: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	factSvc := &appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	decisionSvc := &appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	reconcileSvc := &appreconcile.Service{
		Users: repo, Projects: repo, Interpretations: repo, FactsRepo: repo, Facts: factSvc,
		Decisions: decisionSvc, Contradictions: repo, Issues: issueSvc, IssuesRepo: repo, JobRuns: repo,
	}
	ctx := context.Background()
	userID, err := authSvc.Register(ctx, name+"@example.com", "password123")
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
	return repo, issueSvc, reconcileSvc, userID, orgID, proj.ID
}

func issuePayload(title, note string) string {
	return `{"candidates":[{"kind":"issue","title":"` + title + `","statement":"` + note + `","confidence":0.8}]}`
}

// Issues are created outright, not proposed: an issue is a prompt to look at
// something, not an assertion about what is true.
func TestReconcileCreatesIssueFromCandidate(t *testing.T) {
	repo, _, reconcileSvc, userID, orgID, projectID := setupIssueReconcile(t, "reconcileissue1")
	ctx := context.Background()
	insertPendingInterp(t, repo, orgID, projectID, issuePayload("P-03 seal is leaking", "Reported on site Tuesday"))

	res, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Outcome != "confirm_new" {
		t.Fatalf("outcomes = %+v, want one confirm_new", res.Outcomes)
	}
	if res.Outcomes[0].IssueID == "" {
		t.Error("outcome should carry the created issue id")
	}

	issues, err := repo.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if issues[0].Title != "P-03 seal is leaking" {
		t.Errorf("title = %q", issues[0].Title)
	}
	if issues[0].Status != "open" {
		t.Errorf("status = %q, want open — issues are created, not proposed", issues[0].Status)
	}
	if issues[0].DiscardedAt != nil {
		t.Error("a new issue should not be discarded")
	}
}

// The same problem restated must not create a second issue.
func TestReconcileReinforcesMatchingIssue(t *testing.T) {
	repo, _, reconcileSvc, userID, orgID, projectID := setupIssueReconcile(t, "reconcileissue2")
	ctx := context.Background()

	insertPendingInterp(t, repo, orgID, projectID, issuePayload("P-03 seal is leaking", "first report"))
	if _, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	// Restated with different casing and spacing.
	insertPendingInterp(t, repo, orgID, projectID, issuePayload("p-03  Seal Is Leaking", "second report"))
	res, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Outcome != "reinforce" {
		t.Fatalf("outcomes = %+v, want one reinforce", res.Outcomes)
	}

	issues, err := repo.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1 — a restatement must not create a second", len(issues))
	}
}

// A discarded issue stays discarded: extraction must not resurrect it.
func TestDiscardedIssueIsNotRecreated(t *testing.T) {
	repo, issueSvc, reconcileSvc, userID, orgID, projectID := setupIssueReconcile(t, "reconcileissue3")
	ctx := context.Background()

	insertPendingInterp(t, repo, orgID, projectID, issuePayload("Noise from the AI", "not useful"))
	if _, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	issues, err := repo.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}

	if _, err := issueSvc.Discard(ctx, userID, issues[0].ID); err != nil {
		t.Fatal(err)
	}
	discarded, err := repo.GetIssue(ctx, orgID, issues[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if discarded.DiscardedAt == nil {
		t.Fatal("expected the issue to be discarded")
	}
	// Discarding is not resolving: the status is untouched, so the signal for
	// whether extraction is useful stays intact.
	if discarded.Status == "resolved" {
		t.Error("discarding must not be recorded as resolving")
	}

	// The same candidate arrives again.
	insertPendingInterp(t, repo, orgID, projectID, issuePayload("Noise from the AI", "again"))
	if _, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, iss := range after {
		if iss.DiscardedAt == nil {
			live++
		}
	}
	// A recurrence becomes a new issue rather than reviving the discarded one.
	if live != 1 {
		t.Fatalf("live issues = %d, want 1", live)
	}
	for _, iss := range after {
		if iss.ID == issues[0].ID && iss.DiscardedAt == nil {
			t.Error("the discarded issue was revived")
		}
	}
}

// Issue candidates are ignored rather than erroring when issues are not wired.
func TestReconcileIgnoresIssuesWhenNotConfigured(t *testing.T) {
	repo, _, _, userID, orgID, projectID := setupIssueReconcile(t, "reconcileissue4")
	ctx := context.Background()
	bare := &appreconcile.Service{
		Users: repo, Projects: repo, Interpretations: repo, FactsRepo: repo,
		Contradictions: repo, JobRuns: repo,
	}
	insertPendingInterp(t, repo, orgID, projectID, issuePayload("Something", "note"))
	res, err := bare.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Outcome != "ignore" {
		t.Fatalf("outcomes = %+v, want one ignore", res.Outcomes)
	}
}
