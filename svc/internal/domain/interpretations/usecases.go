package interpretations

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
)

var (
	InterpretationErrNotFound           = errors.New("not found")
	InterpretationErrLLMUnavailable     = errors.New("llm not configured")
	InterpretationErrMixedAccounts      = errors.New("cannot mix mail account_id values in one interpret call")
	InterpretationErrNothingToInterpret = errors.New("no correspondence to interpret")
	InterpretationErrNotPending         = errors.New("interpretation is not pending")
)

type InterpretationCandidateView struct {
	Kind                string
	SubjectKey          string
	Label               string
	Title               string
	Value               any
	Unit                string
	Statement           string
	MessageIDs          []string
	ManualItemIDs       []string
	ConnectorMessageIDs []string
	Confidence          float64
	Reason              string
}

type InterpretationInterpretationView struct {
	Interpretation InterpretationRow
	Sources        []InterpretationSourceRow
	Candidates     []InterpretationCandidateView
}

type InterpretationRunInput struct {
	AccountID           *uuid.UUID
	MessageIDs          []uuid.UUID
	ManualItemIDs       []uuid.UUID
	ConnectorMessageIDs []uuid.UUID
	Trigger             string // api | schedule (stored on job_runs)
}

type InterpretationUseCases interface {
	HasLLM() bool
	ListPending(ctx context.Context, userID, projectID uuid.UUID) ([]InterpretationInterpretationView, error)
	Dismiss(ctx context.Context, userID, interpretationID uuid.UUID) (*InterpretationInterpretationView, error)
	Run(ctx context.Context, userID, projectID uuid.UUID, in InterpretationRunInput) (*InterpretationInterpretationView, error)
}
