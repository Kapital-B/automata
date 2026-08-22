package facts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalidSubjectKey = errors.New("invalid subject_key")
	ErrInvalidStatus     = errors.New("invalid status")
	ErrInvalidEvidence   = errors.New("invalid evidence ref")
	ErrWrongProject      = errors.New("item not on this project")
	ErrSupersedeRequired = errors.New("supersedes_version_id required when an active version exists")
	ErrInvalidSupersedes = errors.New("supersedes_version_id must be the current active version")
	ErrNotProposed       = errors.New("version is not proposed")
)

// Service handles fact CRUD, version confirm/reject, and evidence links.
type Service struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Facts       driven.FactRepository
	Issues      driven.IssueRepository
	Assignments driven.AssignmentRepository
	Manuals     driven.ManualItemRepository
	Messages    driven.MessageRepository
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

// ListInclude controls which versions appear in list responses.
type ListInclude struct {
	Proposed bool
	History  bool
}

type EvidenceRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

type VersionView struct {
	Version  driven.FactVersionRow
	Evidence []driven.FactEvidenceRow
}

type FactView struct {
	Fact     driven.FactRow
	Versions []VersionView
}

type CurrentPositionFact struct {
	FactID        uuid.UUID
	SubjectKey    string
	Label         string
	VersionID     uuid.UUID
	ValueJSON     string
	ValueText     string
	Unit          *string
	EvidenceCount int
}

type CurrentPosition struct {
	Facts []CurrentPositionFact
}

type CreateInput struct {
	SubjectKey          string
	Label               string
	Value               any
	Unit                *string
	IssueID             *uuid.UUID
	Confirm             bool
	SupersedesVersionID *uuid.UUID
	Evidence            []EvidenceRef
	Source              string // user|rule|llm; default user
	InterpretationID    *uuid.UUID
	Confidence          *float64
}

type ConfirmInput struct {
	SupersedesVersionID *uuid.UUID
}

func (s *Service) requireProjectMember(ctx context.Context, userID, projectID uuid.UUID) (orgID uuid.UUID, err error) {
	orgID, err = s.homeOrg(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return uuid.Nil, err
	}
	if p == nil {
		return uuid.Nil, ErrNotFound
	}
	member, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if member == nil {
		return uuid.Nil, ErrNotFound
	}
	return orgID, nil
}

