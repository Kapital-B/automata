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
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// Repository implements account, message, and oauth state persistence.
type Repository struct {
	db            *sql.DB
	OAuthStateTTL time.Duration
}

func NewRepository(db *sql.DB, oauthStateTTL time.Duration) *Repository {
	if oauthStateTTL <= 0 {
		oauthStateTTL = 15 * time.Minute
	}
	return &Repository{db: db, OAuthStateTTL: oauthStateTTL}
}

func formatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// --- accounts.AccountRepository ---

func (r *Repository) InsertAccount(ctx context.Context, a driven.AccountRow, tokenCiphertext []byte) error {
	now := formatRFC3339(time.Now().UTC())
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO accounts (id, user_id, label, provider, ms_account_kind, graph_tenant_id, primary_email, msal_home_account_id,
			connection_status, last_error, token_ciphertext, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID.String(), a.UserID.String(), a.Label, a.Provider, string(a.MsAccountKind), nullStr(a.GraphTenantID), a.PrimaryEmail,
		nullStr(a.MsalHomeAccountID), a.ConnectionStatus, nullStr(a.LastError), tokenCiphertext, now, now,
	)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO account_sync_state (account_id, delta_link, last_synced_at, cursor_json) VALUES (?, NULL, NULL, NULL)`,
		a.ID.String(),
	)
	return err
}

func (r *Repository) UpdateAccountTokens(ctx context.Context, userID uuid.UUID, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error {
	now := formatRFC3339(time.Now().UTC())
	_, err := r.db.ExecContext(ctx, `
		UPDATE accounts SET token_ciphertext = ?, primary_email = ?, graph_tenant_id = ?, msal_home_account_id = ?,
			connection_status = ?, last_error = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		tokenCiphertext, primaryEmail, nullStr(graphTenantID), nullStr(msalHome), status, nullStr(lastErr), now, id.String(), userID.String(),
	)
	return err
}

func (r *Repository) GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.AccountRow, []byte, error) {
	var a driven.AccountRow
	var idStr string
	var kindStr string
	var graphT, msalH, lastE sql.NullString
	var tok []byte
	var createdAt, updatedAt string
	var lastSync sql.NullString

	var userStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT a.user_id, a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.token_ciphertext, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		WHERE a.id = ? AND a.user_id = ?`, id.String(), userID.String(),
	).Scan(
		&userStr, &idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
		&a.ConnectionStatus, &lastE, &tok, &createdAt, &updatedAt, &lastSync,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	uidUser, err := uuid.Parse(userStr)
	if err != nil {
		return nil, nil, err
	}
	a.UserID = uidUser
	uid, err := uuid.Parse(idStr)
	if err != nil {
		return nil, nil, err
	}
	a.ID = uid
	a.MsAccountKind = accounts.MsAccountKind(kindStr)
	a.GraphTenantID = nullStringPtr(graphT)
	a.MsalHomeAccountID = nullStringPtr(msalH)
	a.LastError = nullStringPtr(lastE)
	var err2 error
	a.CreatedAt, err2 = parseTime(createdAt)
	if err2 != nil {
		return nil, nil, err2
	}
	a.UpdatedAt, err2 = parseTime(updatedAt)
	if err2 != nil {
		return nil, nil, err2
	}
	if lastSync.Valid {
		t, e := parseTime(lastSync.String)
		if e != nil {
			return nil, nil, e
		}
		a.LastSyncedAt = &t
	}
	var tokenBytes []byte
	if len(tok) > 0 {
		tokenBytes = append([]byte(nil), tok...)
	}
	return &a, tokenBytes, nil
}

func (r *Repository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]driven.AccountRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.user_id, a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		WHERE a.user_id = ?
		ORDER BY a.created_at ASC`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []driven.AccountRow
	for rows.Next() {
		var a driven.AccountRow
		var idStr, kindStr string
		var graphT, msalH, lastE sql.NullString
		var createdAt, updatedAt string
		var lastSync sql.NullString
		var userStr string
		if err := rows.Scan(
			&userStr, &idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
			&a.ConnectionStatus, &lastE, &createdAt, &updatedAt, &lastSync,
		); err != nil {
			return nil, err
		}
		uidUser, err := uuid.Parse(userStr)
		if err != nil {
			return nil, err
		}
		a.UserID = uidUser
		uid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		a.ID = uid
		a.MsAccountKind = accounts.MsAccountKind(kindStr)
		a.GraphTenantID = nullStringPtr(graphT)
		a.MsalHomeAccountID = nullStringPtr(msalH)
		a.LastError = nullStringPtr(lastE)
		var e error
		a.CreatedAt, e = parseTime(createdAt)
		if e != nil {
			return nil, e
		}
		a.UpdatedAt, e = parseTime(updatedAt)
		if e != nil {
			return nil, e
		}
		if lastSync.Valid {
			t, e2 := parseTime(lastSync.String)
			if e2 != nil {
				return nil, e2
			}
			a.LastSyncedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE account_id IN (SELECT id FROM accounts WHERE id = ? AND user_id = ?)`, id.String(), userID.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_sync_state WHERE account_id IN (SELECT id FROM accounts WHERE id = ? AND user_id = ?)`, id.String(), userID.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id = ? AND user_id = ?`, id.String(), userID.String()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) GetSyncDeltaLink(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*string, error) {
	var delta sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT s.delta_link
		FROM account_sync_state s
		INNER JOIN accounts a ON a.id = s.account_id
		WHERE s.account_id = ? AND a.user_id = ?`,
		accountID.String(), userID.String(),
	).Scan(&delta)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !delta.Valid || strings.TrimSpace(delta.String) == "" {
		return nil, nil
	}
	v := delta.String
	return &v, nil
}

func (r *Repository) UpsertSyncState(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, deltaLink *string, at time.Time) error {
	now := formatRFC3339(at.UTC())
	var link any
	if deltaLink != nil && strings.TrimSpace(*deltaLink) != "" {
		link = strings.TrimSpace(*deltaLink)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_sync_state
		SET delta_link = ?, last_synced_at = ?
		WHERE account_id = ? AND EXISTS (SELECT 1 FROM accounts WHERE id = ? AND user_id = ?)`,
		link, now, accountID.String(), accountID.String(), userID.String(),
	)
	return err
}

func (r *Repository) UpsertSyncStateTime(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, at time.Time) error {
	now := formatRFC3339(at.UTC())
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_sync_state SET last_synced_at = ?
		WHERE account_id = ? AND EXISTS (SELECT 1 FROM accounts WHERE id = ? AND user_id = ?)`,
		now, accountID.String(), accountID.String(), userID.String(),
	)
	return err
}

// --- messages ---

func (r *Repository) UpsertMessage(ctx context.Context, m driven.MessageRow) error {
	now := formatRFC3339(time.Now().UTC())
	fromJSON := m.FromJSON
	if fromJSON == "" {
		fromJSON = "{}"
	}
	toJSON := m.ToJSON
	if toJSON == "" {
		toJSON = "[]"
	}
	ccJSON := m.CcJSON
	if ccJSON == "" {
		ccJSON = "[]"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO messages (id, account_id, provider_message_id, conversation_id, received_at, subject, from_json,
			to_json, cc_json, to_cc_preview, body_text, body_fetched_at, has_attachments, raw_etag, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, provider_message_id) DO UPDATE SET
			conversation_id = excluded.conversation_id,
			received_at = excluded.received_at,
			subject = excluded.subject,
			from_json = excluded.from_json,
			to_json = excluded.to_json,
			cc_json = excluded.cc_json,
			to_cc_preview = excluded.to_cc_preview,
			body_text = COALESCE(excluded.body_text, messages.body_text),
			body_fetched_at = COALESCE(excluded.body_fetched_at, messages.body_fetched_at),
			has_attachments = excluded.has_attachments,
			raw_etag = excluded.raw_etag,
			summary_seen_at = messages.summary_seen_at,
			forward_seen_at = messages.forward_seen_at,
			updated_at = excluded.updated_at`,
		m.ID.String(), m.AccountID.String(), m.ProviderMessageID, nullStr(m.ConversationID),
		formatRFC3339(m.ReceivedAt.UTC()), m.Subject, fromJSON, toJSON, ccJSON,
		nullStr(m.ToCCPreview), nullStr(m.BodyText), nullTimeStr(m.BodyFetchedAt),
		boolInt(m.HasAttachments), nullStr(m.RawEtag), now, now,
	)
	return err
}

func (r *Repository) GetMessageIDByProvider(ctx context.Context, accountID uuid.UUID, providerMessageID string) (uuid.UUID, error) {
	var idStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM messages WHERE account_id = ? AND provider_message_id = ?
	`, accountID.String(), providerMessageID).Scan(&idStr)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, sql.ErrNoRows
	}
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(idStr)
}

