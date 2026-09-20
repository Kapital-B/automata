package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
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
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at, not_relevant_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.Channel,
		formatRFC3339(row.OccurredAt.UTC()), row.Title, row.BodyText, proj,
		row.AssignmentStatus, reason, source, row.CreatedByUserID.String(),
		formatRFC3339(row.CreatedAt.UTC()), nullTimeStr(row.NotRelevantAt))
	return err
}

func (r *Repository) GetManualItem(ctx context.Context, organisationID, id uuid.UUID) (*driven.ManualItemRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at, not_relevant_at
		FROM manual_items WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	m, err := scanManualItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *Repository) UpdateManualItemAssignment(ctx context.Context, organisationID, id uuid.UUID, projectID *uuid.UUID, status, reason, source string, notRelevantAt *time.Time) error {
	var proj any
	if projectID != nil {
		proj = projectID.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE manual_items
		SET project_id = ?, assignment_status = ?, assignment_reason = ?, assignment_source = ?, not_relevant_at = ?
		WHERE id = ? AND organisation_id = ?
	`, proj, status, reason, source, nullTimeStr(notRelevantAt), id.String(), organisationID.String())
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
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at, not_relevant_at
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
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at, not_relevant_at
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
	var proj, reason, source, notRelevantAt sql.NullString
	if err := s.Scan(&idStr, &orgStr, &channel, &occurredAt, &title, &body, &proj,
		&status, &reason, &source, &createdBy, &createdAt, &notRelevantAt); err != nil {
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
	if notRelevantAt.Valid && notRelevantAt.String != "" {
		at, err := parseTime(notRelevantAt.String)
		if err != nil {
			return nil, err
		}
		m.NotRelevantAt = &at
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
		mailItems, err := r.listMailTimelineItems(ctx, userID, projectID)
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

	// Contacts and issue links are attached in bulk once every source has
	// contributed. Resolving them per row is what made this endpoint both slow
	// and deadlock-prone.
	if err := r.hydrateTimelineItems(ctx, organisationID, out); err != nil {
		return nil, err
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

func (r *Repository) listMailTimelineItems(ctx context.Context, userID, projectID uuid.UUID) ([]driven.TimelineItem, error) {
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
		out = append(out, item)
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

// timelineHydrateChunk bounds how many ids go into one IN list.
const timelineHydrateChunk = 200

// hydrateTimelineItems attaches contacts and issue links to an already-built
// timeline.
//
// This must run after every cursor that produced these items has been closed.
// Issuing a query while a result set is still streaming holds two connections
// at once; SQLite here runs a single-connection pool (factory.go), so doing it
// inline is a self-deadlock. Batching also turns 2 queries per row into a
// handful in total. Mirrors the postgres adapter.
func (r *Repository) hydrateTimelineItems(ctx context.Context, organisationID uuid.UUID, items []driven.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	messageIDs := make([]uuid.UUID, 0, len(items))
	manualIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if it.MessageID != nil {
			messageIDs = append(messageIDs, *it.MessageID)
		}
		if it.ManualItemID != nil {
			manualIDs = append(manualIDs, *it.ManualItemID)
		}
	}

	msgContacts, err := r.timelineContactsFor(ctx, organisationID, "message_id", messageIDs)
	if err != nil {
		return err
	}
	manualContacts, err := r.timelineContactsFor(ctx, organisationID, "manual_item_id", manualIDs)
	if err != nil {
		return err
	}
	msgIssues, err := r.issueIDsFor(ctx, "message_id", messageIDs)
	if err != nil {
		return err
	}
	manualIssues, err := r.issueIDsFor(ctx, "manual_item_id", manualIDs)
	if err != nil {
		return err
	}

	for i := range items {
		if id := items[i].MessageID; id != nil {
			items[i].Contacts = msgContacts[*id]
			if issueID, ok := msgIssues[*id]; ok {
				issue := issueID
				items[i].IssueID = &issue
			}
		}
		if id := items[i].ManualItemID; id != nil {
			items[i].Contacts = manualContacts[*id]
			if issueID, ok := manualIssues[*id]; ok {
				issue := issueID
				items[i].IssueID = &issue
			}
		}
	}
	return nil
}

// timelineContactsFor loads participants for many messages or manual items.
// column is a trusted internal literal, never caller input.
func (r *Repository) timelineContactsFor(ctx context.Context, organisationID uuid.UUID, column string, ids []uuid.UUID) (map[uuid.UUID][]driven.TimelineContact, error) {
	out := map[uuid.UUID][]driven.TimelineContact{}
	for _, chunk := range sqlkit.ChunkUUIDs(sqlkit.DedupeUUIDs(ids), timelineHydrateChunk) {
		args := make([]any, 0, len(chunk)+1)
		args = append(args, organisationID.String())
		for _, id := range chunk {
			args = append(args, id.String())
		}
		rows, err := r.db.QueryContext(ctx, `
			SELECT cp.`+column+`, c.id, c.display_name, cp.role
			FROM correspondence_participants cp
			INNER JOIN contacts c ON c.id = cp.contact_id
			WHERE cp.organisation_id = ? AND cp.`+column+` IN (`+sqlkit.PlaceholderList(len(chunk))+`)
			ORDER BY cp.role ASC
		`, args...)
		if err != nil {
			return nil, err
		}
		if err := scanTimelineContactsByOwner(rows, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanTimelineContactsByOwner(rows *sql.Rows, out map[uuid.UUID][]driven.TimelineContact) error {
	defer rows.Close()
	for rows.Next() {
		var ownerStr, contactStr, displayName, role string
		if err := rows.Scan(&ownerStr, &contactStr, &displayName, &role); err != nil {
			return err
		}
		ownerID, err := uuid.Parse(ownerStr)
		if err != nil {
			return err
		}
		contactID, err := uuid.Parse(contactStr)
		if err != nil {
			return err
		}
		out[ownerID] = append(out[ownerID], driven.TimelineContact{
			ID: contactID, DisplayName: displayName, Role: role,
		})
	}
	return rows.Err()
}

// issueIDsFor maps messages or manual items to the issue they belong to.
func (r *Repository) issueIDsFor(ctx context.Context, column string, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := map[uuid.UUID]uuid.UUID{}
	for _, chunk := range sqlkit.ChunkUUIDs(sqlkit.DedupeUUIDs(ids), timelineHydrateChunk) {
		args := make([]any, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id.String())
		}
		rows, err := r.db.QueryContext(ctx, `
			SELECT `+column+`, issue_id FROM issue_items
			WHERE `+column+` IN (`+sqlkit.PlaceholderList(len(chunk))+`)
		`, args...)
		if err != nil {
			return nil, err
		}
		if err := scanIssueIDsByOwner(rows, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanIssueIDsByOwner(rows *sql.Rows, out map[uuid.UUID]uuid.UUID) error {
	defer rows.Close()
	for rows.Next() {
		var ownerStr, issueStr string
		if err := rows.Scan(&ownerStr, &issueStr); err != nil {
			return err
		}
		ownerID, err := uuid.Parse(ownerStr)
		if err != nil {
			return err
		}
		issueID, err := uuid.Parse(issueStr)
		if err != nil {
			return err
		}
		out[ownerID] = issueID
	}
	return rows.Err()
}
