package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) InsertConnectorAccount(ctx context.Context, row driven.ConnectorAccountRow, tokenCiphertext []byte) error {
	scopes, _ := json.Marshal(row.Scopes)
	if row.Scopes == nil {
		scopes = []byte("[]")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_accounts (
			id, user_id, provider, label, external_tenant_id, connection_status,
			last_error, scopes_json, token_ciphertext, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.Provider, row.Label, nullStr(row.ExternalTenantID),
		row.ConnectionStatus, nullStr(row.LastError), string(scopes), tokenCiphertext,
		nullTimeStr(row.LastSyncedAt), formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) GetConnectorAccount(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorAccountRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, label, external_tenant_id, connection_status,
			last_error, scopes_json, last_synced_at, created_at, updated_at
		FROM connector_accounts WHERE id = ? AND user_id = ?
	`, id.String(), userID.String())
	out, err := scanConnectorAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListConnectorAccounts(ctx context.Context, userID uuid.UUID) ([]driven.ConnectorAccountRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, provider, label, external_tenant_id, connection_status,
			last_error, scopes_json, last_synced_at, created_at, updated_at
		FROM connector_accounts WHERE user_id = ? ORDER BY created_at ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorAccountRow, 0)
	for rows.Next() {
		row, err := scanConnectorAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteConnectorAccount(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM connector_accounts WHERE id = ? AND user_id = ?`, id.String(), userID.String())
	return err
}

func (r *Repository) GetConnectorTokenCipher(ctx context.Context, userID, id uuid.UUID) ([]byte, error) {
	var cipher []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT token_ciphertext FROM connector_accounts WHERE id = ? AND user_id = ?
	`, id.String(), userID.String()).Scan(&cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), cipher...), nil
}

func (r *Repository) UpdateConnectorToken(
	ctx context.Context,
	userID, id uuid.UUID,
	tokenCiphertext []byte,
	scopes []string,
	status string,
	lastError *string,
	lastSyncedAt *time.Time,
) error {
	scopesJSON, _ := json.Marshal(scopes)
	if scopes == nil {
		scopesJSON = []byte("[]")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE connector_accounts
		SET token_ciphertext = ?, scopes_json = ?, connection_status = ?, last_error = ?,
			last_synced_at = COALESCE(?, last_synced_at), updated_at = ?
		WHERE id = ? AND user_id = ?
	`, tokenCiphertext, string(scopesJSON), status, nullStr(lastError), nullTimeStr(lastSyncedAt),
		formatRFC3339(time.Now().UTC()), id.String(), userID.String())
	return err
}

func (r *Repository) CreateConnectorBinding(ctx context.Context, row driven.ConnectorBindingRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_bindings (
			id, connector_account_id, organisation_id, external_channel_id,
			project_id, label, sync_cursor, created_at, updated_at
		) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM connector_accounts WHERE id = ?
		)
	`, row.ID.String(), row.ConnectorAccountID.String(), row.OrganisationID.String(), row.ExternalChannelID,
		nullUUID(row.ProjectID), row.Label, nullStr(row.SyncCursor),
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()), row.ConnectorAccountID.String())
	return err
}

func (r *Repository) GetConnectorBinding(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorBindingRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT b.id, b.connector_account_id, b.organisation_id, b.external_channel_id,
			b.project_id, b.label, b.sync_cursor, b.created_at, b.updated_at
		FROM connector_bindings b
		INNER JOIN connector_accounts a ON a.id = b.connector_account_id AND a.user_id = ?
		WHERE b.id = ?
	`, userID.String(), id.String())
	out, err := scanConnectorBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListConnectorBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]driven.ConnectorBindingRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.connector_account_id, b.organisation_id, b.external_channel_id,
			b.project_id, b.label, b.sync_cursor, b.created_at, b.updated_at
		FROM connector_bindings b
		INNER JOIN connector_accounts a ON a.id = b.connector_account_id AND a.user_id = ?
		WHERE b.connector_account_id = ?
		ORDER BY b.created_at ASC
	`, userID.String(), connectorAccountID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorBindingRow, 0)
	for rows.Next() {
		row, err := scanConnectorBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteConnectorBinding(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM connector_bindings
		WHERE id = ? AND connector_account_id IN (
			SELECT id FROM connector_accounts WHERE user_id = ?
		)
	`, id.String(), userID.String())
	return err
}

