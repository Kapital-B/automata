package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) SearchMessages(ctx context.Context, userID uuid.UUID, filter driven.MessageSearchFilter) ([]driven.MessageSearchHit, error) {
	q, args := sqlkit.MessageSearchQuery(userID, filter, func(t time.Time) any { return t })
	if q == "" {
		return nil, nil
	}
	rows, err := r.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []driven.MessageSearchHit
	for rows.Next() {
		var h driven.MessageSearchHit
		var id, account string
		var conv, body sql.NullString
		if err := rows.Scan(&id, &account, &conv, &h.ReceivedAt, &h.Subject, &h.FromJSON, &body, &h.Score); err != nil {
			return nil, err
		}
		if h.ID, err = uuid.Parse(id); err != nil {
			return nil, err
		}
		if h.AccountID, err = uuid.Parse(account); err != nil {
			return nil, err
		}
		if conv.Valid {
			h.ConversationID = &conv.String
		}
		if body.Valid {
			h.BodyText = &body.String
		}
		h.ReceivedAt = h.ReceivedAt.UTC()
		out = append(out, h)
	}
	return out, rows.Err()
}
