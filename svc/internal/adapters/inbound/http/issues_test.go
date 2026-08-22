package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	_ "modernc.org/sqlite"
)

func TestIssuesHTTPOrgIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:issueshttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	issueSvc := &appissues.Service{
		Users: repo, Projects: repo, Issues: repo, Assignments: repo, Manuals: repo, Contacts: repo, Messages: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc, IssueSvc: issueSvc,
		Users: repo, Projects: repo, Assignments: repo, Issues: repo, Messages: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	userA, tokensA := registerAndLogin(t, authSvc, "ia@example.com", "password123")
	_, tokensB := registerAndLogin(t, authSvc, "ib@example.com", "password123")
	p, err := projectSvc.Create(context.Background(), userA, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"title": "Pump P-03"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/projects/"+p.ID.String()+"/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokensA.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", res.StatusCode)
	}
	var created map[string]any
	_ = json.NewDecoder(res.Body).Decode(&created)
	issueID, _ := created["id"].(string)
	if issueID == "" {
		t.Fatal("missing issue id")
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/issues/"+issueID, nil)
	req2.Header.Set("Authorization", "Bearer "+tokensB.AccessToken)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("user B should not see A issue, got %d", res2.StatusCode)
	}

	req3, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/projects/"+p.ID.String()+"/issues", nil)
	req3.Header.Set("Authorization", "Bearer "+tokensB.AccessToken)
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusNotFound && res3.StatusCode != http.StatusOK {
		t.Fatalf("unexpected list status %d", res3.StatusCode)
	}
	if res3.StatusCode == http.StatusOK {
		var list []map[string]any
		_ = json.NewDecoder(res3.Body).Decode(&list)
		if len(list) != 0 {
			t.Fatalf("user B must not list A issues")
		}
	}
}
