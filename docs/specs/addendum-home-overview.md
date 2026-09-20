# Spec Addendum: Home Overview

**Status:** Draft  
**Parent PRD:** [Project-oriented UI shell](../prds/addendum-ui-project-shell.md) — **revises §6.1 and §6.3**  
**Related PRD:** [AI-First Assistant Experience](../prds/addendum-ai-first-assistant.md) §7.1, §7.4  
**Related spec:** [Wave 2](addendum-project-correspondence-wave2.md) §5.4 (attention), [Triage efficiency](addendum-triage-efficiency.md) §5 (set-based read paths)  
**Last updated:** 2026-09-20  

Home currently answers *"what needs me"*. This addendum changes its job to *"what is going on across my projects"* — a portfolio overview with aggregate counts, outstanding actions, and a cross-project activity feed.

The data for the feed exists but is unreachable: every substantive read is project-scoped, so there is no way to ask what changed across projects. Most of this addendum is the backend needed to make that question answerable.

---

## 1. Scope

- A **row of metric cards** at the top of Home, restored as a deliberate reversal of the PRD's "not metric cards" rule (§2.1).
- A **cross-project activity feed**: decisions, facts, contradictions and issues as they change.
- **Aggregate counts** for the card row, served in one request rather than assembled client-side.
- **Real last-activity ordering** for projects, replacing a timestamp that does not track activity.
- **Recency** on attention items, so "recent outstanding actions" is expressible.
- Removing the client-side fan-out that Home currently performs per project.

---

## 2. Decisions taken

Answered by the operator on 2026-09-20; recorded because each reverses or narrows an earlier position.

### 2.1 Metric cards return

[addendum-ui-project-shell.md §6.1](../prds/addendum-ui-project-shell.md) froze Home as *"**Needs my input** list as the hero content (**not** metric cards)"*, and slice U2 (`a241eca`) removed a six-metric row to comply.

That rule is **revised**: a metric row returns at the top of Home.

The original row is not what returns. It was mail-centric — *Action items, FYI, Accounts, Latest summary, Drafts ready* — and reinstating it would re-import the pre-pivot mail-assistant model. The new row is project-portfolio shaped (§5.1).

The intent behind the original rule is preserved: the cards sit **above** the attention list without displacing it, and each card is a link into a filtered view rather than decoration.

### 2.2 Scope is membership, not organisation

The feed and the counts cover **only projects the caller is a member of** (`project_members`), consistent with `/api/attention`.

The trade-off is explicit: a contradiction opening on a home-org project the operator does not belong to will **not** appear on Home. Consistency with existing attention scoping was preferred over portfolio completeness. Revisit if operators report blind spots.

### 2.3 Provisional assignments are excluded

Triage writes a provisional assignment per scored thread, and with the LLM tier (`addendum-triage-efficiency.md` §9.3) that is the highest-volume event in the system by an order of magnitude.

Correspondence filing is therefore **not** an activity kind. Triage owns that queue and Home links to its count. The feed is for changes to what the projects *know* — decisions, facts, contradictions, issues.

---

## 3. Non-goals

- Organisation-wide visibility beyond the caller's memberships (§2.2).
- A per-event audit log. The feed is a recent-changes view, not a compliance trail.
- Replacing Triage, Projects, or the project workspace as the places work gets done.
- Real-time push. Home refreshes on load and on interval like every other surface.
- Chat-first Home. Ask stays as the secondary affordance the shell PRD specifies.
- Reworking `/api/attention`'s semantics; only its cost and ordering change (§7).

---

## 4. Current state

| Finding | Evidence |
| ------- | -------- |
| No cross-project read for facts, decisions, contradictions, issues or timeline — all are `/api/projects/{id}/…` | `http/router.go:129-159` |
| `projects.updated_at` tracks metadata edits only (name, description, client, keywords, archive), not activity | `postgres/domains.go:619`, `sqlite/projects.go:159` — the only writers |
| Attention items carry no timestamp; `sortItems` ranks by category alone | `attention/service.go:266-290`, `Item` struct at `:25-36` |
| `/api/attention` loops every project and runs `ListFactVersions` per fact — an N+1 inside an N+1 | `attention/service.go:73-92`, `:171-220` |
| Home fans out `getCurrentPosition` across 5 projects from the browser | `web/src/pages/AssistantHome.tsx:79-87` |