func (r *Repository) UpdateConnectorBindingCursor(ctx context.Context, userID, id uuid.UUID, cursor *string, updatedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE connector_bindings
		SET sync_cursor = ?, updated_at = ?
		WHERE id = ? AND connector_account_id IN (
			SELECT id FROM connector_accounts WHERE user_id = ?
		)
	`, nullStr(cursor), formatRFC3339(updatedAt.UTC()), id.String(), userID.String())
	return err
}

func (r *Repository) UpsertConnectorMessage(ctx context.Context, row driven.ConnectorMessageRow) error {
	meta := row.MetaJSON
	if meta == "" {
		meta = "{}"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_messages (
			id, connector_account_id, organisation_id, project_id, provider_event_id,
			external_channel_id, title, body_text, author_label, occurred_at,
			meta_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(connector_account_id, provider_event_id) DO UPDATE SET
			organisation_id = excluded.organisation_id,
			project_id = excluded.project_id,
			external_channel_id = excluded.external_channel_id,
			title = excluded.title,
			body_text = excluded.body_text,
			author_label = excluded.author_label,
			occurred_at = excluded.occurred_at,
			meta_json = excluded.meta_json,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.ConnectorAccountID.String(), row.OrganisationID.String(), nullUUID(row.ProjectID),
		row.ProviderEventID, row.ExternalChannelID, row.Title, row.BodyText, row.AuthorLabel,
		formatRFC3339(row.OccurredAt.UTC()), meta, formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) ListConnectorMessagesForProject(ctx context.Context, userID, organisationID, projectID uuid.UUID) ([]driven.ConnectorMessageRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.connector_account_id, m.organisation_id, m.project_id, m.provider_event_id,
			m.external_channel_id, m.title, m.body_text, m.author_label, m.occurred_at,
			m.meta_json, m.created_at, m.updated_at
		FROM connector_messages m
		INNER JOIN connector_accounts a ON a.id = m.connector_account_id AND a.user_id = ?
		WHERE m.organisation_id = ? AND m.project_id = ?
		ORDER BY m.occurred_at DESC
	`, userID.String(), organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorMessageRow, 0)
	for rows.Next() {
		row, err := scanConnectorMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) GetConnectorMessage(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorMessageRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT m.id, m.connector_account_id, m.organisation_id, m.project_id, m.provider_event_id,
			m.external_channel_id, m.title, m.body_text, m.author_label, m.occurred_at,
			m.meta_json, m.created_at, m.updated_at
		FROM connector_messages m
		INNER JOIN connector_accounts a ON a.id = m.connector_account_id AND a.user_id = ?
		WHERE m.id = ?
	`, userID.String(), id.String())
	out, err := scanConnectorMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func scanConnectorAccount(s rowScanner) (*driven.ConnectorAccountRow, error) {
	var row driven.ConnectorAccountRow
	var id, userID, scopesJSON, createdAt, updatedAt string
	var tenant, lastError, lastSynced sql.NullString
	if err := s.Scan(&id, &userID, &row.Provider, &row.Label, &tenant, &row.ConnectionStatus,
		&lastError, &scopesJSON, &lastSynced, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(id); err != nil {
		return nil, err
	}
	if row.UserID, err = uuid.Parse(userID); err != nil {
		return nil, err
	}
	row.ExternalTenantID = nullStringPtr(tenant)
	row.LastError = nullStringPtr(lastError)
	if err := json.Unmarshal([]byte(scopesJSON), &row.Scopes); err != nil {
		return nil, err
	}
	if row.Scopes == nil {
		row.Scopes = []string{}
	}
	if row.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if row.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	if lastSynced.Valid {
		t, err := parseTime(lastSynced.String)
		if err != nil {
			return nil, err
		}
		row.LastSyncedAt = &t
	}
	return &row, nil
}

func scanConnectorBinding(s rowScanner) (*driven.ConnectorBindingRow, error) {
	var row driven.ConnectorBindingRow
	var id, accountID, organisationID, createdAt, updatedAt string
	var projectID, cursor sql.NullString
	if err := s.Scan(&id, &accountID, &organisationID, &row.ExternalChannelID,
		&projectID, &row.Label, &cursor, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(id); err != nil {
		return nil, err
	}
	if row.ConnectorAccountID, err = uuid.Parse(accountID); err != nil {
		return nil, err
	}
	if row.OrganisationID, err = uuid.Parse(organisationID); err != nil {
		return nil, err
	}
	if projectID.Valid {
		id, err := uuid.Parse(projectID.String)
		if err != nil {
			return nil, err
		}
		row.ProjectID = &id
	}
	row.SyncCursor = nullStringPtr(cursor)
	if row.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if row.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &row, nil
}

func scanConnectorMessage(s rowScanner) (*driven.ConnectorMessageRow, error) {
	var row driven.ConnectorMessageRow
	var id, accountID, organisationID, occurredAt, createdAt, updatedAt string
	var projectID sql.NullString
	if err := s.Scan(&id, &accountID, &organisationID, &projectID, &row.ProviderEventID,
		&row.ExternalChannelID, &row.Title, &row.BodyText, &row.AuthorLabel, &occurredAt,
		&row.MetaJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(id); err != nil {
		return nil, err
	}
	if row.ConnectorAccountID, err = uuid.Parse(accountID); err != nil {
		return nil, err
	}
	if row.OrganisationID, err = uuid.Parse(organisationID); err != nil {
		return nil, err
	}
	if projectID.Valid {
		id, err := uuid.Parse(projectID.String)
		if err != nil {
			return nil, err
		}
		row.ProjectID = &id
	}
	if row.OccurredAt, err = parseTime(occurredAt); err != nil {
		return nil, err
	}
	if row.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if row.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &row, nil
}
