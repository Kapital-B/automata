# Spec Addendum: Triage Efficiency

**Status:** Draft  
**Parent PRD:** [Project correspondence intelligence](../prds/addendum-project-correspondence.md)  
**Related spec:** [Project Correspondence Wave 1](addendum-project-correspondence-wave1.md) §6, §7, §9  
**Related spec:** [Aurora DSQL](addendum-aurora-dsql.md) §3, [AWS deployment](aws-deployment.md) §4  
**Last updated:** 2026-09-19  

Triage — the queue at `/triage` that files incoming correspondence onto a project — is slow to load, slow to work through, and appears to do no inference. This addendum specifies the read-path, queue-shape, batch-mutation, and scoring changes that fix it.

Wave 1 §7 remains authoritative for **what** the effective assignment of a message is. This spec changes only **how** it is computed, presented, and mutated, plus it implements the `source = 'llm'` path Wave 1 §7 reserved and never built.

Parent invariants still apply: `account_id` on mail-derived rows, home-organisation authorisation, no secrets in logs, user correction always `source = user` / `status = committed`.

---

## 1. Scope

- Collapse the `/api/unassigned` and `/api/unassigned/summary` **N+1 read path** into set-based queries.
- Present the triage queue in **thread units**, not message units.
- Surface the **existing suggestion** in the UI and make confirming it one action.
- Add **batch assignment** across the API, application service, and UI.
- Improve **deterministic scoring** so that more items arrive pre-suggested.
- Add **LLM scoring** for project assignment (`source = 'llm'`), batched at thread level, behind the Wave 1 §7 confidence policy.
- Make assignment failures **visible** rather than silently successful.

---

## 2. Current state

Findings that motivate each section. File references are at the commit this spec was written against.

### 2.1 There is no AI in triage

`AssignService` (`svc/internal/application/projects/service.go:516-523`) has no `LLM` field, and composition never gives it one — `svc/internal/composition/app.go:413-445` wires `llmClient` into issues, interpret, project AI, categorize, summarize, auto-draft and forward rules, but not assignment.

All inference is `tryAssignOne` (`service.go:620-710`): thread sibling → committed; exactly one project **code** token in subject+body → committed; exactly one project **name/keyword** substring → provisional; otherwise nothing. Two or more code hits deliberately return no suggestion (`service.go:672-674`).

In a real mailbox most correspondence carries neither a literal project code nor exactly one keyword hit, so the queue fills with items that were never scored. `domainprojects.SourceLLM` (`svc/internal/domain/projects/project.go:57`) and the `source IN ('user','rule','llm')` CHECK (`migrate/common/001_baseline.sql:380`) exist; nothing writes `llm`.

### 2.2 The suggestion is computed, serialised, and discarded

The repository sets `ProjectID: eff.ProjectID` (`postgres/domains.go:905`); the handler emits `project_id` (`http/projects.go:324-326`); the web type declares it (`web/src/lib/auth.ts:452`).

`web/src/pages/Triage.tsx` never reads it. `UnassignedRow` initialises `useState("")` and renders an empty "Choose project" select. There is **no confirm action** — accepting a provisional suggestion costs the same clicks as assigning from scratch. From the operator's seat this is indistinguishable from "inference is not running".

### 2.3 The read path is ~6,000 round-trips per page load

`ListUnassigned` selects up to `defaultTimelineCandidateLimit` = **2,000** candidate messages (`postgres/repo.go:23`), then calls `EffectiveAssignment` per row (`postgres/domains.go:853-950`; identical shape in `sqlite/projects.go:480-560`).

`EffectiveAssignment` (`domains.go:800-841`) is itself 2–3 statements: `GetMessage`, `GetMessageOverride`, and `GetThreadAssignment` when a conversation id exists. `GetMessage` selects `body_text`, `to_json`, `cc_json` and joins `message_categories` + `category_definitions` (`repo.go:608-618`) — full mail bodies, to decide an assignment status.

**2,000 × 2–3 = 4,000–6,000 sequential statements** to return at most 100 rows, against a 30 s API Lambda timeout (`svc/terraform/modules/automata/lambda.tf:280`).

Amplifiers:

| Amplifier | Location |
| --------- | -------- |
| `CountUnassignedSummary` re-runs the whole N+1 for two integers | `domains.go:951-965` |
| That summary backs the sidebar badge, so **every page in the app** pays it | `web/src/components/AppSidebar.tsx:60-64` |
| Home queries the same key twice more | `AssistantHome.tsx:62`, `useAssistantHomeData.ts:170` |
| Each single assignment invalidates list **and** summary — two full N+1 passes per click | `Triage.tsx:71-75` |
| `ListMessagesNeedingAssign` has the same N+1 and runs **inside mail sync** | `domains.go:967-985`, `messages/sync.go:336-338` |
| `new QueryClient()` is constructed with no defaults, so nothing is stale-cached | `web/src/App.tsx:30` |

