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

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	_ "modernc.org/sqlite"
)

// The "Check now" verb has to queue the same chain the debounce queues, and it
// has to stay quiet when one is already in flight — otherwise a second click
// reads as an error to the operator.
func TestExtractProjectQueuesChain(t *testing.T) {
	db, err := sql.Open("sqlite", "file:extracthttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	store := memoryjobs.NewStore()
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		Users: repo, Projects: repo, Assignments: repo, Messages: repo,
		JobEnqueuer: &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()},
		JobStore:    store,
		JWTSecret:   jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	userA, tokensA := registerAndLogin(t, authSvc, "xa@example.com", "password123")
	_, tokensB := registerAndLogin(t, authSvc, "xb@example.com", "password123")
	p, err := projectSvc.Create(context.Background(), userA, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC02"})
	if err != nil {
		t.Fatal(err)
	}

	post := func(token string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/projects/"+p.ID.String()+"/extract", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	status, out := post(tokensA.AccessToken)
	if status != http.StatusAccepted {
		t.Fatalf("first extract status %d (%v)", status, out)
	}
	if out["status"] != "queued" {
		t.Fatalf("first extract said %v, want queued", out["status"])
	}

	// A second click lands on the project lock. That is the debounce doing its
	// job, so the caller sees success rather than a conflict.
	status2, out2 := post(tokensA.AccessToken)
	if status2 != http.StatusAccepted {
		t.Fatalf("second extract status %d (%v)", status2, out2)
	}

	// The queued head is the interpret step with reconcile behind it.
	page, err := store.List(context.Background(), driven.JobListFilter{UserID: userA, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Jobs) != 1 {
		t.Fatalf("queued %d jobs, want 1", len(page.Jobs))
	}
	job := page.Jobs[0]
	if job.JobType != appjobs.TypeInterpretProject {
		t.Fatalf("head job is %q, want %q", job.JobType, appjobs.TypeInterpretProject)
	}
	if len(job.RemainingJobs) != 1 || job.RemainingJobs[0] != appjobs.TypeReconcileProject {
		t.Fatalf("remaining jobs %v, want [%s]", job.RemainingJobs, appjobs.TypeReconcileProject)
	}
	if job.Payload.ProjectID == nil || *job.Payload.ProjectID != p.ID {
		t.Fatalf("payload project %v, want %s", job.Payload.ProjectID, p.ID)
	}

	// A non-member cannot make a project think.
	if status, _ := post(tokensB.AccessToken); status != http.StatusNotFound {
		t.Fatalf("non-member extract status %d, want 404", status)
	}
}
