# Spec Addendum: Not-Project-Related Correspondence

**Status:** Draft  
**Parent PRD:** [Project correspondence intelligence](../prds/addendum-project-correspondence.md)  
**Related spec:** [Wave 1](addendum-project-correspondence-wave1.md) §7 (effective assignment), [Triage efficiency](addendum-triage-efficiency.md) §5–§9, [Aurora DSQL](addendum-aurora-dsql.md) §3.2  
**Last updated:** 2026-09-20  

Not all correspondence belongs to a project. Triage has no way to say so, so anything non-project-work stays in the queue permanently. This adds an explicit "not project-related" decision, a way to review those items, and a way to reverse it.

---

## 1. The problem

Wave 1 §7 defines `project_id = NULL` as **Unassigned** — which is exactly the state that keeps an item *in* the triage queue. Clearing an assignment and declaring something non-project-work are the same value today, so the second cannot be expressed.

The effect compounds with the LLM tier ([triage-efficiency §9.3](addendum-triage-efficiency.md)): every newsletter, vendor blast and internal notice is scored, fails to match, and sits in the queue forever. The queue never reaches zero, which is what makes an operator stop trusting it.

---

## 2. Scope

- A third assignment outcome: **not project-related**, recorded with the time it was decided.
- Applying it from triage, individually and in bulk, at thread or message scope.
- Listing those items, and reversing the decision or assigning a project directly from that list.
- Keeping them out of the queue, the badge counts, and the auto-assign candidate set.

---

## 3. Non-goals

- Deletion. The correspondence is retained; only its assignment decision changes.
- Rules that auto-mark future mail as not-relevant. This is an operator decision; the scorer proposes projects, it does not propose dismissal.
- Resurfacing a marked thread when the scorer later finds a project-code match. That would undo an explicit decision; the restore path (§6.2) exists for that.
- A separate retention or archive policy.
- Any change to what "Unassigned" means for mail that simply has not been triaged yet.

---

## 4. Decisions taken

### 4.1 An additive column, not a new status value

The natural model is a third `status` value alongside `committed` and `provisional`. It is rejected on deployment risk.

Every CHECK constraint in the baseline schema is written inline and therefore unnamed — there are zero `CONSTRAINT` keywords in `common/001_baseline.sql`. Widening `status IN ('committed','provisional')` on Aurora DSQL would mean dropping a constraint by its auto-generated name and re-adding it with `NOT VALID` followed by `ALTER TABLE ASYNC … VALIDATE CONSTRAINT`. Two DSQL DDL incompatibilities already reached dev on PR #9 ([triage-efficiency §18](addendum-triage-efficiency.md)); guessing a constraint name is not worth a third.

Instead: a nullable `not_relevant_at TIMESTAMPTZ` on each assignment carrier. `ADD COLUMN IF NOT EXISTS` is within the documented DSQL grammar and was exercised successfully by [home-overview §8.1](addendum-home-overview.md).

It is equally expressive and records *when* the decision was made, which the status value would not.

### 4.2 Thread scope by default

Mirrors assignment: thread by default, with "this message only" available. The common case is that an entire correspondence is not project work.

### 4.3 Behind a filter, not a third section

The triage queue shows "Needs confirmation" and "Needs filing". Marked items grow without bound and represent finished work, so they belong behind a filter with a count, not in a section competing with work that needs doing.

### 4.4 Wording

The UI says **"Not project-related"**. The informal name for this feature is "blackholing"; that term does not appear in the product surface.

---

## 5. Data model

### 5.1 State

An assignment row means:

| `project_id` | `not_relevant_at` | Meaning |
| ------------ | ----------------- | ------- |
| set | NULL | Assigned to that project |
| NULL | NULL | Unassigned — in the triage queue |
| NULL | set | **Not project-related** — decided, out of the queue |
| set | set | Invalid; §5.3 |

### 5.2 Columns

Added nullable to all three carriers of an assignment decision:

```sql
ALTER TABLE thread_assignments ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
ALTER TABLE message_assignment_overrides ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
ALTER TABLE manual_items ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
```

`manual_items` carries its own assignment inline (Wave 1 §7) and needs the same treatment; its `assignment_status` CHECK already permits `unassigned` and is left untouched.

No backfill: NULL is correct for every existing row.

### 5.3 Invariant

Assigning a project **clears** `not_relevant_at` in the same write. An item cannot be both filed and dismissed, and the application is responsible for that — no CHECK is added, consistent with §4.1.

### 5.4 Effective assignment

`EffectiveAssignment` gains `NotRelevantAt *time.Time`, resolved with the same override-then-thread precedence as everything else in Wave 1 §7. A message-scope decision therefore overrides its thread's, in both directions.

---

## 6. Behaviour

### 6.1 Marking

Thread scope upserts `thread_assignments` with `project_id = NULL`, `not_relevant_at = now`, `status = 'committed'`, `source = 'user'`, `reason = 'not_relevant'`. Message scope does the same on `message_assignment_overrides`. Manual items set the column directly.

`status = 'committed'` reads oddly for a row with no project. It means the decision is settled rather than provisional, which is exactly right: the operator committed to "no project".

