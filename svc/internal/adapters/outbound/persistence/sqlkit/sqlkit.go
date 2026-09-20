// Package sqlkit holds small query-building helpers shared by the SQL
// adapters, so batch-hydration logic cannot drift between them.
package sqlkit

import (
	"strings"

	"github.com/google/uuid"
)

// PlaceholderList builds "?,?,?" for an IN clause. The postgres adapter
// rewrites `?` to `$n` on the way out.
func PlaceholderList(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// DedupeUUIDs removes repeats while preserving first-seen order.
func DedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ChunkUUIDs splits ids into runs of at most size. An empty input yields no
// chunks, so callers issue no statement at all.
func ChunkUUIDs(ids []uuid.UUID, size int) [][]uuid.UUID {
	if size <= 0 || len(ids) == 0 {
		if len(ids) == 0 {
			return nil
		}
		return [][]uuid.UUID{ids}
	}
	out := make([][]uuid.UUID, 0, (len(ids)+size-1)/size)
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[start:end])
	}
	return out
}
