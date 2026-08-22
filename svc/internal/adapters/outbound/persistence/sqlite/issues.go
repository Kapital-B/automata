package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) CreateIssue(ctx context.Context, row driven.IssueRow) error {
	var assigneeUser, assigneeContact any
	if row.AssigneeUserID != nil {
		assigneeUser = row.AssigneeUserID.String()
	}
	if row.AssigneeContactID != nil {
		assigneeContact = row.AssigneeContactID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO issues (
			id, organisation_id, project_id, title, current_position_note, status,
			assignee_user_id, assignee_contact_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), row.Title,
		row.CurrentPositionNote, row.Status, assigneeUser, assigneeContact,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) GetIssue(ctx context.Context, organisationID, issueID uuid.UUID) (*driven.IssueRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, title, current_position_note, status,
			assignee_user_id, assignee_contact_id, created_at, updated_at
		FROM issues WHERE id = ? AND organisation_id = ?
	`, issueID.String(), organisationID.String())
	iss, err := scanIssueRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return iss, err
}

func (r *Repository) ListIssuesByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.IssueRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, project_id, title, current_position_note, status,
			assignee_user_id, assignee_contact_id, created_at, updated_at
		FROM issues WHERE organisation_id = ? AND project_id = ?
		ORDER BY updated_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.IssueRow, 0)
	for rows.Next() {
		iss, err := scanIssueRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *iss)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateIssue(ctx context.Context, row driven.IssueRow) error {
	var assigneeUser, assigneeContact any
	if row.AssigneeUserID != nil {
		assigneeUser = row.AssigneeUserID.String()
	}
	if row.AssigneeContactID != nil {
		assigneeContact = row.AssigneeContactID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE issues SET title = ?, current_position_note = ?, status = ?,
			assignee_user_id = ?, assignee_contact_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, row.Title, row.CurrentPositionNote, row.Status, assigneeUser, assigneeContact,
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

func (r *Repository) AddIssueItem(ctx context.Context, row driven.IssueItemRow) error {
	var msgID, manualID any
	if row.MessageID != nil {
		msgID = row.MessageID.String()
	}
	if row.ManualItemID != nil {
		manualID = row.ManualItemID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO issue_items (id, issue_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.IssueID.String(), msgID, manualID, formatRFC3339(row.AddedAt.UTC()))
	return err
}

func (r *Repository) RemoveIssueItem(ctx context.Context, organisationID, issueID, itemID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM issue_items
		WHERE id = ? AND issue_id = ?
		  AND issue_id IN (SELECT id FROM issues WHERE organisation_id = ?)
	`, itemID.String(), issueID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetIssueItem(ctx context.Context, organisationID, itemID uuid.UUID) (*driven.IssueItemRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT ii.id, ii.issue_id, ii.message_id, ii.manual_item_id, ii.added_at
		FROM issue_items ii
		INNER JOIN issues i ON i.id = ii.issue_id
		WHERE ii.id = ? AND i.organisation_id = ?
	`, itemID.String(), organisationID.String())
	item, err := scanIssueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListIssueItems(ctx context.Context, issueID uuid.UUID) ([]driven.IssueItemRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, issue_id, message_id, manual_item_id, added_at
		FROM issue_items WHERE issue_id = ?
		ORDER BY added_at ASC
	`, issueID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.IssueItemRow, 0)
	for rows.Next() {
		item, err := scanIssueItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) FindIssueIDByMessage(ctx context.Context, messageID uuid.UUID) (*uuid.UUID, error) {
	var idStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT issue_id FROM issue_items WHERE message_id = ?
	`, messageID.String()).Scan(&idStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *Repository) FindIssueIDByManualItem(ctx context.Context, manualItemID uuid.UUID) (*uuid.UUID, error) {
	var idStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT issue_id FROM issue_items WHERE manual_item_id = ?
	`, manualItemID.String()).Scan(&idStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func scanIssueRow(s rowScanner) (*driven.IssueRow, error) {
	var idStr, orgStr, projStr, title, note, status, createdAt, updatedAt string
	var assigneeUser, assigneeContact sql.NullString
	if err := s.Scan(&idStr, &orgStr, &projStr, &title, &note, &status, &assigneeUser, &assigneeContact, &createdAt, &updatedAt); err != nil {
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
	row := &driven.IssueRow{
		ID: id, OrganisationID: orgID, ProjectID: projID, Title: title,
		CurrentPositionNote: note, Status: status, CreatedAt: cat, UpdatedAt: uat,
	}
	if assigneeUser.Valid && assigneeUser.String != "" {
		uid, err := uuid.Parse(assigneeUser.String)
		if err != nil {
			return nil, err
		}
		row.AssigneeUserID = &uid
	}
	if assigneeContact.Valid && assigneeContact.String != "" {
		cid, err := uuid.Parse(assigneeContact.String)
		if err != nil {
			return nil, err
		}
		row.AssigneeContactID = &cid
	}
	return row, nil
}

func scanIssueItem(s rowScanner) (*driven.IssueItemRow, error) {
	var idStr, issueStr, addedAt string
	var msg, manual sql.NullString
	if err := s.Scan(&idStr, &issueStr, &msg, &manual, &addedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	issueID, err := uuid.Parse(issueStr)
	if err != nil {
		return nil, err
	}
	at, err := parseTime(addedAt)
	if err != nil {
		return nil, err
	}
	row := &driven.IssueItemRow{ID: id, IssueID: issueID, AddedAt: at}
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