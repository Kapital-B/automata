package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// activityEventsSQL is every change worth surfacing on Home, as one relation.
//
// Scoped by membership: the INNER JOIN on project_members is what keeps the
// feed to projects the caller belongs to, matching /api/attention.
//
// Correspondence filing is deliberately absent. Triage writes a provisional
// assignment per scored thread, and with the LLM tier that is the highest
// volume event in the system — it would bury everything else.
//
// A row can produce more than one event: a decision proposed and later
// accepted appears twice, because the feed shows changes, not current state.
//
// COALESCE on activated_at and resolved_at covers rows written before those
// columns existed, which is why the migration needs no backfill.
const activityEventsSQL = `
	SELECT 'decision_proposed' AS kind, d.created_at AS occurred_at, d.project_id,
		d.statement AS title, 'decision' AS ref_type, d.id AS ref_id, d.source
	FROM decisions d
	INNER JOIN project_members pm ON pm.project_id = d.project_id AND pm.user_id = ?

	UNION ALL
	SELECT 'decision_accepted', d.decided_at, d.project_id,
		d.statement, 'decision', d.id, d.source
	FROM decisions d
	INNER JOIN project_members pm ON pm.project_id = d.project_id AND pm.user_id = ?
	WHERE d.status = 'accepted' AND d.decided_at IS NOT NULL

	UNION ALL
	SELECT 'decision_withdrawn', d.updated_at, d.project_id,
		d.statement, 'decision', d.id, d.source
	FROM decisions d
	INNER JOIN project_members pm ON pm.project_id = d.project_id AND pm.user_id = ?
	WHERE d.status = 'withdrawn'

	UNION ALL
	SELECT 'fact_recorded', COALESCE(fv.activated_at, fv.created_at), f.project_id,
		f.label || ': ' || fv.value_text, 'fact_version', fv.id, fv.source
	FROM fact_versions fv
	INNER JOIN facts f ON f.id = fv.fact_id
	INNER JOIN project_members pm ON pm.project_id = f.project_id AND pm.user_id = ?
	WHERE fv.status = 'active'

	UNION ALL
	SELECT 'fact_superseded', fv.superseded_at, f.project_id,
		f.label || ': ' || fv.value_text, 'fact_version', fv.id, fv.source
	FROM fact_versions fv
	INNER JOIN facts f ON f.id = fv.fact_id
	INNER JOIN project_members pm ON pm.project_id = f.project_id AND pm.user_id = ?
	WHERE fv.superseded_at IS NOT NULL

	UNION ALL
	SELECT 'contradiction_opened', c.created_at, c.project_id,
		c.summary, 'contradiction', c.id, ''
	FROM contradictions c
	INNER JOIN project_members pm ON pm.project_id = c.project_id AND pm.user_id = ?

	UNION ALL
	SELECT 'contradiction_resolved', c.resolved_at, c.project_id,
		c.summary, 'contradiction', c.id, ''
	FROM contradictions c
	INNER JOIN project_members pm ON pm.project_id = c.project_id AND pm.user_id = ?
	WHERE c.status = 'resolved' AND c.resolved_at IS NOT NULL

	UNION ALL
	SELECT 'issue_opened', i.created_at, i.project_id,
		i.title, 'issue', i.id, ''
	FROM issues i
	INNER JOIN project_members pm ON pm.project_id = i.project_id AND pm.user_id = ?

	UNION ALL
	SELECT 'issue_resolved', COALESCE(i.resolved_at, i.updated_at), i.project_id,
		i.title, 'issue', i.id, ''
	FROM issues i
	INNER JOIN project_members pm ON pm.project_id = i.project_id AND pm.user_id = ?
	WHERE i.status = 'resolved'
`

// activityUserArgs repeats the caller id once per UNION branch.
func activityUserArgs(userID uuid.UUID) []any {
	const branches = 9
	args := make([]any, 0, branches)
	for i := 0; i < branches; i++ {
		args = append(args, userID.String())
	}
	return args
}

