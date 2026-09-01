package postgres

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
	"github.com/Kapital-B/automata/svc/internal/domain/organisations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultOAuthStateTTL          = 15 * time.Minute
	defaultSerializationRetries   = 3
	defaultMutationChunkSize      = 500
	defaultTimelineCandidateLimit = 2000
)

var categoryNamespace = uuid.MustParse("f53fb63c-79c4-4cc5-a037-24dc77915f0d")

var defaultCategories = []struct {
	Slug        string
	DisplayName string
	Definition  string
	SortOrder   int
}{
	{"important", "Important", "Emails requiring timely attention, decisions, or follow-up.", 10},
	{"finance", "Finance", "Invoices, receipts, statements, payments, taxes, banking, or accounting.", 20},
	{"personal", "Personal", "Personal correspondence, family, friends, appointments, or non-work life admin.", 30},
	{"newsletter", "Newsletter", "Subscriptions, digests, marketing updates, announcements, or recurring publications.", 40},
	{"spam", "Spam", "Unwanted, suspicious, deceptive, or low-value promotional email.", 50},
	{"other", "Other", "Anything that does not clearly fit another category.", 60},
}

type Repository struct {
	db            *sql.DB
	OAuthStateTTL time.Duration
	txIsolation   sql.IsolationLevel
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewRepository(db *sql.DB, oauthStateTTL time.Duration) *Repository {
	return NewRepositoryWithIsolation(db, oauthStateTTL, sql.LevelSerializable)
}

// NewRepositoryWithIsolation configures transaction isolation.
// Aurora DSQL rejects SERIALIZABLE and runs at Repeatable Read only.
func NewRepositoryWithIsolation(db *sql.DB, oauthStateTTL time.Duration, isolation sql.IsolationLevel) *Repository {
	if oauthStateTTL <= 0 {
		oauthStateTTL = defaultOAuthStateTTL
	}
	if isolation == sql.LevelDefault {
		isolation = sql.LevelSerializable
	}
	return &Repository{db: db, OAuthStateTTL: oauthStateTTL, txIsolation: isolation}
}

func (r *Repository) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return r.db.ExecContext(ctx, rewritePlaceholders(query), args...)
}

func (r *Repository) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.db.QueryContext(ctx, rewritePlaceholders(query), args...)
}

func (r *Repository) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return r.db.QueryRowContext(ctx, rewritePlaceholders(query), args...)
}

func txExecContext(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(ctx, rewritePlaceholders(query), args...)
}

func txQueryContext(ctx context.Context, tx *sql.Tx, query string, args ...any) (*sql.Rows, error) {
	return tx.QueryContext(ctx, rewritePlaceholders(query), args...)
}

func txQueryRowContext(ctx context.Context, tx *sql.Tx, query string, args ...any) *sql.Row {
	return tx.QueryRowContext(ctx, rewritePlaceholders(query), args...)
}

func (r *Repository) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	_, err := withSerializableRetry(ctx, func(ctx context.Context) (struct{}, error) {
		tx, err := r.beginTx(ctx)
		if err != nil {
			return struct{}{}, err
		}
		defer func() { _ = tx.Rollback() }()
		if err := fn(tx); err != nil {
			return struct{}{}, err
		}
		if err := tx.Commit(); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (r *Repository) beginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: r.txIsolation})
}

func withSerializableRetry[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt <= defaultSerializationRetries; attempt++ {
		out, err := fn(ctx)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isSerializationFailure(err) || attempt == defaultSerializationRetries {
			return zero, err
		}
		wait := time.Duration(attempt+1) * 25 * time.Millisecond
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, lastErr
}

func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001"
	}
	return strings.Contains(strings.ToLower(err.Error()), "40001")
}

func rewritePlaceholders(query string) string {
	var b strings.Builder
	inSingle := false
	argNum := 1
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			b.WriteByte(ch)
			if inSingle && i+1 < len(query) && query[i+1] == '\'' {
				i++
				b.WriteByte(query[i])
				continue
			}
			inSingle = !inSingle
			continue
		}
		if ch == '?' && !inSingle {
			b.WriteString(fmt.Sprintf("$%d", argNum))
			argNum++
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func newCategoryID(userID uuid.UUID, slug string) uuid.UUID {
	return uuid.NewSHA1(categoryNamespace, []byte(userID.String()+":"+slug))
}

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

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return v
}

func nullTimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	v := nt.Time.UTC()
	return &v
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

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func parseUUIDStringsJSON(raw string) []uuid.UUID {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

func scanUUID(raw sql.NullString) (*uuid.UUID, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw.String)
	if err != nil {
		return nil, err
	}
	return &id, nil
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

func scanMessageRow(row *sql.Row) (*driven.MessageRow, error) { return scanMessageScanner(row) }

func scanMessageRows(rows *sql.Rows) ([]driven.MessageRow, error) {
	out := make([]driven.MessageRow, 0)
	for rows.Next() {
		m, err := scanMessageScanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func scanMessageScanner(s rowScanner) (*driven.MessageRow, error) {
	var m driven.MessageRow
	var idStr, accStr string
	var conv, preview, body, etag, categorySlug sql.NullString
	var bodyAt, summarySeen, forwardSeen sql.NullTime
	var receivedAt, createdAt, updatedAt time.Time
	var confidence sql.NullFloat64
	var hasAttachments bool
	if err := s.Scan(
		&idStr, &accStr, &m.ProviderMessageID, &conv, &receivedAt, &m.Subject, &m.FromJSON,
		&m.ToJSON, &m.CcJSON, &preview, &body, &bodyAt, &hasAttachments, &etag, &categorySlug, &confidence,
		&createdAt, &updatedAt, &summarySeen, &forwardSeen,
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
	m.BodyFetchedAt = nullTimePtr(bodyAt)
	m.HasAttachments = hasAttachments
	m.RawEtag = nullStringPtr(etag)
	m.CategorySlug = nullStringPtr(categorySlug)
	if confidence.Valid {
		v := confidence.Float64
		m.CategoryConfidence = &v
	}
	m.ReceivedAt = receivedAt.UTC()
	m.CreatedAt = createdAt.UTC()
	m.UpdatedAt = updatedAt.UTC()
	m.SummarySeenAt = nullTimePtr(summarySeen)
	m.ForwardSeenAt = nullTimePtr(forwardSeen)
	return &m, nil
}

func scanJobRunRow(_ rowScanner) (*driven.JobRunRow, error) {
	return nil, fmt.Errorf("job runs are not stored in postgres product persistence")
}

func (r *Repository) InsertAccount(ctx context.Context, a driven.AccountRow, tokenCiphertext []byte) error {
	now := time.Now().UTC()
	_, err := r.execContext(ctx, `
		INSERT INTO accounts (id, user_id, label, provider, ms_account_kind, graph_tenant_id, primary_email, msal_home_account_id,
			connection_status, last_error, token_ciphertext, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID.String(), a.UserID.String(), a.Label, a.Provider, string(a.MsAccountKind), nullStr(a.GraphTenantID), a.PrimaryEmail,
		nullStr(a.MsalHomeAccountID), a.ConnectionStatus, nullStr(a.LastError), tokenCiphertext, now, now)
	if err != nil {
		return err
	}
	_, err = r.execContext(ctx, `
		INSERT INTO account_sync_state (account_id, delta_link, last_synced_at, cursor_json)
		VALUES (?, NULL, NULL, NULL)
		ON CONFLICT (account_id) DO NOTHING
	`, a.ID.String())
	return err
}

func (r *Repository) UpdateAccountTokens(ctx context.Context, userID uuid.UUID, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error {
	_, err := r.execContext(ctx, `
		UPDATE accounts SET token_ciphertext = ?, primary_email = ?, graph_tenant_id = ?, msal_home_account_id = ?,
			connection_status = ?, last_error = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, tokenCiphertext, primaryEmail, nullStr(graphTenantID), nullStr(msalHome), status, nullStr(lastErr), time.Now().UTC(), id.String(), userID.String())
	return err
}

func (r *Repository) GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.AccountRow, []byte, error) {
	var a driven.AccountRow
	var idStr, userStr, kindStr string
	var graphT, msalH, lastE sql.NullString
	var tok []byte
	var createdAt, updatedAt time.Time
	var lastSync sql.NullTime
	err := r.queryRowContext(ctx, `
		SELECT a.user_id, a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.token_ciphertext, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		WHERE a.id = ? AND a.user_id = ?
	`, id.String(), userID.String()).Scan(
		&userStr, &idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
		&a.ConnectionStatus, &lastE, &tok, &createdAt, &updatedAt, &lastSync,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if a.UserID, err = uuid.Parse(userStr); err != nil {
		return nil, nil, err
	}
	if a.ID, err = uuid.Parse(idStr); err != nil {
		return nil, nil, err
	}
	a.MsAccountKind = accounts.MsAccountKind(kindStr)
	a.GraphTenantID = nullStringPtr(graphT)
	a.MsalHomeAccountID = nullStringPtr(msalH)
	a.LastError = nullStringPtr(lastE)
	a.CreatedAt = createdAt.UTC()
	a.UpdatedAt = updatedAt.UTC()
	a.LastSyncedAt = nullTimePtr(lastSync)
	var tokenBytes []byte
	if len(tok) > 0 {
		tokenBytes = append([]byte(nil), tok...)
	}
	return &a, tokenBytes, nil
}

func (r *Repository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]driven.AccountRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT a.user_id, a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		WHERE a.user_id = ?
		ORDER BY a.created_at ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.AccountRow, 0)
	for rows.Next() {
		var a driven.AccountRow
		var idStr, userStr, kindStr string
		var graphT, msalH, lastE sql.NullString
		var createdAt, updatedAt time.Time
		var lastSync sql.NullTime
		if err := rows.Scan(
			&userStr, &idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
			&a.ConnectionStatus, &lastE, &createdAt, &updatedAt, &lastSync,
		); err != nil {
			return nil, err
		}
		var err error
		if a.UserID, err = uuid.Parse(userStr); err != nil {
			return nil, err
		}
		if a.ID, err = uuid.Parse(idStr); err != nil {
			return nil, err
		}
		a.MsAccountKind = accounts.MsAccountKind(kindStr)
		a.GraphTenantID = nullStringPtr(graphT)
		a.MsalHomeAccountID = nullStringPtr(msalH)
		a.LastError = nullStringPtr(lastE)
		a.CreatedAt = createdAt.UTC()
		a.UpdatedAt = updatedAt.UTC()
		a.LastSyncedAt = nullTimePtr(lastSync)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	for {
		rows, err := r.queryContext(ctx, `
			SELECT m.id
			FROM messages m
			INNER JOIN accounts a ON a.id = m.account_id
			WHERE a.id = ? AND a.user_id = ?
			ORDER BY m.received_at DESC, m.id DESC
			LIMIT ?
		`, id.String(), userID.String(), defaultMutationChunkSize)
		if err != nil {
			return err
		}
		messageIDs, err := scanUUIDColumn(rows)
		rows.Close()
		if err != nil {
			return err
		}
		if len(messageIDs) == 0 {
			break
		}
		if err := r.deleteMessagesByID(ctx, id, messageIDs); err != nil {
			return err
		}
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := txExecContext(ctx, tx, `DELETE FROM account_sync_state WHERE account_id IN (SELECT id FROM accounts WHERE id = ? AND user_id = ?)`, id.String(), userID.String()); err != nil {
			return err
		}
		_, err := txExecContext(ctx, tx, `DELETE FROM accounts WHERE id = ? AND user_id = ?`, id.String(), userID.String())
		return err
	})
}

func (r *Repository) deleteMessagesByID(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		var b strings.Builder
		b.WriteString(`DELETE FROM messages WHERE account_id = ? AND id IN (`)
		args := []any{accountID.String()}
		for i, id := range ids {
			if i > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, id.String())
		}
		b.WriteString(`)`)
		_, err := txExecContext(ctx, tx, b.String(), args...)
		return err
	})
}

func (r *Repository) GetSyncDeltaLink(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*string, error) {
	var delta sql.NullString
	err := r.queryRowContext(ctx, `
		SELECT s.delta_link
		FROM account_sync_state s
		INNER JOIN accounts a ON a.id = s.account_id
		WHERE s.account_id = ? AND a.user_id = ?
	`, accountID.String(), userID.String()).Scan(&delta)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil || !delta.Valid || strings.TrimSpace(delta.String) == "" {
		return nil, err
	}
	v := delta.String
	return &v, nil
}

