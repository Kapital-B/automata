package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) CreateContradiction(ctx context.Context, row driven.ContradictionRow) error {
	var note, resolvedAt, resolvedBy any
	if row.ResolutionNote != nil {
		note = *row.ResolutionNote
	}
	if row.ResolvedAt != nil {
		resolvedAt = formatRFC3339(row.ResolvedAt.UTC())
	}
	if row.ResolvedByUserID != nil {
		resolvedBy = row.ResolvedByUserID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contradictions (
			id, organisation_id, project_id, status, summary, resolution_note,
			resolved_at, resolved_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), row.Status, row.Summary,
		note, resolvedAt, resolvedBy,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) AddContradictionSide(ctx context.Context, row driven.ContradictionSideRow) error {
	var fv, dec any
	if row.FactVersionID != nil {
		fv = row.FactVersionID.String()
	}
	if row.DecisionID != nil {
		dec = row.DecisionID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contradiction_sides (id, contradiction_id, fact_version_id, decision_id)
		VALUES (?, ?, ?, ?)
	`, row.ID.String(), row.ContradictionID.String(), fv, dec)
	return err
}

func (r *Repository) GetContradiction(ctx context.Context, organisationID, id uuid.UUID) (*driven.ContradictionRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, status, summary, resolution_note,
			resolved_at, resolved_by_user_id, created_at, updated_at
		FROM contradictions WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	out, err := scanContradiction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListContradictionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]driven.ContradictionRow, error) {
	q := `
		SELECT id, organisation_id, project_id, status, summary, resolution_note,
			resolved_at, resolved_by_user_id, created_at, updated_at
		FROM contradictions WHERE organisation_id = ? AND project_id = ?`
	args := []any{organisationID.String(), projectID.String()}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContradictionRow, 0)
	for rows.Next() {
		c, err := scanContradiction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateContradiction(ctx context.Context, row driven.ContradictionRow) error {
	var note, resolvedAt, resolvedBy any
	if row.ResolutionNote != nil {
		note = *row.ResolutionNote
	}
	if row.ResolvedAt != nil {
		resolvedAt = formatRFC3339(row.ResolvedAt.UTC())
	}
	if row.ResolvedByUserID != nil {
		resolvedBy = row.ResolvedByUserID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE contradictions SET status = ?, summary = ?, resolution_note = ?,
			resolved_at = ?, resolved_by_user_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, row.Status, row.Summary, note, resolvedAt, resolvedBy,
		formatRFC3339(row.UpdatedAt.UTC()), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListContradictionSides(ctx context.Context, contradictionID uuid.UUID) ([]driven.ContradictionSideRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, contradiction_id, fact_version_id, decision_id
		FROM contradiction_sides WHERE contradiction_id = ?
	`, contradictionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContradictionSideRow, 0)
	for rows.Next() {
		s, err := scanContradictionSide(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func scanContradiction(s rowScanner) (*driven.ContradictionRow, error) {
	var idStr, orgStr, projStr, status, summary, createdAt, updatedAt string
	var note, resolvedAt, resolvedBy sql.NullString
	if err := s.Scan(&idStr, &orgStr, &projStr, &status, &summary, &note, &resolvedAt, &resolvedBy, &createdAt, &updatedAt); err != nil {
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
	row := &driven.ContradictionRow{
		ID: id, OrganisationID: orgID, ProjectID: projID, Status: status,
		Summary: summary, CreatedAt: cat, UpdatedAt: uat,
	}
	if note.Valid {
		n := note.String
		row.ResolutionNote = &n
	}
	if resolvedAt.Valid && resolvedAt.String != "" {
		t, err := parseTime(resolvedAt.String)
		if err != nil {
			return nil, err
		}
		row.ResolvedAt = &t
	}
	if resolvedBy.Valid && resolvedBy.String != "" {
		uid, err := uuid.Parse(resolvedBy.String)
		if err != nil {
			return nil, err
		}
		row.ResolvedByUserID = &uid
	}
	return row, nil
}

func scanContradictionSide(s rowScanner) (*driven.ContradictionSideRow, error) {
	var idStr, cidStr string
	var fv, dec sql.NullString
	if err := s.Scan(&idStr, &cidStr, &fv, &dec); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(cidStr)
	if err != nil {
		return nil, err
	}
	row := &driven.ContradictionSideRow{ID: id, ContradictionID: cid}
	if fv.Valid && fv.String != "" {
		vid, err := uuid.Parse(fv.String)
		if err != nil {
			return nil, err
		}
		row.FactVersionID = &vid
	}
	if dec.Valid && dec.String != "" {
		did, err := uuid.Parse(dec.String)
		if err != nil {
			return nil, err
		}
		row.DecisionID = &did
	}
	return row, nil
}
