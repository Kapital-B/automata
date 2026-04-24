package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

// TakeState deletes and returns oauth state if present and not expired.
func (r *Repository) TakeState(ctx context.Context, state string) (accounts.MsAccountKind, *string, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, false, err
	}
	defer tx.Rollback()
	var kind string
	var label sql.NullString
	var created string
	err = tx.QueryRowContext(ctx, `SELECT ms_account_kind, label_hint, created_at FROM oauth_states WHERE state = ?`, state).Scan(&kind, &label, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	createdAt, err := parseTime(created)
	if err != nil {
		return "", nil, false, err
	}
	if time.Now().UTC().Sub(createdAt) > r.OAuthStateTTL {
		_, _ = tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state)
		if err := tx.Commit(); err != nil {
			return "", nil, false, err
		}
		return "", nil, false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return "", nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return "", nil, false, err
	}
	var lh *string
	if label.Valid {
		lh = &label.String
	}
	return accounts.MsAccountKind(kind), lh, true, nil
}