func (r *Repository) UpsertSyncState(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, deltaLink *string, at time.Time) error {
	var link any
	if deltaLink != nil && strings.TrimSpace(*deltaLink) != "" {
		link = strings.TrimSpace(*deltaLink)
	}
	_, err := r.execContext(ctx, `
		UPDATE account_sync_state
		SET delta_link = ?, last_synced_at = ?
		WHERE account_id = ? AND EXISTS (SELECT 1 FROM accounts WHERE id = ? AND user_id = ?)
	`, link, at.UTC(), accountID.String(), accountID.String(), userID.String())
	return err
}

func (r *Repository) UpsertSyncStateTime(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, at time.Time) error {
	_, err := r.execContext(ctx, `
		UPDATE account_sync_state SET last_synced_at = ?
		WHERE account_id = ? AND EXISTS (SELECT 1 FROM accounts WHERE id = ? AND user_id = ?)
	`, at.UTC(), accountID.String(), accountID.String(), userID.String())
	return err
}

func (r *Repository) UpsertMessage(ctx context.Context, m driven.MessageRow) error {
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
	_, err := r.execContext(ctx, `
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
			updated_at = excluded.updated_at
	`, m.ID.String(), m.AccountID.String(), m.ProviderMessageID, nullStr(m.ConversationID), m.ReceivedAt.UTC(), m.Subject,
		fromJSON, toJSON, ccJSON, nullStr(m.ToCCPreview), nullStr(m.BodyText), nullTime(m.BodyFetchedAt),
		m.HasAttachments, nullStr(m.RawEtag), time.Now().UTC(), time.Now().UTC())
	return err
}

func (r *Repository) GetMessageIDByProvider(ctx context.Context, accountID uuid.UUID, providerMessageID string) (uuid.UUID, error) {
	var idStr string
	err := r.queryRowContext(ctx, `SELECT id FROM messages WHERE account_id = ? AND provider_message_id = ?`, accountID.String(), providerMessageID).Scan(&idStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, sql.ErrNoRows
		}
		return uuid.Nil, err
	}
	return uuid.Parse(idStr)
}

func (r *Repository) ListMessagesByAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, limit, offset int) ([]driven.MessageRow, error) {
	return r.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: limit, Offset: offset})
}

func (r *Repository) GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*driven.MessageRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT m.id, m.account_id, m.provider_message_id, m.conversation_id, m.received_at, m.subject, m.from_json,
			m.to_json, m.cc_json, m.to_cc_preview, m.body_text, m.body_fetched_at, m.has_attachments, m.raw_etag,
			cd.slug, mc.confidence, m.created_at, m.updated_at, m.summary_seen_at, m.forward_seen_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_categories mc ON mc.message_id = m.id AND mc.source = 'llm'
		LEFT JOIN category_definitions cd ON cd.id = mc.category_id
		WHERE m.id = ?
	`, userID.String(), id.String())
	m, err := scanMessageRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *Repository) ListMessages(ctx context.Context, userID uuid.UUID, filter driven.MessageListFilter) ([]driven.MessageRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var b strings.Builder
	b.WriteString(`
		SELECT m.id, m.account_id, m.provider_message_id, m.conversation_id, m.received_at, m.subject, m.from_json,
			m.to_json, m.cc_json, m.to_cc_preview, `)
	if filter.OmitBody {
		b.WriteString(`NULL AS body_text`)
	} else {
		b.WriteString(`m.body_text`)
	}
	b.WriteString(`, m.body_fetched_at, m.has_attachments, m.raw_etag,
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
		args = append(args, filter.Since.UTC())
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
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRows(rows)
}

func (r *Repository) MarkMessagesSummarySeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error {
	return r.markMessagesSeen(ctx, "summary_seen_at", userID, messageIDs, at)
}

func (r *Repository) MarkMessagesForwardSeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error {
	return r.markMessagesSeen(ctx, "forward_seen_at", userID, messageIDs, at)
}

func (r *Repository) markMessagesSeen(ctx context.Context, column string, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error {
	if len(messageIDs) == 0 {
		return nil
	}
	for i := 0; i < len(messageIDs); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		chunk := messageIDs[i:end]
		var b strings.Builder
		fmt.Fprintf(&b, `UPDATE messages SET %s = ?, updated_at = ? WHERE account_id IN (SELECT id FROM accounts WHERE user_id = ?) AND id IN (`, column)
		args := []any{at.UTC(), at.UTC(), userID.String()}
		for j, id := range chunk {
			if j > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, id.String())
		}
		b.WriteString(`)`)
		if _, err := r.execContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) UpsertMessageCategory(ctx context.Context, row driven.MessageCategoryRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO message_categories (id, message_id, account_id, category_id, source, confidence, run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id, source) DO UPDATE SET
			category_id = excluded.category_id,
			account_id = excluded.account_id,
			confidence = excluded.confidence,
			run_id = excluded.run_id,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.MessageID.String(), row.AccountID.String(), row.CategoryID.String(), row.Source,
		nullFloat(row.Confidence), row.RunID.String(), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]driven.CategoryDefinitionRow, error) {
	rows, err := r.queryContext(ctx, `
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
		var idStr, userStr string
		var createdAt, updatedAt time.Time
		var row driven.CategoryDefinitionRow
		if err := rows.Scan(&idStr, &userStr, &row.Slug, &row.DisplayName, &row.Definition, &row.SortOrder, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var err error
		if row.ID, err = uuid.Parse(idStr); err != nil {
			return nil, err
		}
		if row.UserID, err = uuid.Parse(userStr); err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt.UTC()
		row.UpdatedAt = updatedAt.UTC()
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
	var idStr, userStr string
	var createdAt, updatedAt time.Time
	err := r.queryRowContext(ctx, query, args...).Scan(&idStr, &userStr, &row.Slug, &row.DisplayName, &row.Definition, &row.SortOrder, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var parseErr error
	if row.ID, parseErr = uuid.Parse(idStr); parseErr != nil {
		return nil, parseErr
	}
	if row.UserID, parseErr = uuid.Parse(userStr); parseErr != nil {
		return nil, parseErr
	}
	row.CreatedAt = createdAt.UTC()
	row.UpdatedAt = updatedAt.UTC()
	return &row, nil
}

func (r *Repository) CreateCategoryDefinition(ctx context.Context, row driven.CategoryDefinitionRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.Slug, row.DisplayName, row.Definition, row.SortOrder, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) UpdateCategoryDefinition(ctx context.Context, row driven.CategoryDefinitionRow) error {
	_, err := r.execContext(ctx, `
		UPDATE category_definitions
		SET slug = ?, display_name = ?, definition = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, row.Slug, row.DisplayName, row.Definition, row.SortOrder, row.UpdatedAt.UTC(), row.ID.String(), row.UserID.String())
	return err
}