func (r *Repository) ListActivity(ctx context.Context, userID, organisationID uuid.UUID, filter driven.ActivityFilter) ([]driven.ActivityItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	args := activityUserArgs(userID)
	args = append(args, organisationID.String())

	var where strings.Builder
	where.WriteString(" WHERE p.organisation_id = ? AND p.archived_at IS NULL AND e.occurred_at IS NOT NULL")
	if filter.ProjectID != nil {
		where.WriteString(" AND e.project_id = ?")
		args = append(args, filter.ProjectID.String())
	}
	if len(filter.Kinds) > 0 {
		where.WriteString(" AND e.kind IN (" + sqlkit.PlaceholderList(len(filter.Kinds)) + ")")
		for _, k := range filter.Kinds {
			args = append(args, k)
		}
	}
	if filter.Before != nil {
		// Keyset, with ref_id breaking ties so events sharing a timestamp
		// cannot loop or be skipped across pages.
		if filter.BeforeID != nil {
			// SQLite has no row-value comparison, so expand the keyset.
			where.WriteString(" AND (e.occurred_at < ? OR (e.occurred_at = ? AND e.ref_id < ?))")
			ts := formatRFC3339(filter.Before.UTC())
			args = append(args, ts, ts, filter.BeforeID.String())
		} else {
			where.WriteString(" AND e.occurred_at < ?")
			args = append(args, formatRFC3339(filter.Before.UTC()))
		}
	}

	q := `
		SELECT e.kind, e.occurred_at, e.project_id, p.code, p.name, e.title, e.ref_type, e.ref_id, e.source
		FROM (` + activityEventsSQL + `) AS e
		INNER JOIN projects p ON p.id = e.project_id` + where.String() + `
		ORDER BY e.occurred_at DESC, e.ref_id DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

func scanActivityRows(rows *sql.Rows) ([]driven.ActivityItem, error) {
	out := make([]driven.ActivityItem, 0)
	for rows.Next() {
		var kind, projectStr, code, name, title, refType, refStr string
		var source sql.NullString
		var occurredAt string
		if err := rows.Scan(&kind, &occurredAt, &projectStr, &code, &name, &title, &refType, &refStr, &source); err != nil {
			return nil, err
		}
		at, err := parseTime(occurredAt)
		if err != nil {
			return nil, err
		}
		projectID, err := uuid.Parse(projectStr)
		if err != nil {
			return nil, err
		}
		refID, err := uuid.Parse(refStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.ActivityItem{
			Kind: kind, OccurredAt: at.UTC(), ProjectID: projectID,
			ProjectCode: code, ProjectName: name, Title: title,
			RefType: refType, RefID: refID, Source: source.String,
		})
	}
	return out, rows.Err()
}

func (r *Repository) CountOverview(ctx context.Context, userID, organisationID uuid.UUID) (driven.OverviewCounts, error) {
	var out driven.OverviewCounts
	row := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM contradictions c
				INNER JOIN project_members pm ON pm.project_id = c.project_id AND pm.user_id = ?
				INNER JOIN projects p ON p.id = c.project_id AND p.organisation_id = ? AND p.archived_at IS NULL
				WHERE c.status = 'open'),
			(SELECT COUNT(*) FROM fact_versions fv
				INNER JOIN facts f ON f.id = fv.fact_id
				INNER JOIN project_members pm ON pm.project_id = f.project_id AND pm.user_id = ?
				INNER JOIN projects p ON p.id = f.project_id AND p.organisation_id = ? AND p.archived_at IS NULL
				WHERE fv.status = 'proposed'),
			(SELECT COUNT(*) FROM decisions d
				INNER JOIN project_members pm ON pm.project_id = d.project_id AND pm.user_id = ?
				INNER JOIN projects p ON p.id = d.project_id AND p.organisation_id = ? AND p.archived_at IS NULL
				WHERE d.status = 'proposed'),
			(SELECT COUNT(*) FROM projects p
				INNER JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
				WHERE p.organisation_id = ? AND p.archived_at IS NULL)
	`,
		userID.String(), organisationID.String(),
		userID.String(), organisationID.String(),
		userID.String(), organisationID.String(),
		userID.String(), organisationID.String(),
	)
	if err := row.Scan(&out.OpenContradictions, &out.ProvisionalFacts, &out.ProposedDecisions, &out.ActiveProjects); err != nil {
		return driven.OverviewCounts{}, err
	}
	return out, nil
}

// ListOverviewProjects derives last activity from the same events as the feed.
//
// There is deliberately no projects.last_activity_at column: it would need a
// write on every path that touches a fact, decision, contradiction or issue,
// and would drift the moment one was missed.
func (r *Repository) ListOverviewProjects(ctx context.Context, userID, organisationID uuid.UUID, limit int) ([]driven.OverviewProject, error) {
	if limit <= 0 {
		limit = 8
	}
	args := activityUserArgs(userID)
	args = append(args, organisationID.String(), userID.String(), organisationID.String(), limit)

	q := `
		WITH events AS (` + activityEventsSQL + `),
		last_activity AS (
			SELECT e.project_id, MAX(e.occurred_at) AS last_activity_at
			FROM events AS e
			INNER JOIN projects p ON p.id = e.project_id
			WHERE p.organisation_id = ? AND e.occurred_at IS NOT NULL
			GROUP BY e.project_id
		)
		SELECT p.id, p.code, p.name, la.last_activity_at
		FROM projects p
		INNER JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
		LEFT JOIN last_activity la ON la.project_id = p.id
		WHERE p.organisation_id = ? AND p.archived_at IS NULL
		ORDER BY (la.last_activity_at IS NULL), la.last_activity_at DESC, p.code ASC
		LIMIT ?`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOverviewProjects(rows)
}