func (r *Repository) ListMessagesByAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, limit, offset int) ([]driven.MessageRow, error) {
	filter := driven.MessageListFilter{
		AccountID: &accountID,
		Limit:     limit,
		Offset:    offset,
	}
	return r.ListMessages(ctx, userID, filter)
}

func (r *Repository) GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.MessageRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT m.id, m.account_id, m.provider_message_id, m.conversation_id, m.received_at, m.subject, m.from_json,
			m.to_json, m.cc_json, m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at, m.summary_seen_at, m.forward_seen_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_categories mc ON mc.message_id = m.id AND mc.source = 'llm'
		LEFT JOIN category_definitions cd ON cd.id = mc.category_id
		WHERE m.id = ?`, userID.String(), id.String(),
	)
	m, err := scanMessageRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *Repository) ListMessages(ctx context.Context, userID uuid.UUID, filter driven.MessageListFilter) ([]driven.MessageRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var b strings.Builder
	b.WriteString(`
		SELECT m.id, m.account_id, m.provider_message_id, m.conversation_id, m.received_at, m.subject, m.from_json,
			m.to_json, m.cc_json, m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at, m.summary_seen_at, m.forward_seen_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_categories mc ON mc.message_id = m.id AND mc.source = 'llm'
		LEFT JOIN category_definitions cd ON cd.id = mc.category_id
		WHERE 1=1`)
	args := []any{userID.String()}
	if filter.AccountID != nil {
		b.WriteString(` AND m.account_id = ?`)
		args = append(args, filter.AccountID.String())
	}
	if filter.Category != "" {
		b.WriteString(` AND cd.slug = ?`)
		args = append(args, filter.Category)
	}
	if filter.Since != nil {
		b.WriteString(` AND m.received_at >= ?`)
		args = append(args, formatRFC3339(filter.Since.UTC()))
	}
	if filter.OnlySummaryUnseen {
		b.WriteString(` AND m.summary_seen_at IS NULL`)
	}
	if filter.OnlyForwardUnseen {
		b.WriteString(` AND m.forward_seen_at IS NULL`)
	}
	if filter.ProjectID != nil {
		b.WriteString(`
		AND EXISTS (
			SELECT 1
			FROM messages mx
			LEFT JOIN message_assignment_overrides ox ON ox.message_id = mx.id
			LEFT JOIN thread_assignments tx
				ON tx.account_id = mx.account_id
				AND mx.conversation_id IS NOT NULL
				AND mx.conversation_id != ''
				AND tx.conversation_id = mx.conversation_id
			WHERE mx.id = m.id
			  AND CASE
					WHEN ox.message_id IS NOT NULL THEN ox.project_id = ?
					ELSE tx.project_id = ?
				END
		)`)
		args = append(args, filter.ProjectID.String(), filter.ProjectID.String())
	}
	b.WriteString(` ORDER BY m.received_at DESC LIMIT ? OFFSET ?`)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRows(rows)
}

func (r *Repository) MarkMessagesSummarySeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error {
	if len(messageIDs) == 0 {
		return nil
	}
	seen := formatRFC3339(at.UTC())
	const batch = 80
	for i := 0; i < len(messageIDs); i += batch {
		j := i + batch
		if j > len(messageIDs) {
			j = len(messageIDs)
		}
		chunk := messageIDs[i:j]
		var b strings.Builder
		b.WriteString(`UPDATE messages SET summary_seen_at = ?, updated_at = ? WHERE account_id IN (SELECT id FROM accounts WHERE user_id = ?) AND id IN (`)
		args := []any{seen, seen, userID.String()}
		for k, id := range chunk {
			if k > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, id.String())
		}
		b.WriteString(`)`)
		if _, err := r.db.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) MarkMessagesForwardSeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error {
	if len(messageIDs) == 0 {
		return nil
	}
	seen := formatRFC3339(at.UTC())
	const batch = 80
	for i := 0; i < len(messageIDs); i += batch {
		j := i + batch
		if j > len(messageIDs) {
			j = len(messageIDs)
		}
		chunk := messageIDs[i:j]
		var b strings.Builder
		b.WriteString(`UPDATE messages SET forward_seen_at = ?, updated_at = ? WHERE account_id IN (SELECT id FROM accounts WHERE user_id = ?) AND id IN (`)
		args := []any{seen, seen, userID.String()}
		for k, id := range chunk {
			if k > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, id.String())
		}
		b.WriteString(`)`)
		if _, err := r.db.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) UpsertMessageCategory(ctx context.Context, row driven.MessageCategoryRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO message_categories (id, message_id, account_id, category_id, source, confidence, run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id, source) DO UPDATE SET
			category_id = excluded.category_id,
			account_id = excluded.account_id,
			confidence = excluded.confidence,
			run_id = excluded.run_id,
			updated_at = excluded.updated_at`,
		row.ID.String(), row.MessageID.String(), row.AccountID.String(), row.CategoryID.String(), row.Source, nullFloat(row.Confidence),
		row.RunID.String(), formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()),
	)
	return err
}