### 6.2 Reversing

Two routes out, both from the marked list:

- **Restore to queue** — clear `not_relevant_at`, leaving `project_id` NULL. The item returns to "Needs filing" and becomes an auto-assign candidate again.
- **Assign a project** — the normal assignment path, which clears `not_relevant_at` per §5.3.

### 6.3 Consequences that need no new code

Three fall out of the existing model and must be asserted rather than implemented:

1. **Later replies stay marked.** New mail in a marked conversation inherits the thread assignment, so it never enters triage. This is the point of thread scope — a newsletter stays dismissed.
2. **The scorer skips marked items.** `ListMessagesNeedingAssign` already requires no override row *and* no thread row; a marked item has one. Neither the rule tier nor the LLM tier re-proposes it.
3. **Sibling inference is unaffected.** `FindCommittedSiblingProject` looks for a committed sibling *with a project*; a marked row has none.

### 6.4 Counts

Marked items are excluded from `unassigned` and `provisional`, and therefore from the nav badge and the Home triage card. `UnassignedSummary` gains a separate `not_relevant` count for the filter chip.

---

## 7. HTTP API

### 7.1 Listing

`GET /api/unassigned?status=not_relevant` returns marked items, newest decision first. The existing values (`unassigned`, `provisional`, `all`) are unchanged and all exclude marked items — including `all`, which means "all of the queue", not "all rows".

Each marked row carries `not_relevant_at`.

`GET /api/unassigned/summary` gains `not_relevant`.

### 7.2 Marking and restoring

Both go through the existing batch endpoint, so bulk selection works unchanged:

```json
POST /api/project-assignments/batch
{ "items": [
    { "kind": "message", "id": "…", "not_relevant": true,  "scope": "thread" },
    { "kind": "message", "id": "…", "not_relevant": false, "scope": "thread" },
    { "kind": "manual",  "id": "…", "not_relevant": true }
] }
```

- `not_relevant: true` marks. `project_id` must be absent or null; sending both is `400 project_and_not_relevant`.
- `not_relevant: false` restores to the queue.
- Omitting the field leaves the flag untouched, so existing callers are unaffected.
- Per-item results and the 200-item cap are unchanged.

---

## 8. UI contract (`web/`)

### 8.1 Marking

- Each triage row gains a **Not project-related** action alongside its assign buttons.
- The bulk action bar gains the same, applying to the whole selection.
- Both respect the thread/message scope already chosen for assignment (§4.2).

### 8.2 The marked list

- A chip near the section headers: `Not project-related (N)`, shown only when N > 0, toggling the queue to the marked list.
- Rows show what they are, when they were marked, and two actions: **Restore to queue** and the normal project picker with **Assign**.
- Empty state explains that nothing has been dismissed rather than rendering a bare list.

### 8.3 Keyboard

`x` marks the focused row as not project-related, consistent with the single-key actions already bound in triage. `u` continues to undo the last batch, which covers an accidental `x`.

---

## 9. Slice exit criteria

**Marking.** Marking a thread removes every row of that conversation from the queue in one action, and decrements the badge by the same amount. A reply arriving afterwards does not reappear in triage. Running `assign_projects` afterwards does not re-suggest it.

**Scope.** Marking at message scope leaves the rest of the thread in the queue. Marking a thread whose message carries its own override does not silently override that message's decision.

**Listing.** `status=not_relevant` returns exactly the marked items and nothing else; `status=all` returns exactly the unmarked queue. The two sets do not intersect.

**Reversing.** Restore returns the item to "Needs filing" and to the auto-assign candidate set. Assigning a project directly from the marked list clears the flag, and the item appears on the project timeline.

**Counts.** The badge excludes marked items; the chip count equals the number of rows the filter returns.

---

## 10. Testing

- Effective assignment: `not_relevant_at` resolves with override-then-thread precedence, including a message that is marked within an assigned thread and a message assigned within a marked thread.
- Queue exclusion, and that `all` and `not_relevant` partition the rows with no overlap.
- A later message in a marked thread never enters the queue.
- `ListMessagesNeedingAssign` excludes marked items, so neither scorer tier re-proposes them.
- Assigning a project clears the flag (§5.3); restoring clears it and returns the item to the queue.
- Batch: mixed mark/restore/assign in one request, per-item results, and the `project_and_not_relevant` rejection.
- Counts exclude marked items and the new count matches the filtered list length.
- Contract coverage on both adapters; query-count guards must not regress.
- DSQL lints cover the new migration.

---

## 11. Risks

| Risk | Mitigation |
| ---- | ---------- |
| An operator marks a thread that later matters | Restore is one action from the filter, and §6.3 keeps the decision visible rather than deleting anything. |
| Thread scope dismisses more than intended | Message scope is offered alongside, exactly as for assignment; §9 asserts the distinction. |
| The marked list grows unmanageable | It is behind a filter and ordered newest-first. Paging already exists on this endpoint. |
| Marked rows leak into project timelines or Ask | They have no `project_id`, so every project-scoped read already excludes them. Asserted in §10. |
