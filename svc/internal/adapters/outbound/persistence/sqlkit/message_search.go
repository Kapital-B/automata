package sqlkit

import (
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

const (
	searchDefaultLimit = 10
	searchMaxLimit     = 50
	searchMaxTerms     = 8
	searchMaxScopeIDs  = 200
)

// LikeContains builds a LIKE pattern matching s anywhere, with LIKE's own
// wildcards in s escaped so they match literally. Use with ESCAPE '\'.
func LikeContains(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// MessageSearchQuery builds the SearchMessages statement shared by both SQL
// adapters. It returns an empty query when the filter has nothing to search
// by, so callers issue no statement rather than returning all mail.
//
// Selected columns: id, account_id, conversation_id, received_at, subject,
// from_json, body_text, score. formatTime renders time arguments in the
// adapter's storage format.
func MessageSearchQuery(userID uuid.UUID, f driven.MessageSearchFilter, formatTime func(time.Time) any) (string, []any) {
	terms := make([]string, 0, len(f.Terms))
	seen := map[string]bool{}
	for _, t := range f.Terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		terms = append(terms, t)
		if len(terms) == searchMaxTerms {
			break
		}
	}
	projects := capIDs(DedupeUUIDs(f.ProjectIDs))
	contacts := capIDs(DedupeUUIDs(f.ContactIDs))
	if len(terms) == 0 && len(projects) == 0 && len(contacts) == 0 {
		return "", nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = searchDefaultLimit
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	// Arguments must follow placeholder order in the text: score first (it
	// is in the select list), then the join, then the filters.
	var args []any
	score := "0"
	if len(terms) > 0 {
		parts := make([]string, 0, len(terms))
		for _, t := range terms {
			parts = append(parts, `CASE WHEN LOWER(m.subject) LIKE ? ESCAPE '\' THEN 2 ELSE 0 END`+
				` + CASE WHEN LOWER(COALESCE(m.body_text, '')) LIKE ? ESCAPE '\' THEN 1 ELSE 0 END`)
			p := LikeContains(t)
			args = append(args, p, p)
		}
		score = strings.Join(parts, " + ")
	}

	var b strings.Builder
	b.WriteString(`
		SELECT id, account_id, conversation_id, received_at, subject, from_json, body_text, score FROM (
			SELECT m.id, m.account_id, m.conversation_id, m.received_at, m.subject, m.from_json, m.body_text,
				(` + score + `) AS score
			FROM messages m
			INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
			WHERE 1=1`)
	args = append(args, userID.String())
	if len(projects) > 0 {
		in := PlaceholderList(len(projects))
		b.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM messages mx
				LEFT JOIN message_assignment_overrides ox ON ox.message_id = mx.id
				LEFT JOIN thread_assignments tx
					ON tx.account_id = mx.account_id
					AND mx.conversation_id IS NOT NULL
					AND mx.conversation_id != ''
					AND tx.conversation_id = mx.conversation_id
				WHERE mx.id = m.id
				  AND CASE
						WHEN ox.message_id IS NOT NULL THEN ox.project_id IN (` + in + `)
						ELSE tx.project_id IN (` + in + `)
					END
			)`)
		args = append(args, UUIDArgs(projects)...)
		args = append(args, UUIDArgs(projects)...)
	}
	if len(contacts) > 0 {
		b.WriteString(`
			AND EXISTS (
				SELECT 1 FROM correspondence_participants cp
				WHERE cp.message_id = m.id AND cp.contact_id IN (` + PlaceholderList(len(contacts)) + `)
			)`)
		args = append(args, UUIDArgs(contacts)...)
	}
	if f.Since != nil {
		b.WriteString(` AND m.received_at >= ?`)
		args = append(args, formatTime(f.Since.UTC()))
	}
	b.WriteString(`
		) hits`)
	if len(terms) > 0 {
		b.WriteString(` WHERE score > 0`)
	}
	b.WriteString(` ORDER BY score DESC, received_at DESC, id DESC LIMIT ?`)
	args = append(args, limit)
	return b.String(), args
}

func capIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) > searchMaxScopeIDs {
		return ids[:searchMaxScopeIDs]
	}
	return ids
}
