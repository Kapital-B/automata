package jobs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TypeSync             = "sync"
	TypeSyncSlack        = "sync_slack"
	TypeCategorize       = "categorize"
	TypeSummarize        = "summarize"
	TypeDraftSuggest     = "draft_suggest"
	TypeForwardRules     = "forward_rules"
	TypeResolveContacts  = "resolve_contacts"
	TypeAssignProjects   = "assign_projects"
	TypeInterpretProject = "interpret_project"
	TypeReconcileProject = "reconcile_project"
	TypeProjectAI        = "project_ai"
)

var (
	ErrUnknownJobType  = errors.New("unknown job type")
	ErrReservedJobType = errors.New("reserved job type")
)

type JobMode string

const (
	ModeStreamed JobMode = "streamed"
	ModeSync     JobMode = "sync"
)

type CursorKind string

const (
	CursorGraphNextLink CursorKind = "graph_next_link"
	CursorSlackHistory  CursorKind = "slack_history"
	CursorMessageKeyset CursorKind = "message_keyset"
	CursorSummaryMap    CursorKind = "summary_map"
	CursorNone          CursorKind = "none"
)

type RetryClass string

const (
	RetryTransient RetryClass = "transient"
	RetryTerminal  RetryClass = "terminal"
	RetryEffect    RetryClass = "effect"
)

type EffectPolicy string

const (
	EffectNone       EffectPolicy = "none"
	EffectAtMostOnce EffectPolicy = "at_most_once"
)

var ReservedJobTypes = map[string]struct{}{
	"sync_teams":          {},
	"sync_whatsapp":       {},
	"ingest_transcript":   {},
	"ingest_doc_revision": {},
}

var enqueueAliases = map[string]string{
	"auto-draft":   TypeDraftSuggest,
	"auto_draft":   TypeDraftSuggest,
	"forward":      TypeForwardRules,
	"forward_rule": TypeForwardRules,
}

const (
	SummarizeMaxPartials = 20
	SummarizeMaxMessages = 240
)

// DefaultTerminalTTL is the POC retention for terminal job items (30 days).
const DefaultTerminalTTL = 30 * 24 * time.Hour

// Definition is one registry entry.
type Definition struct {
	Type         string
	Mode         JobMode
	MaxChunk     int
	CursorKind   CursorKind
	RetryClass   RetryClass
	EffectPolicy EffectPolicy
	RequiresLock bool
	LockScope    string
	Description  string
	Aliases      []string
}

// DefaultMailboxChain is the canonical nightly/mailbox pipeline.
var DefaultMailboxChain = []string{
	TypeSync,
	TypeResolveContacts,
	TypeCategorize,
	TypeAssignProjects,
	TypeSummarize,
	TypeForwardRules,
}

// Registry holds validated job definitions.
type Registry struct {
	ordered []Definition
	byType  map[string]Definition
}

func DefaultRegistry() *Registry {
	defs := []Definition{
		{Type: TypeSync, Mode: ModeStreamed, MaxChunk: 100, CursorKind: CursorGraphNextLink, RetryClass: RetryTransient, EffectPolicy: EffectNone, RequiresLock: true, LockScope: "mailbox", Description: "one Graph delta page"},
		{Type: TypeSyncSlack, Mode: ModeStreamed, MaxChunk: 200, CursorKind: CursorSlackHistory, RetryClass: RetryTransient, EffectPolicy: EffectNone, RequiresLock: true, LockScope: "connector", Description: "one Slack history page"},
		{Type: TypeCategorize, Mode: ModeStreamed, MaxChunk: 25, CursorKind: CursorMessageKeyset, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "≤25 messages / LLM calls"},
		{Type: TypeSummarize, Mode: ModeStreamed, MaxChunk: 240, CursorKind: CursorSummaryMap, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "≤240 messages map/reduce"},
		{Type: TypeDraftSuggest, Mode: ModeStreamed, MaxChunk: 1, CursorKind: CursorNone, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "exactly one message_id", Aliases: []string{"auto-draft", "auto_draft"}},
		{Type: TypeForwardRules, Mode: ModeStreamed, MaxChunk: 10, CursorKind: CursorMessageKeyset, RetryClass: RetryEffect, EffectPolicy: EffectAtMostOnce, Description: "≤10 candidates, at-most-once", Aliases: []string{"forward", "forward_rule"}},
		{Type: TypeResolveContacts, Mode: ModeStreamed, MaxChunk: 100, CursorKind: CursorMessageKeyset, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "≤100 messages"},
		{Type: TypeAssignProjects, Mode: ModeStreamed, MaxChunk: 25, CursorKind: CursorMessageKeyset, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "≤25 messages"},
		{Type: TypeInterpretProject, Mode: ModeStreamed, MaxChunk: 40, CursorKind: CursorNone, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "one project, one LLM call"},
		{Type: TypeReconcileProject, Mode: ModeStreamed, MaxChunk: 100, CursorKind: CursorMessageKeyset, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "≤100 candidates"},
		{Type: TypeProjectAI, Mode: ModeSync, MaxChunk: 8, CursorKind: CursorNone, RetryClass: RetryTransient, EffectPolicy: EffectNone, Description: "sync audit, ≤8 projects"},
	}
	r := &Registry{
		ordered: make([]Definition, 0, len(defs)),
		byType:  make(map[string]Definition, len(defs)),
	}
	for _, d := range defs {
		d.Aliases = append([]string(nil), d.Aliases...)
		r.ordered = append(r.ordered, d)
		r.byType[d.Type] = d
	}
	return r
}