The third and fourth rows matter most: "recent outstanding action items" is not currently expressible, and the endpoint that would back it is the most expensive in the system.

---

## 5. UI contract (`web/`)

Route is unchanged (`/`). Composition, top to bottom:

### 5.1 Metric cards

One row, five cards, responsive to two columns on narrow screens. Each card is a link.

| Card | Value | Links to |
| ---- | ----- | -------- |
| Needs you | `attention.counts.total` | scrolls to the list below |
| Triage | unassigned + provisional | `/triage` |
| Contradictions | open contradictions | `/projects?filter=contradictions` |
| Unconfirmed | provisional facts + proposed decisions | `/projects?filter=unconfirmed` |
| Projects | active project count | `/projects` |

A number earns a card only if a non-zero value changes what the operator does next. Totals that are merely descriptive (facts recorded, messages filed) do not qualify — that is what made the original row noise.

Zero is rendered as a muted zero, never hidden: a card that disappears when empty makes the row jump between loads and teaches operators not to trust the layout.

### 5.2 Needs you

The existing merged attention list (`mergeNeedsMeRows`), unchanged in behaviour, now ordered by severity **then recency** (§7.2). Keeps its current empty state.

### 5.3 What changed

Reverse-chronological feed across the caller's projects, grouped by day (`Today`, `Yesterday`, then dates).

Each row carries: project code, event label, one-line title, relative time, provenance badge (`user` / `rule` / `llm`), and a deep link to the referenced object.

Provenance is load-bearing, not decoration: with the LLM tier writing facts and decisions, "who claimed this" is the first question an operator asks.

Default 25 items with *Show more*. Filter chips for kind and project are optional polish, not required for the slice.

### 5.4 Projects

Up to 8, ordered by **real** last activity (§6.3), each with code, name, relative last-activity time, the existing current-position teaser, and an attention count badge when non-zero.

The teaser must come from the overview payload, not a per-project request (§4, last row).

### 5.5 Ask

Unchanged, and stays below the feed. It remains secondary to the lists, per the shell PRD.

### 5.6 Accessibility

Cards are links with accessible names that include both label and value ("Needs you, 4"), not bare numbers. The feed is a list with day headings as real headings, so it can be navigated by landmark. Relative times carry an absolute `title`.

---

## 6. HTTP API

Snake_case JSON. Auth as today. Both endpoints are scoped by membership (§2.2).

### 6.1 Overview

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/overview` | Card counts, plus per-project summary for §5.4. |

```json
{
  "counts": {
    "needs_you": 4,
    "triage_unassigned": 12,
    "triage_provisional": 7,
    "open_contradictions": 2,
    "provisional_facts": 5,
    "proposed_decisions": 3,
    "active_projects": 9
  },
  "projects": [
    {
      "id": "uuid",
      "code": "DC01",
      "name": "Riverside",
      "last_activity_at": "2026-09-20T09:12:00Z",
      "attention_count": 2,
      "teaser": "Pump P-03 duty: 90 kW"
    }
  ]
}
```

One request replaces the current projects list + N position requests + summary request.

### 6.2 Activity

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/activity?limit=&before=&kind=&project_id=` | Cross-project change feed. |

```json
{
  "items": [
    {
      "kind": "decision_accepted",
      "occurred_at": "2026-09-20T09:12:00Z",
      "project_id": "uuid",
      "project_code": "DC01",
      "title": "Proceed with 90 kW and update M-402 to Rev C",
      "ref_type": "decision",
      "ref_id": "uuid",
      "source": "llm"
    }
  ],
  "next_before": "2026-09-19T17:40:00Z"
}
```

