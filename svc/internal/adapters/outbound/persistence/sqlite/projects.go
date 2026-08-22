package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) ListContactIDsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT contact_id FROM correspondence_participants
		WHERE organisation_id = ? AND message_id = ?
	`, organisationID.String(), messageID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func (r *Repository) ListContactIDsForThread(ctx context.Context, organisationID, accountID uuid.UUID, conversationID string) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT cp.contact_id
		FROM correspondence_participants cp
		INNER JOIN messages m ON m.id = cp.message_id
		WHERE cp.organisation_id = ? AND m.account_id = ? AND m.conversation_id = ?
	`, organisationID.String(), accountID.String(), conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func scanUUIDColumn(rows *sql.Rows) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) ListProjects(ctx context.Context, organisationID uuid.UUID, filter driven.ProjectListFilter) ([]driven.ProjectRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	q := `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE organisation_id = ?`
	args := []any{organisationID.String()}
	if !filter.IncludeArchived {
		q += ` AND archived_at IS NULL`
	}
	q += ` ORDER BY code ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ProjectRow, 0)
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, organisationID, projectID uuid.UUID) (*driven.ProjectRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE id = ? AND organisation_id = ?
	`, projectID.String(), organisationID.String())
	p, err := scanProjectRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) GetProjectByCode(ctx context.Context, organisationID uuid.UUID, code string) (*driven.ProjectRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE organisation_id = ? AND code = ?
	`, organisationID.String(), code)
	p, err := scanProjectRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) CreateProject(ctx context.Context, project driven.ProjectRow, member driven.ProjectMemberRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	kw, _ := json.Marshal(project.Keywords)
	if project.Keywords == nil {
		kw = []byte("[]")
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO projects (id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
	`, project.ID.String(), project.OrganisationID.String(), project.Name, project.Code,
		nullStr(project.Description), nullStr(project.Client), string(kw),
		formatRFC3339(project.CreatedAt.UTC()), formatRFC3339(project.UpdatedAt.UTC()))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO project_members (
			id, project_id, user_id, role, discipline, responsibilities, current_scope, approval_authority, out_of_scope, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, member.ID.String(), member.ProjectID.String(), member.UserID.String(), member.Role,
		nullStr(member.Discipline), nullStr(member.Responsibilities), nullStr(member.CurrentScope),
		nullStr(member.ApprovalAuthority), nullStr(member.OutOfScope),
		formatRFC3339(member.CreatedAt.UTC()), formatRFC3339(member.UpdatedAt.UTC()))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpdateProject(ctx context.Context, project driven.ProjectRow) error {
	kw, _ := json.Marshal(project.Keywords)
	if project.Keywords == nil {
		kw = []byte("[]")
	}
	var archived any
	if project.ArchivedAt != nil {
		archived = formatRFC3339(project.ArchivedAt.UTC())
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE projects SET name = ?, description = ?, client = ?, keywords_json = ?, archived_at = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, project.Name, nullStr(project.Description), nullStr(project.Client), string(kw), archived,
		formatRFC3339(project.UpdatedAt.UTC()), project.ID.String(), project.OrganisationID.String())
	return err
}

func (r *Repository) GetProjectMember(ctx context.Context, projectID, userID uuid.UUID) (*driven.ProjectMemberRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, user_id, role, discipline, responsibilities, current_scope, approval_authority, out_of_scope, created_at, updated_at
		FROM project_members WHERE project_id = ? AND user_id = ?
	`, projectID.String(), userID.String())
	m, err := scanMemberRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *Repository) UpdateProjectMember(ctx context.Context, member driven.ProjectMemberRow) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE project_members
		SET role = ?, discipline = ?, responsibilities = ?, current_scope = ?, approval_authority = ?, out_of_scope = ?, updated_at = ?
		WHERE project_id = ? AND user_id = ?
	`, member.Role, nullStr(member.Discipline), nullStr(member.Responsibilities), nullStr(member.CurrentScope),
		nullStr(member.ApprovalAuthority), nullStr(member.OutOfScope), formatRFC3339(member.UpdatedAt.UTC()),
		member.ProjectID.String(), member.UserID.String())
	return err
}

func (r *Repository) UpsertProjectParticipant(ctx context.Context, projectID, contactID uuid.UUID, firstSeenAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO project_participants (project_id, contact_id, first_seen_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id, contact_id) DO NOTHING
	`, projectID.String(), contactID.String(), formatRFC3339(firstSeenAt.UTC()))
	return err
}

func (r *Repository) CountProjectMembers(ctx context.Context, projectID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM project_members WHERE project_id = ?`, projectID.String()).Scan(&n)
	return n, err
}

