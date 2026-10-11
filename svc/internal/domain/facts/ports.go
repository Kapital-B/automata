package facts

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// FactRow is a stable project fact identity (subject lineage).
type FactRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ProjectID      uuid.UUID
	IssueID        *uuid.UUID
	SubjectKey     string
	Label          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// FactVersionRow is one version of a fact value.
type FactVersionRow struct {
	ID                    uuid.UUID
	FactID                uuid.UUID
	Status                string
	ValueJSON             string
	ValueText             string
	Unit                  *string
	Source                string
	Confidence            *float64
	InterpretationID      *uuid.UUID
	SupersedesVersionID   *uuid.UUID
	SupersededByVersionID *uuid.UUID
	SupersededAt          *time.Time
	CreatedByUserID       *uuid.UUID
	CreatedAt             time.Time
	// ActivatedAt is when this version became the active one. Nil on rows
	// written before the column existed; readers fall back to CreatedAt.
	ActivatedAt *time.Time
}

// FactEvidenceRow links correspondence evidence to a fact version.
type FactEvidenceRow struct {
	ID            uuid.UUID
	FactVersionID uuid.UUID
	MessageID     *uuid.UUID
	ManualItemID  *uuid.UUID
	AddedAt       time.Time
}

// FactRepository persists facts, versions, and evidence links.
type FactRepository interface {
	CreateFact(ctx context.Context, row FactRow) error
	UpdateFact(ctx context.Context, row FactRow) error
	GetFact(ctx context.Context, organisationID, factID uuid.UUID) (*FactRow, error)
	GetFactBySubject(ctx context.Context, organisationID, projectID uuid.UUID, subjectKey string) (*FactRow, error)
	ListFactsByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]FactRow, error)
	CreateFactVersion(ctx context.Context, row FactVersionRow) error
	GetFactVersion(ctx context.Context, organisationID, versionID uuid.UUID) (*FactVersionRow, error)
	ListFactVersions(ctx context.Context, factID uuid.UUID) ([]FactVersionRow, error)
	GetActiveFactVersion(ctx context.Context, factID uuid.UUID) (*FactVersionRow, error)
	UpdateFactVersion(ctx context.Context, row FactVersionRow) error
	AddFactEvidence(ctx context.Context, row FactEvidenceRow) error
	RemoveFactEvidence(ctx context.Context, organisationID, versionID, evidenceID uuid.UUID) error
	ListFactEvidence(ctx context.Context, versionID uuid.UUID) ([]FactEvidenceRow, error)
	ListFactEvidenceForFact(ctx context.Context, factID uuid.UUID) ([]FactEvidenceRow, error)
	// ListActiveFactVersionsForFacts returns the active version of each fact
	// that has one, in batched statements rather than one per fact.
	ListActiveFactVersionsForFacts(ctx context.Context, factIDs []uuid.UUID) ([]FactVersionRow, error)
	// ListFactEvidenceForVersions returns the evidence of every listed
	// version, oldest first within a version, in batched statements.
	ListFactEvidenceForVersions(ctx context.Context, versionIDs []uuid.UUID) ([]FactEvidenceRow, error)
}