- `limit` defaults to 25, caps at 100.
- `before` is an `occurred_at` keyset, not an offset. The feed is ordered by a timestamp over a mutating set; offset paging would skip rows exactly as it did in `assign_projects` (`addendum-triage-efficiency.md` §2.6).
- `kind` and `project_id` are optional repeatable filters.

### 6.3 Event kinds

| `kind` | Source | `occurred_at` |
| ------ | ------ | ------------- |
| `decision_proposed` | `decisions` | `created_at` |
| `decision_accepted` | `decisions` where `status = 'accepted'` | `decided_at` |
| `decision_withdrawn` | `decisions` where `status = 'withdrawn'` | `updated_at` |
| `fact_recorded` | `fact_versions` where `status = 'active'` | `activated_at` (§8.1) |
| `fact_superseded` | `fact_versions` where `superseded_at IS NOT NULL` | `superseded_at` |
| `contradiction_opened` | `contradictions` | `created_at` |
| `contradiction_resolved` | `contradictions` where `status = 'resolved'` | `resolved_at` |
| `issue_opened` | `issues` | `created_at` |
| `issue_resolved` | `issues` where `status = 'resolved'` | `resolved_at` (§8.1) |

One `SELECT` branch per row of that table, `UNION ALL`-ed. A single decision can therefore contribute both a `decision_proposed` and a `decision_accepted` event, which is the point: the feed shows the change, not the current state.

Correspondence filing is absent by decision (§2.3).

---

## 7. Attention

### 7.1 Cost

`ForUser` must stop looping projects. Replace with set-based queries joined through `project_members`, in the manner of `addendum-triage-efficiency.md` §5.1:

- one query for open contradictions across the caller's projects,
- one for proposed decisions,
- one for provisional fact versions,
- one for issues assigned to the caller,
- the existing mail action-item query.

The per-fact `ListFactVersions` call (`attention/service.go:197`) must go; provisional versions are selectable directly by status.

This is a prerequisite, not a cleanup: Home makes `/api/attention` the most-hit endpoint in the product.

### 7.2 Recency

`attention.Item` gains `occurred_at`, sourced from the underlying row's creation time. `sortItems` keeps `why_me` rank as the primary key and uses `occurred_at DESC` as the tiebreak, replacing the current stable-sort-by-insertion.

---

## 8. Data model

### 8.1 Two timestamps do not exist

The feed's value is chronological accuracy, and two events cannot currently be dated:

| Event | Problem |
| ----- | ------- |
| `fact_recorded` | `fact_versions` has `created_at` and `superseded_at` but no activation time. A version proposed on the 1st and confirmed on the 5th would be dated the 1st. |
| `issue_resolved` | `issues` has no `resolved_at`. Using `updated_at` misdates any issue edited after resolution. |

Add both as nullable columns, backfilled from the existing approximation:

```sql
ALTER TABLE fact_versions ADD COLUMN activated_at TIMESTAMPTZ;
UPDATE fact_versions SET activated_at = created_at WHERE status = 'active' AND activated_at IS NULL;

ALTER TABLE issues ADD COLUMN resolved_at TIMESTAMPTZ;
UPDATE issues SET resolved_at = updated_at WHERE status = 'resolved' AND resolved_at IS NULL;
```

Written on confirm and on resolve respectively. Backfilled rows are no worse than the approximation they replace; rows written after the migration are exact.

DSQL notes: `ALTER TABLE … ADD COLUMN` is one DDL statement per transaction, and the backfill is DML, so they are separate statements — which the migrator already handles, one statement at a time. The backfill must respect the 3,000-row mutation cap; chunk it if the table is larger.

### 8.2 Last activity

`projects.last_activity_at` is **not** added. `last_activity_at` in §6.1 is derived as `MAX(occurred_at)` per project from the same union that backs the feed.

A maintained column would need a write on every path that touches a fact, decision, contradiction or issue, and would drift the moment one is missed. The union already exists; derive from it and revisit only if measurement says otherwise.

### 8.3 Indexes

The feed filters by project and orders by timestamp per branch. Add, in both `postgres/` and `dsql/` sets:

```sql
CREATE INDEX ... ON decisions(project_id, created_at);
CREATE INDEX ... ON contradictions(project_id, created_at);
CREATE INDEX ... ON issues(project_id, created_at);
CREATE INDEX ... ON fact_versions(fact_id, created_at);
CREATE INDEX ... ON project_members(user_id, project_id);
```

No `DESC` in the DSQL set: its `CREATE INDEX` grammar has no sort order on keys (`addendum-aurora-dsql.md` §3.2). `TestDSQLIndexesHaveNoSortOrder` enforces this.

---

## 9. Performance

Home must issue a **bounded** number of statements regardless of project or event count. Specifically:

- `/api/overview` — one statement per count group plus one for the project list; no per-project query.
- `/api/activity` — one statement, the union, with project code joined in. No per-row hydration.
- `/api/attention` — §7.1.

No endpoint on this path selects message bodies.

The `persistencetest` query counter (`addendum-triage-efficiency.md` §16.3) is the enforcement mechanism; see §11.

---

## 10. Slices

| Slice | Name | Delivers |
| ----- | ---- | -------- |
| **H1** | Timestamps | §8.1 migration and the write paths that set the new columns |
| **H2** | Read APIs | §6.1, §6.2, §8.3 indexes |
| **H3** | Attention | §7.1 cost, §7.2 recency |
| **H4** | Home UI | §5, and removal of the client fan-out |

H2 depends on H1 for `fact_recorded` and `issue_resolved` to be correctly dated. H4 depends on H2. H3 is independent and can ship in parallel.

---

## 11. Slice exit criteria

**H1.** A fact version confirmed today reports `activated_at` today, not its proposal date. An issue resolved today then edited tomorrow still reports today. Existing rows are backfilled and no row has a null timestamp where its status implies one.

**H2.** `GET /api/activity?limit=25` issues one statement for the feed, verifiable by query counter, over a fixture with events in 4+ projects. A decision proposed and later accepted appears as **two** events. Events from a project the caller is not a member of never appear. No provisional assignment appears. Paging with `before` returns each event exactly once across pages.

**H3.** `/api/attention` issues a bounded number of statements over 50 projects, and no statement is issued per fact. Two items of the same `why_me` are ordered newest first.

**H4.** Home issues three API requests, independent of project count. Each card links to its filtered destination. A project with a decision accepted today sorts above one whose keywords were edited today — the case the current page gets backwards.

---

## 12. Testing

- Activity: one event per state change; both events for a propose-then-accept decision; membership filtering; provisional assignments absent; keyset paging with no repeats or gaps across pages; `kind` and `project_id` filters.
- Ordering is stable for events sharing a timestamp (tiebreak on id), so paging cannot loop.
- Overview: counts match the equivalent per-project queries on the same fixture; `last_activity_at` equals the newest event for that project; a project with no activity still appears with a null timestamp.
- Attention: query-count guard; recency tiebreak within a `why_me` group; existing Wave 2 attention tests pass unchanged.
- Migration: `activated_at` backfill covers exactly the active versions; `resolved_at` exactly the resolved issues; re-running is idempotent.
- DSQL lints (`addendum-triage-efficiency.md` §18) cover the new index migration.
- Contract-suite coverage runs against both adapters, and against real Postgres in CI.

---

## 13. Risks

| Risk | Mitigation |
| ---- | ---------- |
| Membership scoping hides real problems on projects the operator does not belong to (§2.2) | Accepted deliberately. Revisit if operators report blind spots; the query changes by one join. |
| The feed becomes noise as volume grows | Provisional assignments already excluded (§2.3). If it still floods, group consecutive same-kind events per project before adding filters. |
| The union is slow on DSQL | §8.3 indexes; one statement, no hydration. Confirm the plan on dev before promotion, as with triage. |
| Backfilled timestamps misdate historical events | Documented in §8.1. No worse than today, where the timestamps do not exist at all. |
| Card row drifts back toward vanity metrics | §5.1 states the admission rule: a card must change what the operator does next. |
