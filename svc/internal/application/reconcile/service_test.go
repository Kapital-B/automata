package reconcile_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupReconcile(t *testing.T, name string) (*sqlite.Repository, *appfacts.Service, *appreconcile.Service, uuid.UUID, uuid.UUID) {
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
	decisionSvc := &appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	reconcileSvc := &appreconcile.Service{
		Users: repo, Projects: repo, Interpretations: repo, FactsRepo: repo, Facts: factSvc,
		Decisions: decisionSvc, Contradictions: repo, JobRuns: repo,
	}
	userID, err := authSvc.Register(context.Background(), name+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	return repo, factSvc, reconcileSvc, userID, proj.ID
}

func insertPendingInterp(t *testing.T, repo *sqlite.Repository, orgID, projectID uuid.UUID, payload string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := repo.CreateInterpretation(context.Background(), driven.InterpretationRow{
		ID: id, OrganisationID: orgID, ProjectID: projectID, Status: "pending",
		PayloadJSON: payload, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestReconcileConfirmNewAndReinforce(t *testing.T) {
	repo, _, reconcileSvc, userID, projectID := setupReconcile(t, "reconnew")
	ctx := context.Background()
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{{
			"kind": "fact", "subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty",
			"value": 75.0, "unit": "kW", "confidence": 0.9,
		}},
	})
	insertPendingInterp(t, repo, orgID, projectID, string(payload))
	res, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProcessedInterpretations != 1 || len(res.Outcomes) != 1 || res.Outcomes[0].Outcome != "confirm_new" {
		t.Fatalf("unexpected result: %+v", res)
	}

	payload2, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{{
			"kind": "fact", "subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty",
			"value": 75.0, "unit": "kW", "confidence": 0.8,
		}},
	})
	insertPendingInterp(t, repo, orgID, projectID, string(payload2))
	res2, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Outcomes[0].Outcome != "reinforce" {
		t.Fatalf("want reinforce, got %+v", res2.Outcomes[0])
	}
}

func TestReconcileContradictionAndResolveSupersede(t *testing.T) {
	repo, factSvc, reconcileSvc, userID, projectID := setupReconcile(t, "reconcontr")
	ctx := context.Background()
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := factSvc.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Pump P-03 duty", Value: 75.0, Unit: strPtr("kW"), Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activeID := first.Versions[0].Version.ID

	payload, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{{
			"kind": "fact", "subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty",
			"value": 90.0, "unit": "kW", "confidence": 0.4,
		}},
	})
	insertPendingInterp(t, repo, orgID, projectID, string(payload))
	res, err := reconcileSvc.Run(ctx, userID, projectID, appreconcile.ReconcileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes[0].Outcome != "contradiction" || res.ContradictionsOpened != 1 {
		t.Fatalf("want contradiction, got %+v", res)
	}
	cid, err := uuid.Parse(res.Outcomes[0].ContradictionID)
	if err != nil {
		t.Fatal(err)
	}
	proposedID, err := uuid.Parse(res.Outcomes[0].VersionID)
	if err != nil {
		t.Fatal(err)
	}

	view, err := reconcileSvc.Resolve(ctx, userID, cid, appreconcile.ResolveInput{
		Resolution: "supersede", KeepFactVersionID: &proposedID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Contradiction.Status != "resolved" {
		t.Fatalf("want resolved, got %s", view.Contradiction.Status)
	}
	active, err := repo.GetActiveFactVersion(ctx, first.Fact.ID)
	if err != nil || active == nil {
		t.Fatalf("active: %v %#v", err, active)
	}
	if active.ID != proposedID {
		t.Fatalf("want proposed active, got %s (was %s)", active.ID, activeID)
	}
}

func strPtr(s string) *string { return &s }