func (r *Repository) ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]driven.CategoryDefinitionRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, slug, display_name, definition, sort_order, created_at, updated_at
		FROM category_definitions
		WHERE user_id = ?
		ORDER BY sort_order ASC, slug ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.CategoryDefinitionRow, 0)
	for rows.Next() {
		var idStr, userIDStr, createdAt, updatedAt string
		var row driven.CategoryDefinitionRow
		if err := rows.Scan(&idStr, &userIDStr, &row.Slug, &row.DisplayName, &row.Definition, &row.SortOrder, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		uid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		row.ID = uid
		userUID, err := uuid.Parse(userIDStr)
		if err != nil {
			return nil, err
		}
		row.UserID = userUID
		row.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		row.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) GetCategoryDefinitionBySlug(ctx context.Context, userID uuid.UUID, slug string) (*driven.CategoryDefinitionRow, error) {
	return r.getCategoryDefinition(ctx, `SELECT id, user_id, slug, display_name, definition, sort_order, created_at, updated_at FROM category_definitions WHERE user_id = ? AND slug = ?`, userID.String(), slug)
}

func (r *Repository) GetCategoryDefinitionByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.CategoryDefinitionRow, error) {
	return r.getCategoryDefinition(ctx, `SELECT id, user_id, slug, display_name, definition, sort_order, created_at, updated_at FROM category_definitions WHERE user_id = ? AND id = ?`, userID.String(), id.String())
}

func (r *Repository) getCategoryDefinition(ctx context.Context, query string, args ...any) (*driven.CategoryDefinitionRow, error) {
	var row driven.CategoryDefinitionRow
	var idStr, userIDStr, createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&idStr, &userIDStr, &row.Slug, &row.DisplayName, &row.Definition, &row.SortOrder, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	row.ID = uid
	row.UserID, err = uuid.Parse(userIDStr)
	if err != nil {
		return nil, err
	}
	row.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	row.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) CreateCategoryDefinition(ctx context.Context, row driven.CategoryDefinitionRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		row.ID.String(), row.UserID.String(), row.Slug, row.DisplayName, row.Definition, row.SortOrder,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()),
	)
	return err
}

func (r *Repository) UpdateCategoryDefinition(ctx context.Context, row driven.CategoryDefinitionRow) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE category_definitions
		SET slug = ?, display_name = ?, definition = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`,
		row.Slug, row.DisplayName, row.Definition, row.SortOrder, formatRFC3339(row.UpdatedAt.UTC()), row.ID.String(), row.UserID.String(),
	)
	return err
}

func (r *Repository) ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE message_categories
		SET category_id = ?, updated_at = ?
		WHERE category_id = ?
		  AND account_id IN (SELECT id FROM accounts WHERE user_id = ?)
	`,
		toCategoryID.String(), formatRFC3339(time.Now().UTC()), fromCategoryID.String(), userID.String(),
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *Repository) CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM message_categories mc
		INNER JOIN accounts a ON a.id = mc.account_id
		WHERE mc.category_id = ? AND a.user_id = ?
	`, categoryID.String(), userID.String()).Scan(&n)
	return n, err
}

func (r *Repository) DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM category_definitions WHERE id = ? AND user_id = ?`, id.String(), userID.String())
	return err
}

