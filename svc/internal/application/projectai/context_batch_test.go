package projectai_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistencetest"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
)

type promptCapturingLLM struct{ prompt string }

func (c *promptCapturingLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	c.prompt = messages[len(messages)-1].Content
	return &driven.LLMResponse{Content: `{"schema_version":1,"answer":"ok","citations":[],"confidence":0.1}`}, nil
}

// The context for a question must cost a fixed number of statements however
// many facts a project has. It used to issue two per fact.
func TestAskContextStatementsDoNotGrowWithFacts(t *testing.T) {
	db, counter, err := persistencetest.OpenCounting("sqlite", "file:askbatch?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	authSvc := auth.NewService(repo, repo, repo, nil, nil, []byte("abcdefghijklmnopqrstuvwxyz123456"), time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	llm := &promptCapturingLLM{}
	askSvc := &appprojectai.Service{
		Users: repo, Projects: repo, Facts: repo, Decisions: repo, Issues: repo, Timeline: repo, LLM: llm,
	}
	ctx := context.Background()
	userID, err := authSvc.Register(ctx, "askbatch@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	// Another member's project in the same organisation must stay out of
	// the across-projects context.
	otherID, err := authSvc.Register(ctx, "other@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	hidden := uuid.New()
	if err := repo.CreateProject(ctx, driven.ProjectRow{ID: hidden, OrganisationID: orgID, Name: "Hidden", Code: "HX99", CreatedAt: now, UpdatedAt: now},
		driven.ProjectMemberRow{ID: uuid.New(), ProjectID: hidden, UserID: otherID, Role: "owner", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	added := 0
	addFacts := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			added++
			factID, verID, evidence := uuid.New(), uuid.New(), uuid.New()
			if err := repo.CreateFact(ctx, driven.FactRow{ID: factID, OrganisationID: orgID, ProjectID: proj.ID,
				SubjectKey: fmt.Sprintf("s%02d", added), Label: fmt.Sprintf("Fact %02d", added), CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateFactVersion(ctx, driven.FactVersionRow{ID: verID, FactID: factID, Status: "active",
				ValueJSON: `{}`, ValueText: fmt.Sprintf("v%02d", added), Source: "user", CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := repo.AddFactEvidence(ctx, driven.FactEvidenceRow{ID: uuid.New(), FactVersionID: verID,
				ManualItemID: &evidence, AddedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
	}
	statements := func(ask func() error) int {
		t.Helper()
		counter.Reset()
		if err := ask(); err != nil {
			t.Fatal(err)
		}
		return counter.Count()
	}
	single := func() error { _, err := askSvc.Ask(ctx, userID, proj.ID, "duty?"); return err }
	across := func() error { _, err := askSvc.AskAcross(ctx, userID, "duty?"); return err }

	addFacts(2)
	singleFew, acrossFew := statements(single), statements(across)
	addFacts(10)
	singleMany := statements(single)
	if singleMany != singleFew {
		t.Errorf("Ask issued %d statements with 2 facts and %d with 12; want the same", singleFew, singleMany)
	}
	acrossMany := statements(across)
	t.Logf("statements: Ask %d→%d, AskAcross %d→%d (2→12 facts)", singleFew, singleMany, acrossFew, acrossMany)
	if acrossMany != acrossFew {
		t.Errorf("AskAcross issued %d statements with 2 facts and %d with 12; want the same", acrossFew, acrossMany)
	}

	// The batched context still carries every active fact and its evidence,
	// and nothing from a project the user is not a member of.
	for i := 1; i <= added; i++ {
		if !strings.Contains(llm.prompt, fmt.Sprintf("label=\"Fact %02d\" value=v%02d", i, i)) {
			t.Errorf("prompt is missing fact %02d", i)
		}
	}
	if got := strings.Count(llm.prompt, "evidence manual_item_id="); got != added {
		t.Errorf("prompt has %d evidence lines, want %d", got, added)
	}
	if strings.Contains(llm.prompt, "HX99") {
		t.Error("prompt includes a project the user is not a member of")
	}
}
