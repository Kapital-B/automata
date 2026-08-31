package jobs

import (
	"testing"

	"github.com/google/uuid"
)

func TestDefaultRegistryHasElevenTypes(t *testing.T) {
	r := DefaultRegistry()
	if got := len(r.All()); got != 11 {
		t.Fatalf("expected 11 job types, got %d", got)
	}
	required := []string{
		TypeSync, TypeSyncSlack, TypeCategorize, TypeSummarize, TypeDraftSuggest,
		TypeForwardRules, TypeResolveContacts, TypeAssignProjects, TypeInterpretProject,
		TypeReconcileProject, TypeProjectAI,
	}
	for _, name := range required {
		if _, ok := r.Get(name); !ok {
			t.Fatalf("missing registry entry %q", name)
		}
	}
}

func TestNormalizeAliasesAndReserved(t *testing.T) {
	r := DefaultRegistry()
	got, err := r.ValidateType(TypeDraftSuggest)
	if err != nil || got != TypeDraftSuggest {
		t.Fatalf("canonical draft_suggest: got %q err %v", got, err)
	}
	got, err = r.ValidateType(TypeForwardRules)
	if err != nil || got != TypeForwardRules {
		t.Fatalf("canonical forward_rules: got %q err %v", got, err)
	}
	if _, err := r.ValidateType("auto-draft"); err == nil {
		t.Fatal("expected alias auto-draft to be rejected outside enqueue")
	}
	if _, err := r.ValidateType("forward"); err == nil {
		t.Fatal("expected alias forward to be rejected outside enqueue")
	}
	if _, err := r.ValidateType("sync_teams"); err == nil {
		t.Fatal("expected reserved rejection for sync_teams")
	}
	if _, err := r.ValidateType("not_a_job"); err == nil {
		t.Fatal("expected unknown rejection")
	}
}

func TestValidateDefaultMailboxChain(t *testing.T) {
	r := DefaultRegistry()
	got, err := r.ValidateChain(DefaultMailboxChain)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("expected 6 steps, got %d", len(got))
	}
	if _, err := r.ValidateChain([]string{TypeSync, TypeProjectAI}); err == nil {
		t.Fatal("expected sync job rejected from streamed chain")
	}
}

func TestDeterministicJobIDs(t *testing.T) {
	chain := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	a := DeterministicJobID(chain, 0, TypeSync)
	b := DeterministicJobID(chain, 0, TypeSync)
	c := DeterministicJobID(chain, 1, TypeResolveContacts)
	if a != b {
		t.Fatal("deterministic id not stable")
	}
	if a == c {
		t.Fatal("different steps must differ")
	}
	sched := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	when := "2026-08-29T00:00:00Z"
	if DeterministicScheduleJobID(sched, when, TypeSync) != DeterministicScheduleJobID(sched, when, TypeSync) {
		t.Fatal("schedule job id not stable")
	}
	if DeterministicChainID(sched, when) != DeterministicChainID(sched, when) {
		t.Fatal("schedule chain id not stable")
	}
}

func TestSummarizeMapBatchClamp(t *testing.T) {
	if SummarizeMapBatchSize(5) != 12 {
		t.Fatal("expected clamp low")
	}
	if SummarizeMapBatchSize(50) != 30 {
		t.Fatal("expected clamp high")
	}
	if SummarizeMapBatchSize(20) != 20 {
		t.Fatal("expected passthrough")
	}
	if SummarizeMaxMessages/12 > SummarizeMaxPartials {
		t.Fatal("partials cap inconsistent with message cap")
	}
}