No index supports the candidate query's `ORDER BY m.received_at`; `migrate/dsql/001_indexes.sql` has `idx_messages_account` only.

### 2.4 The queue is inflated by thread duplicates

`ListUnassigned` emits one row per message with no dedupe on `conversation_id`. A 20-message unassigned thread is 20 rows, all resolved by one "Assign thread" click. The visible queue is an order of magnitude longer than the real decision count.

### 2.5 Assignment failures are invisible

`sync.go:337` discards the error (`_ = s.Assign.AssignAfterSync(...)`); inside, per-message errors `continue` (`service.go:655-658`) while the run is still recorded `"success"` (`service.go:661-665`). A wholly failing pass is indistinguishable from "nothing matched". Nothing in `web/` can trigger `assign_projects`, though it is a registered streamed job (`jobs/registry.go:124`).

### 2.6 The assign cursor is an offset labelled as a keyset

`AssignAccountChunk` pages with `jobkit.DecodeOffsetCursor` / `EncodeOffsetCursor` (`service.go:583-617`), which encode an integer offset under `Kind: "message_keyset"` (`jobkit/helpers.go`). The registry declares `CursorKind: CursorMessageKeyset` (`jobs/registry.go:124`).

An offset counts rows behind it rather than naming the last row read. The scan is ordered `received_at DESC`, so **rows removed above the cursor** — retention, account cleanup — shift everything below them up and the next chunk steps straight over messages it never saw. (New mail arriving between chunks is benign: it causes re-processing, not skipping, and assignment is idempotent.)

---

## 3. Non-goals

- Changing the Wave 1 §7 effective-assignment precedence (override → thread → unassigned).
- Changing the `thread_assignments` / `message_assignment_overrides` schema.
- Auto-committing LLM suggestions without operator confirmation.
- Auto-assigning manual items in bulk from inference (paste-onto-project stays `committed` / `user`).
- Replacing the Inbox or project-timeline assignment controls (they reuse the same services).
- A general ACL engine, or triage across organisations.
- Reworking `interpret` / `reconcile`.

---

## 4. Slices

Must ship in order. T1 is a prerequisite for T3: batch UI over the current read path feels worse, not better.

| Slice | Name | Delivers |
| ----- | ---- | -------- |
| **T1** | Read path | §5 single-query list + counts, index, projection, caching |
| **T2** | Queue shape | §6 thread-unit rows, §11.1 suggestion surfaced, confirm action |
| **T3** | Batch | §7 batch service + §8.1 API + §11.2 multi-select UI |
| **T4** | Signals | §9.1–9.2 deterministic scoring, ranked candidates |
| **T5** | LLM | §9.3 LLM scoring, §10 contract, §8.3 rescan endpoint |

T1 and T2 are independent of T4/T5 and deliver most of the perceived speed.

---

## 5. Read path

### 5.1 Single-query effective assignment

`ListUnassigned` must issue **one** statement for the mail side. The precedence in Wave 1 §7 is expressible as a join: `message_assignment_overrides` is PK'd on `message_id`, and `thread_assignments` carries `UNIQUE (account_id, conversation_id)` (`migrate/common/001_baseline.sql:387`), so both joins are index-backed.

```sql
WITH eff AS (
  SELECT
    m.id, m.account_id, a.label AS account_label, m.subject, m.from_json,
    m.conversation_id, m.received_at,
    CASE WHEN o.message_id IS NOT NULL THEN 'message'
         WHEN t.id         IS NOT NULL THEN 'thread'
         ELSE 'none' END AS scope,
    CASE WHEN o.message_id IS NOT NULL THEN o.project_id ELSE t.project_id END AS project_id,
    CASE WHEN o.message_id IS NOT NULL THEN o.status     ELSE t.status     END AS raw_status,
    CASE WHEN o.message_id IS NOT NULL THEN o.reason     ELSE t.reason     END AS reason,
    CASE WHEN o.message_id IS NOT NULL THEN o.source     ELSE t.source     END AS source,
    CASE WHEN o.message_id IS NOT NULL THEN o.confidence ELSE t.confidence END AS confidence
  FROM messages m
  INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
  LEFT JOIN message_assignment_overrides o
         ON o.message_id = m.id
  LEFT JOIN thread_assignments t
         ON t.account_id = m.account_id
        AND t.conversation_id = m.conversation_id
        AND m.conversation_id IS NOT NULL
        AND m.conversation_id <> ''
)
SELECT ..., CASE WHEN project_id IS NULL THEN 'unassigned' ELSE raw_status END AS status
FROM eff
WHERE project_id IS NULL OR raw_status = 'provisional'
ORDER BY received_at DESC
LIMIT ? OFFSET ?
```

Invariants:

