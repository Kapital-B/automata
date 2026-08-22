package decisions_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupDecisions(t *testing.T, name string) (*appdecisions.Service, uuid.UUID, uuid.UUID) {
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
	decisionSvc := &appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	userID, err := authSvc.Register(context.Background(), name+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	return decisionSvc, userID, proj.ID
}

func TestCreateConfirmDecision(t *testing.T) {
	svc, userID, projectID := setupDecisions(t, "decconfirm")
	ctx := context.Background()
	view, err := svc.Create(ctx, userID, projectID, appdecisions.CreateInput{
		Statement: "Proceed with 90 kW duty for Pump P-03", Confirm: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Decision.Status != "proposed" {
		t.Fatalf("want proposed, got %s", view.Decision.Status)
	}
	confirmed, err := svc.Confirm(ctx, userID, view.Decision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Decision.Status != "accepted" || confirmed.Decision.DecidedAt == nil {
		t.Fatalf("want accepted with decided_at, got %+v", confirmed.Decision)
	}
}
