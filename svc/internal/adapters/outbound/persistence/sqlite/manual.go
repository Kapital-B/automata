package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) ListContactIDsForManualItem(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT contact_id FROM correspondence_participants
		WHERE organisation_id = ? AND manual_item_id = ?
	`, organisationID.String(), manualItemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func (r *Repository) CreateManualItem(ctx context.Context, row driven.ManualItemRow) error {
	var proj, reason, source any
	if row.ProjectID != nil {
		proj = row.ProjectID.String()
	}
	if row.AssignmentReason != nil {
		reason = *row.AssignmentReason
	}
	if row.AssignmentSource != nil {
		source = *row.AssignmentSource
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO manual_items (
			id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.Channel,
		formatRFC3339(row.OccurredAt.UTC()), row.Title, row.BodyText, proj,
		row.AssignmentStatus, reason, source, row.CreatedByUserID.String(),
		formatRFC3339(row.CreatedAt.UTC()))
	return err
}

func (r *Repository) GetManualItem(ctx context.Context, organisationID, id uuid.UUID) (*driven.ManualItemRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	m, err := scanManualItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *Repository) UpdateManualItemAssignment(ctx context.Context, organisationID, id uuid.UUID, projectID *uuid.UUID, status, reason, source string) error {
	var proj any
	if projectID != nil {
		proj = projectID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE manual_items
		SET project_id = ?, assignment_status = ?, assignment_reason = ?, assignment_source = ?
		WHERE id = ? AND organisation_id = ?
	`, proj, status, reason, source, id.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListManualItemsForProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.ManualItemRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items
		WHERE organisation_id = ? AND project_id = ?
		ORDER BY occurred_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManualItemRows(rows)
}