- An override row with `project_id IS NULL` means **unassigned even if the thread is assigned** (Wave 1 §7.1). The `LEFT JOIN` ordering above preserves this: when `o.message_id IS NOT NULL` the thread row is never consulted.
- `status` is normalised to `'unassigned'` whenever the effective `project_id` is null, matching `EffectiveAssignment` (`domains.go:816-818`).
- The `status` filter (`unassigned` | `provisional` | `all`) is applied **in SQL**, not in Go. The current code truncates candidates at 2,000 before filtering, so `offset` is already incorrect for deep pages.
- `LIMIT`/`OFFSET` are the caller's. `defaultTimelineCandidateLimit` no longer applies to this path.

`EffectiveAssignment(ctx, userID, messageID)` stays on `driven.AssignmentRepository` for the single-message callers (`AssignMessage`, `clearMessageOverride`). It must not be used inside a loop.

### 5.2 Projection

**Never select `body_text`, `to_json`, or `cc_json` on the triage read path.** The queue renders subject, sender, timestamp and reason only. Aurora DSQL caps query memory at 128 MiB and the DSQL addendum sanctions the "scan 2000 messages" shape (`addendum-aurora-dsql.md:36-46`) — that budget holds only if bodies are excluded.

### 5.3 Counts

`CountUnassignedSummary` must not call `ListUnassigned`. Replace with a single grouped count over the same `eff` CTE:

```sql
SELECT CASE WHEN project_id IS NULL THEN 'unassigned' ELSE raw_status END AS status,
       COUNT(*)
FROM eff
WHERE project_id IS NULL OR raw_status = 'provisional'
GROUP BY 1
```

Counts are over **threads** once §6 lands, so the sidebar badge and the queue length agree.

### 5.4 `ListMessagesNeedingAssign`

Same treatment: one statement selecting messages whose effective `project_id IS NULL` **and** `scope = 'none'` (no override and no thread row), preserving the current predicate at `domains.go:978-982`. This path may select `body_text`, because §9 scoring needs it — but it must select it only for the rows it will actually score, bounded by the job's chunk size.

### 5.5 Indexes

New migration pair, `003_triage_indexes.sql`, in `migrate/postgres/` and `migrate/dsql/`. Filenames must match so `List` orders them identically per engine (`migrate/migrate.go:81-95`).

```sql
-- postgres/003_triage_indexes.sql
CREATE INDEX IF NOT EXISTS idx_messages_account_received ON messages(account_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_thread_assignments_account_conv ON thread_assignments(account_id, conversation_id);
```

```sql
-- dsql/003_triage_indexes.sql
CREATE INDEX ASYNC IF NOT EXISTS idx_messages_account_received ON messages(account_id, received_at DESC);
CREATE INDEX ASYNC IF NOT EXISTS idx_thread_assignments_account_conv ON thread_assignments(account_id, conversation_id);
```

Constraints from the DSQL addendum §3: no partial indexes, `CREATE INDEX ASYNC` only, migrations wait via `sys.wait_for_job`, and 24 indexes per table maximum — `messages` currently carries well under that, so this is safe.

`thread_assignments` already has `UNIQUE (account_id, conversation_id)`, which most planners will use for the join; the explicit index is belt-and-braces and may be dropped if the DSQL plan confirms the unique constraint is used. Verify on dev DSQL before promotion.

### 5.6 Client caching

- Give `QueryClient` real defaults (`web/src/App.tsx:30`): `staleTime` of 30 s for `unassigned-summary`, `retry: 1`, no refetch-on-focus for the badge.
- Assignment mutations invalidate **once**, after the mutation settles — not three sequential awaited `invalidateQueries` calls (`Triage.tsx:71-75`, `ProjectAssignControl.tsx:44-47`).

---

## 6. Queue shape

The triage queue is a list of **decisions**, so its unit is the thread.

- Mail rows collapse to one row per `(account_id, conversation_id)`, represented by the **most recent** message in that conversation.
- Messages with null/empty `conversation_id` remain their own row (consistent with Wave 1 §7, which allows only an override for them).
- Each row carries `thread_count` and `conversation_id`.
- Manual items are already one row each; unchanged.

Implemented in the same statement as §5.1 with a window function over the CTE:

```sql
ROW_NUMBER() OVER (
  PARTITION BY COALESCE(NULLIF(conversation_id, ''), id::text)
  ORDER BY received_at DESC
) AS rn,
COUNT(*) OVER (
  PARTITION BY COALESCE(NULLIF(conversation_id, ''), id::text)
) AS thread_count
```

filtered to `rn = 1`. Window functions are standard PostgreSQL and Aurora DSQL is PostgreSQL-compatible, but this is the one construct in this spec that the DSQL addendum does not explicitly bless — **confirm against dev DSQL before T2 is promoted**. Fallback if unsupported: dedupe in Go over the already-single-query result set, accepting that `LIMIT` must then over-fetch.

`thread_count` is advisory display only. It must not change the assignment semantics: `scope = "thread"` already resolves every message in the conversation.

---

## 7. Batch assignment

### 7.1 Application service

Add to `appprojects.Service`:

```go
type BatchAssignItem struct {
    Kind      string      // "message" | "manual"
    ID        uuid.UUID
    ProjectID *uuid.UUID  // nil clears
    Scope     domainprojects.AssignScope // messages only
}

type BatchAssignResult struct {
    ID    uuid.UUID
    OK    bool
    Error string // machine-readable: "not_found" | "conversation_required" | "invalid_scope" | ...
}

func (s *Service) AssignBatch(ctx context.Context, userID uuid.UUID, items []BatchAssignItem) ([]BatchAssignResult, error)
```

Rules:

- **Partial success.** One bad item must not sink the batch. Per-item results, in request order.
- **Same semantics per item** as `AssignMessage` / `AssignManualItem` — this is a loop over validated work, not a second assignment code path. Status is always `committed`, source always `user`, reason always `user_assign` (Wave 1 §7).
- **Project validation is hoisted.** Resolve the distinct `project_id` set once per batch rather than calling `GetProject` per item (`service.go:267-275`).
- **`AfterProjectCorrespondence` fires once per distinct project**, after all writes commit — not once per message. Today it is per-call (`service.go:313-317`) and would otherwise enqueue one interpret/reconcile chain per item.
- **Participant upserts are deduped** across the batch before writing.

### 7.2 Repository

Add plural methods to `driven.AssignmentRepository` (`ports/driven/persistence.go:816-829`):

```go
UpsertThreadAssignments(ctx context.Context, rows []AssignmentRow) error
UpsertMessageOverrides(ctx context.Context, rows []AssignmentRow) error
```

Both chunk at **≤ 100 items per transaction**. The DSQL cap is 3,000 mutated rows with cascades counted (`addendum-aurora-dsql.md:38-48`); each item writes one assignment row plus participant upserts, so 100 leaves a wide margin. Retry `SQLSTATE 40001` through the existing `withSerializableRetry` (`postgres/repo.go:114`).

### 7.3 Idempotency

Assignment writes are upserts keyed on `(account_id, conversation_id)` or `message_id`, so replaying a batch is naturally idempotent. No `Idempotency-Key` header is required. `AfterProjectCorrespondence` is not idempotent and must therefore fire only on the post-commit path described in §7.1.

---

## 8. HTTP API

Snake_case JSON. Auth as today. Existing single-item routes (`router.go:160-165`) are unchanged and remain the contract for Inbox and project timeline.

### 8.1 Batch assignment (T3)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/project-assignments/batch` | Assign or clear many items in one call. |

Request:

```json
{
  "items": [
    { "kind": "message", "id": "uuid", "project_id": "uuid", "scope": "thread" },
    { "kind": "message", "id": "uuid", "project_id": "uuid", "scope": "message" },
    { "kind": "manual",  "id": "uuid", "project_id": "uuid" }
  ]
}
```

Response `200`:

```json
{
  "results": [
    { "id": "uuid", "ok": true },
    { "id": "uuid", "ok": false, "error": "conversation_required" }
  ],
  "assigned": 2,
  "failed": 1
}
```

- `scope` defaults to `thread` for messages, as today.
- `project_id: null` clears the assignment.
- Maximum **200 items** per request; over that → `400 { "error": "too_many_items" }`.
- `200` with per-item failures, not `207` and not all-or-nothing. A transport or auth failure is still a normal error status.
- **`project_id` is always explicit per item.** "Confirm all suggestions" sends the project ids the operator was shown, which makes the request its own guard against a suggestion that changed server-side between render and click.

### 8.2 Unassigned list additions (T2)

`GET /api/unassigned` gains, per mail row:

| Field | Meaning |
| ----- | ------- |
| `thread_count` | Messages in this conversation currently in the queue. Always ≥ 1. |
| `confidence` | Scorer confidence, `null` when the row was never scored. |

`project_id`, `reason` and `source` are already emitted (`http/projects.go:296-327`) and become load-bearing.

### 8.3 Rescan (T5)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/unassigned/rescan` | `{ "account_id?" }` → enqueues `assign_projects`; returns the run id. |

Gives the operator a way to re-run inference after editing project codes/keywords, and a visible run to inspect when it fails. Returns `503 { "error": "assignment not configured" }` when the executor is unregistered.

---

## 9. Scoring

Wave 1 §9's rule order is preserved as the **deterministic tier**; new signals extend it, and the LLM runs only on what the deterministic tier leaves unscored.

### 9.1 Ranked candidates, not all-or-nothing

`tryAssignOne` currently discards multi-hit matches (`service.go:672-674`, `service.go:706-708`). Replace the boolean outcome with a ranked candidate list:

```go
type candidate struct {
    Project    driven.ProjectRow
    Confidence float64
    Reason     string
}
```

Policy (Wave 1 §7, unchanged in effect):

| Condition | Outcome |
| --------- | ------- |
| Committed sibling in the same conversation | `committed`, `source = rule`, `reason = thread_sibling` |
| Exactly one project **code** token, confidence ≥ 0.9 | `committed`, `source = rule` |
| Anything else with a top candidate ≥ the provisional floor | `provisional` |
| No candidate above the floor | unassigned |

