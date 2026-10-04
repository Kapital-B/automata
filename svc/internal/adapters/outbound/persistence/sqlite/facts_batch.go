package sqlite

import (
	"context"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func (r *Repository) ListActiveFactVersionsForFacts(ctx context.Context, factIDs []uuid.UUID) ([]driven.FactVersionRow, error) {
	out := make([]driven.FactVersionRow, 0, len(factIDs))
	for _, chunk := range sqlkit.ChunkUUIDs(sqlkit.DedupeUUIDs(factIDs), timelineHydrateChunk) {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, fact_id, status, value_json, value_text, unit, source, confidence, interpretation_id,
				supersedes_version_id, superseded_by_version_id, superseded_at, created_by_user_id, created_at, activated_at
			FROM fact_versions WHERE status = 'active' AND fact_id IN (`+sqlkit.PlaceholderList(len(chunk))+`)
		`, sqlkit.UUIDArgs(chunk)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			v, err := scanFactVersionRow(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, *v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Repository) ListFactEvidenceForVersions(ctx context.Context, versionIDs []uuid.UUID) ([]driven.FactEvidenceRow, error) {
	out := make([]driven.FactEvidenceRow, 0)
	for _, chunk := range sqlkit.ChunkUUIDs(sqlkit.DedupeUUIDs(versionIDs), timelineHydrateChunk) {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, fact_version_id, message_id, manual_item_id, added_at
			FROM fact_evidence WHERE fact_version_id IN (`+sqlkit.PlaceholderList(len(chunk))+`)
			ORDER BY fact_version_id, added_at ASC
		`, sqlkit.UUIDArgs(chunk)...)
		if err != nil {
			return nil, err
		}
		items, err := scanFactEvidenceRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}
