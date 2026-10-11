package contradictions

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
)

type ReconciliationResolveInput struct {
	Resolution          string
	KeepFactVersionID   *uuid.UUID
	RejectFactVersionID *uuid.UUID
	ResolutionNote      string
}

type ReconciliationContradictionView struct {
	Contradiction ContradictionRow
	Sides         []ContradictionSideRow
}

var (
	ReconciliationErrNotFound           = errors.New("not found")
	ReconciliationErrNothingToReconcile = errors.New("no pending interpretations to reconcile")
	ReconciliationErrInvalidResolution  = errors.New("invalid resolution")
	ReconciliationErrNotOpen            = errors.New("contradiction is not open")
)

type ReconciliationCandidateOutcome struct {
	Kind            string `json:"kind"`
	Outcome         string `json:"outcome"`
	Subject         string `json:"subject_key,omitempty"`
	Reason          string `json:"reason"`
	FactID          string `json:"fact_id,omitempty"`
	VersionID       string `json:"version_id,omitempty"`
	DecisionID      string `json:"decision_id,omitempty"`
	IssueID         string `json:"issue_id,omitempty"`
	ContradictionID string `json:"contradiction_id,omitempty"`
}

type ReconciliationReconcileInput struct {
	InterpretationIDs []uuid.UUID // empty = all pending
}

type ReconciliationReconcileResult struct {
	ProcessedInterpretations int                              `json:"processed_interpretations"`
	Outcomes                 []ReconciliationCandidateOutcome `json:"outcomes"`
	ContradictionsOpened     int                              `json:"contradictions_opened"`
}

type ReconciliationUseCases interface {
	Run(ctx context.Context, userID, projectID uuid.UUID, in ReconciliationReconcileInput) (*ReconciliationReconcileResult, error)
	ListContradictions(ctx context.Context, userID, projectID uuid.UUID, status string) ([]ReconciliationContradictionView, error)
	Resolve(ctx context.Context, userID, contradictionID uuid.UUID, in ReconciliationResolveInput) (*ReconciliationContradictionView, error)
}