Ambiguity now produces the **top-ranked provisional** rather than silence. The operator sees a suggestion with a stated reason and can reject it in one click, which is strictly better than an unscored row.

### 9.2 New deterministic signals

Both are cheap, need no LLM, and are computable from tables already populated.

| Signal | Basis | Notes |
| ------ | ----- | ----- |
| **Sender-domain affinity** | Committed assignments grouped by the sender's email domain | A contractor domain that has only ever been filed under DC01 is strong evidence. Requires a minimum support count before it scores; a single prior assignment is not a pattern. |
| **Participant overlap** | `correspondence_participants` ∩ `project_participants` | Already populated by contact resolution and `upsertParticipants` (`service.go:714-725`). |

Both must degrade to "no candidate" on a cold organisation rather than guessing from one data point.

### 9.3 LLM tier (T5)

Add `LLM driven.LLMClient` to `AssignService` and wire it in the `llmClient != nil` block of `composition/app.go:413-445`. `AssignService` must keep working with a nil `LLM` — deterministic tiers only — exactly as `forwardRulesSvc` does today (`app.go:446-456`).

- **Closed set, not an open question.** The model picks from the supplied candidate projects or returns nothing. It never invents a project.
- **Thread-level.** Score one representative message per conversation, not every message.
- **Batched.** 10–25 threads per call, honouring the in-handler batching and semaphore rule (`addendum-redis-asynq-jobs.md:129-131`) and the registered `MaxChunk: 25` for `assign_projects` (`jobs/registry.go:124`).
- **Deterministic tier short-circuits first.** Anything already committed by rule never reaches the LLM.
- **Output is always `provisional`** with `source = 'llm'`. Per Wave 1 §7, `committed` requires confidence ≥ 0.9 **and** a project **code** token match — which is the rule tier, not the LLM tier. The LLM never auto-commits.
- Reuse the JSON parse + repair-retry + fence-strip pattern from `classifyMessage` (`messages/categorize.go:148-198`) and `normalizeJSONContent`.

Budget: scoring 2,000 threads one call at a time is neither fast nor affordable. Batching plus thread-level scoring reduces a full mailbox rescan by roughly an order of magnitude against the naive shape.

---

## 10. LLM contract

Input per call: the candidate project list (code, name, description, client, keywords) and up to 25 thread digests (subject, sender name + domain, recipient count, clamped snippet). Reuse the clamp limits from `classifyMessage` (`categorize.go:150-153`).

Response:

```json
{
  "schema_version": 1,
  "assignments": [
    {
      "ref": "t1",
      "project_code": "DC01",
      "confidence": 0.0,
      "reason": ""
    }
  ]
}
```

- `ref` is an **opaque per-call label** the server assigns to each thread digest. The model never sees or returns a uuid, which removes uuid hallucination as a failure mode and cuts prompt tokens.
- Unknown `ref` values and unknown/archived `project_code` values are **dropped server-side** before any write — the same defensive filtering as `filterCitations` (`projectai/service.go:538-576`).
- Omitting a thread from `assignments` means "no confident project". The model must be instructed to omit rather than guess.
- `reason` is persisted to `assignment_reason` and shown in the UI, so it must be a short human-readable phrase, not a chain of thought.

---

## 11. UI contract (`web/`)

Routes are unchanged. All changes are within `web/src/pages/Triage.tsx` and its row component, plus `QueryClient` defaults.

### 11.1 Suggestions (T2)

- The project select initialises from `item.project_id` — `useState(item.project_id ?? "")`.
- Provisional rows gain a **primary `Confirm` action** labelled with the target, e.g. `Confirm → DC01 · Riverside Plant`. The select remains as the override path.
- Rows show provenance: source badge (`rule` / `llm`), confidence when present, and the `reason` phrase. Today `reason` renders as the raw `name_or_keyword:DC01` string; render it as a sentence.
- Rows show `thread_count` when > 1, so "Assign thread" visibly resolves more than one item.

### 11.2 Batch (T3)

- Checkbox per row, header select-all per section, **shift-click range selection**.
- Sticky action bar when a selection exists: `[12 selected] [Choose project ▾] [Assign thread] [This message only] [Clear selection]`.
- **Confirm all suggestions** — one action accepting every provisional row in the section at its suggested project.
- **Group by suggested project** in the "Needs confirmation" section, so the interaction is "14 items look like DC01 — accept all, minus these three" rather than a flat list.
- Optimistic removal of assigned rows, with rollback and a toast on per-item failure. Report partial failures explicitly: `10 assigned, 2 failed`.

### 11.3 Keyboard (T3)

`j` / `k` move, `space` selects, `shift+j/k` extends, `enter` confirms the suggestion, `1`–`9` assign to the *n*th most recently used project, `u` undoes the last batch.

### 11.4 Accessibility

