package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) CreateDecision(ctx context.Context, row driven.DecisionRow) error {
	var issueID, decidedAt, assigneeUser, assigneeContact, conf, supersedes, createdBy any
	if row.IssueID != nil {
		issueID = row.IssueID.String()
	}
	if row.DecidedAt != nil {
		decidedAt = formatRFC3339(row.DecidedAt.UTC())
	}
	if row.AssigneeUserID != nil {
		assigneeUser = row.AssigneeUserID.String()
	}
	if row.AssigneeContactID != nil {
		assigneeContact = row.AssigneeContactID.String()
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	if row.SupersedesDecisionID != nil {
		supersedes = row.SupersedesDecisionID.String()
	}
	if row.CreatedByUserID != nil {
		createdBy = row.CreatedByUserID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO decisions (
			id, organisation_id, project_id, issue_id, statement, status, decided_at,
			assignee_user_id, assignee_contact_id, source, confidence, supersedes_decision_id,
			created_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), issueID, row.Statement, row.Status,
		decidedAt, assigneeUser, assigneeContact, row.Source, conf, supersedes, createdBy,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) GetDecision(ctx context.Context, organisationID, id uuid.UUID) (*driven.DecisionRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, statement, status, decided_at,
			assignee_user_id, assignee_contact_id, source, confidence, supersedes_decision_id,
			created_by_user_id, created_at, updated_at
		FROM decisions WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	out, err := scanDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListDecisionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]driven.DecisionRow, error) {
	q := `
		SELECT id, organisation_id, project_id, issue_id, statement, status, decided_at,
			assignee_user_id, assignee_contact_id, source, confidence, supersedes_decision_id,
			created_by_user_id, created_at, updated_at
		FROM decisions WHERE organisation_id = ? AND project_id = ?`
	args := []any{organisationID.String(), projectID.String()}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY COALESCE(decided_at, updated_at) DESC, created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.DecisionRow, 0)
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateDecision(ctx context.Context, row driven.DecisionRow) error {
	var issueID, decidedAt, assigneeUser, assigneeContact, conf, supersedes, createdBy any
	if row.IssueID != nil {
		issueID = row.IssueID.String()
	}
	if row.DecidedAt != nil {
		decidedAt = formatRFC3339(row.DecidedAt.UTC())
	}
	if row.AssigneeUserID != nil {
		assigneeUser = row.AssigneeUserID.String()
	}
	if row.AssigneeContactID != nil {
		assigneeContact = row.AssigneeContactID.String()
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	if row.SupersedesDecisionID != nil {
		supersedes = row.SupersedesDecisionID.String()
	}
	if row.CreatedByUserID != nil {
		createdBy = row.CreatedByUserID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE decisions SET issue_id = ?, statement = ?, status = ?, decided_at = ?,
			assignee_user_id = ?, assignee_contact_id = ?, source = ?, confidence = ?,
			supersedes_decision_id = ?, created_by_user_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, issueID, row.Statement, row.Status, decidedAt, assigneeUser, assigneeContact, row.Source, conf,
		supersedes, createdBy, formatRFC3339(row.UpdatedAt.UTC()), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) AddDecisionEvidence(ctx context.Context, row driven.DecisionEvidenceRow) error {
	var msgID, manualID any
	if row.MessageID != nil {
		msgID = row.MessageID.String()
	}
	if row.ManualItemID != nil {
		manualID = row.ManualItemID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO decision_evidence (id, decision_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.DecisionID.String(), msgID, manualID, formatRFC3339(row.AddedAt.UTC()))
	return err
}

func (r *Repository) RemoveDecisionEvidence(ctx context.Context, organisationID, decisionID, evidenceID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM decision_evidence
		WHERE id = ? AND decision_id = ?
		  AND decision_id IN (SELECT id FROM decisions WHERE organisation_id = ?)
	`, evidenceID.String(), decisionID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListDecisionEvidence(ctx context.Context, decisionID uuid.UUID) ([]driven.DecisionEvidenceRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, decision_id, message_id, manual_item_id, added_at
		FROM decision_evidence WHERE decision_id = ?
		ORDER BY added_at ASC
	`, decisionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.DecisionEvidenceRow, 0)
	for rows.Next() {
		ev, err := scanDecisionEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

func scanDecision(s rowScanner) (*driven.DecisionRow, error) {
	var idStr, orgStr, projStr, statement, status, source, createdAt, updatedAt string
	var issueID, decidedAt, assigneeUser, assigneeContact, supersedes, createdBy sql.NullString
	var conf sql.NullFloat64
	if err := s.Scan(&idStr, &orgStr, &projStr, &issueID, &statement, &status, &decidedAt,
		&assigneeUser, &assigneeContact, &source, &conf, &supersedes, &createdBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	orgID, err := uuid.Parse(orgStr)
	if err != nil {
		return nil, err
	}
	projID, err := uuid.Parse(projStr)
	if err != nil {
		return nil, err
	}
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	uat, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	row := &driven.DecisionRow{
		ID: id, OrganisationID: orgID, ProjectID: projID, Statement: statement,
		Status: status, Source: source, CreatedAt: cat, UpdatedAt: uat,
	}
	if issueID.Valid && issueID.String != "" {
		v, err := uuid.Parse(issueID.String)
		if err != nil {
			return nil, err
		}
		row.IssueID = &v
	}
	if decidedAt.Valid && decidedAt.String != "" {
		t, err := parseTime(decidedAt.String)
		if err != nil {
			return nil, err
		}
		row.DecidedAt = &t
	}
	if assigneeUser.Valid && assigneeUser.String != "" {
		v, err := uuid.Parse(assigneeUser.String)
		if err != nil {
			return nil, err
		}
		row.AssigneeUserID = &v
	}
	if assigneeContact.Valid && assigneeContact.String != "" {
		v, err := uuid.Parse(assigneeContact.String)
		if err != nil {
			return nil, err
		}
		row.AssigneeContactID = &v
	}
	if conf.Valid {
		c := conf.Float64
		row.Confidence = &c
	}
	if supersedes.Valid && supersedes.String != "" {
		v, err := uuid.Parse(supersedes.String)
		if err != nil {
			return nil, err
		}
		row.SupersedesDecisionID = &v
	}
	if createdBy.Valid && createdBy.String != "" {
		v, err := uuid.Parse(createdBy.String)
		if err != nil {
			return nil, err
		}
		row.CreatedByUserID = &v
	}
	return row, nil
}

func scanDecisionEvidence(s rowScanner) (*driven.DecisionEvidenceRow, error) {
	var idStr, didStr, addedAt string
	var msgID, manualID sql.NullString
	if err := s.Scan(&idStr, &didStr, &msgID, &manualID, &addedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	did, err := uuid.Parse(didStr)
	if err != nil {
		return nil, err
	}
	at, err := parseTime(addedAt)
	if err != nil {
		return nil, err
	}
	row := &driven.DecisionEvidenceRow{ID: id, DecisionID: did, AddedAt: at}
	if msgID.Valid && msgID.String != "" {
		v, err := uuid.Parse(msgID.String)
		if err != nil {
			return nil, err
		}
		row.MessageID = &v
	}
	if manualID.Valid && manualID.String != "" {
		v, err := uuid.Parse(manualID.String)
		if err != nil {
			return nil, err
		}
		row.ManualItemID = &v
	}
	return row, nil
}
