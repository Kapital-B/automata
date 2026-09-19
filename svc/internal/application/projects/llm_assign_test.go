package projects

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type fakeLLM struct {
	responses []string
	calls     int
	prompts   []string
	err       error
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, msgs []driven.LLMMessage) (*driven.LLMResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, m := range msgs {
		if m.Role == "user" {
			f.prompts = append(f.prompts, m.Content)
		}
	}
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return &driven.LLMResponse{Content: f.responses[idx]}, nil
}

// recordingAssignments captures writes so tests can assert status and source
// without a database.
type recordingAssignments struct {
	driven.AssignmentRepository
	threads   []driven.AssignmentRow
	overrides []driven.AssignmentRow
}

func (r *recordingAssignments) UpsertThreadAssignment(ctx context.Context, row driven.AssignmentRow) error {
	r.threads = append(r.threads, row)
	return nil
}

func (r *recordingAssignments) UpsertMessageOverride(ctx context.Context, row driven.AssignmentRow) error {
	r.overrides = append(r.overrides, row)
	return nil
}

func llmTestMessage(subject, conv string) driven.MessageRow {
	body := "some body text"
	m := driven.MessageRow{ID: uuid.New(), Subject: subject, BodyText: &body, FromJSON: `{"address":"a@b.com"}`}
	if conv != "" {
		c := conv
		m.ConversationID = &c
	}
	return m
}

func TestScoreWithLLMWritesProvisionalNeverCommitted(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"assignments":[{"ref":"t1","project_code":"DC01","confidence":0.97,"reason":"chiller commissioning"}]}`,
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}

	msgs := []driven.MessageRow{llmTestMessage("commissioning plan", "conv-1")}
	n, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("assigned = %d, want 1", n)
	}
	if len(rec.threads) != 1 {
		t.Fatalf("thread writes = %d, want 1", len(rec.threads))
	}
	got := rec.threads[0]
	// Even at 0.97, an LLM suggestion stays provisional: Wave 1 §7 reserves
	// committed for a project code token.
	if got.Status != "provisional" {
		t.Errorf("status = %q, want provisional", got.Status)
	}
	if got.Source != "llm" {
		t.Errorf("source = %q, want llm", got.Source)
	}
	if got.Confidence == nil || *got.Confidence > maxConfidence {
		t.Errorf("confidence = %v", got.Confidence)
	}
	if got.Reason != "chiller commissioning" {
		t.Errorf("reason = %q", got.Reason)
	}
}

func TestScoreWithLLMDropsUnknownRefAndCode(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"assignments":[
			{"ref":"t1","project_code":"NOPE","confidence":0.9,"reason":"invented project"},
			{"ref":"t99","project_code":"DC01","confidence":0.9,"reason":"invented thread"}
		]}`,
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}

	msgs := []driven.MessageRow{llmTestMessage("subject", "conv-1")}
	n, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(rec.threads) != 0 || len(rec.overrides) != 0 {
		t.Fatalf("hallucinated ref/code must write nothing: n=%d threads=%d overrides=%d",
			n, len(rec.threads), len(rec.overrides))
	}
}

func TestScoreWithLLMDropsLowConfidence(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"assignments":[{"ref":"t1","project_code":"DC01","confidence":0.05,"reason":"weak"}]}`,
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}
	msgs := []driven.MessageRow{llmTestMessage("subject", "conv-1")}
	n, _ := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC())
	if n != 0 {
		t.Fatalf("confidence below the provisional floor must not be written, got %d", n)
	}
}

func TestScoreWithLLMRepairsMalformedJSONOnce(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		"here you go: not json at all",
		"```json\n{\"schema_version\":1,\"assignments\":[{\"ref\":\"t1\",\"project_code\":\"DC01\",\"confidence\":0.8,\"reason\":\"ok\"}]}\n```",
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}
	msgs := []driven.MessageRow{llmTestMessage("subject", "conv-1")}
	n, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("assigned = %d, want 1 after repair", n)
	}
	if llm.calls != 2 {
		t.Errorf("llm calls = %d, want 2 (original + one repair)", llm.calls)
	}
}

func TestScoreWithLLMNilClientIsNoOp(t *testing.T) {
	rec := &recordingAssignments{}
	svc := &AssignService{Assignments: rec}
	msgs := []driven.MessageRow{llmTestMessage("subject", "conv-1")}
	n, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs,
		[]driven.ProjectRow{proj("DC01", "Cooling")}, nil, nowUTC())
	if err != nil || n != 0 {
		t.Fatalf("nil LLM must be a no-op, got n=%d err=%v", n, err)
	}
}

func TestScoreWithLLMScoresThreadsNotMessages(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{`{"schema_version":1,"assignments":[]}`}}
	svc := &AssignService{Assignments: rec, LLM: llm}

	// Twelve messages across three conversations must cost one call describing
	// three threads, not twelve.
	msgs := []driven.MessageRow{}
	for i := 0; i < 4; i++ {
		msgs = append(msgs, llmTestMessage("a", "conv-a"))
		msgs = append(msgs, llmTestMessage("b", "conv-b"))
		msgs = append(msgs, llmTestMessage("c", "conv-c"))
	}
	if _, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC()); err != nil {
		t.Fatal(err)
	}
	if llm.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", llm.calls)
	}
	prompt := llm.prompts[0]
	if got := strings.Count(prompt, "ref=t"); got != 3 {
		t.Errorf("prompt described %d threads, want 3", got)
	}
}

func TestScoreWithLLMBatchesLargeInput(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{`{"schema_version":1,"assignments":[]}`}}
	svc := &AssignService{Assignments: rec, LLM: llm}

	msgs := []driven.MessageRow{}
	for i := 0; i < llmBatchSize*2+3; i++ {
		msgs = append(msgs, llmTestMessage("s", uuid.NewString()))
	}
	if _, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC()); err != nil {
		t.Fatal(err)
	}
	if llm.calls != 3 {
		t.Fatalf("llm calls = %d, want 3 batches", llm.calls)
	}
}

func TestScoreWithLLMNeverOffersArchivedProject(t *testing.T) {
	archived := proj("DC01", "Cooling")
	at := nowUTC()
	archived.ArchivedAt = &at
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"assignments":[{"ref":"t1","project_code":"DC01","confidence":0.9,"reason":"x"}]}`,
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}
	msgs := []driven.MessageRow{llmTestMessage("subject", "conv-1")}
	n, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{archived}, nil, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("archived project must not be assignable, got %d", n)
	}
	if llm.calls != 0 {
		t.Errorf("no active projects should mean no LLM call, got %d", llm.calls)
	}
}

func TestScoreWithLLMMessageWithoutConversationWritesOverride(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	rec := &recordingAssignments{}
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"assignments":[{"ref":"t1","project_code":"DC01","confidence":0.8,"reason":"ok"}]}`,
	}}
	svc := &AssignService{Assignments: rec, LLM: llm}
	msgs := []driven.MessageRow{llmTestMessage("subject", "")}
	if _, err := svc.scoreWithLLM(context.Background(), uuid.New(), uuid.New(), msgs, []driven.ProjectRow{dc01}, nil, nowUTC()); err != nil {
		t.Fatal(err)
	}
	if len(rec.overrides) != 1 || len(rec.threads) != 0 {
		t.Fatalf("a message with no conversation must write an override: overrides=%d threads=%d",
			len(rec.overrides), len(rec.threads))
	}
}

func nowUTC() time.Time { return time.Now().UTC() }