Selection state must be announced (`aria-selected`, live region for the selection count). The action bar is reachable by tab order from the list, not trapped. Confirm actions name their target in the accessible label, not only in colour or position.

---

## 12. Jobs and observability

| job_type | Change |
| -------- | ------ |
| `assign_projects` | Gains the LLM tier (§9.3). Remains `ModeStreamed`, `MaxChunk: 25`. |

- **Move `AssignAfterSync` off the sync request path.** `messages/sync.go:336-338` calls it inline; enqueue `assign_projects` for the account instead. Sync latency should not include assignment, and §5.4's query is no longer cheap enough to hide.
- **Stop reporting success on failure.** `AssignAfterSync` must count per-message errors and mark the run `failed` when any occurred, with the count in `meta_json` (`service.go:653-665`). Today errors `continue` and the run is unconditionally `"success"`.
- Run meta carries `{ "threads_considered", "committed_rule", "provisional_rule", "provisional_llm", "unscored", "errors" }` so a run that scored nothing is distinguishable from a run that failed.
- **Fix the cursor.** `assign_projects` declares `CursorKind: CursorMessageKeyset` but encodes an integer offset (`jobkit/helpers.go`, `service.go:583-617`). When rows are removed above the cursor the next chunk skips unscanned messages (§2.6). Implement a real `(received_at, id)` keyset rather than re-declaring the cursor kind.

---

## 13. Slice exit criteria

**T1.** `GET /api/unassigned?limit=100` and `GET /api/unassigned/summary` each issue a **constant** number of statements regardless of mailbox size, verifiable by query counter in the repository contract test (the home-org lookup plus one set-based query; mail and manual items are unioned into that single statement). No statement on either path selects `body_text`. Wall-clock behaviour against the 30 s API Lambda timeout is a property of the hosted cluster, not of the test suite: the contract test asserts the statement count that makes it achievable, and the DSQL plan must be confirmed on dev before promotion.

**T2.** A provisional row renders its suggested project name and a `Confirm` button; clicking it assigns without touching the select. A 20-message unassigned thread appears as **one** row showing `20 messages`. The sidebar badge equals the number of rows in the queue.

**T3.** Selecting 12 rows and assigning to one project issues **one** HTTP request and resolves all 12. A batch containing one message with no `conversation_id` at `scope = "thread"` returns `ok: true` for the other 11 and `conversation_required` for that one. Interpret/reconcile is enqueued once per distinct project, not once per item.

**T4.** Mail from a sender domain previously filed under DC01 three or more times, with no code or keyword hit, arrives `provisional` on DC01 with a stated reason. An organisation with one project and no history produces no false suggestion.

**T5.** With `LLM` configured, a thread with no code, keyword, domain or participant hit but obvious topical content arrives `provisional` with `source = 'llm'`, a confidence, and a readable reason. With `LLM` nil, assignment still runs and commits code-token matches. No LLM suggestion is ever written `committed`. A response naming an unknown `project_code` writes nothing.

---

## 14. Testing

- Effective-assignment parity: the §5.1 query and `EffectiveAssignment` agree on every permutation of {override present/absent × project null/set × thread present/absent × conversation empty/set}. Table-driven, run against both adapters.
- Override with `project_id IS NULL` on an assigned thread yields `unassigned`, not the thread's project.
- Status filter and pagination are correct at `offset` beyond the first page — the current implementation is wrong here and the test must fail before the fix.
- Query-count assertion on `ListUnassigned` and `CountUnassignedSummary` to prevent N+1 regression.
- Thread dedupe: N messages in one conversation yield one row with `thread_count = N`; null-conversation messages are never merged with each other.
- Batch: partial failure, per-item ordering, 200-item cap, chunk boundary at 100, `AfterProjectCorrespondence` called once per distinct project, replay is idempotent.
- Scoring: ranked candidates are deterministic given equal inputs; no suggestion below the provisional floor; archived projects are never candidates (`matchProjectCodes` already skips them, `service.go:736-738`).
- LLM: unknown `ref` dropped; unknown `project_code` dropped; malformed JSON repaired once then abandoned; a nil `LLM` leaves deterministic behaviour unchanged.
- `assign_projects` marks a run `failed` when any message errors.
- Existing Wave 1 tests must pass unchanged **except** `TestAutoAssignSiblingCodeNameAmbiguous`, which asserts the pre-§9.1 rule that ambiguity produces no suggestion. That is the one assignment semantic this spec deliberately changes; the test is updated to assert the new contract — ambiguity yields the top ranked **provisional** candidate and still never auto-commits.

---

## 15. Risks and open decisions