func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID, include ListInclude) ([]FactView, error) {
	orgID, err := s.requireProjectMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	facts, err := s.Facts.ListFactsByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]FactView, 0, len(facts))
	for _, fact := range facts {
		view, err := s.buildFactView(ctx, fact, include, false)
		if err != nil {
			return nil, err
		}
		if len(view.Versions) == 0 && !include.History && !include.Proposed {
			continue
		}
		if !include.History && !include.Proposed {
			// Default: only facts that currently have an active version.
			hasActive := false
			for _, v := range view.Versions {
				if v.Version.Status == string(domainfacts.StatusActive) {
					hasActive = true
					break
				}
			}
			if !hasActive {
				continue
			}
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, factID uuid.UUID) (*FactView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	fact, err := s.Facts.GetFact(ctx, orgID, factID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, fact.ProjectID); err != nil {
		return nil, err
	}
	return s.buildFactView(ctx, *fact, ListInclude{Proposed: true, History: true}, true)
}

func (s *Service) CurrentPosition(ctx context.Context, userID, projectID uuid.UUID) (*CurrentPosition, error) {
	views, err := s.List(ctx, userID, projectID, ListInclude{})
	if err != nil {
		return nil, err
	}
	out := &CurrentPosition{Facts: make([]CurrentPositionFact, 0, len(views))}
	for _, view := range views {
		for _, ver := range view.Versions {
			if ver.Version.Status != string(domainfacts.StatusActive) {
				continue
			}
			out.Facts = append(out.Facts, CurrentPositionFact{
				FactID:        view.Fact.ID,
				SubjectKey:    view.Fact.SubjectKey,
				Label:         view.Fact.Label,
				VersionID:     ver.Version.ID,
				ValueJSON:     ver.Version.ValueJSON,
				ValueText:     ver.Version.ValueText,
				Unit:          ver.Version.Unit,
				EvidenceCount: len(ver.Evidence),
			})
		}
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in CreateInput) (*FactView, error) {
	orgID, err := s.requireProjectMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	subjectKey := strings.TrimSpace(strings.ToLower(in.SubjectKey))
	if !domainfacts.ValidSubjectKey(subjectKey) {
		return nil, ErrInvalidSubjectKey
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return nil, fmt.Errorf("label required")
	}
	valueJSON, valueText, err := encodeValue(in.Value)
	if err != nil {
		return nil, err
	}
	var unit *string
	if in.Unit != nil {
		u := strings.TrimSpace(*in.Unit)
		if u != "" {
			unit = &u
		}
	}
	if in.IssueID != nil {
		iss, err := s.Issues.GetIssue(ctx, orgID, *in.IssueID)
		if err != nil {
			return nil, err
		}
		if iss == nil || iss.ProjectID != projectID {
			return nil, ErrWrongProject
		}
	}
	for _, ref := range in.Evidence {
		if err := s.validateEvidenceOnProject(ctx, userID, orgID, projectID, ref); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	existing, err := s.Facts.GetFactBySubject(ctx, orgID, projectID, subjectKey)
	if err != nil {
		return nil, err
	}

	var fact driven.FactRow
	if existing == nil {
		fact = driven.FactRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID,
			IssueID: in.IssueID, SubjectKey: subjectKey, Label: label,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.Facts.CreateFact(ctx, fact); err != nil {
			return nil, err
		}
	} else {
		fact = *existing
		fact.Label = label
		if in.IssueID != nil {
			fact.IssueID = in.IssueID
		}
		fact.UpdatedAt = now
		if err := s.Facts.UpdateFact(ctx, fact); err != nil {
			return nil, err
		}
	}

	status := domainfacts.StatusProposed
	if in.Confirm {
		status = domainfacts.StatusActive
	}
	active, err := s.Facts.GetActiveFactVersion(ctx, fact.ID)
	if err != nil {
		return nil, err
	}
	var supersedes *uuid.UUID
	if in.Confirm && active != nil {
		if in.SupersedesVersionID == nil {
			return nil, ErrSupersedeRequired
		}
		if *in.SupersedesVersionID != active.ID {
			return nil, ErrInvalidSupersedes
		}
		supersedes = in.SupersedesVersionID
	} else if in.SupersedesVersionID != nil {
		supersedes = in.SupersedesVersionID
	}

	uid := userID
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = string(domainfacts.SourceUser)
	}
	if !domainfacts.Source(source).Valid() {
		return nil, fmt.Errorf("invalid source")
	}
	ver := driven.FactVersionRow{
		ID: uuid.New(), FactID: fact.ID, Status: string(status),
		ValueJSON: valueJSON, ValueText: valueText, Unit: unit,
		Source: source, Confidence: in.Confidence, InterpretationID: in.InterpretationID,
		CreatedByUserID: &uid, SupersedesVersionID: supersedes, CreatedAt: now,
	}
	if err := s.Facts.CreateFactVersion(ctx, ver); err != nil {
		return nil, err
	}

	if in.Confirm && active != nil {
		active.Status = string(domainfacts.StatusSuperseded)
		active.SupersededByVersionID = &ver.ID
		active.SupersededAt = &now
		if err := s.Facts.UpdateFactVersion(ctx, *active); err != nil {
			return nil, err
		}
	}

	for _, ref := range in.Evidence {
		ev := driven.FactEvidenceRow{
			ID: uuid.New(), FactVersionID: ver.ID,
			MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
		}
		if err := s.Facts.AddFactEvidence(ctx, ev); err != nil {
			return nil, err
		}
	}

	return s.Get(ctx, userID, fact.ID)
}

func (s *Service) Confirm(ctx context.Context, userID, versionID uuid.UUID, in ConfirmInput) (*FactView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	ver, err := s.Facts.GetFactVersion(ctx, orgID, versionID)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, ErrNotFound
	}
	fact, err := s.Facts.GetFact(ctx, orgID, ver.FactID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, fact.ProjectID); err != nil {
		return nil, err
	}
	if ver.Status != string(domainfacts.StatusProposed) {
		return nil, ErrNotProposed
	}

	active, err := s.Facts.GetActiveFactVersion(ctx, fact.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if active != nil {
		supersedes := in.SupersedesVersionID
		if supersedes == nil {
			supersedes = ver.SupersedesVersionID
		}
		if supersedes == nil {
			return nil, ErrSupersedeRequired
		}
		if *supersedes != active.ID {
			return nil, ErrInvalidSupersedes
		}
		ver.SupersedesVersionID = supersedes
		active.Status = string(domainfacts.StatusSuperseded)
		active.SupersededByVersionID = &ver.ID
		active.SupersededAt = &now
		if err := s.Facts.UpdateFactVersion(ctx, *active); err != nil {
			return nil, err
		}
	}

	ver.Status = string(domainfacts.StatusActive)
	if err := s.Facts.UpdateFactVersion(ctx, *ver); err != nil {
		return nil, err
	}
	fact.UpdatedAt = now
	if err := s.Facts.UpdateFact(ctx, *fact); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, fact.ID)
}

func (s *Service) Reject(ctx context.Context, userID, versionID uuid.UUID) (*FactView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	ver, err := s.Facts.GetFactVersion(ctx, orgID, versionID)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, ErrNotFound
	}
	fact, err := s.Facts.GetFact(ctx, orgID, ver.FactID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, fact.ProjectID); err != nil {
		return nil, err
	}
	if ver.Status != string(domainfacts.StatusProposed) {
		return nil, ErrNotProposed
	}
	ver.Status = string(domainfacts.StatusRejected)
	if err := s.Facts.UpdateFactVersion(ctx, *ver); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, fact.ID)
}