func scanOverviewProjects(rows *sql.Rows) ([]driven.OverviewProject, error) {
	out := make([]driven.OverviewProject, 0)
	for rows.Next() {
		var idStr, code, name string
		var lastActivity sql.NullString
		if err := rows.Scan(&idStr, &code, &name, &lastActivity); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("parse project id: %w", err)
		}
		item := driven.OverviewProject{ID: id, Code: code, Name: name}
		if lastActivity.Valid && lastActivity.String != "" {
			t, err := parseTime(lastActivity.String)
			if err != nil {
				return nil, err
			}
			utc := t.UTC()
			item.LastActivityAt = &utc
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// attentionRowsSQL resolves every kind of pending item in one relation.
//
// Replaces a loop over every project that ran issues + facts + a fact-version
// query per fact + decisions + contradictions. Home makes this the most-hit
// endpoint in the product, so it cannot stay an N+1 inside an N+1.
//
// The member_role branch requires only membership: memberTouchesRole always
// returned true for a member, so "any member sees awaiting_input" is the rule
// that was actually in force.
const attentionRowsSQL = `
	SELECT 'issue_assignee' AS kind, i.title, i.project_id, 'issue' AS ref_type, i.id AS ref_id, i.created_at
	FROM issues i
	INNER JOIN project_members pm ON pm.project_id = i.project_id AND pm.user_id = ?
	WHERE i.status <> 'resolved' AND i.assignee_user_id = ?

	UNION ALL
	SELECT 'member_role', i.title, i.project_id, 'issue', i.id, i.created_at
	FROM issues i
	INNER JOIN project_members pm ON pm.project_id = i.project_id AND pm.user_id = ?
	WHERE i.status = 'awaiting_input'
	  AND (i.assignee_user_id IS NULL OR i.assignee_user_id <> ?)

	UNION ALL
	SELECT 'provisional_fact', f.label, f.project_id, 'fact_version', fv.id, fv.created_at
	FROM fact_versions fv
	INNER JOIN facts f ON f.id = fv.fact_id
	INNER JOIN project_members pm ON pm.project_id = f.project_id AND pm.user_id = ?
	WHERE fv.status = 'proposed'

	UNION ALL
	SELECT 'provisional_decision', d.statement, d.project_id, 'decision', d.id, d.created_at
	FROM decisions d
	INNER JOIN project_members pm ON pm.project_id = d.project_id AND pm.user_id = ?
	WHERE d.status = 'proposed'

	UNION ALL
	SELECT 'open_contradiction', c.summary, c.project_id, 'contradiction', c.id, c.created_at
	FROM contradictions c
	INNER JOIN project_members pm ON pm.project_id = c.project_id AND pm.user_id = ?
	WHERE c.status = 'open'
`

func (r *Repository) ListAttention(ctx context.Context, userID, organisationID uuid.UUID) ([]driven.AttentionRow, error) {
	uid := userID.String()
	args := []any{uid, uid, uid, uid, uid, uid, uid, organisationID.String()}
	q := `
		SELECT a.kind, a.title, a.project_id, p.name, a.ref_type, a.ref_id, a.created_at
		FROM (` + attentionRowsSQL + `) AS a
		INNER JOIN projects p ON p.id = a.project_id
		WHERE p.organisation_id = ? AND p.archived_at IS NULL`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttentionRows(rows)
}

func scanAttentionRows(rows *sql.Rows) ([]driven.AttentionRow, error) {
	out := make([]driven.AttentionRow, 0)
	for rows.Next() {
		var kind, title, projectStr, projectName, refType, refStr string
		var occurredAt string
		if err := rows.Scan(&kind, &title, &projectStr, &projectName, &refType, &refStr, &occurredAt); err != nil {
			return nil, err
		}
		at, err := parseTime(occurredAt)
		if err != nil {
			return nil, err
		}
		projectID, err := uuid.Parse(projectStr)
		if err != nil {
			return nil, err
		}
		refID, err := uuid.Parse(refStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.AttentionRow{
			Kind: kind, Title: title, ProjectID: projectID, ProjectName: projectName,
			RefType: refType, RefID: refID, OccurredAt: at.UTC(),
		})
	}
	return out, rows.Err()
}