| Risk | Mitigation |
| ---- | ---------- |
| Window function unsupported or slow on Aurora DSQL | Validated against PostgreSQL 16 in the contract suite, so the syntax and semantics are sound. DSQL itself is still unverified — §6 fallback stands: dedupe in Go over the single-query result. Confirm on dev DSQL before promoting T2. |
| The §5.1 plan degrades on DSQL's distributed executor | Measure with the dev cluster before promotion; §5.5 indexes exist precisely for this join shape. |
| Sender-domain affinity over-fits on a young organisation | Minimum support threshold; §13 T4 tests the cold case explicitly. |
| LLM cost on a full rescan | Thread-level scoring + batching + deterministic short-circuit; rescan is operator-triggered (§8.3), not automatic. |
| Operators batch-assign carelessly once it is fast | `u` to undo the last batch (§11.3); explicit partial-failure reporting; assignment remains reversible by design. |

**Open:** whether `thread_count` should count all messages in the conversation or only those currently unassigned. This spec says **only those in the queue**, so the number matches what disappears on click. Revisit if operators read it as thread length.

---

## 16. Implementation record

Implemented across three commits on `feat/triage-efficiency`. Deviations from
the spec as drafted, and findings that surfaced while building it.

### 16.1 Deviations

| Spec | What was built | Why |
| ---- | -------------- | --- |
| §5.3 "one statement for the mail side and one for manual items" | **One** statement total — mail and manual items are unioned inside the same CTE chain | Merging two result sets in Go cannot paginate correctly, which is the bug §5.1 sets out to fix |
| §5.6 "give `QueryClient` real defaults … `staleTime` of 30 s" | Global default is `retry: 1` only; the stale window and `refetchOnWindowFocus: false` are scoped to the `unassigned-summary` query via `unassignedSummaryQueryOptions` | A global `staleTime` would stale-cache every screen in the app. The expensive query is the badge; only the badge needs the window |
| §9.1 ranked candidates | Added a small **corroboration bonus** so independent signals that agree outrank a single signal | Without it, two projects tying on the same signal were separated by project code sort order. Ambiguity resolved alphabetically is not a defensible suggestion |

### 16.2 Found while building

Pre-existing defects outside this spec's scope, fixed here only where they
blocked the work:

- **The postgres contract tests had never run.** `AUTOMATA_TEST_POSTGRES_DSN` is
  unset in CI, so the suite always skipped. With a database attached it
  deadlocked, then failed on a syntax error. Both are fixed (§16.3), and the
  suite now runs against PostgreSQL 16.
- **`ListProjectTimeline` deadlocked on a small pool.** It issued
  `timelineContactsForMessage` and `FindIssueIDByMessage` per row *while the
  outer result set was still streaming*, holding two connections at once. With
  `MaxOpenConns: 1` that is a guaranteed self-deadlock; the factory defaults
  postgres/DSQL to 3 (`factory.go:77-83`), so production survived on pool
  headroom rather than by design and could still starve under concurrency.
  **Fixed — see §17.**

### 16.3 Test-harness changes

- Added a statement-counting SQL driver wrapper (`persistencetest/counting.go`)
  so the N+1 guard in §13 T1 is enforceable, and so §5.2 ("never read bodies")
  is assertable rather than aspirational.
- Raised the postgres contract pool from 1 to 3 to match production, and moved
  `search_path` onto the DSN. A session-level `SET search_path` only binds the
  connection that ran it, so with a multi-connection pool every test was
  silently writing into `public` instead of its own schema.
- Fixed `ensureLegacyJobRunIfPresent`, which wrote raw `?` placeholders straight
  to the handle, bypassing the repository's placeholder rewriting and failing
  every postgres contract run on syntax.

### 16.4 Verification

| Check | Result |
| ----- | ------ |
| `go build ./...`, `go vet ./...` | clean |
| `go test ./...` (SQLite) | pass |
| Postgres contract suite (PostgreSQL 16, real database) | pass |
| `npm run lint` | 0 errors (8 pre-existing fast-refresh warnings in `ui/`) |
| `npm run test` | 43 pass |
| `npm run build` | pass |
| Aurora DSQL | Partially verified via the PR #9 dev deploy — see §18 |


---

## 17. Timeline hydration (follow-up)

Found while implementing §16.2 and fixed in the same branch. Out of the
original scope — this is the project timeline, not triage — but the same
defect class as §5.1, and a hard hang rather than a slow page.

### 17.1 The defect

`ListProjectTimeline` built each item inside the loop that was still streaming
the outer result set, calling `timelineContactsForMessage` and
`FindIssueIDByMessage` per row. A query issued while a cursor is open needs a
second connection, and the first is not released until its rows are drained:

| Pool size | Behaviour |
| --------- | --------- |
| 1 | Hangs forever. The nested query waits on a connection the outer query will not release. |
| 3 (postgres/DSQL default) | Works for a single caller; three concurrent timeline requests can hold all three connections and starve each other. |

It was also an N+1: up to 500 mail rows plus every manual item, two statements
each.

### 17.2 The fix

Hydration moved out of the streaming loop and into a bulk pass that runs once
per request, after every source has contributed its rows:

1. Each source (mail, manual, connector) produces bare `TimelineItem`s and
   closes its cursor.
