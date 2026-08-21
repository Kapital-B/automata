package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/domain/organisations"
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

func (r *Repository) CreateUserWithHomeOrg(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time, identityProvider, identitySubject, identityEmail string) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback() }()

	em := normalizeEmail(email)
	var ph any
	if passwordHash != nil {
		ph = *passwordHash
	}
	ts := formatRFC3339(now.UTC())
	orgID := uuid.New()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organisations (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, orgID.String(), organisations.DefaultName, ts, ts); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, home_organisation_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id.String(), em, ph, orgID.String(), ts, ts); err != nil {
		return uuid.Nil, err
	}
	memberID := uuid.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organisation_members (id, organisation_id, user_id, org_role, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, memberID.String(), orgID.String(), id.String(), string(organisations.RoleOwner), ts); err != nil {
		return uuid.Nil, err
	}
	if strings.TrimSpace(identityProvider) != "" {
		identEmail := normalizeEmail(identityEmail)
		if identEmail == "" {
			identEmail = em
		}
		if _, err := tx.ExecContext(ctx, `
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

func (r *Repository) GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var orgStr sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT home_organisation_id FROM users WHERE id = ?`, userID.String()).Scan(&orgStr)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, sql.ErrNoRows
	}
	if err != nil {
		return uuid.Nil, err
	}
	if !orgStr.Valid || orgStr.String == "" {
		return uuid.Nil, sql.ErrNoRows
	}
	return uuid.Parse(orgStr.String)
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
