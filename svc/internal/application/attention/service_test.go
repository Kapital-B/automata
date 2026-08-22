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
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
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