func (r *Repository) DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	_, err := r.execContext(ctx, `DELETE FROM category_definitions WHERE id = ? AND user_id = ?`, id.String(), userID.String())
	return err
}

func (r *Repository) ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error) {
	ids := make([]uuid.UUID, 0)
	rows, err := r.queryContext(ctx, `
		SELECT mc.id
		FROM message_categories mc
		INNER JOIN accounts a ON a.id = mc.account_id
		WHERE mc.category_id = ? AND a.user_id = ?
		ORDER BY mc.id ASC
	`, fromCategoryID.String(), userID.String())
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return 0, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	total := 0
	for i := 0; i < len(ids); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		var b strings.Builder
		b.WriteString(`UPDATE message_categories SET category_id = ?, updated_at = ? WHERE id IN (`)
		args := []any{toCategoryID.String(), time.Now().UTC()}
		for j, id := range chunk {
			if j > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, id.String())
		}
		b.WriteString(`)`)
		res, err := r.execContext(ctx, b.String(), args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

func (r *Repository) CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error) {
	var n int
	err := r.queryRowContext(ctx, `
		SELECT COUNT(1)
		FROM message_categories mc
		INNER JOIN accounts a ON a.id = mc.account_id
		WHERE mc.category_id = ? AND a.user_id = ?
	`, categoryID.String(), userID.String()).Scan(&n)
	return n, err
}

