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

func projectSvc(repo *sqlite.Repository) *appprojects.Service {
	return &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo,
		Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
}

func TestManualCreateAndTimelineOrder(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := projectSvc(repo)
	ctx := context.Background()
	userID, _, accountID := seedUserAccount(t, repo)
	p, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	early := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	late := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)

	conv := "conv-tl"
	msgID := uuid.New()
	body := "pump sizing"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "pm-tl",
		ReceivedAt: early, Subject: "Outlook: pump", FromJSON: `{}`,
		ConversationID: &conv, BodyText: &body,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &p.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}

	manual, err := svc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "teams", OccurredAt: late, Title: "Teams note",
		BodyText: "Consider 90 kW", ProjectID: &p.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.BodyText != "Consider 90 kW" {
		t.Fatalf("body=%q", manual.BodyText)
	}

	items, err := svc.GetTimeline(ctx, userID, p.ID, driven.TimelineFilter{Source: "all", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("want >=2 timeline items, got %d", len(items))
	}
	if items[0].Source != "manual" || items[0].ManualItemID == nil || *items[0].ManualItemID != manual.ID {
		t.Fatalf("first should be manual: %+v", items[0])
	}
	if items[1].Source != "mail" || items[1].MessageID == nil || *items[1].MessageID != msgID {
		t.Fatalf("second should be mail: %+v", items[1])
	}
}

func TestManualUnassignedAndAssign(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := projectSvc(repo)
	ctx := context.Background()
	userID, _, _ := seedUserAccount(t, repo)
	p, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	manual, err := svc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "whatsapp", OccurredAt: time.Now().UTC(), Title: "WA",
		BodyText: "approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.AssignmentStatus != "unassigned" || manual.ProjectID != nil {
		t.Fatalf("manual=%+v", manual)
	}

	list, err := svc.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range list {
		if it.Kind == "manual" && it.ManualItemID != nil && *it.ManualItemID == manual.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("manual not in unassigned")
	}

	updated, err := svc.AssignManualItem(ctx, userID, manual.ID, &p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProjectID == nil || *updated.ProjectID != p.ID || updated.AssignmentStatus != "committed" {
		t.Fatalf("updated=%+v", updated)
	}

	tl, err := svc.GetTimeline(ctx, userID, p.ID, driven.TimelineFilter{Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tl) != 1 {
		t.Fatalf("timeline manuals=%d", len(tl))
	}
}

func TestManualOrgIsolation(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := projectSvc(repo)
	ctx := context.Background()
	u1, _, _ := seedUserAccount(t, repo)
	u2, _, _ := seedUserAccount(t, repo)
	p1, _ := svc.Create(ctx, u1, appprojects.CreateProjectInput{Name: "A", Code: "AA01"})
	m, err := svc.CreateManualItem(ctx, u1, appprojects.CreateManualInput{
		Channel: "note", OccurredAt: time.Now().UTC(), Title: "private",
		BodyText: "secret", ProjectID: &p1.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetTimeline(ctx, u2, p1.ID, driven.TimelineFilter{})
	if err == nil {
		t.Fatalf("user2 must not load user1 project, got %d items", len(got))
	}
	if err != appprojects.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	_, err = svc.AssignManualItem(ctx, u2, m.ID, nil)
	if err != appprojects.ErrNotFound {
		t.Fatalf("want not found for other org, got %v", err)
	}
}

func TestManualParticipantsOnCommitted(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	svc := projectSvc(repo)
	ctx := context.Background()
	userID, orgID, _ := seedUserAccount(t, repo)
	p, _ := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "A", Code: "AA01"})
	contactID, err := repo.ResolveEmailContact(ctx, orgID, "bob@ex.com", "Bob", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "call", OccurredAt: time.Now().UTC(), Title: "Call",
		BodyText: "notes", ProjectID: &p.ID, ParticipantContactIDs: []uuid.UUID{contactID},
	})
	if err != nil {
		t.Fatal(err)
	}
	manuals, err := repo.ListManualItemsForProject(ctx, orgID, p.ID)
	if err != nil || len(manuals) != 1 {
		t.Fatalf("manuals=%v err=%v", manuals, err)
	}
	cids, err := repo.ListContactIDsForManualItem(ctx, orgID, manuals[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cids) != 1 || cids[0] != contactID {
		t.Fatalf("participants=%v", cids)
	}
}
