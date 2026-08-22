package projectai_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	_ "modernc.org/sqlite"
)

type fakeLLM struct {
	content string
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	return &driven.LLMResponse{Content: f.content}, nil
}

func TestAskCitesActiveFact(t *testing.T) {
	db, err := sql.Open("sqlite", "file:ask1?mode=memory&cache=shared")
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
	askSvc := &appprojectai.Service{
		Users: repo, Projects: repo, Facts: repo, Decisions: repo, Issues: repo, Timeline: repo, JobRuns: repo,
	}
	userID, err := authSvc.Register(context.Background(), "ask@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	view, err := factSvc.Create(ctx, userID, proj.ID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Pump P-03 duty", Value: 90.0, Unit: strPtr("kW"), Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	vid := view.Versions[0].Version.ID.String()
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"answer":         "Pump P-03 duty is 90 kW",
		"citations":      []map[string]string{{"type": "fact_version", "id": vid}},
		"confidence":     0.95,
	})
	askSvc.LLM = &fakeLLM{content: string(payload)}
	ans, err := askSvc.Ask(ctx, userID, proj.ID, "What is Pump P-03 duty?")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Answer == "" || len(ans.Citations) == 0 || ans.Citations[0].ID != vid {
		t.Fatalf("unexpected answer: %+v", ans)
	}
}

func strPtr(s string) *string { return &s }