func (r *Registry) Get(jobType string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	d, ok := r.byType[jobType]
	return d, ok
}

func (r *Registry) MustGet(jobType string) Definition {
	d, ok := r.Get(jobType)
	if !ok {
		panic("unknown job type: " + jobType)
	}
	return d
}

func (r *Registry) All() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, 0, len(r.ordered))
	for _, d := range r.ordered {
		cp := d
		cp.Aliases = append([]string(nil), d.Aliases...)
		out = append(out, cp)
	}
	return out
}

func normalizeCanonicalType(raw string) (string, error) {
	name := strings.TrimSpace(strings.ToLower(raw))
	if name == "" {
		return "", fmt.Errorf("%w: empty", ErrUnknownJobType)
	}
	if _, reserved := ReservedJobTypes[name]; reserved {
		return "", fmt.Errorf("%w: %q", ErrReservedJobType, name)
	}
	return name, nil
}

func normalizeEnqueueType(raw string) (string, error) {
	name, err := normalizeCanonicalType(raw)
	if err != nil {
		return "", err
	}
	if alias, ok := enqueueAliases[name]; ok {
		return alias, nil
	}
	return name, nil
}

func (r *Registry) ValidateType(raw string) (string, error) {
	name, err := normalizeCanonicalType(raw)
	if err != nil {
		return "", err
	}
	if _, ok := r.Get(name); !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownJobType, name)
	}
	return name, nil
}

func (r *Registry) ValidateChain(steps []string) ([]string, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("empty job chain")
	}
	out := make([]string, 0, len(steps))
	for _, raw := range steps {
		name, err := r.ValidateType(raw)
		if err != nil {
			return nil, err
		}
		def := r.MustGet(name)
		if def.Mode != ModeStreamed && len(steps) > 1 {
			return nil, fmt.Errorf("sync job %q cannot appear in a streamed chain", name)
		}
		out = append(out, name)
	}
	return out, nil
}

func ValidateType(raw string) (string, error) {
	return DefaultRegistry().ValidateType(raw)
}

func ValidateChain(steps []string) ([]string, error) {
	return DefaultRegistry().ValidateChain(steps)
}

func SummarizeMapBatchSize(configured int) int {
	if configured < 12 {
		return 12
	}
	if configured > 30 {
		return 30
	}
	return configured
}

var ChainNamespace = uuid.MustParse("a7f3c2e1-9b4d-4f6a-8c1e-2d5b7a9e0f31")

func DeterministicJobID(chainID uuid.UUID, stepIndex int, jobType string) uuid.UUID {
	return uuid.NewSHA1(ChainNamespace, []byte(fmt.Sprintf("%s:%d:%s", chainID.String(), stepIndex, jobType)))
}

func DeterministicScheduleJobID(scheduleID uuid.UUID, scheduledForRFC3339, jobType string) uuid.UUID {
	return uuid.NewSHA1(ChainNamespace, []byte(fmt.Sprintf("sched:%s:%s:%s", scheduleID.String(), scheduledForRFC3339, jobType)))
}

func DeterministicChainID(scheduleID uuid.UUID, scheduledForRFC3339 string) uuid.UUID {
	return uuid.NewSHA1(ChainNamespace, []byte(fmt.Sprintf("chain:%s:%s", scheduleID.String(), scheduledForRFC3339)))
}
