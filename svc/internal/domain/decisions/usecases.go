package decisions

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
)

var (
	DecisionErrNotFound      = errors.New("not found")
	DecisionErrNotProposed   = errors.New("decision is not proposed")
	DecisionErrInvalidStatus = errors.New("invalid status")
	DecisionErrWrongProject  = errors.New("wrong project")
	DecisionErrBadAssignee   = errors.New("assignee xor violated")
	DecisionErrBadEvidence   = errors.New("evidence must be exactly one of message or manual")
)

type DecisionEvidenceRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

type DecisionDecisionView struct {
	Decision DecisionRow
	Evidence []DecisionEvidenceRow
}

type DecisionCreateInput struct {
	Statement         string
	IssueID           *uuid.UUID
	Confirm           bool
	AssigneeUserID    *uuid.UUID
	AssigneeContactID *uuid.UUID
	Evidence          []DecisionEvidenceRef
	Source            string
	Confidence        *float64
	SupersedesID      *uuid.UUID
}

type DecisionPatchInput struct {
	Statement         *string
	AssigneeUserID    *uuid.UUID
	AssigneeContactID *uuid.UUID
	ClearAssignee     bool
}

type DecisionUseCases interface {
	List(ctx context.Context, userID, projectID uuid.UUID, status string) ([]DecisionDecisionView, error)
	ListAcceptedRecent(ctx context.Context, userID, projectID uuid.UUID, limit int) ([]DecisionDecisionView, error)
	Get(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionDecisionView, error)
	Create(ctx context.Context, userID, projectID uuid.UUID, in DecisionCreateInput) (*DecisionDecisionView, error)
	Confirm(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionDecisionView, error)
	Withdraw(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionDecisionView, error)
	Patch(ctx context.Context, userID, decisionID uuid.UUID, in DecisionPatchInput) (*DecisionDecisionView, error)
	AddEvidence(ctx context.Context, userID, decisionID uuid.UUID, ref DecisionEvidenceRef) (*DecisionDecisionView, error)
}
