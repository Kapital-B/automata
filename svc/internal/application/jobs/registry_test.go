package jobs

import (
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
)

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDefaultRegistryHasElevenTypes(t *testing.T) {
	r := domainjobs.DefaultRegistry()
	if got := len(r.All()); got != 11 {
		t.Fatalf("expected 11 job types, got %d", got)
	}
	required := []string{
		domainjobs.TypeSync, domainjobs.TypeSyncSlack, domainjobs.TypeCategorize, domainjobs.TypeSummarize, domainjobs.TypeDraftSuggest,
		domainjobs.TypeForwardRules, domainjobs.TypeResolveContacts, domainjobs.TypeAssignProjects, domainjobs.TypeInterpretProject,
		domainjobs.TypeReconcileProject, domainjobs.TypeProjectAI,
	}
	for _, name := range required {
		if _, ok := r.Get(name); !ok {
			t.Fatalf("missing registry entry %q", name)
		}
	}
}

func TestNormalizeAliasesAndReserved(t *testing.T) {
	r := domainjobs.DefaultRegistry()
	got, err := r.ValidateType(domainjobs.TypeDraftSuggest)
	if err != nil || got != domainjobs.TypeDraftSuggest {
		t.Fatalf("canonical draft_suggest: got %q err %v", got, err)
	}
	got, err = r.ValidateType(domainjobs.TypeForwardRules)
	if err != nil || got != domainjobs.TypeForwardRules {
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
	r := domainjobs.DefaultRegistry()
	got, err := r.ValidateChain(domainjobs.DefaultMailboxChain)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("expected 6 steps, got %d", len(got))
	}
	if _, err := r.ValidateChain([]string{domainjobs.TypeSync, domainjobs.TypeProjectAI}); err == nil {
		t.Fatal("expected sync job rejected from streamed chain")
	}
}

func TestDeterministicJobIDs(t *testing.T) {
	chain := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	a := domainjobs.DeterministicJobID(chain, 0, domainjobs.TypeSync)
	b := domainjobs.DeterministicJobID(chain, 0, domainjobs.TypeSync)
	c := domainjobs.DeterministicJobID(chain, 1, domainjobs.TypeResolveContacts)
	if a != b {
		t.Fatal("deterministic id not stable")
	}
	if a == c {
		t.Fatal("different steps must differ")
	}
	sched := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	when := "2026-08-29T00:00:00Z"
	if domainjobs.DeterministicScheduleJobID(sched, when, domainjobs.TypeSync) != domainjobs.DeterministicScheduleJobID(sched, when, domainjobs.TypeSync) {
		t.Fatal("schedule job id not stable")
	}
	if domainjobs.DeterministicChainID(sched, when) != domainjobs.DeterministicChainID(sched, when) {
		t.Fatal("schedule chain id not stable")
	}
}

func TestSummarizeMapBatchClamp(t *testing.T) {
	if domainjobs.SummarizeMapBatchSize(5) != 12 {
		t.Fatal("expected clamp low")
	}
	if domainjobs.SummarizeMapBatchSize(50) != 30 {
		t.Fatal("expected clamp high")
	}
	if domainjobs.SummarizeMapBatchSize(20) != 20 {
		t.Fatal("expected passthrough")
	}
	if domainjobs.SummarizeMaxMessages/12 > domainjobs.SummarizeMaxPartials {
		t.Fatal("partials cap inconsistent with message cap")
	}
}

func TestNormalizeScheduleChain(t *testing.T) {
	r := domainjobs.DefaultRegistry()
	got, err := r.NormalizeScheduleChain([]string{" Sync ", "forward"})
	if err != nil || len(got) != 2 || got[0] != domainjobs.TypeSync || got[1] != domainjobs.TypeForwardRules {
		t.Fatalf("got %v, %v; want aliases resolved to canonical names", got, err)
	}
	cases := map[string]struct {
		chain []string
		want  error
	}{
		"typo":            {[]string{"sync", "categorise"}, domainjobs.ErrUnknownJobType},
		"needs a message": {[]string{"auto-draft"}, domainjobs.ErrNotSchedulable},
		"per project":     {[]string{"interpret_project"}, domainjobs.ErrNotSchedulable},
		"reserved":        {[]string{"sync_teams"}, domainjobs.ErrReservedJobType},
	}
	for name, c := range cases {
		if _, err := r.NormalizeScheduleChain(c.chain); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if _, err := r.NormalizeScheduleChain(nil); err == nil {
		t.Error("an empty chain was accepted")
	}
}
