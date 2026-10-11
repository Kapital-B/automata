package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlkit "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	"github.com/google/uuid"
)

func (r *Repository) InsertAuthSession(ctx context.Context, sessionID, userID uuid.UUID, tokenHash string, createdAt, expiresAt time.Time) (portResult0 error) {
	defer func() {
		portResult0 = sqlkit.DomainError(portResult0)
	}()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		sessionID.String(), userID.String(), tokenHash,
		formatRFC3339(createdAt.UTC()), formatRFC3339(expiresAt.UTC()),
	)
	return err
}

func (r *Repository) ConsumeAuthSession(ctx context.Context, tokenHash string) (portResult0 uuid.UUID, portResult1 bool, portResult2 error) {
	defer func() {
		portResult2 = sqlkit.DomainError(portResult2)
	}()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, false, err
	}
	defer tx.Rollback()
	var idStr, userStr, expStr string
	err = tx.QueryRowContext(ctx, `SELECT id, user_id, expires_at FROM auth_sessions WHERE token_hash = ?`, tokenHash).Scan(&idStr, &userStr, &expStr)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	exp, err := parseTime(expStr)
	if err != nil {
		return uuid.Nil, false, err
	}
	if !time.Now().UTC().Before(exp) {
		_, _ = tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE id = ?`, idStr)
		if err := tx.Commit(); err != nil {
			return uuid.Nil, false, err
		}
		return uuid.Nil, false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE id = ?`, idStr); err != nil {
		return uuid.Nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, false, err
	}
	uid, err := uuid.Parse(userStr)
	if err != nil {
		return uuid.Nil, false, err
	}
	return uid, true, nil
}