func scanProjectRow(s rowScanner) (*driven.ProjectRow, error) {
	var idStr, orgStr, name, code, kwJSON, createdAt, updatedAt string
	var desc, client, archived sql.NullString
	if err := s.Scan(&idStr, &orgStr, &name, &code, &desc, &client, &kwJSON, &archived, &createdAt, &updatedAt); err != nil {
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
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	uat, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	var keywords []string
	if strings.TrimSpace(kwJSON) != "" {
		_ = json.Unmarshal([]byte(kwJSON), &keywords)
	}
	if keywords == nil {
		keywords = []string{}
	}
	p := &driven.ProjectRow{
		ID: id, OrganisationID: orgID, Name: name, Code: code,
		Description: nullStringPtr(desc), Client: nullStringPtr(client),
		Keywords: keywords, CreatedAt: cat, UpdatedAt: uat,
	}
	if archived.Valid && archived.String != "" {
		t, err := parseTime(archived.String)
		if err != nil {
			return nil, err
		}
		p.ArchivedAt = &t
	}
	return p, nil
}

func scanMemberRow(s rowScanner) (*driven.ProjectMemberRow, error) {
	var idStr, projStr, userStr, role, createdAt, updatedAt string
	var discipline, resp, scope, approval, out sql.NullString
	if err := s.Scan(&idStr, &projStr, &userStr, &role, &discipline, &resp, &scope, &approval, &out, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	projID, err := uuid.Parse(projStr)
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(userStr)
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
	return &driven.ProjectMemberRow{
		ID: id, ProjectID: projID, UserID: userID, Role: role,
		Discipline: nullStringPtr(discipline), Responsibilities: nullStringPtr(resp),
		CurrentScope: nullStringPtr(scope), ApprovalAuthority: nullStringPtr(approval),
		OutOfScope: nullStringPtr(out), CreatedAt: cat, UpdatedAt: uat,
	}, nil
}

func (r *Repository) UpsertThreadAssignment(ctx context.Context, row driven.AssignmentRow) error {
	now := formatRFC3339(row.UpdatedAt.UTC())
	created := formatRFC3339(row.CreatedAt.UTC())
	var proj, runID, assignedBy any
	if row.ProjectID != nil {
		proj = row.ProjectID.String()
	}
	if row.RunID != nil {
		runID = row.RunID.String()
	}
	if row.AssignedByUserID != nil {
		assignedBy = row.AssignedByUserID.String()
	}
	var conf any
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO thread_assignments (
			id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, conversation_id) DO UPDATE SET
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.OrganisationID.String(), row.AccountID.String(), row.ConversationID,
		proj, row.Status, conf, row.Reason, row.Source, runID, assignedBy, created, now)
	return err
}

func (r *Repository) GetThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) (*driven.AssignmentRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		FROM thread_assignments WHERE account_id = ? AND conversation_id = ?
	`, accountID.String(), conversationID)
	a, err := scanThreadAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *Repository) DeleteThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM thread_assignments WHERE account_id = ? AND conversation_id = ?
	`, accountID.String(), conversationID)
	return err
}

