package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) CreateFact(ctx context.Context, row driven.FactRow) error {
	var issueID any
	if row.IssueID != nil {
		issueID = row.IssueID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO facts (
			id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), issueID,
		row.SubjectKey, row.Label, formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) UpdateFact(ctx context.Context, row driven.FactRow) error {
	var issueID any
	if row.IssueID != nil {
		issueID = row.IssueID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE facts SET issue_id = ?, label = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, issueID, row.Label, formatRFC3339(row.UpdatedAt.UTC()), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetFact(ctx context.Context, organisationID, factID uuid.UUID) (*driven.FactRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE id = ? AND organisation_id = ?
	`, factID.String(), organisationID.String())
	fact, err := scanFactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return fact, err
}

func (r *Repository) GetFactBySubject(ctx context.Context, organisationID, projectID uuid.UUID, subjectKey string) (*driven.FactRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE organisation_id = ? AND project_id = ? AND subject_key = ?
	`, organisationID.String(), projectID.String(), subjectKey)
	fact, err := scanFactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return fact, err
}

func (r *Repository) ListFactsByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.FactRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE organisation_id = ? AND project_id = ?
		ORDER BY label ASC, subject_key ASC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FactRow, 0)
	for rows.Next() {
		fact, err := scanFactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *fact)
	}
	return out, rows.Err()
}

func (r *Repository) CreateFactVersion(ctx context.Context, row driven.FactVersionRow) error {
	var unit, conf, interp, supersedes, supersededBy, supersededAt, createdBy any
	if row.Unit != nil {
		unit = *row.Unit
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	if row.InterpretationID != nil {
		interp = row.InterpretationID.String()
	}
	if row.SupersedesVersionID != nil {
		supersedes = row.SupersedesVersionID.String()
	}
	if row.SupersededByVersionID != nil {
		supersededBy = row.SupersededByVersionID.String()
	}
	if row.SupersededAt != nil {
		supersededAt = formatRFC3339(row.SupersededAt.UTC())
	}
	if row.CreatedByUserID != nil {
		createdBy = row.CreatedByUserID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO fact_versions (
			id, fact_id, status, value_json, value_text, unit, source, confidence,
			interpretation_id, supersedes_version_id, superseded_by_version_id,
			superseded_at, created_by_user_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.FactID.String(), row.Status, row.ValueJSON, row.ValueText,
		unit, row.Source, conf, interp, supersedes, supersededBy, supersededAt, createdBy,
		formatRFC3339(row.CreatedAt.UTC()))
	return err
}

func (r *Repository) GetFactVersion(ctx context.Context, organisationID, versionID uuid.UUID) (*driven.FactVersionRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT fv.id, fv.fact_id, fv.status, fv.value_json, fv.value_text, fv.unit, fv.source,
			fv.confidence, fv.interpretation_id, fv.supersedes_version_id, fv.superseded_by_version_id,
			fv.superseded_at, fv.created_by_user_id, fv.created_at
		FROM fact_versions fv
		INNER JOIN facts f ON f.id = fv.fact_id
		WHERE fv.id = ? AND f.organisation_id = ?
	`, versionID.String(), organisationID.String())
	ver, err := scanFactVersionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ver, err
}

func (r *Repository) ListFactVersions(ctx context.Context, factID uuid.UUID) ([]driven.FactVersionRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fact_id, status, value_json, value_text, unit, source,
			confidence, interpretation_id, supersedes_version_id, superseded_by_version_id,
			superseded_at, created_by_user_id, created_at
		FROM fact_versions WHERE fact_id = ?
		ORDER BY created_at ASC
	`, factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FactVersionRow, 0)
	for rows.Next() {
		ver, err := scanFactVersionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ver)
	}
	return out, rows.Err()
}

