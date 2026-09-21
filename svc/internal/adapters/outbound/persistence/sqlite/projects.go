package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at, last_extracted_at
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
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at, last_extracted_at
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
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at, last_extracted_at
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
	var desc, client, archived, lastExtracted sql.NullString
	if err := s.Scan(&idStr, &orgStr, &name, &code, &desc, &client, &kwJSON, &archived, &createdAt, &updatedAt, &lastExtracted); err != nil {
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
	if lastExtracted.Valid && lastExtracted.String != "" {
		t, err := parseTime(lastExtracted.String)
		if err != nil {
			return nil, err
		}
		p.LastExtractedAt = &t
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

// assignmentUpsertChunk bounds how many assignment rows go into one
// transaction, matching the postgres adapter.
const assignmentUpsertChunk = 100

const upsertThreadAssignmentSQL = `
		INSERT INTO thread_assignments (
			id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at, not_relevant_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, conversation_id) DO UPDATE SET
			not_relevant_at = excluded.not_relevant_at,
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at`

func assignmentNullables(row driven.AssignmentRow) (proj, runID, assignedBy, conf any) {
	if row.ProjectID != nil {
		proj = row.ProjectID.String()
	}
	if row.RunID != nil {
		runID = row.RunID.String()
	}
	if row.AssignedByUserID != nil {
		assignedBy = row.AssignedByUserID.String()
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	return proj, runID, assignedBy, conf
}

func threadAssignmentArgs(row driven.AssignmentRow) []any {
	proj, runID, assignedBy, conf := assignmentNullables(row)
	return []any{row.ID.String(), row.OrganisationID.String(), row.AccountID.String(), row.ConversationID,
		proj, row.Status, conf, row.Reason, row.Source, runID, assignedBy,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()),
		nullTimeStr(row.NotRelevantAt)}
}

func (r *Repository) UpsertThreadAssignment(ctx context.Context, row driven.AssignmentRow) error {
	_, err := r.db.ExecContext(ctx, upsertThreadAssignmentSQL, threadAssignmentArgs(row)...)
	return err
}

func (r *Repository) UpsertThreadAssignments(ctx context.Context, rows []driven.AssignmentRow) error {
	return r.chunkedAssignmentTx(ctx, rows, upsertThreadAssignmentSQL, threadAssignmentArgs)
}

// chunkedAssignmentTx applies the statement to every row, committing one
// transaction per chunk.
func (r *Repository) chunkedAssignmentTx(ctx context.Context, rows []driven.AssignmentRow, stmt string, args func(driven.AssignmentRow) []any) error {
	for start := 0; start < len(rows); start += assignmentUpsertChunk {
		end := start + assignmentUpsertChunk
		if end > len(rows) {
			end = len(rows)
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, row := range rows[start:end] {
			if _, err := tx.ExecContext(ctx, stmt, args(row)...); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return nil
}

func (r *Repository) GetThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) (*driven.AssignmentRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at, not_relevant_at
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

const upsertMessageOverrideSQL = `
		INSERT INTO message_assignment_overrides (
			message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at, not_relevant_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET
			not_relevant_at = excluded.not_relevant_at,
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at`

func messageOverrideArgs(row driven.AssignmentRow) []any {
	proj, runID, assignedBy, conf := assignmentNullables(row)
	return []any{row.MessageID.String(), row.OrganisationID.String(), row.AccountID.String(),
		proj, row.Status, conf, row.Reason, row.Source, runID, assignedBy,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()),
		nullTimeStr(row.NotRelevantAt)}
}

func (r *Repository) UpsertMessageOverride(ctx context.Context, row driven.AssignmentRow) error {
	if row.MessageID == nil {
		return fmt.Errorf("message override requires message_id")
	}
	_, err := r.db.ExecContext(ctx, upsertMessageOverrideSQL, messageOverrideArgs(row)...)
	return err
}

func (r *Repository) UpsertMessageOverrides(ctx context.Context, rows []driven.AssignmentRow) error {
	for _, row := range rows {
		if row.MessageID == nil {
			return fmt.Errorf("message override requires message_id")
		}
	}
	return r.chunkedAssignmentTx(ctx, rows, upsertMessageOverrideSQL, messageOverrideArgs)
}

func (r *Repository) GetMessageOverride(ctx context.Context, messageID uuid.UUID) (*driven.AssignmentRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at, not_relevant_at
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
		out.NotRelevantAt = ov.NotRelevantAt
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
			out.NotRelevantAt = th.NotRelevantAt
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

// unassignedEffCTE mirrors the postgres adapter: one statement resolves the
// Wave 1 §7 effective assignment for every mail message on the caller's
// accounts. An override row wins outright (including when it clears the
// project), otherwise the thread row applies.
const unassignedEffCTE = `
	WITH eff AS (
		SELECT
			m.id AS message_id,
			m.account_id,
			a.label AS account_label,
			m.subject,
			m.from_json,
			m.conversation_id,
			m.received_at,
			CASE WHEN o.message_id IS NOT NULL THEN o.project_id ELSE t.project_id END AS project_id,
			CASE WHEN o.message_id IS NOT NULL THEN o.status     ELSE t.status     END AS raw_status,
			CASE WHEN o.message_id IS NOT NULL THEN o.reason     ELSE t.reason     END AS reason,
			CASE WHEN o.message_id IS NOT NULL THEN o.source     ELSE t.source     END AS source,
			CASE WHEN o.message_id IS NOT NULL THEN o.confidence ELSE t.confidence END AS confidence,
			CASE WHEN o.message_id IS NOT NULL THEN o.not_relevant_at ELSE t.not_relevant_at END AS not_relevant_at,
			CASE
				WHEN o.message_id IS NOT NULL THEN NULL
				WHEN m.conversation_id IS NULL OR m.conversation_id = '' THEN NULL
				ELSE m.conversation_id
			END AS thread_key
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_assignment_overrides o ON o.message_id = m.id
		LEFT JOIN thread_assignments t
			ON t.account_id = m.account_id
			AND t.conversation_id = m.conversation_id
			AND m.conversation_id IS NOT NULL
			AND m.conversation_id <> ''
	),
	queued AS (
		SELECT
			message_id, account_id, account_label, subject, from_json, conversation_id,
			received_at, project_id, not_relevant_at,
			COALESCE(reason, '') AS reason,
			COALESCE(source, '') AS source,
			confidence,
			CASE WHEN project_id IS NULL THEN 'unassigned' ELSE raw_status END AS status,
			COALESCE(thread_key, message_id) AS group_key
		FROM eff
		WHERE (project_id IS NULL OR raw_status = 'provisional')
	),
	grouped AS (
		SELECT
			queued.*,
			ROW_NUMBER() OVER (PARTITION BY group_key ORDER BY received_at DESC, message_id DESC) AS rn,
			COUNT(*) OVER (PARTITION BY group_key) AS thread_count
		FROM queued
	),
	manual AS (
		SELECT
			mi.id AS manual_item_id,
			mi.title,
			mi.channel,
			mi.occurred_at,
			mi.project_id,
			mi.not_relevant_at,
			CASE WHEN mi.project_id IS NULL THEN 'unassigned' ELSE mi.assignment_status END AS status,
			COALESCE(mi.assignment_reason, '') AS reason,
			COALESCE(mi.assignment_source, '') AS source
		FROM manual_items mi
		WHERE mi.organisation_id = ?
		  AND (mi.project_id IS NULL OR mi.assignment_status = 'provisional')
	)`

// notRelevantStatus lists dismissed correspondence instead of the queue.
const notRelevantStatus = "not_relevant"

func normalizeUnassignedStatus(status string) string {
	s := strings.TrimSpace(strings.ToLower(status))
	if s == "" {
		return "all"
	}
	return s
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
	status := normalizeUnassignedStatus(filter.Status)

	orgID, err := r.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return nil, err
	}

	args := []any{userID.String(), orgID.String()}
	mailWhere := "rn = 1"
	manualWhere := "1 = 1"
	// not_relevant partitions the rows: the queue statuses all exclude
	// dismissed items, including "all", which means all of the queue.
	if status == notRelevantStatus {
		mailWhere += " AND not_relevant_at IS NOT NULL"
		manualWhere += " AND not_relevant_at IS NOT NULL"
	} else {
		mailWhere += " AND not_relevant_at IS NULL"
		manualWhere += " AND not_relevant_at IS NULL"
		if status != "all" {
			mailWhere += " AND status = ?"
			manualWhere += " AND status = ?"
		}
	}

	q := unassignedEffCTE + `
	SELECT 'message' AS kind, message_id AS id, account_id,
		account_label, subject, from_json, '' AS channel, conversation_id,
		received_at AS occurred_at, project_id, status, reason, source,
		confidence, thread_count, not_relevant_at
	FROM grouped
	WHERE ` + mailWhere + `
	UNION ALL
	SELECT 'manual', manual_item_id, NULL, '', title, '', channel, NULL,
		occurred_at, project_id, status, reason, source,
		NULL, 1, not_relevant_at
	FROM manual
	WHERE ` + manualWhere + `
	ORDER BY occurred_at DESC
	LIMIT ? OFFSET ?`

	if status != "all" && status != notRelevantStatus {
		args = append(args, status, status)
	}
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUnassignedRows(rows)
}

func scanUnassignedRows(rows *sql.Rows) ([]driven.UnassignedItem, error) {
	out := make([]driven.UnassignedItem, 0)
	for rows.Next() {
		var kind, idStr, accountLabel, subject, fromJSON, channel, status, reason, source string
		var accountID, conversationID, projectID sql.NullString
		var confidence sql.NullFloat64
		var threadCount int64
		var occurredAt string
		var notRelevantAt sql.NullString
		if err := rows.Scan(&kind, &idStr, &accountID, &accountLabel, &subject, &fromJSON,
			&channel, &conversationID, &occurredAt, &projectID, &status, &reason, &source,
			&confidence, &threadCount, &notRelevantAt); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		at, err := parseTime(occurredAt)
		if err != nil {
			return nil, err
		}
		if threadCount < 1 {
			threadCount = 1
		}
		item := driven.UnassignedItem{
			Kind:        kind,
			Subject:     subject,
			Channel:     channel,
			OccurredAt:  at.UTC(),
			Status:      status,
			Reason:      reason,
			Source:      source,
			ThreadCount: int(threadCount),
		}
		if notRelevantAt.Valid && notRelevantAt.String != "" {
			at, err := parseTime(notRelevantAt.String)
			if err != nil {
				return nil, err
			}
			item.NotRelevantAt = &at
		}
		if kind == "manual" {
			mid := id
			item.ManualItemID = &mid
		} else {
			mid := id
			item.MessageID = &mid
			item.AccountLabel = accountLabel
			item.FromJSON = fromJSON
			item.ConversationID = nullStringPtr(conversationID)
			if accountID.Valid {
				acc, err := uuid.Parse(accountID.String)
				if err != nil {
					return nil, err
				}
				item.AccountID = &acc
			}
		}
		if projectID.Valid {
			pid, err := uuid.Parse(projectID.String)
			if err != nil {
				return nil, err
			}
			item.ProjectID = &pid
		}
		if confidence.Valid {
			c := confidence.Float64
			item.Confidence = &c
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) CountUnassignedSummary(ctx context.Context, userID uuid.UUID) (driven.UnassignedSummary, error) {
	var sum driven.UnassignedSummary
	orgID, err := r.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return sum, err
	}
	q := unassignedEffCTE + `
	SELECT status, not_relevant_at IS NOT NULL AS dismissed, COUNT(*) FROM (
		SELECT status, not_relevant_at FROM grouped WHERE rn = 1
		UNION ALL
		SELECT status, not_relevant_at FROM manual
	) counted
	GROUP BY status, not_relevant_at IS NOT NULL`
	rows, err := r.db.QueryContext(ctx, q, userID.String(), orgID.String())
	if err != nil {
		return sum, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var dismissed bool
		var n int
		if err := rows.Scan(&status, &dismissed, &n); err != nil {
			return driven.UnassignedSummary{}, err
		}
		// Dismissed items are reported separately and never counted toward the
		// badge, whatever status their row carries.
		if dismissed {
			sum.NotRelevant += n
			continue
		}
		if status == "provisional" {
			sum.Provisional += n
		} else {
			sum.Unassigned += n
		}
	}
	return sum, rows.Err()
}

// ListMessagesNeedingAssign returns messages on the account with no override row
// and no thread row, i.e. Wave 1 §9's "effective assignment is Unassigned".
func (r *Repository) ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]driven.MessageRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.account_id, m.provider_message_id, m.conversation_id, m.received_at, m.subject, m.from_json,
			m.to_json, m.cc_json, m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at, m.summary_seen_at, m.forward_seen_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_categories mc ON mc.message_id = m.id AND mc.source = 'llm'
		LEFT JOIN category_definitions cd ON cd.id = mc.category_id
		LEFT JOIN message_assignment_overrides o ON o.message_id = m.id
		LEFT JOIN thread_assignments t
			ON t.account_id = m.account_id
			AND t.conversation_id = m.conversation_id
			AND m.conversation_id IS NOT NULL
			AND m.conversation_id <> ''
		WHERE m.account_id = ?
		  AND o.message_id IS NULL
		  AND t.id IS NULL
		ORDER BY m.received_at DESC
		LIMIT ?
	`, userID.String(), accountID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRows(rows)
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
	var notRelevantAt sql.NullString
	if err := s.Scan(&idStr, &orgStr, &accStr, &conv, &proj, &status, &conf, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt, &notRelevantAt); err != nil {
		return nil, err
	}
	return buildAssignmentRow(idStr, orgStr, accStr, conv, nil, proj, status, conf, reason, source, runID, assignedBy, createdAt, updatedAt, notRelevantAt)
}

func scanOverrideAssignment(s rowScanner) (*driven.AssignmentRow, error) {
	var msgStr, orgStr, accStr, status, reason, source, createdAt, updatedAt string
	var proj, runID, assignedBy sql.NullString
	var conf sql.NullFloat64
	var notRelevantAt sql.NullString
	if err := s.Scan(&msgStr, &orgStr, &accStr, &proj, &status, &conf, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt, &notRelevantAt); err != nil {
		return nil, err
	}
	msgID, err := uuid.Parse(msgStr)
	if err != nil {
		return nil, err
	}
	return buildAssignmentRow(uuid.Nil.String(), orgStr, accStr, "", &msgID, proj, status, conf, reason, source, runID, assignedBy, createdAt, updatedAt, notRelevantAt)
}

func buildAssignmentRow(idStr, orgStr, accStr, conv string, messageID *uuid.UUID, proj sql.NullString, status string, conf sql.NullFloat64, reason, source string, runID, assignedBy sql.NullString, createdAt, updatedAt string, notRelevantAt sql.NullString) (*driven.AssignmentRow, error) {
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
	if notRelevantAt.Valid && notRelevantAt.String != "" {
		at, err := parseTime(notRelevantAt.String)
		if err != nil {
			return nil, err
		}
		row.NotRelevantAt = &at
	}
	return row, nil
}

func (r *Repository) ListProjectParticipants(ctx context.Context, organisationID uuid.UUID) ([]driven.ProjectParticipantRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pp.project_id, pp.contact_id
		FROM project_participants pp
		INNER JOIN projects p ON p.id = pp.project_id
		WHERE p.organisation_id = ? AND p.archived_at IS NULL
	`, organisationID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ProjectParticipantRow, 0)
	for rows.Next() {
		var projectStr, contactStr string
		if err := rows.Scan(&projectStr, &contactStr); err != nil {
			return nil, err
		}
		projectID, err := uuid.Parse(projectStr)
		if err != nil {
			return nil, err
		}
		contactID, err := uuid.Parse(contactStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.ProjectParticipantRow{ProjectID: projectID, ContactID: contactID})
	}
	return out, rows.Err()
}

func (r *Repository) ListCommittedAssignmentSignals(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]driven.AssignmentSignalRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(o.project_id, t.project_id) AS project_id, m.from_json
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_assignment_overrides o ON o.message_id = m.id
		LEFT JOIN thread_assignments t
			ON t.account_id = m.account_id
			AND t.conversation_id = m.conversation_id
			AND m.conversation_id IS NOT NULL
			AND m.conversation_id <> ''
		WHERE m.account_id = ?
		  AND CASE WHEN o.message_id IS NOT NULL THEN o.status ELSE t.status END = 'committed'
		  AND COALESCE(o.project_id, t.project_id) IS NOT NULL
		ORDER BY m.received_at DESC
		LIMIT ?
	`, userID.String(), accountID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.AssignmentSignalRow, 0)
	for rows.Next() {
		var projectStr, fromJSON string
		if err := rows.Scan(&projectStr, &fromJSON); err != nil {
			return nil, err
		}
		projectID, err := uuid.Parse(projectStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.AssignmentSignalRow{ProjectID: projectID, FromJSON: fromJSON})
	}
	return out, rows.Err()
}
