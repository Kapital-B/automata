package projects

import (
	context "context"
	errors "errors"
	fmt "fmt"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	uuid "github.com/google/uuid"
	strings "strings"
	time "time"
)

type ProjectAssignInput struct {
	ProjectID *uuid.UUID
	Scope     AssignScope
	Status    AssignmentStatus
}

// OverviewViewCounts is the metric-card payload.
type OverviewViewCounts struct {
	NeedsYou           int
	TriageUnassigned   int
	TriageProvisional  int
	OpenContradictions int
	ProvisionalFacts   int
	ProposedDecisions  int
	ActiveProjects     int
}

// ProjectAssignTally breaks a run down by outcome so a run that scored nothing is
// distinguishable from a run that failed.
type ProjectAssignTally struct {
	CommittedRule  int
	ProvisionalLLM int
	Unscored       int
	Errors         int
}

// ProjectBatchAssignItem is one assignment in a batch. A nil ProjectID clears the
// assignment, matching the single-item endpoints.
type ProjectBatchAssignItem struct {
	Kind      string // "message" | "manual"
	ID        uuid.UUID
	ProjectID *uuid.UUID
	Scope     AssignScope // messages only; defaults to thread
	// NotRelevant marks (true) or restores (false) correspondence that is not
	// project work. Nil leaves the flag untouched, so existing callers are
	// unaffected.
	NotRelevant *bool
}

var (
	ProjectAIErrNotFound       = errors.New("not found")
	ProjectAIErrLLMUnavailable = errors.New("llm not configured")
	ProjectAIErrEmptyQuestion  = errors.New("question required")
)

type ProjectAssignChunkResult struct {
	MessagesProcessed int
	MessagesAssigned  int
	NextCursor        *domainjobs.JobCursor
	Done              bool
	Tally             ProjectAssignTally
}

type ProjectAIAnswer struct {
	Answer     string              `json:"answer"`
	Citations  []ProjectAICitation `json:"citations"`
	Confidence float64             `json:"confidence"`
}

var (
	ProjectErrNotFound      = errors.New("not found")
	ProjectErrInvalidCode   = errors.New("invalid project code")
	ProjectErrCodeTaken     = errors.New("project code already exists")
	ProjectErrCodeImmutable = errors.New("project code is immutable")
)

var (
	ProjectErrInvalidChannel = errors.New("invalid channel")
	ProjectErrInvalidBody    = errors.New("body_text required")
)

type ProjectProjectDetail struct {
	Project ProjectRow
	Member  *ProjectMemberRow
}

type OverviewActivityPage struct {
	Items      []ActivityItem
	NextBefore *time.Time
	NextID     *uuid.UUID
}

// ProjectErrTooManyItems is returned when a batch exceeds ProjectMaxBatchAssignItems.
var ProjectErrTooManyItems = fmt.Errorf("too_many_items")

// OverviewActivityInput is a validated feed request.
type OverviewActivityInput struct {
	Limit     int
	Before    *time.Time
	BeforeID  *uuid.UUID
	Kinds     []string
	ProjectID *uuid.UUID
}