func (r *Repository) GetActiveFactVersion(ctx context.Context, factID uuid.UUID) (*driven.FactVersionRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, fact_id, status, value_json, value_text, unit, source,
			confidence, interpretation_id, supersedes_version_id, superseded_by_version_id,
			superseded_at, created_by_user_id, created_at
		FROM fact_versions WHERE fact_id = ? AND status = 'active'
	`, factID.String())
	ver, err := scanFactVersionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ver, err
}

func (r *Repository) UpdateFactVersion(ctx context.Context, row driven.FactVersionRow) error {
	var unit, conf, interp, supersedes, supersededBy, supersededAt, createdBy any
	if row.Unit != nil {
		unit = *row.Unit
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	if row.InterpretationID != nil {
		interp = row.InterpretationID.String()
	}
	if row.SupersedesVersionID != nil {
		supersedes = row.SupersedesVersionID.String()
	}
	if row.SupersededByVersionID != nil {
		supersededBy = row.SupersededByVersionID.String()
	}
	if row.SupersededAt != nil {
		supersededAt = formatRFC3339(row.SupersededAt.UTC())
	}
	if row.CreatedByUserID != nil {
		createdBy = row.CreatedByUserID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE fact_versions SET status = ?, value_json = ?, value_text = ?, unit = ?, source = ?,
			confidence = ?, interpretation_id = ?, supersedes_version_id = ?,
			superseded_by_version_id = ?, superseded_at = ?, created_by_user_id = ?
		WHERE id = ?
	`, row.Status, row.ValueJSON, row.ValueText, unit, row.Source, conf, interp,
		supersedes, supersededBy, supersededAt, createdBy, row.ID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) AddFactEvidence(ctx context.Context, row driven.FactEvidenceRow) error {
	var msgID, manualID any
	if row.MessageID != nil {
		msgID = row.MessageID.String()
	}
	if row.ManualItemID != nil {
		manualID = row.ManualItemID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO fact_evidence (id, fact_version_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.FactVersionID.String(), msgID, manualID, formatRFC3339(row.AddedAt.UTC()))
	return err
}

func (r *Repository) RemoveFactEvidence(ctx context.Context, organisationID, versionID, evidenceID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM fact_evidence
		WHERE id = ? AND fact_version_id = ?
		  AND fact_version_id IN (
			SELECT fv.id FROM fact_versions fv
			INNER JOIN facts f ON f.id = fv.fact_id
			WHERE f.organisation_id = ?
		  )
	`, evidenceID.String(), versionID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListFactEvidence(ctx context.Context, versionID uuid.UUID) ([]driven.FactEvidenceRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fact_version_id, message_id, manual_item_id, added_at
		FROM fact_evidence WHERE fact_version_id = ?
		ORDER BY added_at ASC
	`, versionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactEvidenceRows(rows)
}

func (r *Repository) ListFactEvidenceForFact(ctx context.Context, factID uuid.UUID) ([]driven.FactEvidenceRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT fe.id, fe.fact_version_id, fe.message_id, fe.manual_item_id, fe.added_at
		FROM fact_evidence fe
		INNER JOIN fact_versions fv ON fv.id = fe.fact_version_id
		WHERE fv.fact_id = ?
		ORDER BY fe.added_at ASC
	`, factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactEvidenceRows(rows)
}

func scanFactEvidenceRows(rows *sql.Rows) ([]driven.FactEvidenceRow, error) {
	out := make([]driven.FactEvidenceRow, 0)
	for rows.Next() {
		ev, err := scanFactEvidenceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

func scanFactRow(s rowScanner) (*driven.FactRow, error) {
	var idStr, orgStr, projStr, subjectKey, label, createdAt, updatedAt string
	var issueID sql.NullString
	if err := s.Scan(&idStr, &orgStr, &projStr, &issueID, &subjectKey, &label, &createdAt, &updatedAt); err != nil {
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
	row := &driven.FactRow{
		ID: id, OrganisationID: orgID, ProjectID: projID,
		SubjectKey: subjectKey, Label: label, CreatedAt: cat, UpdatedAt: uat,
	}
	if issueID.Valid && issueID.String != "" {
		iid, err := uuid.Parse(issueID.String)
		if err != nil {
			return nil, err
		}
		row.IssueID = &iid
	}
	return row, nil
}

func scanFactVersionRow(s rowScanner) (*driven.FactVersionRow, error) {
	var idStr, factStr, status, valueJSON, valueText, source, createdAt string
	var unit sql.NullString
	var conf sql.NullFloat64
	var interp, supersedes, supersededBy, supersededAt, createdBy sql.NullString
	if err := s.Scan(
		&idStr, &factStr, &status, &valueJSON, &valueText, &unit, &source,
		&conf, &interp, &supersedes, &supersededBy, &supersededAt, &createdBy, &createdAt,
	); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	factID, err := uuid.Parse(factStr)
	if err != nil {
		return nil, err
	}
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	row := &driven.FactVersionRow{
		ID: id, FactID: factID, Status: status, ValueJSON: valueJSON,
		ValueText: valueText, Source: source, CreatedAt: cat,
	}
	if unit.Valid {
		u := unit.String
		row.Unit = &u
	}
	if conf.Valid {
		c := conf.Float64
		row.Confidence = &c
	}
	if interp.Valid && interp.String != "" {
		uid, err := uuid.Parse(interp.String)
		if err != nil {
			return nil, err
		}
		row.InterpretationID = &uid
	}
	if supersedes.Valid && supersedes.String != "" {
		uid, err := uuid.Parse(supersedes.String)
		if err != nil {
			return nil, err
		}
		row.SupersedesVersionID = &uid
	}
	if supersededBy.Valid && supersededBy.String != "" {
		uid, err := uuid.Parse(supersededBy.String)
		if err != nil {
			return nil, err
		}
		row.SupersededByVersionID = &uid
	}
	if supersededAt.Valid && supersededAt.String != "" {
		t, err := parseTime(supersededAt.String)
		if err != nil {
			return nil, err
		}
		row.SupersededAt = &t
	}
	if createdBy.Valid && createdBy.String != "" {
		uid, err := uuid.Parse(createdBy.String)
		if err != nil {
			return nil, err
		}
		row.CreatedByUserID = &uid
	}
	return row, nil
}

func scanFactEvidenceRow(s rowScanner) (*driven.FactEvidenceRow, error) {
	var idStr, versionStr, addedAt string
	var msg, manual sql.NullString
	if err := s.Scan(&idStr, &versionStr, &msg, &manual, &addedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	versionID, err := uuid.Parse(versionStr)
	if err != nil {
		return nil, err
	}
	at, err := parseTime(addedAt)
	if err != nil {
		return nil, err
	}
	row := &driven.FactEvidenceRow{ID: id, FactVersionID: versionID, AddedAt: at}
	if msg.Valid && msg.String != "" {
		mid, err := uuid.Parse(msg.String)
		if err != nil {
			return nil, err
		}
		row.MessageID = &mid
	}
	if manual.Valid && manual.String != "" {
		mid, err := uuid.Parse(manual.String)
		if err != nil {
			return nil, err
		}
		row.ManualItemID = &mid
	}
	return row, nil
}