func (r *Repository) UpsertMessageOverride(ctx context.Context, row driven.AssignmentRow) error {
	if row.MessageID == nil {
		return fmt.Errorf("message override requires message_id")
	}
	now := formatRFC3339(row.UpdatedAt.UTC())
	created := formatRFC3339(row.CreatedAt.UTC())
	var proj, runID, assignedBy any
	if row.ProjectID != nil {
		proj = row.ProjectID.String()
	}
	if row.RunID != nil {
		runID = row.RunID.String()
	}
	if row.AssignedByUserID != nil {
		assignedBy = row.AssignedByUserID.String()
	}
	var conf any
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO message_assignment_overrides (
			message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at
	`, row.MessageID.String(), row.OrganisationID.String(), row.AccountID.String(),
		proj, row.Status, conf, row.Reason, row.Source, runID, assignedBy, created, now)
	return err
}

func (r *Repository) GetMessageOverride(ctx context.Context, messageID uuid.UUID) (*driven.AssignmentRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		FROM message_assignment_overrides WHERE message_id = ?
	`, messageID.String())
	a, err := scanOverrideAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *Repository) DeleteMessageOverride(ctx context.Context, messageID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM message_assignment_overrides WHERE message_id = ?`, messageID.String())
	return err
}

func (r *Repository) EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*driven.EffectiveAssignment, error) {
	msg, err := r.GetMessage(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	out := &driven.EffectiveAssignment{
		Status:    "unassigned",
		Scope:     "none",
		AccountID: msg.AccountID,
		MessageID: msg.ID,
	}
	if msg.ConversationID != nil {
		out.ConversationID = msg.ConversationID
	}
	ov, err := r.GetMessageOverride(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if ov != nil {
		out.ProjectID = ov.ProjectID
		out.Status = ov.Status
		if ov.ProjectID == nil {
			out.Status = "unassigned"
		}
		out.Reason = ov.Reason
		out.Source = ov.Source
		out.Scope = "message"
		return out, nil
	}
	if msg.ConversationID != nil && strings.TrimSpace(*msg.ConversationID) != "" {
		th, err := r.GetThreadAssignment(ctx, msg.AccountID, *msg.ConversationID)
		if err != nil {
			return nil, err
		}
		if th != nil {
			out.ProjectID = th.ProjectID
			out.Status = th.Status
			if th.ProjectID == nil {
				out.Status = "unassigned"
			}
			out.Reason = th.Reason
			out.Source = th.Source
			out.Scope = "thread"
			return out, nil
		}
	}
	return out, nil
}

// EffectiveProjectIDsForMessages returns committed/provisional project ids for the given messages.
func (r *Repository) EffectiveProjectIDsForMessages(ctx context.Context, userID uuid.UUID, messages []driven.MessageRow) (map[uuid.UUID]*uuid.UUID, error) {
	out := make(map[uuid.UUID]*uuid.UUID, len(messages))
	if len(messages) == 0 {
		return out, nil
	}
	for _, m := range messages {
		eff, err := r.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil {
			return nil, err
		}
		if eff != nil && eff.ProjectID != nil {
			out[m.ID] = eff.ProjectID
		}
	}
	return out, nil
}

func (r *Repository) ListUnassigned(ctx context.Context, userID uuid.UUID, filter driven.UnassignedListFilter) ([]driven.UnassignedItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	if status == "" {
		status = "all"
	}

	out := make([]driven.UnassignedItem, 0)

	// Mail candidates
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.account_id, a.label, m.subject, m.from_json, m.conversation_id, m.received_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		ORDER BY m.received_at DESC
		LIMIT 2000
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var idStr, accStr, label, subject, fromJSON, receivedAt string
		var conv sql.NullString
		if err := rows.Scan(&idStr, &accStr, &label, &subject, &fromJSON, &conv, &receivedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		acc, _ := uuid.Parse(accStr)
		rt, err := parseTime(receivedAt)
		if err != nil {
			return nil, err
		}
		eff, err := r.EffectiveAssignment(ctx, userID, id)
		if err != nil || eff == nil {
			continue
		}
		itemStatus := "unassigned"
		if eff.ProjectID != nil && eff.Status == "provisional" {
			itemStatus = "provisional"
		} else if eff.ProjectID != nil && eff.Status == "committed" {
			continue
		} else if eff.ProjectID == nil {
			itemStatus = "unassigned"
		}
		if status == "unassigned" && itemStatus != "unassigned" {
			continue
		}
		if status == "provisional" && itemStatus != "provisional" {
			continue
		}
		msgID, accID := id, acc
		out = append(out, driven.UnassignedItem{
			Kind: "message", MessageID: &msgID, AccountID: &accID, AccountLabel: label,
			Subject: subject, FromJSON: fromJSON, ConversationID: nullStringPtr(conv),
			OccurredAt: rt, Status: itemStatus, Reason: eff.Reason,
			ProjectID: eff.ProjectID, Source: eff.Source,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Manual candidates
	orgID, err := r.GetHomeOrganisationID(ctx, userID)
	if err == nil {
		manuals, err := r.ListUnassignedManualItems(ctx, orgID, 500)
		if err == nil {
			for _, m := range manuals {
				itemStatus := "unassigned"
				if m.AssignmentStatus == "provisional" && m.ProjectID != nil {
					itemStatus = "provisional"
				} else if m.AssignmentStatus == "committed" && m.ProjectID != nil {
					continue
				}
				if status == "unassigned" && itemStatus != "unassigned" {
					continue
				}
				if status == "provisional" && itemStatus != "provisional" {
					continue
				}
				mid := m.ID
				reason := ""
				if m.AssignmentReason != nil {
					reason = *m.AssignmentReason
				}
				src := ""
				if m.AssignmentSource != nil {
					src = *m.AssignmentSource
				}
				out = append(out, driven.UnassignedItem{
					Kind: "manual", ManualItemID: &mid, Subject: m.Title, Channel: m.Channel,
					OccurredAt: m.OccurredAt, Status: itemStatus, Reason: reason,
					ProjectID: m.ProjectID, Source: src,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].OccurredAt.After(out[j].OccurredAt)
	})
	if offset >= len(out) {
		return []driven.UnassignedItem{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *Repository) CountUnassignedSummary(ctx context.Context, userID uuid.UUID) (driven.UnassignedSummary, error) {
	items, err := r.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 2000})
	if err != nil {
		return driven.UnassignedSummary{}, err
	}
	var sum driven.UnassignedSummary
	for _, it := range items {
		if it.Status == "provisional" {
			sum.Provisional++
		} else {
			sum.Unassigned++
		}
	}
	return sum, nil
}

func (r *Repository) ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]driven.MessageRow, error) {
	if limit <= 0 {
		limit = 500
	}
	msgs, err := r.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]driven.MessageRow, 0)
	for _, m := range msgs {
		eff, err := r.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil || eff == nil {
			continue
		}
		if eff.ProjectID != nil {
			continue
		}
		// Only truly unassigned (no override and no thread) for auto-assign candidates.
		if eff.Scope != "none" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Repository) FindCommittedSiblingProject(ctx context.Context, userID, accountID uuid.UUID, conversationID string, excludeMessageID uuid.UUID) (*uuid.UUID, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, nil
	}
	msgs, err := r.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		if m.ID == excludeMessageID {
			continue
		}
		if m.ConversationID == nil || *m.ConversationID != conversationID {
			continue
		}
		eff, err := r.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil || eff == nil {
			continue
		}
		if eff.ProjectID != nil && eff.Status == "committed" {
			return eff.ProjectID, nil
		}
	}
	return nil, nil
}

func scanThreadAssignment(s rowScanner) (*driven.AssignmentRow, error) {
	var idStr, orgStr, accStr, conv, status, reason, source, createdAt, updatedAt string
	var proj, runID, assignedBy sql.NullString
	var conf sql.NullFloat64
	if err := s.Scan(&idStr, &orgStr, &accStr, &conv, &proj, &status, &conf, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return buildAssignmentRow(idStr, orgStr, accStr, conv, nil, proj, status, conf, reason, source, runID, assignedBy, createdAt, updatedAt)
}

func scanOverrideAssignment(s rowScanner) (*driven.AssignmentRow, error) {
	var msgStr, orgStr, accStr, status, reason, source, createdAt, updatedAt string
	var proj, runID, assignedBy sql.NullString
	var conf sql.NullFloat64
	if err := s.Scan(&msgStr, &orgStr, &accStr, &proj, &status, &conf, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	msgID, err := uuid.Parse(msgStr)
	if err != nil {
		return nil, err
	}
	return buildAssignmentRow(uuid.Nil.String(), orgStr, accStr, "", &msgID, proj, status, conf, reason, source, runID, assignedBy, createdAt, updatedAt)
}

func buildAssignmentRow(idStr, orgStr, accStr, conv string, messageID *uuid.UUID, proj sql.NullString, status string, conf sql.NullFloat64, reason, source string, runID, assignedBy sql.NullString, createdAt, updatedAt string) (*driven.AssignmentRow, error) {
	var id uuid.UUID
	var err error
	if idStr != "" && idStr != uuid.Nil.String() {
		id, err = uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
	} else if messageID != nil {
		id = *messageID
	}
	orgID, err := uuid.Parse(orgStr)
	if err != nil {
		return nil, err
	}
	accID, err := uuid.Parse(accStr)
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
	row := &driven.AssignmentRow{
		ID: id, OrganisationID: orgID, AccountID: accID, ConversationID: conv,
		MessageID: messageID, Status: status, Reason: reason, Source: source,
		CreatedAt: cat, UpdatedAt: uat,
	}
	if proj.Valid && proj.String != "" {
		pid, err := uuid.Parse(proj.String)
		if err != nil {
			return nil, err
		}
		row.ProjectID = &pid
	}
	if conf.Valid {
		v := conf.Float64
		row.Confidence = &v
	}
	if runID.Valid && runID.String != "" {
		rid, err := uuid.Parse(runID.String)
		if err != nil {
			return nil, err
		}
		row.RunID = &rid
	}
	if assignedBy.Valid && assignedBy.String != "" {
		aid, err := uuid.Parse(assignedBy.String)
		if err != nil {
			return nil, err
		}
		row.AssignedByUserID = &aid
	}
	return row, nil
}
