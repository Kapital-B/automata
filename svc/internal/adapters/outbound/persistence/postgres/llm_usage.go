package postgres

import (
	"context"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) InsertLLMUsage(ctx context.Context, row driven.LLMUsageRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO llm_usage (id, user_id, account_id, feature, model,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), nullUUID(row.UserID), nullUUID(row.AccountID), row.Feature, row.Model,
		row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens, row.CreatedAt.UTC())
	return err
}

func (r *Repository) SumLLMUsageByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]driven.LLMUsageTotal, error) {
	rows, err := r.queryContext(ctx, `
		SELECT feature, model, COUNT(*),
			COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_write_tokens), 0)
		FROM llm_usage
		WHERE user_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY feature, model
		ORDER BY feature, model
	`, userID.String(), from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []driven.LLMUsageTotal
	for rows.Next() {
		var t driven.LLMUsageTotal
		if err := rows.Scan(&t.Feature, &t.Model, &t.Calls, &t.InputTokens, &t.OutputTokens, &t.CacheReadTokens, &t.CacheWriteTokens); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
