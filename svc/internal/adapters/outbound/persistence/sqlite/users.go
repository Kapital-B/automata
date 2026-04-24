package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func (r *Repository) CreateUser(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time) error {
	em := normalizeEmail(email)
	var ph any
	if passwordHash != nil {
		ph = *passwordHash
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id.String(), em, ph, formatRFC3339(now.UTC()), formatRFC3339(now.UTC()),
	)
	return err
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (uuid.UUID, *string, error) {
	var idStr string
	var ph sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE email = ?`, normalizeEmail(email)).Scan(&idStr, &ph)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil, sql.ErrNoRows
	}
	if err != nil {
		return uuid.Nil, nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if !ph.Valid {
		return id, nil, nil
	}
	s := ph.String
	return id, &s, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (string, error) {
	var email string
	err := r.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, id.String()).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}

func (r *Repository) FindIdentity(ctx context.Context, provider, providerSubject string) (uuid.UUID, bool, error) {
	var uidStr string
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM user_identities WHERE provider = ? AND provider_subject = ?`,
		provider, providerSubject).Scan(&uidStr)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	id, err := uuid.Parse(uidStr)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func (r *Repository) AttachIdentity(ctx context.Context, id uuid.UUID, userID uuid.UUID, provider, providerSubject, emailAtLink string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO user_identities (id, user_id, provider, provider_subject, email_at_link, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id.String(), userID.String(), provider, providerSubject, normalizeEmail(emailAtLink), formatRFC3339(now.UTC()),
	)
	return err
}