2. `hydrateTimelineItems` collects the message and manual-item ids, then
   resolves participants and issue links with one statement per relation,
   batched with `IN` lists chunked at 200 ids.
3. The `unassigned_to_issue` filter runs after hydration, since it depends on
   the issue links.

Statement count goes from `2N + 2M` to at most four (plus a chunk per 200 ids),
and no statement is ever issued while a cursor is open.

The per-row helpers (`timelineContactsForMessage`, `timelineContactsForManual`)
are deleted rather than left in place, so the pattern cannot be reintroduced by
reaching for the convenient function.

Shared helpers live in `persistence/sqlkit` so the two adapters cannot drift.

### 17.3 Tests

- `TestProjectTimelineOnSingleConnectionPool` (both adapters) pins the pool to
  one connection and fails on a deadline rather than hanging CI. Verified to
  reproduce the original defect: it hangs for the full 15 s and fails against
  the pre-fix code, and passes in ~20 ms after.
- `timeline_hydration_is_batched_and_correct` in the shared contract suite
  asserts contacts and issue links are attached to the right items across mail
  and manual sources, that `unassigned_to_issue` still filters correctly, and
  that the whole call stays within a bounded statement count.
- `sqlkit` has unit tests for the chunking and placeholder helpers, including
  that an empty id set issues no statement at all rather than an `IN ()`.

### 17.4 Not addressed

The connector/slack branch still calls `GetConnectorAccount` once per distinct
connector account. That runs after its cursor is closed, so it is not a
deadlock risk, and it is bounded by the number of connected accounts rather
than by timeline length. Left alone deliberately.


---

## 18. Aurora DSQL findings

The PR opened for this work applies to dev, which ran migration `003` against
a real DSQL cluster — the verification §16.4 recorded as missing.

### 18.1 No sort order on index keys

`003_triage_indexes.sql` failed the dev deploy:

```
dsql/003_triage_indexes.sql: ERROR: specifying sort order not supported
for index keys (SQLSTATE 0A000)
```

DSQL's `CREATE INDEX` grammar has no `ASC`/`DESC` — only `NULLS FIRST|LAST`.
Vanilla Postgres accepts `received_at DESC`, which is why it passed both the
local contract suite and CI.

Fixed by dropping the keyword from the `dsql/` migration only; the `postgres/`
one keeps it. An ascending index still serves `ORDER BY received_at DESC`, so
§5.5's intent is unchanged.

Because CI has no DSQL cluster, `TestDSQLIndexesHaveNoSortOrder` and
`TestDSQLIndexesAreAsync` now lint the `dsql/*.sql` set for this grammar, with
`TestPostgresIndexesAreNotAsync` as the mirror. Verified to reproduce the
failure: re-adding `DESC` fails the lint with the same diagnosis.

Recorded in [addendum-aurora-dsql.md §3.2](addendum-aurora-dsql.md), whose
limits list did not mention it.

### 18.2 A comment header changed how a migration executed

With the sort order fixed, the next deploy failed differently:

```
dsql/003_triage_indexes.sql: ERROR: multiple ddl statements not supported
in a transaction (SQLSTATE 0A000)
```

`CREATE INDEX ASYNC` returns a job id, so the migrator sends it down the query
path and then waits on `sys.wait_for_job`. That routing was decided with
`strings.HasPrefix(stmt, "CREATE INDEX ASYNC")` against the raw chunk — and
`003` is the first migration in the repo to open with a `--` comment header.
The prefix check failed, the statement fell through to the generic exec path,
and DSQL rejected it.

Postgres accepts the same statement either way, so nothing local or in CI
could see it. Every pre-existing `dsql/*.sql` file starts directly with DDL,
which is why the bug had never fired.

Fixed by classifying on the statement body rather than the raw chunk:
`classifyStatements` strips leading blank lines and `--` comments, drops
comment-only chunks, and marks async indexes. `applyStatements` iterates its
output, so documenting a migration can no longer change how it runs.

`TestDSQLAsyncIndexesAreClassifiedAsync` asserts every index statement in the
`dsql/` set classifies async, and `TestClassifyStatementsIgnoresCommentHeaders`
pins the behaviour directly. Both verified to fail against the pre-fix
classification.

### 18.3 Still unverified

| Construct | Status |
| --------- | ------ |
| Window functions (§6 thread dedupe) | Documented as supported in [addendum-aurora-dsql.md §2](addendum-aurora-dsql.md); not yet executed against DSQL |
| Row-value comparison `(received_at, id) < (?, ?)` (§12 keyset) | Not documented either way; not yet executed against DSQL |
| `UNION ALL` with casts in branches (§5.1) | `UNION` documented as supported; this exact shape not executed |
| The §5.1 query plan | Unknown — needs `EXPLAIN` on dev once the deploy is green |

These only execute at request time, so a successful migration does not
exercise them. They need a dev smoke test of `/api/unassigned` and a project
timeline after deploy.
