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
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appretrieval "github.com/Kapital-B/automata/svc/internal/application/retrieval"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// scriptedLLM records the prompt and answers with whatever citations the
// test asks for, so citation filtering runs against the real context.
type scriptedLLM struct {
	prompt    string
	citations []map[string]string
}

func (s *scriptedLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	s.prompt = messages[len(messages)-1].Content
	cites := s.citations
	if cites == nil {
		cites = []map[string]string{}
	}
	out, _ := json.Marshal(map[string]any{"schema_version": 1, "answer": "ok", "citations": cites, "confidence": 0.5})
	return &driven.LLMResponse{Content: string(out)}, nil
}

type retrievalFixture struct {
	t          *testing.T
	ctx        context.Context
	repo       *sqlite.Repository
	projects   *appprojects.Service
	ask        *appprojectai.Service
	llm        *scriptedLLM
	userID     uuid.UUID
	orgID      uuid.UUID
	accountID  uuid.UUID
	authSvc    *auth.Service
	registered int
}

func newRetrievalFixture(t *testing.T) *retrievalFixture {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+uuid.NewString()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	f := &retrievalFixture{t: t, ctx: context.Background(), repo: repo, llm: &scriptedLLM{}}
	f.authSvc = auth.NewService(repo, repo, repo, nil, nil, []byte("abcdefghijklmnopqrstuvwxyz123456"), time.Hour, 30*24*time.Hour)
	f.projects = &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	f.ask = &appprojectai.Service{
		Users: repo, Projects: repo, Facts: repo, Decisions: repo, Issues: repo, Timeline: repo, LLM: f.llm,
		Resolver: &appretrieval.Resolver{Projects: repo, Contacts: repo}, Search: repo, Assignments: repo,
	}
	f.userID = f.register()
	if f.orgID, err = repo.GetHomeOrganisationID(f.ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	f.accountID = f.account(f.userID)
	return f
}

func (f *retrievalFixture) register() uuid.UUID {
	f.t.Helper()
	f.registered++
	id, err := f.authSvc.Register(f.ctx, fmt.Sprintf("user%d@example.com", f.registered), "password123")
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *retrievalFixture) account(userID uuid.UUID) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	if err := f.repo.InsertAccount(f.ctx, driven.AccountRow{
		UserID: userID, ID: id, Label: "Work", Provider: "m365", MsAccountKind: "work",
		PrimaryEmail: id.String() + "@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *retrievalFixture) project(code string) uuid.UUID {
	f.t.Helper()
	p, err := f.projects.Create(f.ctx, f.userID, appprojects.CreateProjectInput{Name: "Project " + code, Code: code})
	if err != nil {
		f.t.Fatal(err)
	}
	return p.ID
}

func (f *retrievalFixture) message(account uuid.UUID, subject, body string, at time.Time, project *uuid.UUID) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	conv := "conv-" + id.String()
	if err := f.repo.UpsertMessage(f.ctx, driven.MessageRow{
		ID: id, AccountID: account, ProviderMessageID: id.String(), Subject: subject, FromJSON: `{}`,
		BodyText: &body, ConversationID: &conv, ReceivedAt: at, CreatedAt: at, UpdatedAt: at,
	}); err != nil {
		f.t.Fatal(err)
	}
	if project != nil {
		if err := f.repo.UpsertThreadAssignment(f.ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: f.orgID, AccountID: account, ConversationID: conv, ProjectID: project,
			Status: "committed", Reason: "test", Source: string(domainprojects.SourceUser), CreatedAt: at, UpdatedAt: at,
		}); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

// A project the question names is in the context even when attention and
// recency would have left it out of the eight.
func TestAskAcrossIncludesNamedProjectBeyondTheUsualEight(t *testing.T) {
	f := newRetrievalFixture(t)
	f.project("DC01") // created first, so least recently updated
	for i := 2; i <= 10; i++ {
		f.project(fmt.Sprintf("DC%02d", i))
	}
	// Someone else's project: naming it must not bring it in.
	other := f.register()
	hidden := uuid.New()
	now := time.Now().UTC()
	if err := f.repo.CreateProject(f.ctx, driven.ProjectRow{ID: hidden, OrganisationID: f.orgID, Name: "Hidden", Code: "HX99", CreatedAt: now, UpdatedAt: now},
		driven.ProjectMemberRow{ID: uuid.New(), ProjectID: hidden, UserID: other, Role: "owner", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.ask.AskAcross(f.ctx, f.userID, "What is happening on DC01 and HX99?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.llm.prompt, "PROJECT code=DC01 ") {
		t.Error("named project DC01 is missing from the context")
	}
	if strings.Contains(f.llm.prompt, "HX99 ") || strings.Contains(f.llm.prompt, hidden.String()) {
		t.Error("a project the user is not a member of reached the context")
	}
	if got := strings.Count(f.llm.prompt, "==== PROJECT code="); got != 8 {
		t.Errorf("context has %d projects, want the cap of 8", got)
	}
}

// Mail older than the snapshot's recent items is found by search, and the
// model may cite it.
func TestAskFindsOlderMailAndCitesIt(t *testing.T) {
	f := newRetrievalFixture(t)
	pid := f.project("DC07")
	now := time.Now().UTC()
	old := f.message(f.accountID, "Revised pump curve", "Attached is the revised curve for P-03.", now.AddDate(0, -6, 0), &pid)
	for i := 0; i < 15; i++ {
		f.message(f.accountID, fmt.Sprintf("Site note %d", i), "Routine.", now.Add(-time.Duration(i)*time.Hour), &pid)
	}
	f.llm.citations = []map[string]string{{"type": "message", "id": old.String()}}

	ans, err := f.ask.Ask(f.ctx, f.userID, pid, "Where is the revised pump curve for DC07?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.llm.prompt, "## Correspondence matching the question") ||
		!strings.Contains(f.llm.prompt, "message_id="+old.String()+" project=DC07") {
		t.Fatalf("old message not in context:\n%s", f.llm.prompt)
	}
	if len(ans.Citations) != 1 || ans.Citations[0].ID != old.String() || ans.Citations[0].ProjectCode != "DC07" {
		t.Fatalf("citations = %+v, want the old message under DC07", ans.Citations)
	}
}

// Unfiled mail from a named contact is found across projects and cited
// without a project. Another user's matching mail never appears.
func TestAskAcrossFindsUnfiledMailFromNamedContact(t *testing.T) {
	f := newRetrievalFixture(t)
	f.project("DC07")
	now := time.Now().UTC()
	fromJan := f.message(f.accountID, "Seal", "The P-03 seal needs replacing.", now.AddDate(0, -3, 0), nil)
	f.message(f.accountID, "Seal", "A different seal, not from Jan.", now, nil)
	other := f.register()
	theirs := f.message(f.account(other), "P-03 seal", "Not yours to see.", now, nil)

	jan, err := f.repo.ResolveEmailContact(f.ctx, f.orgID, "jan@example.com", "Jan de Vries", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpsertParticipant(f.ctx, driven.CorrespondenceParticipantRow{
		ID: uuid.New(), OrganisationID: f.orgID, ContactID: jan, Role: "from", MessageID: &fromJan,
	}); err != nil {
		t.Fatal(err)
	}
	f.llm.citations = []map[string]string{{"type": "message", "id": fromJan.String()}}

	ans, err := f.ask.AskAcross(f.ctx, f.userID, "What did Jan say about the P-03 seal?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.llm.prompt, "message_id="+fromJan.String()+" project=none") {
		t.Fatalf("Jan's message not in context:\n%s", f.llm.prompt)
	}
	if strings.Contains(f.llm.prompt, "not from Jan") {
		t.Error("the contact filter let through mail Jan was not on")
	}
	if strings.Contains(f.llm.prompt, theirs.String()) || strings.Contains(f.llm.prompt, "Not yours to see") {
		t.Fatal("another user's mail reached the context")
	}
	if len(ans.Citations) != 1 || ans.Citations[0].ID != fromJan.String() || ans.Citations[0].ProjectID != "" {
		t.Fatalf("citations = %+v, want Jan's message with no project", ans.Citations)
	}
}
