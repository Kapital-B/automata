package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// Repository implements account, message, and oauth state persistence.
type Repository struct {
	db              *sql.DB
	OAuthStateTTL   time.Duration
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
		INSERT INTO accounts (id, label, provider, ms_account_kind, graph_tenant_id, primary_email, msal_home_account_id,
			connection_status, last_error, token_ciphertext, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID.String(), a.Label, a.Provider, string(a.MsAccountKind), nullStr(a.GraphTenantID), a.PrimaryEmail,
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

func (r *Repository) UpdateAccountTokens(ctx context.Context, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error {
	now := formatRFC3339(time.Now().UTC())
	_, err := r.db.ExecContext(ctx, `
		UPDATE accounts SET token_ciphertext = ?, primary_email = ?, graph_tenant_id = ?, msal_home_account_id = ?,
			connection_status = ?, last_error = ?, updated_at = ?
		WHERE id = ?`,
		tokenCiphertext, primaryEmail, nullStr(graphTenantID), nullStr(msalHome), status, nullStr(lastErr), now, id.String(),
	)
	return err
}

func (r *Repository) GetAccount(ctx context.Context, id uuid.UUID) (*driven.AccountRow, []byte, error) {
	var a driven.AccountRow
	var idStr string
	var kindStr string
	var graphT, msalH, lastE sql.NullString
	var tok []byte
	var createdAt, updatedAt string
	var lastSync sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.token_ciphertext, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		WHERE a.id = ?`, id.String(),
	).Scan(
		&idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
		&a.ConnectionStatus, &lastE, &tok, &createdAt, &updatedAt, &lastSync,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
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

func (r *Repository) ListAccounts(ctx context.Context) ([]driven.AccountRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.label, a.provider, a.ms_account_kind, a.graph_tenant_id, a.primary_email, a.msal_home_account_id,
			a.connection_status, a.last_error, a.created_at, a.updated_at, s.last_synced_at
		FROM accounts a
		LEFT JOIN account_sync_state s ON s.account_id = a.id
		ORDER BY a.created_at ASC`)
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
		if err := rows.Scan(
			&idStr, &a.Label, &a.Provider, &kindStr, &graphT, &a.PrimaryEmail, &msalH,
			&a.ConnectionStatus, &lastE, &createdAt, &updatedAt, &lastSync,
		); err != nil {
			return nil, err
		}
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

func (r *Repository) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE account_id = ?`, id.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_sync_state WHERE account_id = ?`, id.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id.String()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpsertSyncStateTime(ctx context.Context, accountID uuid.UUID, at time.Time) error {
	now := formatRFC3339(at.UTC())
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_sync_state SET last_synced_at = ? WHERE account_id = ?`,
		now, accountID.String(),
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

func (r *Repository) ListMessagesByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]driven.MessageRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, provider_message_id, conversation_id, received_at, subject, from_json,
			to_cc_preview, body_text, body_fetched_at, has_attachments, raw_etag, created_at, updated_at
		FROM messages WHERE account_id = ? ORDER BY received_at DESC LIMIT ? OFFSET ?`,
		accountID.String(), limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageRows(rows)
}

func (r *Repository) GetMessage(ctx context.Context, id uuid.UUID) (*driven.MessageRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, provider_message_id, conversation_id, received_at, subject, from_json,
			to_cc_preview, body_text, body_fetched_at, has_attachments, raw_etag, created_at, updated_at
		FROM messages WHERE id = ?`, id.String(),
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
	var hasAtt int
	if err := s.Scan(
		&idStr, &accStr, &m.ProviderMessageID, &conv, &receivedAt, &m.Subject, &m.FromJSON,
		&preview, &body, &bodyAt, &hasAtt, &etag, &createdAt, &updatedAt,
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

func (r *Repository) InsertState(ctx context.Context, state string, kind accounts.MsAccountKind, labelHint *string, createdAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oauth_states (state, ms_account_kind, label_hint, created_at) VALUES (?, ?, ?, ?)`,
		state, string(kind), nullStr(labelHint), formatRFC3339(createdAt.UTC()),
	)
	return err
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
			finished_at = excluded.finished_at,
			error_message = excluded.error_message,
			meta_json = excluded.meta_json`,
		id.String(), accountID.String(), jobType, trigger, status,
		formatRFC3339(startedAt.UTC()), fin, nullStr(errMsg), metaJSON,
	)
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
