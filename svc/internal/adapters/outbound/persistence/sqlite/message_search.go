package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) SearchMessages(ctx context.Context, userID uuid.UUID, filter driven.MessageSearchFilter) ([]driven.MessageSearchHit, error) {
	q, args := sqlkit.MessageSearchQuery(userID, filter, func(t time.Time) any { return formatRFC3339(t) })
	if q == "" {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []driven.MessageSearchHit
	for rows.Next() {
		var h driven.MessageSearchHit
		var id, account, received string
		var conv, body sql.NullString
		if err := rows.Scan(&id, &account, &conv, &received, &h.Subject, &h.FromJSON, &body, &h.Score); err != nil {
			return nil, err
		}
		if h.ID, err = uuid.Parse(id); err != nil {
			return nil, err
		}
		if h.AccountID, err = uuid.Parse(account); err != nil {
			return nil, err
		}
		if h.ReceivedAt, err = parseTime(received); err != nil {
			return nil, err
		}
		if conv.Valid {
			h.ConversationID = &conv.String
		}
		if body.Valid {
			h.BodyText = &body.String
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
