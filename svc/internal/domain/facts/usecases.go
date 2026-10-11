package facts

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
)

var (
	FactErrNotFound          = errors.New("not found")
	FactErrInvalidSubjectKey = errors.New("invalid subject_key")
	FactErrInvalidStatus     = errors.New("invalid status")
	FactErrInvalidEvidence   = errors.New("invalid evidence ref")
	FactErrWrongProject      = errors.New("item not on this project")
	FactErrSupersedeRequired = errors.New("supersedes_version_id required when an active version exists")
	FactErrInvalidSupersedes = errors.New("supersedes_version_id must be the current active version")
	FactErrNotProposed       = errors.New("version is not proposed")
)

type FactEvidenceRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

type FactFactView struct {
	Fact     FactRow
	Versions []FactVersionView
}

type FactVersionView struct {
	Version  FactVersionRow
	Evidence []FactEvidenceRow
}

type FactCreateInput struct {
	SubjectKey          string
	Label               string
	Value               any
	Unit                *string
	IssueID             *uuid.UUID
	Confirm             bool
	SupersedesVersionID *uuid.UUID
	Evidence            []FactEvidenceRef
	Source              string // user|rule|llm; default user
	InterpretationID    *uuid.UUID
	Confidence          *float64
}

type FactCurrentPosition struct {
	Facts []FactCurrentPositionFact
}

// FactListInclude controls which versions appear in list responses.
type FactListInclude struct {
	Proposed bool
	History  bool
}

type FactCurrentPositionFact struct {
	FactID        uuid.UUID
	SubjectKey    string
	Label         string
	VersionID     uuid.UUID
	ValueJSON     string
	ValueText     string
	Unit          *string
	EvidenceCount int
}

type FactConfirmInput struct {
	SupersedesVersionID *uuid.UUID
}

type FactUseCases interface {
	List(ctx context.Context, userID, projectID uuid.UUID, include FactListInclude) ([]FactFactView, error)
	Get(ctx context.Context, userID, factID uuid.UUID) (*FactFactView, error)
	CurrentPosition(ctx context.Context, userID, projectID uuid.UUID) (*FactCurrentPosition, error)
	Create(ctx context.Context, userID, projectID uuid.UUID, in FactCreateInput) (*FactFactView, error)
	Confirm(ctx context.Context, userID, versionID uuid.UUID, in FactConfirmInput) (*FactFactView, error)
	Reject(ctx context.Context, userID, versionID uuid.UUID) (*FactFactView, error)
	AddEvidence(ctx context.Context, userID, versionID uuid.UUID, ref FactEvidenceRef) (*FactFactView, error)
	RemoveEvidence(ctx context.Context, userID, versionID, evidenceID uuid.UUID) (*FactFactView, error)
}