type ProjectAICitation struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectCode string `json:"project_code,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
}

// OverviewViewProject is one row of the Home projects list.
type OverviewViewProject struct {
	ID             uuid.UUID
	Code           string
	Name           string
	Teaser         string
	LastActivityAt *time.Time
	AttentionCount int
}

type ProjectUpdateProjectInput struct {
	Name        *string
	Description *string
	Client      *string
	Keywords    *[]string
	Archive     *bool
}

// ProjectBatchAssignResult reports the outcome of one item. Errors are machine
// readable so the UI can explain a partial failure without parsing prose.
type ProjectBatchAssignResult struct {
	ID    uuid.UUID
	OK    bool
	Error string
}

// OverviewValidKind reports whether kind is one the feed emits.
func OverviewValidKind(kind string) bool {
	_, ok := OverviewValidKinds[strings.TrimSpace(kind)]
	return ok
}

// Kind values the feed can emit. Requests naming anything else are rejected
// rather than silently returning everything.
var OverviewValidKinds = map[string]struct{}{
	"decision_proposed":      {},
	"decision_accepted":      {},
	"decision_withdrawn":     {},
	"fact_recorded":          {},
	"fact_superseded":        {},
	"contradiction_opened":   {},
	"contradiction_resolved": {},
	"issue_opened":           {},
	"issue_resolved":         {},
}

type OverviewResult struct {
	Counts   OverviewViewCounts
	Projects []OverviewViewProject
}

type ProjectUpdateMemberInput struct {
	Role              *string
	Discipline        *string
	Responsibilities  *string
	CurrentScope      *string
	ApprovalAuthority *string
	OutOfScope        *string
}

// ProjectMaxBatchAssignItems caps one batch request. Triage selections are operator
// sized; a larger request is a client bug, not a workload.
const ProjectMaxBatchAssignItems = 200

type ProjectCreateProjectInput struct {
	Name              string
	Code              string
	Description       *string
	Client            *string
	Keywords          []string
	MemberRole        string
	Discipline        *string
	Responsibilities  *string
	CurrentScope      *string
	ApprovalAuthority *string
	OutOfScope        *string
}

type ProjectCreateManualInput struct {
	Channel               string
	OccurredAt            time.Time
	Title                 string
	BodyText              string
	ProjectID             *uuid.UUID
	ParticipantContactIDs []uuid.UUID
}

type OverviewUseCases interface {
	Get(ctx context.Context, userID uuid.UUID) (*OverviewResult, error)
	Activity(ctx context.Context, userID uuid.UUID, in OverviewActivityInput) (*OverviewActivityPage, error)
}

type ProjectAIUseCases interface {
	HasLLM() bool
	Ask(ctx context.Context, userID, projectID uuid.UUID, question string) (*ProjectAIAnswer, error)
	AskAcross(ctx context.Context, userID uuid.UUID, question string) (*ProjectAIAnswer, error)
}

type ProjectUseCases interface {
	AssignBatch(ctx context.Context, userID uuid.UUID, items []ProjectBatchAssignItem) ([]ProjectBatchAssignResult, error)
	List(ctx context.Context, userID uuid.UUID, filter ProjectListFilter) ([]ProjectRow, error)
	Get(ctx context.Context, userID, projectID uuid.UUID) (*ProjectProjectDetail, error)
	Create(ctx context.Context, userID uuid.UUID, in ProjectCreateProjectInput) (*ProjectRow, error)
	Update(ctx context.Context, userID, projectID uuid.UUID, in ProjectUpdateProjectInput) (*ProjectRow, error)
	UpdateMember(ctx context.Context, userID, projectID uuid.UUID, in ProjectUpdateMemberInput) (*ProjectMemberRow, error)
	AssignMessage(ctx context.Context, userID, messageID uuid.UUID, in ProjectAssignInput) (*EffectiveAssignment, error)
	ClearOverride(ctx context.Context, userID, messageID uuid.UUID) (*EffectiveAssignment, error)
	ListUnassigned(ctx context.Context, userID uuid.UUID, filter UnassignedListFilter) ([]UnassignedItem, error)
	UnassignedSummary(ctx context.Context, userID uuid.UUID) (UnassignedSummary, error)
	EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*EffectiveAssignment, error)
	CreateManualItem(ctx context.Context, userID uuid.UUID, in ProjectCreateManualInput) (*ManualItemRow, error)
	AssignManualItem(ctx context.Context, userID, manualItemID uuid.UUID, projectID *uuid.UUID) (*ManualItemRow, error)
	GetTimeline(ctx context.Context, userID, projectID uuid.UUID, filter TimelineFilter) ([]TimelineItem, error)
}

type ProjectAssignUseCases interface {
	AssignAfterSync(ctx context.Context, userID, accountID uuid.UUID) error
	AssignAccountChunk(ctx context.Context, run domainjobs.RunContext) (*ProjectAssignChunkResult, error)
}

func (t *ProjectAssignTally) Record(o ProjectAssignOutcome) {
	switch o {
	case ProjectOutcomeCommittedRule:
		t.CommittedRule++
	case ProjectOutcomeProvisionalLLM:
		t.ProvisionalLLM++
	default:
		t.Unscored++
	}
}

func (t ProjectAssignTally) Assigned() int {
	return t.CommittedRule + t.ProvisionalLLM
}

func (t ProjectAssignTally) Meta(considered int) map[string]any {
	return map[string]any{
		"messages_considered": considered,
		"assigned":            t.Assigned(),
		"committed_rule":      t.CommittedRule,
		"provisional_llm":     t.ProvisionalLLM,
		"unscored":            t.Unscored,
		"errors":              t.Errors,
	}
}

// ProjectAssignOutcome records what the scorer decided, for run metadata.
type ProjectAssignOutcome int

const (
	ProjectOutcomeUnscored ProjectAssignOutcome = iota
	ProjectOutcomeCommittedRule
	ProjectOutcomeProvisionalLLM
)
