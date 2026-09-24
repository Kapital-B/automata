package projects_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
)

// placeEverythingLLM files every thread it is shown under one project, as a
// model that finds everything vaguely related would.
type placeEverythingLLM struct {
	code  string
	shown []string
}

func (l *placeEverythingLLM) ChatCompletion(ctx context.Context, msgs []driven.LLMMessage) (*driven.LLMResponse, error) {
	var refs []string
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		l.shown = append(l.shown, m.Content)
		for _, line := range strings.Split(m.Content, "\n") {
			// Threads are listed as "- ref=t1".
			if ref, ok := strings.CutPrefix(strings.TrimSpace(line), "- ref="); ok {
				refs = append(refs, ref)
			}
		}
	}
	var parts []string
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf(`{"ref":%q,"project_code":%q,"confidence":0.8,"reason":"looks related"}`, r, l.code))
	}
	return &driven.LLMResponse{Content: `{"schema_version":1,"assignments":[` + strings.Join(parts, ",") + `]}`}, nil
}

// Triaged mail kept coming back: every sync queued the assign_projects job,
// which re-scored every thread in the mailbox and let the model's provisional
// guess replace the operator's decision.
func TestAssignJobLeavesTriagedThreadsAlone(t *testing.T) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()
	if _, err := repo.CreateUserWithHomeOrg(ctx, userID, "u@ex.com", nil, now, "password", userID.String(), "u@ex.com"); err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{UserID: userID, ID: accountID, Label: "Work", Provider: "m365", MsAccountKind: "work", PrimaryEmail: "u@ex.com", ConnectionStatus: "connected"}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	svc := &appprojects.Service{Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo}
	cooling, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Office move", Code: "OF02"}); err != nil {
		t.Fatal(err)
	}
	msg := func(subject, conv string, age time.Duration) uuid.UUID {
		id := uuid.New()
		body := subject + " body"
		c := conv
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: id, AccountID: accountID, ProviderMessageID: id.String(), Subject: subject,
			FromJSON: `{"address":"someone@x.com"}`, BodyText: &body, ConversationID: &c,
			ReceivedAt: now.Add(-age), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	newsletter := msg("Weekly newsletter", "c-news", time.Minute)
	chiller := msg("Chiller delivery", "c-chiller", 2*time.Minute)
	fresh := msg("Pump quote", "c-pump", 3*time.Minute)

	yes := true
	coolingID := cooling.ID
	res, err := svc.AssignBatch(ctx, userID, []appprojects.BatchAssignItem{
		{Kind: "message", ID: newsletter, NotRelevant: &yes},
		{Kind: "message", ID: chiller, ProjectID: &coolingID},
	})
	if err != nil || !res[0].OK || !res[1].OK {
		t.Fatalf("triage: %v %+v", err, res)
	}

	llm := &placeEverythingLLM{code: "OF02"}
	assign := &appprojects.AssignService{Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo, LLM: llm}
	run := driven.RunContext{RunID: uuid.New(), UserID: userID, AccountID: &accountID}
	for i := 0; ; i++ {
		out, err := assign.AssignAccountChunk(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		if out.Done || i > 10 {
			break
		}
		run.Cursor = out.NextCursor
	}

	// The model never saw the triaged threads...
	for _, p := range llm.shown {
		if strings.Contains(p, "Weekly newsletter") || strings.Contains(p, "Chiller delivery") {
			t.Fatalf("a triaged thread was sent to the model:\n%s", p)
		}
	}
	// ...they kept the operator's decision...
	if row, _ := repo.GetThreadAssignment(ctx, accountID, "c-news"); row == nil || row.NotRelevantAt == nil || row.Source != "user" {
		t.Errorf("dismissal lost: %+v", row)
	}
	if row, _ := repo.GetThreadAssignment(ctx, accountID, "c-chiller"); row == nil || row.ProjectID == nil || *row.ProjectID != coolingID || row.Status != "committed" {
		t.Errorf("filing lost: %+v", row)
	}
	// ...and only the undecided thread is in triage, with the model's guess.
	queue, err := repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].MessageID == nil || *queue[0].MessageID != fresh || queue[0].Status != "provisional" {
		t.Fatalf("triage queue = %+v, want only the undecided thread, as a suggestion", queue)
	}
}