func scanMessageRows(rows *sql.Rows) ([]driven.MessageRow, error) {
	var out []driven.MessageRow
	for rows.Next() {
		m, err := scanMessageScanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessageRow(row *sql.Row) (*driven.MessageRow, error) {
	return scanMessageScanner(row)
}

func scanMessageScanner(s rowScanner) (*driven.MessageRow, error) {
	var m driven.MessageRow
	var idStr, accStr string
	var conv, preview, body, bodyAt, etag sql.NullString
	var toJSON, ccJSON string
	var receivedAt, createdAt, updatedAt string
	var categorySlug sql.NullString
	var confidence sql.NullFloat64
	var hasAtt int
	var summarySeen sql.NullString
	var forwardSeen sql.NullString
	if err := s.Scan(
		&idStr, &accStr, &m.ProviderMessageID, &conv, &receivedAt, &m.Subject, &m.FromJSON,
		&toJSON, &ccJSON, &preview, &body, &bodyAt, &hasAtt, &etag, &categorySlug, &confidence, &createdAt, &updatedAt, &summarySeen, &forwardSeen,
	); err != nil {
		return nil, err
	}
	var err error
	m.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	m.AccountID, err = uuid.Parse(accStr)
	if err != nil {
		return nil, err
	}
	m.ConversationID = nullStringPtr(conv)
	m.ToJSON = toJSON
	m.CcJSON = ccJSON
	m.ToCCPreview = nullStringPtr(preview)
	m.BodyText = nullStringPtr(body)
	m.CategorySlug = nullStringPtr(categorySlug)
	if confidence.Valid {
		v := confidence.Float64
		m.CategoryConfidence = &v
	}
	if bodyAt.Valid {
		t, err := parseTime(bodyAt.String)
		if err != nil {
			return nil, err
		}
		m.BodyFetchedAt = &t
	}
	m.HasAttachments = hasAtt != 0
	m.RawEtag = nullStringPtr(etag)
	m.ReceivedAt, err = parseTime(receivedAt)
	if err != nil {
		return nil, err
	}
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	m.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	if summarySeen.Valid {
		t, err := parseTime(summarySeen.String)
		if err != nil {
			return nil, err
		}
		m.SummarySeenAt = &t
	}
	if forwardSeen.Valid {
		t, err := parseTime(forwardSeen.String)
		if err != nil {
			return nil, err
		}
		m.ForwardSeenAt = &t
	}
	return &m, nil
}

// --- oauth state ---

func (r *Repository) InsertOAuthState(ctx context.Context, state, flow string, userID *uuid.UUID, payloadJSON string, createdAt time.Time) error {
	var uid any
	if userID != nil {
		uid = userID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oauth_states (state, flow, user_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		state, flow, uid, payloadJSON, formatRFC3339(createdAt.UTC()),
	)
	return err
}

func (r *Repository) TakeOAuthState(ctx context.Context, state string) (flow string, userID *uuid.UUID, payloadJSON string, ok bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, "", false, err
	}
	defer tx.Rollback()
	var f, payload, created string
	var uid sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT flow, user_id, payload_json, created_at FROM oauth_states WHERE state = ?`, state).Scan(&f, &uid, &payload, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, "", false, nil
	}
	if err != nil {
		return "", nil, "", false, err
	}
	createdAt, err := parseTime(created)
	if err != nil {
		return "", nil, "", false, err
	}
	if time.Now().UTC().Sub(createdAt) > r.OAuthStateTTL {
		_, _ = tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state)
		if err := tx.Commit(); err != nil {
			return "", nil, "", false, err
		}
		return "", nil, "", false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return "", nil, "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", nil, "", false, err
	}
	var u *uuid.UUID
	if uid.Valid {
		parsed, e := uuid.Parse(uid.String)
		if e != nil {
			return "", nil, "", false, e
		}
		u = &parsed
	}
	return f, u, payload, true, nil
}

func (r *Repository) DeleteExpiredStates(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE created_at < ?`, formatRFC3339(before.UTC()))
	return err
}

// PromoteJobRunToRunning marks a previously enqueued pending run as running without wiping meta_json
// or changing started_at (enqueue time stays authoritative).
func (r *Repository) PromoteJobRunToRunning(ctx context.Context, id uuid.UUID, startedAt time.Time) error {
	_ = startedAt
	res, err := r.db.ExecContext(ctx, `
		UPDATE job_runs
		SET status = 'running',
		    finished_at = NULL,
		    error_message = NULL
		WHERE id = ? AND status IN ('pending', 'running')`,
		id.String(),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job run %s not found or not pending/running", id.String())
	}
	return nil
}

// InsertJobRun inserts or updates a job run row (same id: final status update).
func (r *Repository) InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error {
	var fin any
	if !finishedAt.IsZero() {
		fin = formatRFC3339(finishedAt.UTC())
	}
	var account any
	if accountID != uuid.Nil {
		account = accountID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO job_runs (id, account_id, job_type, trigger_kind, status, time_window_start, time_window_end,
			started_at, finished_at, error_message, meta_json)
		VALUES (?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			started_at = CASE WHEN job_runs.started_at IS NULL OR job_runs.started_at = '' THEN excluded.started_at ELSE job_runs.started_at END,
			finished_at = excluded.finished_at,
			error_message = excluded.error_message,
			meta_json = excluded.meta_json`,
		id.String(), account, jobType, trigger, status,
		formatRFC3339(startedAt.UTC()), fin, nullStr(errMsg), metaJSON,
	)
	return err
}

func (r *Repository) UpdateJobRunMeta(ctx context.Context, id uuid.UUID, metaJSON string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE job_runs SET meta_json = ? WHERE id = ?`, metaJSON, id.String())
	return err
}

func (r *Repository) UpdateJobRunStatus(ctx context.Context, id uuid.UUID, status string, finishedAt *time.Time, errMsg *string, metaJSON string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE job_runs
		SET status = ?, finished_at = ?, error_message = ?, meta_json = ?
		WHERE id = ?
	`, status, nullTimeStr(finishedAt), nullStr(errMsg), metaJSON, id.String())
	return err
}

func (r *Repository) ListJobRuns(ctx context.Context, userID uuid.UUID, filter driven.JobRunListFilter) ([]driven.JobRunRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var b strings.Builder
	b.WriteString(`
		SELECT j.id, j.account_id, COALESCE(a.label, ca.label), j.job_type, j.trigger_kind, j.status,
			j.time_window_start, j.time_window_end, j.started_at, j.finished_at, j.error_message, j.meta_json
		FROM job_runs j
		LEFT JOIN accounts a ON a.id = j.account_id
		LEFT JOIN connector_accounts ca ON ca.id = CASE
			WHEN json_valid(j.meta_json) THEN json_extract(j.meta_json, '$.connector_account_id')
			ELSE NULL
		END
		WHERE (a.user_id = ? OR ca.user_id = ?)`)
	args := []any{userID.String(), userID.String()}
	if filter.AccountID != nil {
		b.WriteString(` AND j.account_id = ?`)
		args = append(args, filter.AccountID.String())
	}
	if filter.JobType != "" {
		b.WriteString(` AND j.job_type = ?`)
		args = append(args, filter.JobType)
	}
	b.WriteString(` ORDER BY j.started_at DESC LIMIT ? OFFSET ?`)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]driven.JobRunRow, 0)
	for rows.Next() {
		item, err := scanJobRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.JobRunRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT j.id, j.account_id, COALESCE(a.label, ca.label), j.job_type, j.trigger_kind, j.status,
			j.time_window_start, j.time_window_end, j.started_at, j.finished_at, j.error_message, j.meta_json
		FROM job_runs j
		LEFT JOIN accounts a ON a.id = j.account_id
		LEFT JOIN connector_accounts ca ON ca.id = CASE
			WHEN json_valid(j.meta_json) THEN json_extract(j.meta_json, '$.connector_account_id')
			ELSE NULL
		END
		WHERE j.id = ? AND (a.user_id = ? OR ca.user_id = ?)`,
		id.String(), userID.String(), userID.String(),
	)
	item, err := scanJobRunRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) GetSummarySettings(ctx context.Context, userID uuid.UUID) (*driven.SummarySettingsRow, error) {
	var includeJSON, excludeJSON, updatedAt string
	var chunkSize int
	err := r.db.QueryRowContext(ctx, `
		SELECT include_category_slugs, exclude_category_slugs, chunk_size, updated_at
		FROM summary_settings
		WHERE user_id = ?
	`, userID.String()).Scan(&includeJSON, &excludeJSON, &chunkSize, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row := &driven.SummarySettingsRow{UserID: userID}
	if err := json.Unmarshal([]byte(includeJSON), &row.IncludeCategorySlugs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(excludeJSON), &row.ExcludeCategorySlugs); err != nil {
		return nil, err
	}
	t, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	row.UpdatedAt = t
	row.ChunkSize = chunkSize
	return row, nil
}

func (r *Repository) UpsertSummarySettings(ctx context.Context, row driven.SummarySettingsRow) error {
	includeJSON, _ := json.Marshal(row.IncludeCategorySlugs)
	excludeJSON, _ := json.Marshal(row.ExcludeCategorySlugs)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO summary_settings (user_id, include_category_slugs, exclude_category_slugs, chunk_size, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			include_category_slugs = excluded.include_category_slugs,
			exclude_category_slugs = excluded.exclude_category_slugs,
			chunk_size = excluded.chunk_size,
			updated_at = excluded.updated_at
	`, row.UserID.String(), string(includeJSON), string(excludeJSON), row.ChunkSize, formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) InsertSummarySnapshot(ctx context.Context, row driven.SummarySnapshotRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO summary_snapshots (id, user_id, account_id, run_id, window_start, window_end, general_summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), nullUUID(row.AccountID), row.RunID.String(),
		formatRFC3339(row.WindowStart.UTC()), formatRFC3339(row.WindowEnd.UTC()), row.GeneralSummary, formatRFC3339(row.CreatedAt.UTC()))
	return err
}

func (r *Repository) ListSummarySnapshots(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]driven.SummarySnapshotRow, error) {
	if limit <= 0 {
		limit = 1
	}
	var b strings.Builder
	b.WriteString(`SELECT id, user_id, account_id, run_id, window_start, window_end, general_summary, created_at FROM summary_snapshots WHERE user_id = ?`)
	args := []any{userID.String()}
	if accountID != nil {
		b.WriteString(` AND account_id = ?`)
		args = append(args, accountID.String())
	}
	b.WriteString(` ORDER BY created_at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.SummarySnapshotRow, 0)
	for rows.Next() {
		var idStr, uidStr, runStr, ws, we, summary, createdAt string
		var acc sql.NullString
		if err := rows.Scan(&idStr, &uidStr, &acc, &runStr, &ws, &we, &summary, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(uidStr)
		runID, _ := uuid.Parse(runStr)
		wst, _ := parseTime(ws)
		wet, _ := parseTime(we)
		cat, _ := parseTime(createdAt)
		item := driven.SummarySnapshotRow{ID: id, UserID: uid, RunID: runID, WindowStart: wst, WindowEnd: wet, GeneralSummary: summary, CreatedAt: cat}
		if acc.Valid {
			aid, _ := uuid.Parse(acc.String)
			item.AccountID = &aid
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) InsertActionItems(ctx context.Context, rows []driven.ActionItemRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, row := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO action_items (id, user_id, account_id, message_id, run_id, text, due_at, status, actioned_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RunID.String(), row.Text,
			nullTimeStr(row.DueAt), row.Status, nullTimeStr(row.ActionedAt), formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListOpenActionItems(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]driven.ActionItemRow, error) {
	var b strings.Builder
	b.WriteString(`SELECT id, user_id, account_id, message_id, run_id, text, due_at, status, actioned_at, auto_draft_seen_at, created_at, updated_at FROM action_items WHERE user_id = ? AND status = 'open'`)
	args := []any{userID.String()}
	if accountID != nil {
		b.WriteString(` AND account_id = ?`)
		args = append(args, accountID.String())
	}
	b.WriteString(` ORDER BY created_at DESC`)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ActionItemRow, 0)
	for rows.Next() {
		var idStr, uidStr, accStr, msgStr, runStr, text, status, createdAt, updatedAt string
		var dueAt, actioned, autoDraftSeen sql.NullString
		if err := rows.Scan(&idStr, &uidStr, &accStr, &msgStr, &runStr, &text, &dueAt, &status, &actioned, &autoDraftSeen, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(uidStr)
		acc, _ := uuid.Parse(accStr)
		msg, _ := uuid.Parse(msgStr)
		runID, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		upd, _ := parseTime(updatedAt)
		item := driven.ActionItemRow{ID: id, UserID: uid, AccountID: acc, MessageID: msg, RunID: runID, Text: text, Status: status, CreatedAt: cat, UpdatedAt: upd}
		if dueAt.Valid {
			t, _ := parseTime(dueAt.String)
			item.DueAt = &t
		}
		if actioned.Valid {
			t, _ := parseTime(actioned.String)
			item.ActionedAt = &t
		}
		if autoDraftSeen.Valid {
			t, _ := parseTime(autoDraftSeen.String)
			item.AutoDraftSeenAt = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE action_items SET status = 'done', actioned_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, formatRFC3339(at.UTC()), formatRFC3339(at.UTC()), itemID.String(), userID.String())
	return err
}

func (r *Repository) InsertFYI(ctx context.Context, rows []driven.FYIRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, row := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO fyi_items (id, user_id, account_id, message_id, run_id, text, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RunID.String(), row.Text, formatRFC3339(row.CreatedAt.UTC()))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListFYIByRun(ctx context.Context, userID uuid.UUID, runID uuid.UUID) ([]driven.FYIRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, account_id, message_id, run_id, text, created_at
		FROM fyi_items WHERE user_id = ? AND run_id = ? ORDER BY created_at DESC
	`, userID.String(), runID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FYIRow, 0)
	for rows.Next() {
		var idStr, uidStr, accStr, msgStr, runStr, text, createdAt string
		if err := rows.Scan(&idStr, &uidStr, &accStr, &msgStr, &runStr, &text, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(uidStr)
		acc, _ := uuid.Parse(accStr)
		msg, _ := uuid.Parse(msgStr)
		rid, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		out = append(out, driven.FYIRow{ID: id, UserID: uid, AccountID: acc, MessageID: msg, RunID: rid, Text: text, CreatedAt: cat})
	}
	return out, rows.Err()
}

func (r *Repository) ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]driven.FYIRow, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	var b strings.Builder
	b.WriteString(`
		SELECT id, user_id, account_id, message_id, run_id, text, created_at
		FROM fyi_items
		WHERE user_id = ?
	`)
	args := []any{userID.String()}
	if accountID != nil {
		b.WriteString(` AND account_id = ?`)
		args = append(args, accountID.String())
	}
	b.WriteString(` ORDER BY created_at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FYIRow, 0)
	for rows.Next() {
		var idStr, uidStr, accStr, msgStr, runStr, text, createdAt string
		if err := rows.Scan(&idStr, &uidStr, &accStr, &msgStr, &runStr, &text, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(uidStr)
		acc, _ := uuid.Parse(accStr)
		msg, _ := uuid.Parse(msgStr)
		rid, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		out = append(out, driven.FYIRow{ID: id, UserID: uid, AccountID: acc, MessageID: msg, RunID: rid, Text: text, CreatedAt: cat})
	}
	return out, rows.Err()
}

func (r *Repository) DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM fyi_items
		WHERE id = ? AND user_id = ?
	`, id.String(), userID.String())
	return err
}

func (r *Repository) ListActionItemsForAutoDraft(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, onlyUnseen bool, limit int) ([]driven.ActionItemRow, error) {
	if limit <= 0 {
		limit = 50
	}
	var b strings.Builder
	b.WriteString(`SELECT id, user_id, account_id, message_id, run_id, text, due_at, status, actioned_at, auto_draft_seen_at, created_at, updated_at FROM action_items WHERE user_id = ? AND account_id = ? AND status = 'open'`)
	args := []any{userID.String(), accountID.String()}
	if onlyUnseen {
		b.WriteString(` AND auto_draft_seen_at IS NULL`)
	}
	b.WriteString(` ORDER BY created_at ASC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ActionItemRow, 0)
	for rows.Next() {
		var idStr, uidStr, accStr, msgStr, runStr, text, status, createdAt, updatedAt string
		var dueAt, actioned, autoDraftSeen sql.NullString
		if err := rows.Scan(&idStr, &uidStr, &accStr, &msgStr, &runStr, &text, &dueAt, &status, &actioned, &autoDraftSeen, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(uidStr)
		acc, _ := uuid.Parse(accStr)
		msg, _ := uuid.Parse(msgStr)
		runID, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		upd, _ := parseTime(updatedAt)
		item := driven.ActionItemRow{ID: id, UserID: uid, AccountID: acc, MessageID: msg, RunID: runID, Text: text, Status: status, CreatedAt: cat, UpdatedAt: upd}
		if dueAt.Valid {
			t, _ := parseTime(dueAt.String)
			item.DueAt = &t
		}
		if actioned.Valid {
			t, _ := parseTime(actioned.String)
			item.ActionedAt = &t
		}
		if autoDraftSeen.Valid {
			t, _ := parseTime(autoDraftSeen.String)
			item.AutoDraftSeenAt = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkActionItemsAutoDraftSeen(ctx context.Context, userID uuid.UUID, itemIDs []uuid.UUID, at time.Time) error {
	if len(itemIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range itemIDs {
		_, err := tx.ExecContext(ctx, `
			UPDATE action_items
			SET auto_draft_seen_at = ?, updated_at = ?
			WHERE id = ? AND user_id = ?
		`, formatRFC3339(at.UTC()), formatRFC3339(at.UTC()), id.String(), userID.String())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) InsertDraftSuggestions(ctx context.Context, rows []driven.DraftSuggestionRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, row := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO draft_suggestions (id, user_id, account_id, message_id, action_item_id, run_id, subject, body, model, status, sent_at, discarded_at, updated_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.ActionItemID.String(),
			row.RunID.String(), row.Subject, row.Body, row.Model, "ready", nil, nil, formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.CreatedAt.UTC()),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]driven.DraftSuggestionRow, error) {
	if limit <= 0 {
		limit = 100
	}
	var b strings.Builder
	b.WriteString(`
		SELECT ds.id, ds.user_id, ds.account_id, ds.message_id, ds.action_item_id, ds.run_id, ds.subject, ds.body, ds.model, m.from_json, ds.status, ds.sent_at, ds.discarded_at, ds.updated_at, ds.created_at
		FROM draft_suggestions ds
		INNER JOIN messages m ON m.id = ds.message_id
		WHERE ds.user_id = ? AND ds.status = 'ready'
	`)
	args := []any{userID.String()}
	if accountID != nil {
		b.WriteString(` AND ds.account_id = ?`)
		args = append(args, accountID.String())
	}
	b.WriteString(` ORDER BY ds.created_at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.DraftSuggestionRow, 0)
	for rows.Next() {
		var idStr, userStr, accStr, msgStr, actionStr, runStr, subject, body, model, fromJSON, status, createdAt string
		var sentAt, discardedAt, updatedAt sql.NullString
		if err := rows.Scan(&idStr, &userStr, &accStr, &msgStr, &actionStr, &runStr, &subject, &body, &model, &fromJSON, &status, &sentAt, &discardedAt, &updatedAt, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		acc, _ := uuid.Parse(accStr)
		msg, _ := uuid.Parse(msgStr)
		actionID, _ := uuid.Parse(actionStr)
		runID, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		item := driven.DraftSuggestionRow{
			ID:           id,
			UserID:       uid,
			AccountID:    acc,
			MessageID:    msg,
			ActionItemID: actionID,
			RunID:        runID,
			Subject:      subject,
			Body:         body,
			Model:        model,
			FromJSON:     fromJSON,
			Status:       status,
			CreatedAt:    cat,
		}
		if sentAt.Valid {
			t, _ := parseTime(sentAt.String)
			item.SentAt = &t
		}
		if discardedAt.Valid {
			t, _ := parseTime(discardedAt.String)
			item.DiscardedAt = &t
		}
		if updatedAt.Valid {
			t, _ := parseTime(updatedAt.String)
			item.UpdatedAt = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) GetDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID) (*driven.DraftSuggestionRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT ds.id, ds.user_id, ds.account_id, ds.message_id, ds.action_item_id, ds.run_id, ds.subject, ds.body, ds.model, m.from_json, ds.status, ds.sent_at, ds.discarded_at, ds.updated_at, ds.created_at
		FROM draft_suggestions ds
		INNER JOIN messages m ON m.id = ds.message_id
		WHERE ds.id = ? AND ds.user_id = ?
	`, draftID.String(), userID.String())
	var idStr, userStr, accStr, msgStr, actionStr, runStr, subject, body, model, fromJSON, status, createdAt string
	var sentAt, discardedAt, updatedAt sql.NullString
	if err := row.Scan(&idStr, &userStr, &accStr, &msgStr, &actionStr, &runStr, &subject, &body, &model, &fromJSON, &status, &sentAt, &discardedAt, &updatedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	uid, _ := uuid.Parse(userStr)
	acc, _ := uuid.Parse(accStr)
	msg, _ := uuid.Parse(msgStr)
	actionID, _ := uuid.Parse(actionStr)
	runID, _ := uuid.Parse(runStr)
	cat, _ := parseTime(createdAt)
	item := &driven.DraftSuggestionRow{
		ID:           id,
		UserID:       uid,
		AccountID:    acc,
		MessageID:    msg,
		ActionItemID: actionID,
		RunID:        runID,
		Subject:      subject,
		Body:         body,
		Model:        model,
		FromJSON:     fromJSON,
		Status:       status,
		CreatedAt:    cat,
	}
	if sentAt.Valid {
		t, _ := parseTime(sentAt.String)
		item.SentAt = &t
	}
	if discardedAt.Valid {
		t, _ := parseTime(discardedAt.String)
		item.DiscardedAt = &t
	}
	if updatedAt.Valid {
		t, _ := parseTime(updatedAt.String)
		item.UpdatedAt = &t
	}
	return item, nil
}

func (r *Repository) UpdateDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, subject, body string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE draft_suggestions
		SET subject = ?, body = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND status = 'ready'
	`, subject, body, formatRFC3339(at.UTC()), draftID.String(), userID.String())
	return err
}

func (r *Repository) MarkDraftSuggestionStatus(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, status string, at time.Time) error {
	var sentAt any
	var discardedAt any
	if status == "sent" {
		sentAt = formatRFC3339(at.UTC())
	}
	if status == "discarded" {
		discardedAt = formatRFC3339(at.UTC())
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE draft_suggestions
		SET status = ?, sent_at = COALESCE(?, sent_at), discarded_at = COALESCE(?, discarded_at), updated_at = ?
		WHERE id = ? AND user_id = ?
	`, status, sentAt, discardedAt, formatRFC3339(at.UTC()), draftID.String(), userID.String())
	return err
}

func (r *Repository) InsertSendAttempt(ctx context.Context, row driven.SendAttemptRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO send_attempts (id, user_id, account_id, draft_id, message_id, status, provider_message_id, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.DraftID.String(), row.MessageID.String(),
		row.Status, nullStr(row.ProviderMessageID), nullStr(row.ErrorMessage), formatRFC3339(row.CreatedAt.UTC()))
	return err
}

func (r *Repository) ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]driven.SendAttemptRow, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, account_id, draft_id, message_id, status, provider_message_id, error_message, created_at
		FROM send_attempts
		WHERE user_id = ? AND draft_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, userID.String(), draftID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.SendAttemptRow, 0, limit)
	for rows.Next() {
		var idStr, userStr, accountStr, draftStr, msgStr, status, createdAt string
		var providerMsg, errMsg sql.NullString
		if err := rows.Scan(&idStr, &userStr, &accountStr, &draftStr, &msgStr, &status, &providerMsg, &errMsg, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		acc, _ := uuid.Parse(accountStr)
		did, _ := uuid.Parse(draftStr)
		msg, _ := uuid.Parse(msgStr)
		cat, _ := parseTime(createdAt)
		item := driven.SendAttemptRow{
			ID:        id,
			UserID:    uid,
			AccountID: acc,
			DraftID:   did,
			MessageID: msg,
			Status:    status,
			CreatedAt: cat,
		}
		item.ProviderMessageID = nullStringPtr(providerMsg)
		item.ErrorMessage = nullStringPtr(errMsg)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]driven.ForwardAllowlistRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, email, created_at
		FROM forward_allowlist
		WHERE user_id = ?
		ORDER BY email ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ForwardAllowlistRow, 0)
	for rows.Next() {
		var idStr, userStr, email, createdAt string
		if err := rows.Scan(&idStr, &userStr, &email, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		cat, _ := parseTime(createdAt)
		out = append(out, driven.ForwardAllowlistRow{ID: id, UserID: uid, Email: email, CreatedAt: cat})
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM forward_allowlist WHERE user_id = ?`, userID.String()); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, email := range emails {
		trimmed := strings.ToLower(strings.TrimSpace(email))
		if trimmed == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO forward_allowlist (id, user_id, email, created_at)
			VALUES (?, ?, ?, ?)
		`, uuid.New().String(), userID.String(), trimmed, formatRFC3339(now))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]driven.ForwardRuleRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, account_id, name, mode, condition_json, forward_to, enabled, created_at, updated_at
		FROM forward_rules
		WHERE user_id = ? AND account_id = ?
		ORDER BY created_at DESC
	`, userID.String(), accountID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ForwardRuleRow, 0)
	for rows.Next() {
		var idStr, userStr, accountStr, name, mode, conditionJSON, forwardTo, createdAt, updatedAt string
		var enabled bool
		if err := rows.Scan(&idStr, &userStr, &accountStr, &name, &mode, &conditionJSON, &forwardTo, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		acc, _ := uuid.Parse(accountStr)
		cat, _ := parseTime(createdAt)
		uat, _ := parseTime(updatedAt)
		out = append(out, driven.ForwardRuleRow{
			ID: id, UserID: uid, AccountID: acc, Name: name, Mode: mode, ConditionJSON: conditionJSON,
			ForwardTo: forwardTo, Enabled: enabled, CreatedAt: cat, UpdatedAt: uat,
		})
	}
	return out, rows.Err()
}

func (r *Repository) CreateForwardRule(ctx context.Context, row driven.ForwardRuleRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO forward_rules (id, user_id, account_id, name, mode, condition_json, forward_to, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.Name, row.Mode, row.ConditionJSON, row.ForwardTo, row.Enabled, formatRFC3339(row.CreatedAt), formatRFC3339(row.UpdatedAt))
	return err
}

func (r *Repository) UpdateForwardRule(ctx context.Context, row driven.ForwardRuleRow) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE forward_rules
		SET name = ?, mode = ?, condition_json = ?, forward_to = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, row.Name, row.Mode, row.ConditionJSON, row.ForwardTo, row.Enabled, formatRFC3339(row.UpdatedAt), row.ID.String(), row.UserID.String())
	return err
}

func (r *Repository) DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM forward_rules WHERE id = ? AND user_id = ?`, ruleID.String(), userID.String())
	return err
}

func (r *Repository) ListForwardAuditByRun(ctx context.Context, userID, runID uuid.UUID) ([]driven.ForwardAuditRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, account_id, message_id, rule_id, run_id, status, reason, created_at
		FROM forward_audit
		WHERE user_id = ? AND run_id = ?
		ORDER BY created_at DESC
	`, userID.String(), runID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ForwardAuditRow, 0)
	for rows.Next() {
		var idStr, userStr, accountStr, messageStr, ruleStr, runStr, status, createdAt string
		var reason sql.NullString
		if err := rows.Scan(&idStr, &userStr, &accountStr, &messageStr, &ruleStr, &runStr, &status, &reason, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		acc, _ := uuid.Parse(accountStr)
		msg, _ := uuid.Parse(messageStr)
		rid, _ := uuid.Parse(ruleStr)
		jid, _ := uuid.Parse(runStr)
		cat, _ := parseTime(createdAt)
		out = append(out, driven.ForwardAuditRow{
			ID: id, UserID: uid, AccountID: acc, MessageID: msg, RuleID: rid, RunID: jid,
			Status: status, Reason: nullStringPtr(reason), CreatedAt: cat,
		})
	}
	return out, rows.Err()
}

func (r *Repository) InsertForwardAudit(ctx context.Context, row driven.ForwardAuditRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO forward_audit (id, user_id, account_id, message_id, rule_id, run_id, status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RuleID.String(), row.RunID.String(), row.Status, nullStr(row.Reason), formatRFC3339(row.CreatedAt))
	return err
}

func (r *Repository) InsertManualForwardAudit(ctx context.Context, row driven.ManualForwardAuditRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO manual_forward_audit (id, user_id, account_id, message_id, to_email, comment, status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.ToEmail, nullStr(row.Comment), row.Status, nullStr(row.Reason), formatRFC3339(row.CreatedAt))
	return err
}

func (r *Repository) ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]driven.ScheduleChainRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, account_id, jobs_json, interval_minutes, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM schedule_chains
		WHERE user_id = ?
		ORDER BY created_at ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ScheduleChainRow, 0)
	for rows.Next() {
		item, err := scanScheduleChainRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceSchedulesByUser(ctx context.Context, userID uuid.UUID, rows []driven.ScheduleChainRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM schedule_chains WHERE user_id = ?`, userID.String()); err != nil {
		return err
	}
	for _, row := range rows {
		jobsJSON, _ := json.Marshal(row.Jobs)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO schedule_chains (id, user_id, name, account_id, jobs_json, interval_minutes, enabled, last_run_at, next_run_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			row.ID.String(), userID.String(), row.Name, nullUUID(row.AccountID), string(jobsJSON), row.IntervalMinutes, boolInt(row.Enabled),
			nullTimeStr(row.LastRunAt), formatRFC3339(row.NextRunAt.UTC()), formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]driven.ScheduleChainRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, account_id, jobs_json, interval_minutes, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM schedule_chains
		WHERE enabled = 1 AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?
	`, formatRFC3339(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ScheduleChainRow, 0)
	for rows.Next() {
		item, err := scanScheduleChainRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkScheduleExecuted(ctx context.Context, id uuid.UUID, lastRunAt, nextRunAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE schedule_chains
		SET last_run_at = ?, next_run_at = ?, updated_at = ?
		WHERE id = ?
	`, formatRFC3339(lastRunAt.UTC()), formatRFC3339(nextRunAt.UTC()), formatRFC3339(time.Now().UTC()), id.String())
	return err
}

// --- helpers ---

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func nullTimeStr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatRFC3339(t.UTC())
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullUUID(v *uuid.UUID) any {
	if v == nil {
		return nil
	}
	return v.String()
}

func scanJobRunRow(s rowScanner) (*driven.JobRunRow, error) {
	var item driven.JobRunRow
	var idStr string
	var accountID sql.NullString
	var accountLabel sql.NullString
	var twStart sql.NullString
	var twEnd sql.NullString
	var startedAt string
	var finishedAt sql.NullString
	var errMsg sql.NullString
	if err := s.Scan(
		&idStr, &accountID, &accountLabel, &item.JobType, &item.TriggerKind, &item.Status,
		&twStart, &twEnd, &startedAt, &finishedAt, &errMsg, &item.MetaJSON,
	); err != nil {
		return nil, err
	}
	var err error
	item.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	if accountID.Valid {
		aid, err := uuid.Parse(accountID.String)
		if err != nil {
			return nil, err
		}
		item.AccountID = &aid
	}
	item.AccountLabel = nullStringPtr(accountLabel)
	item.ErrorMessage = nullStringPtr(errMsg)
	item.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t, err := parseTime(finishedAt.String)
		if err != nil {
			return nil, err
		}
		item.FinishedAt = &t
	}
	if twStart.Valid {
		t, err := parseTime(twStart.String)
		if err != nil {
			return nil, err
		}
		item.TimeWindowStart = &t
	}
	if twEnd.Valid {
		t, err := parseTime(twEnd.String)
		if err != nil {
			return nil, err
		}
		item.TimeWindowEnd = &t
	}
	return &item, nil
}

func scanScheduleChainRow(s rowScanner) (*driven.ScheduleChainRow, error) {
	var item driven.ScheduleChainRow
	var idStr, userStr, jobsJSON, nextRunAt, createdAt, updatedAt string
	var accountID sql.NullString
	var lastRunAt sql.NullString
	var enabled int
	if err := s.Scan(
		&idStr, &userStr, &item.Name, &accountID, &jobsJSON, &item.IntervalMinutes, &enabled, &lastRunAt, &nextRunAt, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(userStr)
	if err != nil {
		return nil, err
	}
	item.ID = id
	item.UserID = userID
	item.Enabled = enabled == 1
	if accountID.Valid {
		acc, err := uuid.Parse(accountID.String)
		if err != nil {
			return nil, err
		}
		item.AccountID = &acc
	}
	if err := json.Unmarshal([]byte(jobsJSON), &item.Jobs); err != nil {
		return nil, err
	}
	if lastRunAt.Valid {
		t, err := parseTime(lastRunAt.String)
		if err != nil {
			return nil, err
		}
		item.LastRunAt = &t
	}
	item.NextRunAt, err = parseTime(nextRunAt)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}