func (s *Service) AddEvidence(ctx context.Context, userID, versionID uuid.UUID, ref EvidenceRef) (*FactView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	ver, err := s.Facts.GetFactVersion(ctx, orgID, versionID)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, ErrNotFound
	}
	fact, err := s.Facts.GetFact(ctx, orgID, ver.FactID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, fact.ProjectID); err != nil {
		return nil, err
	}
	if err := s.validateEvidenceOnProject(ctx, userID, orgID, fact.ProjectID, ref); err != nil {
		return nil, err
	}
	ev := driven.FactEvidenceRow{
		ID: uuid.New(), FactVersionID: versionID,
		MessageID: ref.MessageID, ManualItemID: ref.ManualItemID,
		AddedAt: time.Now().UTC(),
	}
	if err := s.Facts.AddFactEvidence(ctx, ev); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, fact.ID)
}

func (s *Service) RemoveEvidence(ctx context.Context, userID, versionID, evidenceID uuid.UUID) (*FactView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	ver, err := s.Facts.GetFactVersion(ctx, orgID, versionID)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, ErrNotFound
	}
	fact, err := s.Facts.GetFact(ctx, orgID, ver.FactID)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, fact.ProjectID); err != nil {
		return nil, err
	}
	if err := s.Facts.RemoveFactEvidence(ctx, orgID, versionID, evidenceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.Get(ctx, userID, fact.ID)
}

func (s *Service) buildFactView(ctx context.Context, fact driven.FactRow, include ListInclude, allVersions bool) (*FactView, error) {
	versions, err := s.Facts.ListFactVersions(ctx, fact.ID)
	if err != nil {
		return nil, err
	}
	evidenceAll, err := s.Facts.ListFactEvidenceForFact(ctx, fact.ID)
	if err != nil {
		return nil, err
	}
	byVersion := map[uuid.UUID][]driven.FactEvidenceRow{}
	for _, ev := range evidenceAll {
		byVersion[ev.FactVersionID] = append(byVersion[ev.FactVersionID], ev)
	}
	out := &FactView{Fact: fact, Versions: make([]VersionView, 0, len(versions))}
	for _, ver := range versions {
		if !allVersions {
			switch domainfacts.Status(ver.Status) {
			case domainfacts.StatusActive:
				// always include when listing defaults
			case domainfacts.StatusProposed:
				if !include.Proposed {
					continue
				}
			case domainfacts.StatusSuperseded, domainfacts.StatusRejected:
				if !include.History {
					continue
				}
			default:
				continue
			}
		}
		out.Versions = append(out.Versions, VersionView{
			Version:  ver,
			Evidence: byVersion[ver.ID],
		})
	}
	return out, nil
}

func (s *Service) validateEvidenceOnProject(ctx context.Context, userID, orgID, projectID uuid.UUID, ref EvidenceRef) error {
	msgSet := ref.MessageID != nil
	manualSet := ref.ManualItemID != nil
	if !domainfacts.ValidEvidenceXOR(msgSet, manualSet) {
		return ErrInvalidEvidence
	}
	if ref.MessageID != nil {
		eff, err := s.Assignments.EffectiveAssignment(ctx, userID, *ref.MessageID)
		if err != nil {
			return err
		}
		if eff == nil || eff.ProjectID == nil || *eff.ProjectID != projectID {
			return ErrWrongProject
		}
		return nil
	}
	manual, err := s.Manuals.GetManualItem(ctx, orgID, *ref.ManualItemID)
	if err != nil {
		return err
	}
	if manual == nil || manual.ProjectID == nil || *manual.ProjectID != projectID {
		return ErrWrongProject
	}
	return nil
}

func encodeValue(v any) (valueJSON, valueText string, err error) {
	if v == nil {
		return "", "", fmt.Errorf("value required")
	}
	switch t := v.(type) {
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		return string(b), t, nil
	case float64:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		return string(b), trimFloat(t), nil
	case json.Number:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		return string(b), t.String(), nil
	case bool:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		if t {
			return string(b), "true", nil
		}
		return string(b), "false", nil
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		text := string(b)
		if amount, ok := t["amount"]; ok {
			unit, _ := t["unit"].(string)
			text = fmt.Sprintf("%v", amount)
			if unit != "" {
				text = text + " " + unit
			}
		}
		return string(b), text, nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", "", err
		}
		return string(b), string(b), nil
	}
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%g", f)
	return s
}
