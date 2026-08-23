package projectai_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestSelectAskAcrossProjectsPrefersAttentionThenRecent(t *testing.T) {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	attentionID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	oldQuietID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	projects := make([]driven.ProjectRow, 0, 10)
	for i := 0; i < 8; i++ {
		projects = append(projects, driven.ProjectRow{
			ID:        uuid.New(),
			Code:      fmt.Sprintf("R%02d", i),
			Name:      fmt.Sprintf("Recent %d", i),
			UpdatedAt: base.Add(time.Duration(i) * time.Hour),
		})
	}
	projects = append(projects,
		driven.ProjectRow{ID: attentionID, Code: "DC01", Name: "Cooling", UpdatedAt: base.Add(-48 * time.Hour)},
		driven.ProjectRow{ID: oldQuietID, Code: "ZZ99", Name: "Archive", UpdatedAt: base.Add(-72 * time.Hour)},
	)
	selected := appprojectai.SelectAskAcrossProjects(projects, map[uuid.UUID]struct{}{attentionID: {}}, 8)
	if len(selected) != 8 {
		t.Fatalf("want cap 8, got %d", len(selected))
	}
	if selected[0].Code != "DC01" {
		t.Fatalf("want attention project first, got %s", selected[0].Code)
	}
	for _, p := range selected {
		if p.ID == oldQuietID {
			t.Fatalf("quiet old project should be dropped by the cap: %+v", selected)
		}
	}
}

type capturingLLM struct {
	lastUser string
	content  string
}

func (c *capturingLLM) ChatCompletion(_ context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	for _, m := range messages {
		if m.Role == "user" {
			c.lastUser = m.Content
		}
	}
	return &driven.LLMResponse{Content: c.content}, nil
}

func TestAskAcrossEvalCapsContextToAttentionAndRecent(t *testing.T) {
	db, err := sql.Open("sqlite", "file:ask-eval?mode=memory&cache=shared")
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
	attn := &attention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo, Decisions: repo, Contradictions: repo, Summaries: repo,
	}
	askSvc := &appprojectai.Service{
		Users: repo, Projects: repo, Facts: repo, Decisions: repo, Issues: repo, Timeline: repo, JobRuns: repo,
		Attention: attn,
	}
	ctx := context.Background()
	userID, err := authSvc.Register(ctx, "ask-eval@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}

	dc01, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := factSvc.Create(ctx, userID, dc01.ID, appfacts.CreateInput{
		SubjectKey: "pump.p03.duty_kw", Label: "Pump P-03 duty", Value: 90.0, Unit: strPtr("kW"), Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}).Create(ctx, userID, dc01.ID, appdecisions.CreateInput{
		Statement: "Approve vendor quote", Confirm: false,
	}); err != nil {
		t.Fatal(err)
	}
	// Age DC01 so recency alone would drop it; attention (proposed fact) should keep it.
	if _, err := db.Exec(`UPDATE projects SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-72*time.Hour).Format(time.RFC3339Nano), dc01.ID.String()); err != nil {
		t.Fatal(err)
	}

	droppedCode := ""
	for i := 0; i < 9; i++ {
		p, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{
			Name: fmt.Sprintf("Site %d", i), Code: fmt.Sprintf("S%02d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			droppedCode = p.Code
			if _, err := db.Exec(`UPDATE projects SET updated_at = ? WHERE id = ?`,
				time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano), p.ID.String()); err != nil {
				t.Fatal(err)
			}
		}
	}

	vid := view.Versions[0].Version.ID.String()
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"answer":         "On DC01, Pump P-03 duty is 90 kW",
		"citations":      []map[string]string{{"type": "fact_version", "id": vid}},
		"confidence":     0.9,
	})
	llm := &capturingLLM{content: string(payload)}
	askSvc.LLM = llm

	ans, err := askSvc.AskAcross(ctx, userID, "What is Pump P-03 duty?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ans.Answer, "90") {
		t.Fatalf("eval answer missing grounded value: %+v", ans)
	}
	if !strings.Contains(llm.lastUser, "code=DC01") {
		t.Fatalf("eval context dropped attention project DC01:\n%s", llm.lastUser)
	}
	if droppedCode != "" && strings.Contains(llm.lastUser, "code="+droppedCode) {
		t.Fatalf("eval context included capped-out project %s:\n%s", droppedCode, llm.lastUser)
	}
	if len(ans.Citations) == 0 || ans.Citations[0].ProjectCode != "DC01" {
		t.Fatalf("eval citations %+v", ans.Citations)
	}
}
