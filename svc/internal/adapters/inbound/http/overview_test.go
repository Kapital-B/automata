package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	appattention "github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appoverview "github.com/Kapital-B/automata/svc/internal/application/overview"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type overviewFixture struct {
	srv        *httptest.Server
	token      string
	repo       *sqlite.Repository
	projectSvc *appprojects.Service
	userID     uuid.UUID
}

func newOverviewFixture(t *testing.T, dbName string) overviewFixture {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo, Manuals: repo,
	}
	attentionSvc := &appattention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo,
		Decisions: repo, Contradictions: repo, Summaries: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		AttentionSvc: attentionSvc,
		OverviewSvc: &appoverview.Service{
			Users: repo, Projects: repo, Attention: attentionSvc, Triage: repo,
		},
		Users: repo, Messages: repo, Projects: repo, Assignments: repo, Contacts: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	userID, tokens := registerAndLogin(t, authSvc, dbName+"@example.com", "password123")
	return overviewFixture{srv: srv, token: tokens.AccessToken, repo: repo, projectSvc: projectSvc, userID: userID}
}

func (f overviewFixture) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestOverviewHTTP(t *testing.T) {
	f := newOverviewFixture(t, "overviewhttp")
	ctx := context.Background()
	now := time.Now().UTC()

	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	orgID, err := f.repo.GetHomeOrganisationID(ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.CreateContradiction(ctx, driven.ContradictionRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: p.ID, Status: "open",
		Summary: "conflict", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	status, out := f.get(t, "/api/overview")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	counts := out["counts"].(map[string]any)
	if int(counts["open_contradictions"].(float64)) != 1 {
		t.Errorf("open_contradictions = %v, want 1", counts["open_contradictions"])
	}
	if int(counts["active_projects"].(float64)) != 1 {
		t.Errorf("active_projects = %v, want 1", counts["active_projects"])
	}
	projects := out["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	row := projects[0].(map[string]any)
	if row["code"] != "DC01" {
		t.Errorf("code = %v", row["code"])
	}
	if row["last_activity_at"] == nil {
		t.Error("expected a last_activity_at from the contradiction")
	}
}

func TestActivityHTTP(t *testing.T) {
	f := newOverviewFixture(t, "activityhttp")
	ctx := context.Background()
	now := time.Now().UTC()

	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	orgID, err := f.repo.GetHomeOrganisationID(ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	decided := now.Add(-time.Hour)
	if err := f.repo.CreateDecision(ctx, driven.DecisionRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: p.ID,
		Statement: "Proceed with 90 kW", Status: "accepted", Source: "llm",
		DecidedAt: &decided, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: decided,
	}); err != nil {
		t.Fatal(err)
	}

	status, out := f.get(t, "/api/activity?limit=25")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (proposed + accepted)", len(items))
	}
	first := items[0].(map[string]any)
	if first["kind"] != "decision_accepted" {
		t.Errorf("newest kind = %v, want decision_accepted", first["kind"])
	}
	if first["project_code"] != "DC01" {
		t.Errorf("project_code = %v", first["project_code"])
	}
	if first["source"] != "llm" {
		t.Errorf("source = %v, want llm", first["source"])
	}
	if first["ref_type"] != "decision" {
		t.Errorf("ref_type = %v", first["ref_type"])
	}

	// Filtering to one kind narrows the feed.
	status, out = f.get(t, "/api/activity?kind=decision_accepted")
	if status != http.StatusOK {
		t.Fatalf("filtered status = %d", status)
	}
	if got := len(out["items"].([]any)); got != 1 {
		t.Errorf("filtered items = %d, want 1", got)
	}

	// An unknown kind is an error, not a silently unfiltered feed.
	status, out = f.get(t, "/api/activity?kind=not_a_kind")
	if status != http.StatusBadRequest {
		t.Errorf("unknown kind status = %d, want 400", status)
	}
}

func TestActivityRequiresAuth(t *testing.T) {
	f := newOverviewFixture(t, "activityauth")
	for _, path := range []string{"/api/overview", "/api/activity"} {
		req, _ := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s unauthenticated status = %d, want 401", path, res.StatusCode)
		}
	}
}