func (r *Repository) InsertOAuthState(ctx context.Context, state, flow string, userID *uuid.UUID, payloadJSON string, createdAt time.Time) error {
	_, err := r.execContext(ctx, `
		INSERT INTO oauth_states (state, flow, user_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, state, flow, nullUUID(userID), payloadJSON, createdAt.UTC())
	return err
}

func (r *Repository) TakeOAuthState(ctx context.Context, state string) (flow string, userID *uuid.UUID, payloadJSON string, ok bool, err error) {
	var uid sql.NullString
	var createdAt time.Time
	err = r.queryRowContext(ctx, `
		DELETE FROM oauth_states
		WHERE state = ?
		RETURNING flow, user_id, payload_json, created_at
	`, state).Scan(&flow, &uid, &payloadJSON, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, "", false, nil
	}
	if err != nil {
		return "", nil, "", false, err
	}
	if time.Since(createdAt.UTC()) > r.OAuthStateTTL {
		return "", nil, "", false, nil
	}
	userID, err = scanUUID(uid)
	if err != nil {
		return "", nil, "", false, err
	}
	return flow, userID, payloadJSON, true, nil
}

func (r *Repository) DeleteExpiredStates(ctx context.Context, before time.Time) error {
	_, err := r.execContext(ctx, `DELETE FROM oauth_states WHERE created_at < ?`, before.UTC())
	return err
}

func (r *Repository) GetSummarySettings(ctx context.Context, userID uuid.UUID) (*driven.SummarySettingsRow, error) {
	var includeJSON, excludeJSON string
	var chunkSize int
	var updatedAt time.Time
	err := r.queryRowContext(ctx, `
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
	row := &driven.SummarySettingsRow{UserID: userID, ChunkSize: chunkSize, UpdatedAt: updatedAt.UTC()}
	if err := json.Unmarshal([]byte(includeJSON), &row.IncludeCategorySlugs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(excludeJSON), &row.ExcludeCategorySlugs); err != nil {
		return nil, err
	}
	return row, nil
}

func (r *Repository) UpsertSummarySettings(ctx context.Context, row driven.SummarySettingsRow) error {
	includeJSON, _ := json.Marshal(row.IncludeCategorySlugs)
	excludeJSON, _ := json.Marshal(row.ExcludeCategorySlugs)
	_, err := r.execContext(ctx, `
		INSERT INTO summary_settings (user_id, include_category_slugs, exclude_category_slugs, chunk_size, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			include_category_slugs = excluded.include_category_slugs,
			exclude_category_slugs = excluded.exclude_category_slugs,
			chunk_size = excluded.chunk_size,
			updated_at = excluded.updated_at
	`, row.UserID.String(), string(includeJSON), string(excludeJSON), row.ChunkSize, row.UpdatedAt.UTC())
	return err
}

func (r *Repository) InsertSummarySnapshot(ctx context.Context, row driven.SummarySnapshotRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO summary_snapshots (id, user_id, account_id, run_id, window_start, window_end, general_summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			general_summary = excluded.general_summary,
			created_at = excluded.created_at
	`, row.ID.String(), row.UserID.String(), nullUUID(row.AccountID), row.RunID.String(), row.WindowStart.UTC(), row.WindowEnd.UTC(), row.GeneralSummary, row.CreatedAt.UTC())
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
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.SummarySnapshotRow, 0)
	for rows.Next() {
		var idStr, userStr, runStr string
		var account sql.NullString
		var windowStart, windowEnd, createdAt time.Time
		var item driven.SummarySnapshotRow
		if err := rows.Scan(&idStr, &userStr, &account, &runStr, &windowStart, &windowEnd, &item.GeneralSummary, &createdAt); err != nil {
			return nil, err
		}
		var err error
		if item.ID, err = uuid.Parse(idStr); err != nil {
			return nil, err
		}
		if item.UserID, err = uuid.Parse(userStr); err != nil {
			return nil, err
		}
		if item.RunID, err = uuid.Parse(runStr); err != nil {
			return nil, err
		}
		item.WindowStart = windowStart.UTC()
		item.WindowEnd = windowEnd.UTC()
		item.CreatedAt = createdAt.UTC()
		if item.AccountID, err = scanUUID(account); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) InsertActionItems(ctx context.Context, rows []driven.ActionItemRow) error {
	return r.insertActionItems(ctx, rows)
}

func (r *Repository) insertActionItems(ctx context.Context, rows []driven.ActionItemRow) error {
	for i := 0; i < len(rows); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]
		if err := r.withTx(ctx, func(tx *sql.Tx) error {
			for _, row := range chunk {
				if _, err := txExecContext(ctx, tx, `
					INSERT INTO action_items (id, user_id, account_id, message_id, run_id, text, due_at, status, actioned_at, auto_draft_seen_at, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT (id) DO UPDATE SET
						text = excluded.text,
						due_at = excluded.due_at,
						status = excluded.status,
						actioned_at = excluded.actioned_at,
						auto_draft_seen_at = excluded.auto_draft_seen_at,
						updated_at = excluded.updated_at
				`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RunID.String(), row.Text,
					nullTime(row.DueAt), row.Status, nullTime(row.ActionedAt), nullTime(row.AutoDraftSeenAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC()); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
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
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActionItems(rows)
}

func scanActionItems(rows *sql.Rows) ([]driven.ActionItemRow, error) {
	out := make([]driven.ActionItemRow, 0)
	for rows.Next() {
		var idStr, userStr, accountStr, messageStr, runStr, text, status string
		var dueAt, actionedAt, autoDraftSeenAt sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&idStr, &userStr, &accountStr, &messageStr, &runStr, &text, &dueAt, &status, &actionedAt, &autoDraftSeenAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		userID, _ := uuid.Parse(userStr)
		accountID, _ := uuid.Parse(accountStr)
		messageID, _ := uuid.Parse(messageStr)
		runID, _ := uuid.Parse(runStr)
		out = append(out, driven.ActionItemRow{
			ID:              id,
			UserID:          userID,
			AccountID:       accountID,
			MessageID:       messageID,
			RunID:           runID,
			Text:            text,
			DueAt:           nullTimePtr(dueAt),
			Status:          status,
			ActionedAt:      nullTimePtr(actionedAt),
			AutoDraftSeenAt: nullTimePtr(autoDraftSeenAt),
			CreatedAt:       createdAt.UTC(),
			UpdatedAt:       updatedAt.UTC(),
		})
	}
	return out, rows.Err()
}

func (r *Repository) MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error {
	_, err := r.execContext(ctx, `
		UPDATE action_items SET status = 'done', actioned_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, at.UTC(), at.UTC(), itemID.String(), userID.String())
	return err
}

func (r *Repository) InsertFYI(ctx context.Context, rows []driven.FYIRow) error {
	for i := 0; i < len(rows); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]
		if err := r.withTx(ctx, func(tx *sql.Tx) error {
			for _, row := range chunk {
				if _, err := txExecContext(ctx, tx, `
					INSERT INTO fyi_items (id, user_id, account_id, message_id, run_id, text, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT (id) DO UPDATE SET
						text = excluded.text,
						created_at = excluded.created_at
				`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RunID.String(), row.Text, row.CreatedAt.UTC()); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListFYIByRun(ctx context.Context, userID uuid.UUID, runID uuid.UUID) ([]driven.FYIRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, user_id, account_id, message_id, run_id, text, created_at
		FROM fyi_items WHERE user_id = ? AND run_id = ? ORDER BY created_at DESC
	`, userID.String(), runID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFYIRows(rows)
}

func (r *Repository) ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]driven.FYIRow, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	var b strings.Builder
	b.WriteString(`SELECT id, user_id, account_id, message_id, run_id, text, created_at FROM fyi_items WHERE user_id = ?`)
	args := []any{userID.String()}
	if accountID != nil {
		b.WriteString(` AND account_id = ?`)
		args = append(args, accountID.String())
	}
	b.WriteString(` ORDER BY created_at DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFYIRows(rows)
}

func scanFYIRows(rows *sql.Rows) ([]driven.FYIRow, error) {
	out := make([]driven.FYIRow, 0)
	for rows.Next() {
		var idStr, userStr, accountStr, messageStr, runStr, text string
		var createdAt time.Time
		if err := rows.Scan(&idStr, &userStr, &accountStr, &messageStr, &runStr, &text, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		userID, _ := uuid.Parse(userStr)
		accountID, _ := uuid.Parse(accountStr)
		messageID, _ := uuid.Parse(messageStr)
		runID, _ := uuid.Parse(runStr)
		out = append(out, driven.FYIRow{
			ID:        id,
			UserID:    userID,
			AccountID: accountID,
			MessageID: messageID,
			RunID:     runID,
			Text:      text,
			CreatedAt: createdAt.UTC(),
		})
	}
	return out, rows.Err()
}

func (r *Repository) DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	_, err := r.execContext(ctx, `DELETE FROM fyi_items WHERE id = ? AND user_id = ?`, id.String(), userID.String())
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
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActionItems(rows)
}

func (r *Repository) UpsertSummaryJobChunk(ctx context.Context, row driven.SummaryJobChunkRow) error {
	messageIDsJSON, _ := json.Marshal(uuidStrings(row.MessageIDs))
	_, err := r.execContext(ctx, `
		INSERT INTO summary_job_chunks (id, run_id, account_id, chunk_index, phase, message_ids_json, payload_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			message_ids_json = excluded.message_ids_json,
			payload_json = excluded.payload_json,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.RunID.String(), row.AccountID.String(), row.ChunkIndex, row.Phase, string(messageIDsJSON), row.PayloadJSON, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) ListSummaryJobChunks(ctx context.Context, runID uuid.UUID) ([]driven.SummaryJobChunkRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, run_id, account_id, chunk_index, phase, message_ids_json, payload_json, created_at, updated_at
		FROM summary_job_chunks
		WHERE run_id = ?
		ORDER BY chunk_index ASC, created_at ASC
	`, runID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.SummaryJobChunkRow, 0)
	for rows.Next() {
		var idStr, runIDStr, accountIDStr, messageIDsJSON string
		var createdAt, updatedAt time.Time
		var item driven.SummaryJobChunkRow
		if err := rows.Scan(&idStr, &runIDStr, &accountIDStr, &item.ChunkIndex, &item.Phase, &messageIDsJSON, &item.PayloadJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.ID, _ = uuid.Parse(idStr)
		item.RunID, _ = uuid.Parse(runIDStr)
		item.AccountID, _ = uuid.Parse(accountIDStr)
		item.MessageIDs = parseUUIDStringsJSON(messageIDsJSON)
		item.CreatedAt = createdAt.UTC()
		item.UpdatedAt = updatedAt.UTC()
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkActionItemsAutoDraftSeen(ctx context.Context, userID uuid.UUID, itemIDs []uuid.UUID, at time.Time) error {
	if len(itemIDs) == 0 {
		return nil
	}
	for i := 0; i < len(itemIDs); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		chunk := itemIDs[i:end]
		if err := r.withTx(ctx, func(tx *sql.Tx) error {
			for _, id := range chunk {
				if _, err := txExecContext(ctx, tx, `
					UPDATE action_items
					SET auto_draft_seen_at = ?, updated_at = ?
					WHERE id = ? AND user_id = ?
				`, at.UTC(), at.UTC(), id.String(), userID.String()); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) InsertDraftSuggestions(ctx context.Context, rows []driven.DraftSuggestionRow) error {
	if len(rows) == 0 {
		return nil
	}
	for i := 0; i < len(rows); i += defaultMutationChunkSize {
		end := i + defaultMutationChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]
		if err := r.withTx(ctx, func(tx *sql.Tx) error {
			for _, row := range chunk {
				if _, err := txExecContext(ctx, tx, `
					INSERT INTO draft_suggestions (id, user_id, account_id, message_id, action_item_id, run_id, subject, body, model, status, sent_at, discarded_at, updated_at, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT (id) DO UPDATE SET
						action_item_id = excluded.action_item_id,
						subject = excluded.subject,
						body = excluded.body,
						model = excluded.model,
						status = excluded.status,
						sent_at = excluded.sent_at,
						discarded_at = excluded.discarded_at,
						updated_at = excluded.updated_at
				`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.ActionItemID.String(),
					row.RunID.String(), row.Subject, row.Body, row.Model, "ready", nullTime(row.SentAt), nullTime(row.DiscardedAt),
					nullTime(row.UpdatedAt), row.CreatedAt.UTC()); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
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
	rows, err := r.queryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDraftSuggestions(rows)
}

func (r *Repository) GetDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID) (*driven.DraftSuggestionRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT ds.id, ds.user_id, ds.account_id, ds.message_id, ds.action_item_id, ds.run_id, ds.subject, ds.body, ds.model, m.from_json, ds.status, ds.sent_at, ds.discarded_at, ds.updated_at, ds.created_at
		FROM draft_suggestions ds
		INNER JOIN messages m ON m.id = ds.message_id
		WHERE ds.id = ? AND ds.user_id = ?
	`, draftID.String(), userID.String())
	item, err := scanDraftSuggestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func scanDraftSuggestions(rows *sql.Rows) ([]driven.DraftSuggestionRow, error) {
	out := make([]driven.DraftSuggestionRow, 0)
	for rows.Next() {
		item, err := scanDraftSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanDraftSuggestion(s rowScanner) (*driven.DraftSuggestionRow, error) {
	var idStr, userStr, accountStr, messageStr, actionStr, runStr, subject, body, model, fromJSON, status string
	var sentAt, discardedAt, updatedAt sql.NullTime
	var createdAt time.Time
	if err := s.Scan(&idStr, &userStr, &accountStr, &messageStr, &actionStr, &runStr, &subject, &body, &model, &fromJSON, &status, &sentAt, &discardedAt, &updatedAt, &createdAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	userID, _ := uuid.Parse(userStr)
	accountID, _ := uuid.Parse(accountStr)
	messageID, _ := uuid.Parse(messageStr)
	actionID, _ := uuid.Parse(actionStr)
	runID, _ := uuid.Parse(runStr)
	return &driven.DraftSuggestionRow{
		ID:           id,
		UserID:       userID,
		AccountID:    accountID,
		MessageID:    messageID,
		ActionItemID: actionID,
		RunID:        runID,
		Subject:      subject,
		Body:         body,
		Model:        model,
		FromJSON:     fromJSON,
		Status:       status,
		SentAt:       nullTimePtr(sentAt),
		DiscardedAt:  nullTimePtr(discardedAt),
		UpdatedAt:    nullTimePtr(updatedAt),
		CreatedAt:    createdAt.UTC(),
	}, nil
}

func (r *Repository) UpdateDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, subject, body string, at time.Time) error {
	_, err := r.execContext(ctx, `
		UPDATE draft_suggestions
		SET subject = ?, body = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND status = 'ready'
	`, subject, body, at.UTC(), draftID.String(), userID.String())
	return err
}

func (r *Repository) MarkDraftSuggestionStatus(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, status string, at time.Time) error {
	var sentAt any
	var discardedAt any
	if status == "sent" {
		sentAt = at.UTC()
	}
	if status == "discarded" {
		discardedAt = at.UTC()
	}
	_, err := r.execContext(ctx, `
		UPDATE draft_suggestions
		SET status = ?, sent_at = COALESCE(?, sent_at), discarded_at = COALESCE(?, discarded_at), updated_at = ?
		WHERE id = ? AND user_id = ?
	`, status, sentAt, discardedAt, at.UTC(), draftID.String(), userID.String())
	return err
}

func (r *Repository) InsertSendAttempt(ctx context.Context, row driven.SendAttemptRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO send_attempts (id, user_id, account_id, draft_id, message_id, status, provider_message_id, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.DraftID.String(), row.MessageID.String(),
		row.Status, nullStr(row.ProviderMessageID), nullStr(row.ErrorMessage), row.CreatedAt.UTC())
	return err
}

func (r *Repository) ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]driven.SendAttemptRow, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.queryContext(ctx, `
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
		var idStr, userStr, accountStr, draftStr, messageStr, status string
		var providerMessageID, errorMessage sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&idStr, &userStr, &accountStr, &draftStr, &messageStr, &status, &providerMessageID, &errorMessage, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		userID, _ := uuid.Parse(userStr)
		accountID, _ := uuid.Parse(accountStr)
		draftID, _ := uuid.Parse(draftStr)
		messageID, _ := uuid.Parse(messageStr)
		out = append(out, driven.SendAttemptRow{
			ID:                id,
			UserID:            userID,
			AccountID:         accountID,
			DraftID:           draftID,
			MessageID:         messageID,
			Status:            status,
			ProviderMessageID: nullStringPtr(providerMessageID),
			ErrorMessage:      nullStringPtr(errorMessage),
			CreatedAt:         createdAt.UTC(),
		})
	}
	return out, rows.Err()
}

func (r *Repository) ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]driven.ForwardAllowlistRow, error) {
	rows, err := r.queryContext(ctx, `
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
		var idStr, userStr, email string
		var createdAt time.Time
		if err := rows.Scan(&idStr, &userStr, &email, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		uid, _ := uuid.Parse(userStr)
		out = append(out, driven.ForwardAllowlistRow{ID: id, UserID: uid, Email: email, CreatedAt: createdAt.UTC()})
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := txExecContext(ctx, tx, `DELETE FROM forward_allowlist WHERE user_id = ?`, userID.String()); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, email := range emails {
			trimmed := strings.ToLower(strings.TrimSpace(email))
			if trimmed == "" {
				continue
			}
			if _, err := txExecContext(ctx, tx, `
				INSERT INTO forward_allowlist (id, user_id, email, created_at)
				VALUES (?, ?, ?, ?)
			`, uuid.New().String(), userID.String(), trimmed, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]driven.ForwardRuleRow, error) {
	rows, err := r.queryContext(ctx, `
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
		var idStr, userStr, accountStr, name, mode, conditionJSON, forwardTo string
		var enabled bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&idStr, &userStr, &accountStr, &name, &mode, &conditionJSON, &forwardTo, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		userID, _ := uuid.Parse(userStr)
		accountID, _ := uuid.Parse(accountStr)
		out = append(out, driven.ForwardRuleRow{
			ID: id, UserID: userID, AccountID: accountID, Name: name, Mode: mode, ConditionJSON: conditionJSON,
			ForwardTo: forwardTo, Enabled: enabled, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC(),
		})
	}
	return out, rows.Err()
}

func (r *Repository) CreateForwardRule(ctx context.Context, row driven.ForwardRuleRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO forward_rules (id, user_id, account_id, name, mode, condition_json, forward_to, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.Name, row.Mode, row.ConditionJSON, row.ForwardTo, row.Enabled, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) UpdateForwardRule(ctx context.Context, row driven.ForwardRuleRow) error {
	_, err := r.execContext(ctx, `
		UPDATE forward_rules
		SET name = ?, mode = ?, condition_json = ?, forward_to = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, row.Name, row.Mode, row.ConditionJSON, row.ForwardTo, row.Enabled, row.UpdatedAt.UTC(), row.ID.String(), row.UserID.String())
	return err
}

func (r *Repository) DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error {
	_, err := r.execContext(ctx, `DELETE FROM forward_rules WHERE id = ? AND user_id = ?`, ruleID.String(), userID.String())
	return err
}

func (r *Repository) ListForwardAuditByRun(ctx context.Context, userID, runID uuid.UUID) ([]driven.ForwardAuditRow, error) {
	rows, err := r.queryContext(ctx, `
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
		var idStr, userStr, accountStr, messageStr, ruleStr, runStr, status string
		var reason sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&idStr, &userStr, &accountStr, &messageStr, &ruleStr, &runStr, &status, &reason, &createdAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		userID, _ := uuid.Parse(userStr)
		accountID, _ := uuid.Parse(accountStr)
		messageID, _ := uuid.Parse(messageStr)
		ruleID, _ := uuid.Parse(ruleStr)
		runID, _ := uuid.Parse(runStr)
		out = append(out, driven.ForwardAuditRow{
			ID: id, UserID: userID, AccountID: accountID, MessageID: messageID, RuleID: ruleID, RunID: runID,
			Status: status, Reason: nullStringPtr(reason), CreatedAt: createdAt.UTC(),
		})
	}
	return out, rows.Err()
}

func (r *Repository) InsertForwardAudit(ctx context.Context, row driven.ForwardAuditRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO forward_audit (id, user_id, account_id, message_id, rule_id, run_id, status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (message_id, rule_id) DO UPDATE SET
			run_id = excluded.run_id,
			status = excluded.status,
			reason = excluded.reason,
			created_at = excluded.created_at
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.RuleID.String(), row.RunID.String(), row.Status, nullStr(row.Reason), row.CreatedAt.UTC())
	return err
}

func (r *Repository) InsertManualForwardAudit(ctx context.Context, row driven.ManualForwardAuditRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO manual_forward_audit (id, user_id, account_id, message_id, to_email, comment, status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.AccountID.String(), row.MessageID.String(), row.ToEmail, nullStr(row.Comment), row.Status, nullStr(row.Reason), row.CreatedAt.UTC())
	return err
}

func (r *Repository) ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]driven.ScheduleChainRow, error) {
	rows, err := r.queryContext(ctx, `
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
	return r.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := txExecContext(ctx, tx, `DELETE FROM schedule_chains WHERE user_id = ?`, userID.String()); err != nil {
			return err
		}
		for _, row := range rows {
			jobsJSON, _ := json.Marshal(row.Jobs)
			if _, err := txExecContext(ctx, tx, `
				INSERT INTO schedule_chains (id, user_id, name, account_id, jobs_json, interval_minutes, enabled, last_run_at, next_run_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, row.ID.String(), userID.String(), row.Name, nullUUID(row.AccountID), string(jobsJSON), row.IntervalMinutes, row.Enabled,
				nullTime(row.LastRunAt), row.NextRunAt.UTC(), row.CreatedAt.UTC(), row.UpdatedAt.UTC()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]driven.ScheduleChainRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.queryContext(ctx, `
		SELECT id, user_id, name, account_id, jobs_json, interval_minutes, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM schedule_chains
		WHERE enabled = TRUE AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?
	`, now.UTC(), limit)
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
	_, err := r.execContext(ctx, `
		UPDATE schedule_chains
		SET last_run_at = ?, next_run_at = ?, updated_at = ?
		WHERE id = ?
	`, lastRunAt.UTC(), nextRunAt.UTC(), time.Now().UTC(), id.String())
	return err
}

func (r *Repository) MarkScheduleExecutedIfDue(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) (bool, error) {
	res, err := r.execContext(ctx, `
		UPDATE schedule_chains
		SET last_run_at = ?, next_run_at = ?, updated_at = ?
		WHERE id = ? AND next_run_at = ?
	`, lastRunAt.UTC(), nextRunAt.UTC(), time.Now().UTC(), id.String(), scheduledFor.UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func scanScheduleChainRow(s rowScanner) (*driven.ScheduleChainRow, error) {
	var item driven.ScheduleChainRow
	var idStr, userStr, jobsJSON string
	var accountID sql.NullString
	var lastRunAt sql.NullTime
	var nextRunAt, createdAt, updatedAt time.Time
	var enabled bool
	if err := s.Scan(&idStr, &userStr, &item.Name, &accountID, &jobsJSON, &item.IntervalMinutes, &enabled, &lastRunAt, &nextRunAt, &createdAt, &updatedAt); err != nil {
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
	item.Enabled = enabled
	if item.AccountID, err = scanUUID(accountID); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(jobsJSON), &item.Jobs); err != nil {
		return nil, err
	}
	item.LastRunAt = nullTimePtr(lastRunAt)
	item.NextRunAt = nextRunAt.UTC()
	item.CreatedAt = createdAt.UTC()
	item.UpdatedAt = updatedAt.UTC()
	return &item, nil
}

func (r *Repository) CreateUser(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time) error {
	_, err := r.execContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id.String(), normalizeEmail(email), nullStr(passwordHash), now.UTC(), now.UTC())
	return err
}

func (r *Repository) CreateUserWithHomeOrg(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time, identityProvider, identitySubject, identityEmail string) (uuid.UUID, error) {
	return withSerializableRetry(ctx, func(ctx context.Context) (uuid.UUID, error) {
		tx, err := r.beginTx(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		defer func() { _ = tx.Rollback() }()
		orgID := uuid.New()
		ts := now.UTC()
		if _, err := txExecContext(ctx, tx, `
			INSERT INTO organisations (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`, orgID.String(), organisations.DefaultName, ts, ts); err != nil {
			return uuid.Nil, err
		}
		if _, err := txExecContext(ctx, tx, `
			INSERT INTO users (id, email, password_hash, home_organisation_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id.String(), normalizeEmail(email), nullStr(passwordHash), orgID.String(), ts, ts); err != nil {
			return uuid.Nil, err
		}
		if _, err := txExecContext(ctx, tx, `
			INSERT INTO organisation_members (id, organisation_id, user_id, org_role, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, uuid.New().String(), orgID.String(), id.String(), "owner", ts); err != nil {
			return uuid.Nil, err
		}
		for _, def := range defaultCategories {
			if _, err := txExecContext(ctx, tx, `
				INSERT INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (user_id, slug) DO NOTHING
			`, newCategoryID(id, def.Slug).String(), id.String(), def.Slug, def.DisplayName, def.Definition, def.SortOrder, ts, ts); err != nil {
				return uuid.Nil, err
			}
		}
		if strings.TrimSpace(identityProvider) != "" {
			identEmail := normalizeEmail(identityEmail)
			if identEmail == "" {
				identEmail = normalizeEmail(email)
			}
			if _, err := txExecContext(ctx, tx, `
				INSERT INTO user_identities (id, user_id, provider, provider_subject, email_at_link, created_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, uuid.New().String(), id.String(), identityProvider, identitySubject, identEmail, ts); err != nil {
				return uuid.Nil, err
			}
		}
		if err := tx.Commit(); err != nil {
			return uuid.Nil, err
		}
		return orgID, nil
	})
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (uuid.UUID, *string, error) {
	var idStr string
	var hash sql.NullString
	err := r.queryRowContext(ctx, `SELECT id, password_hash FROM users WHERE email = ?`, normalizeEmail(email)).Scan(&idStr, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil, sql.ErrNoRows
		}
		return uuid.Nil, nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if !hash.Valid {
		return id, nil, nil
	}
	s := hash.String
	return id, &s, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (string, error) {
	var email string
	err := r.queryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, id.String()).Scan(&email)
	return email, err
}

func (r *Repository) GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var orgID sql.NullString
	err := r.queryRowContext(ctx, `SELECT home_organisation_id FROM users WHERE id = ?`, userID.String()).Scan(&orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, sql.ErrNoRows
		}
		return uuid.Nil, err
	}
	if !orgID.Valid || orgID.String == "" {
		return uuid.Nil, sql.ErrNoRows
	}
	return uuid.Parse(orgID.String)
}

func (r *Repository) FindIdentity(ctx context.Context, provider, providerSubject string) (uuid.UUID, bool, error) {
	var userID string
	err := r.queryRowContext(ctx, `
		SELECT user_id FROM user_identities WHERE provider = ? AND provider_subject = ?
	`, provider, providerSubject).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	id, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func (r *Repository) AttachIdentity(ctx context.Context, id uuid.UUID, userID uuid.UUID, provider, providerSubject, emailAtLink string, now time.Time) error {
	_, err := r.execContext(ctx, `
		INSERT INTO user_identities (id, user_id, provider, provider_subject, email_at_link, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, provider_subject) DO NOTHING
	`, id.String(), userID.String(), provider, providerSubject, normalizeEmail(emailAtLink), now.UTC())
	return err
}

func (r *Repository) GetOrganisation(ctx context.Context, id uuid.UUID) (*driven.OrganisationRow, error) {
	var row driven.OrganisationRow
	var createdAt, updatedAt time.Time
	err := r.queryRowContext(ctx, `SELECT name, created_at, updated_at FROM organisations WHERE id = ?`, id.String()).Scan(&row.Name, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.ID = id
	row.CreatedAt = createdAt.UTC()
	row.UpdatedAt = updatedAt.UTC()
	return &row, nil
}

func (r *Repository) InsertAuthSession(ctx context.Context, sessionID, userID uuid.UUID, tokenHash string, createdAt, expiresAt time.Time) error {
	_, err := r.execContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID.String(), userID.String(), tokenHash, createdAt.UTC(), expiresAt.UTC())
	return err
}

func (r *Repository) ConsumeAuthSession(ctx context.Context, tokenHash string) (uuid.UUID, bool, error) {
	var userStr string
	var expiresAt time.Time
	err := r.queryRowContext(ctx, `
		DELETE FROM auth_sessions
		WHERE token_hash = ?
		RETURNING user_id, expires_at
	`, tokenHash).Scan(&userStr, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	if !time.Now().UTC().Before(expiresAt.UTC()) {
		return uuid.Nil, false, nil
	}
	userID, err := uuid.Parse(userStr)
	if err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}