func (r *Repository) ListUnassignedManualItems(ctx context.Context, organisationID uuid.UUID, limit int) ([]driven.ManualItemRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items
		WHERE organisation_id = ?
		  AND (
			assignment_status = 'unassigned'
			OR project_id IS NULL
			OR assignment_status = 'provisional'
		  )
		ORDER BY occurred_at DESC
		LIMIT ?
	`, organisationID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManualItemRows(rows)
}

func scanManualItemRows(rows *sql.Rows) ([]driven.ManualItemRow, error) {
	out := make([]driven.ManualItemRow, 0)
	for rows.Next() {
		m, err := scanManualItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func scanManualItem(s rowScanner) (*driven.ManualItemRow, error) {
	var idStr, orgStr, channel, occurredAt, title, body, status, createdBy, createdAt string
	var proj, reason, source sql.NullString
	if err := s.Scan(&idStr, &orgStr, &channel, &occurredAt, &title, &body, &proj,
		&status, &reason, &source, &createdBy, &createdAt); err != nil {
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
	uid, err := uuid.Parse(createdBy)
	if err != nil {
		return nil, err
	}
	ot, err := parseTime(occurredAt)
	if err != nil {
		return nil, err
	}
	ct, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	m := &driven.ManualItemRow{
		ID: id, OrganisationID: orgID, Channel: channel, OccurredAt: ot,
		Title: title, BodyText: body, AssignmentStatus: status,
		CreatedByUserID: uid, CreatedAt: ct,
	}
	if proj.Valid && proj.String != "" {
		pid, err := uuid.Parse(proj.String)
		if err != nil {
			return nil, err
		}
		m.ProjectID = &pid
	}
	if reason.Valid {
		m.AssignmentReason = &reason.String
	}
	if source.Valid {
		m.AssignmentSource = &source.String
	}
	return m, nil
}

func (r *Repository) ListProjectTimeline(ctx context.Context, userID, organisationID, projectID uuid.UUID, filter driven.TimelineFilter) ([]driven.TimelineItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	source := strings.TrimSpace(strings.ToLower(filter.Source))
	if source == "" {
		source = "all"
	}

	out := make([]driven.TimelineItem, 0)

	if source == "all" || source == "mail" {
		mailItems, err := r.listMailTimelineItems(ctx, userID, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		out = append(out, mailItems...)
	}
	if source == "all" || source == "manual" {
		manuals, err := r.ListManualItemsForProject(ctx, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		for _, m := range manuals {
			item := driven.TimelineItem{
				Source: "manual", OccurredAt: m.OccurredAt, Title: m.Title,
				Snippet: snippetText(m.BodyText, 160), Channel: m.Channel, BodyText: m.BodyText,
			}
			mid := m.ID
			item.ManualItemID = &mid
			contacts, _ := r.timelineContactsForManual(ctx, organisationID, m.ID)
			item.Contacts = contacts
			if issueID, err := r.FindIssueIDByManualItem(ctx, m.ID); err == nil {
				item.IssueID = issueID
			}
			out = append(out, item)
		}
	}
	if source == "all" || source == "slack" {
		messages, err := r.ListConnectorMessagesForProject(ctx, userID, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		accountLabels := map[uuid.UUID]string{}
		for _, message := range messages {
			label, ok := accountLabels[message.ConnectorAccountID]
			if !ok {
				account, err := r.GetConnectorAccount(ctx, userID, message.ConnectorAccountID)
				if err != nil {
					return nil, err
				}
				if account != nil {
					label = account.Label
				}
				accountLabels[message.ConnectorAccountID] = label
			}
			messageID := message.ID
			accountID := message.ConnectorAccountID
			out = append(out, driven.TimelineItem{
				Source: "slack", OccurredAt: message.OccurredAt, Title: message.Title,
				Snippet: snippetText(message.BodyText, 160), BodyText: message.BodyText,
				AccountLabel: label, ConnectorMessageID: &messageID,
				ConnectorAccountID: &accountID, Channel: message.ExternalChannelID,
			})
		}
	}

	if filter.UnassignedToIssue {
		filtered := make([]driven.TimelineItem, 0, len(out))
		for _, it := range out {
			if it.IssueID == nil {
				filtered = append(filtered, it)
			}
		}
		out = filtered
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].OccurredAt.After(out[j].OccurredAt)
	})
	if offset >= len(out) {
		return []driven.TimelineItem{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *Repository) listMailTimelineItems(ctx context.Context, userID, organisationID, projectID uuid.UUID) ([]driven.TimelineItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.account_id, a.label, m.subject, m.body_text, m.received_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_assignment_overrides o ON o.message_id = m.id
		LEFT JOIN thread_assignments t
			ON t.account_id = m.account_id
			AND m.conversation_id IS NOT NULL
			AND m.conversation_id != ''
			AND t.conversation_id = m.conversation_id
		WHERE
			CASE
				WHEN o.message_id IS NOT NULL THEN o.project_id = ?
				ELSE t.project_id = ?
			END
		ORDER BY m.received_at DESC
		LIMIT 500
	`, userID.String(), projectID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]driven.TimelineItem, 0)
	for rows.Next() {
		var idStr, accStr, label, subject string
		var body sql.NullString
		var receivedAt string
		if err := rows.Scan(&idStr, &accStr, &label, &subject, &body, &receivedAt); err != nil {
			return nil, err
		}
		msgID, _ := uuid.Parse(idStr)
		accID, _ := uuid.Parse(accStr)
		rt, err := parseTime(receivedAt)
		if err != nil {
			return nil, err
		}
		bodyText := ""
		if body.Valid {
			bodyText = body.String
		}
		item := driven.TimelineItem{
			Source: "mail", OccurredAt: rt, Title: subject, Snippet: snippetText(bodyText, 160),
			AccountLabel: label,
		}
		item.AccountID = &accID
		item.MessageID = &msgID
		contacts, _ := r.timelineContactsForMessage(ctx, organisationID, msgID)
		item.Contacts = contacts
		if issueID, err := r.FindIssueIDByMessage(ctx, msgID); err == nil {
			item.IssueID = issueID
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) timelineContactsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]driven.TimelineContact, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.display_name, cp.role
		FROM correspondence_participants cp
		INNER JOIN contacts c ON c.id = cp.contact_id
		WHERE cp.organisation_id = ? AND cp.message_id = ?
		ORDER BY cp.role ASC
	`, organisationID.String(), messageID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTimelineContacts(rows)
}

func (r *Repository) timelineContactsForManual(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]driven.TimelineContact, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.display_name, cp.role
		FROM correspondence_participants cp
		INNER JOIN contacts c ON c.id = cp.contact_id
		WHERE cp.organisation_id = ? AND cp.manual_item_id = ?
		ORDER BY cp.role ASC
	`, organisationID.String(), manualItemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTimelineContacts(rows)
}

func scanTimelineContacts(rows *sql.Rows) ([]driven.TimelineContact, error) {
	out := make([]driven.TimelineContact, 0)
	for rows.Next() {
		var idStr, name, role string
		if err := rows.Scan(&idStr, &name, &role); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.TimelineContact{ID: id, DisplayName: name, Role: role})
	}
	return out, rows.Err()
}

func snippetText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
