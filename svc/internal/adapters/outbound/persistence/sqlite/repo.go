package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO messages (id, account_id, provider_message_id, conversation_id, received_at, subject, from_json,
			to_cc_preview, body_text, body_fetched_at, has_attachments, raw_etag, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, provider_message_id) DO UPDATE SET
			conversation_id = excluded.conversation_id,
			received_at = excluded.received_at,
			subject = excluded.subject,
			from_json = excluded.from_json,
			to_cc_preview = excluded.to_cc_preview,
			body_text = COALESCE(excluded.body_text, messages.body_text),
			body_fetched_at = COALESCE(excluded.body_fetched_at, messages.body_fetched_at),
			has_attachments = excluded.has_attachments,
			raw_etag = excluded.raw_etag,
			updated_at = excluded.updated_at`,
		m.ID.String(), m.AccountID.String(), m.ProviderMessageID, nullStr(m.ConversationID),
		formatRFC3339(m.ReceivedAt.UTC()), m.Subject, fromJSON,
		nullStr(m.ToCCPreview), nullStr(m.BodyText), nullTimeStr(m.BodyFetchedAt),
		boolInt(m.HasAttachments), nullStr(m.RawEtag), now, now,
	)
	return err
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
			m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at
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
			m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at
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
	b.WriteString(` ORDER BY m.received_at DESC LIMIT ? OFFSET ?`)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRows(rows)
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
	var receivedAt, createdAt, updatedAt string
	var categorySlug sql.NullString
	var confidence sql.NullFloat64
	var hasAtt int
	if err := s.Scan(
		&idStr, &accStr, &m.ProviderMessageID, &conv, &receivedAt, &m.Subject, &m.FromJSON,
		&preview, &body, &bodyAt, &hasAtt, &etag, &categorySlug, &confidence, &createdAt, &updatedAt,
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

// InsertJobRun inserts or updates a job run row (same id: final status update).
func (r *Repository) InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error {
	var fin any
	if !finishedAt.IsZero() {
		fin = formatRFC3339(finishedAt.UTC())
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
		id.String(), accountID.String(), jobType, trigger, status,
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
		SELECT j.id, j.account_id, a.label, j.job_type, j.trigger_kind, j.status,
			j.time_window_start, j.time_window_end, j.started_at, j.finished_at, j.error_message, j.meta_json
		FROM job_runs j
		LEFT JOIN accounts a ON a.id = j.account_id
		WHERE a.user_id = ?`)
	args := []any{userID.String()}
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
		SELECT j.id, j.account_id, a.label, j.job_type, j.trigger_kind, j.status,
			j.time_window_start, j.time_window_end, j.started_at, j.finished_at, j.error_message, j.meta_json
		FROM job_runs j
		LEFT JOIN accounts a ON a.id = j.account_id
		WHERE j.id = ? AND a.user_id = ?`,
		id.String(), userID.String(),
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
	b.WriteString(`SELECT id, user_id, account_id, message_id, run_id, text, due_at, status, actioned_at, created_at, updated_at FROM action_items WHERE user_id = ? AND status = 'open'`)
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
		var dueAt, actioned sql.NullString
		if err := rows.Scan(&idStr, &uidStr, &accStr, &msgStr, &runStr, &text, &dueAt, &status, &actioned, &createdAt, &updatedAt); err != nil {
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

func (r *Repository) DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM fyi_items
		WHERE id = ? AND user_id = ?
	`, id.String(), userID.String())
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
