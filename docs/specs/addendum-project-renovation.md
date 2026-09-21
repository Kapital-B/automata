# Spec Addendum: Project Renovation

**Status:** Draft  
**Parent PRD:** [Project correspondence intelligence — Wave 2](../prds/addendum-project-correspondence-wave2.md)  
**Related spec:** [Wave 2](addendum-project-correspondence-wave2.md) §7 (two-stage pipeline), [Triage efficiency](addendum-triage-efficiency.md), [Home Overview](addendum-home-overview.md)  
**Last updated:** 2026-09-21  

Correspondence reaches a project reliably now. What happens next does not: the extraction pipeline stops halfway, the UI names its own internals, and nothing explains what facts, issues, decisions and contradictions are or where they come from.

This completes the pipeline, adds issue extraction, and renovates the project page around the question it should answer: **where does this project stand, and what needs me?**

---

## 1. The problem

### 1.1 The pipeline stops halfway

Assigning correspondence fires `AfterProjectCorrespondence`, which calls `interpretSvc.TryRunBestEffort` — **synchronous, inline, per item, and silent**: it discards both result and error (`_, _ = s.Run(...)`).

That produces *interpretations*: an intermediate, pending state.

Then nothing. **`reconcile_project` is never enqueued anywhere.** It is a registered job type and an HTTP endpoint, but no code path triggers it. Interpretations accumulate as `pending` and never become facts or decisions.

So the honest answer to "how does correspondence become a fact?" is: *it does not, unless the operator finds the Reconcile button.*

### 1.2 The UI names internals

`ProjectDetail` has buttons labelled **Interpret** and **Reconcile**, one tooltipped *"Apply pending interpretations (Stage B)"*. Those are this spec family's own stage names. **Interpretations** are rendered as a first-class list.

An interpretation is a pipeline artefact. The operator should never need the word.

### 1.3 Issues are not extracted

Interpret emits only `fact` and `decision` candidates (`domain/interpretations`: `KindFact`, `KindDecision`). Issues have a separate, manual, one-at-a-time *Suggest issue* button.

### 1.4 Nothing explains the model

Current position is **derived** — active facts plus accepted decisions — but nothing says so, so the causal link between confirming a fact and the position changing is invisible. Nor does anything distinguish a fact from an issue.

---

## 2. Decisions taken

Answered by the operator on 2026-09-21.

### 2.1 Auto-apply the safe outcomes, confirm the rest

**This is already what `reconcile` does.** The service classifies each candidate and acts:

| Situation | Outcome | Today's behaviour | Needs a human? |
| --------- | ------- | ----------------- | -------------- |
| Fact, no prior subject | `confirm_new` | Created **active** (`Confirm: true`) | No |
| Fact, compatible value | `reinforce` | Evidence attached to the active version | No |
| Fact, incompatible, confidence ≥ 0.7 | `supersede` | New version created **proposed** | **Yes** |
| Fact, incompatible, confidence < 0.7 | `contradiction` | Contradiction opened; no winner picked | **Yes** |
| Decision, matches an accepted one | `reinforce` | Evidence attached | No |
| Decision, new | `confirm_new` | Created **proposed** (`Confirm: false`) | **Yes** |

The policy therefore needs **no change**. What is missing is that reconcile never runs. That makes this renovation mostly wiring, not new judgement.

**Deliberate asymmetry:** a brand-new *fact* is auto-applied, a brand-new *decision* is not. A fact is a value that can be superseded with evidence; a decision asserts that the project *committed* to something, which is a claim about authority. Being wrong about the second is worse and harder to walk back. This asymmetry is pre-existing and is being kept on purpose.

### 2.2 Debounce: one minute of quiet, five minute ceiling

Extraction runs when **no correspondence has been assigned to the project for one minute**, or when the oldest unextracted assignment reaches **five minutes**, whichever comes first.

The ceiling stops a steady trickle deferring extraction indefinitely.

### 2.3 The AI creates issues; a human discards them

Issues are extracted and created `open`, not proposed. Discarding is one action.

An issue is a *prompt to look at something*, not an assertion about what is true, so a wrong one costs attention rather than correctness — the opposite trade-off from a decision (§2.1). Auto-creating them is also what makes the project page useful without the operator curating it.

---

## 3. Non-goals

- Changing what a fact, decision, issue or contradiction *is*. Wave 2's model is sound; it is unexplained, not wrong.
- Autonomous supersession or auto-resolving contradictions. Those stay operator-gated.
- Turning issues into a task tracker.
- Replacing the timeline. The trail stays the history of record.
- Reworking triage, Home or the sidebar.
- Extracting from correspondence that was never assigned to a project.

---

## 4. Slices

| Slice | Name | Delivers |
| ----- | ---- | -------- |
| **R1** | Complete the pipeline | §5 — chain interpret→reconcile, off the request path, failures visible |
| **R2** | Debounce | §6 — the scheduler pass and `last_extracted_at` |
| **R3** | Issue extraction | §7 — the third candidate kind |
| **R4** | Project UI | §8 — review surface, provenance, the verbs removed |

R4 depends on R1: no UI can feel right while the pipeline does not complete. R2 and R3 are independent of each other.

---

## 5. Completing the pipeline

### 5.1 Chain the two stages

`interpret_project` and `reconcile_project` are both registered `ModeStreamed`, so `Enqueuer.EnqueueChain` accepts `["interpret_project", "reconcile_project"]` as-is. Extraction becomes that chain, carrying `payload.project_id`.

### 5.2 Off the request path

`AfterProjectCorrespondence` stops calling interpret inline. Assignment records that the project has unextracted correspondence (§6.1) and returns; the scheduler enqueues the chain.

Assignment latency stops including LLM calls — the same correction made for `assign_projects` in [triage-efficiency §12](addendum-triage-efficiency.md).

### 5.3 Failures become visible

`TryRunBestEffort` discards its error. As a job, a failed extraction is a failed run with a message, visible in Runs and countable — the same correction as the assignment runs.

`TryRunBestEffort` is removed rather than left as an unused alternative path.

---

## 6. Debounce

### 6.1 One column of state

```sql
ALTER TABLE projects ADD COLUMN IF NOT EXISTS last_extracted_at TIMESTAMPTZ;
```

Set when an extraction chain completes. Nullable; NULL means never extracted, which is correct for every existing row, so no backfill.

**No new table and no queue mutation API.** Whether a project is due is *derived* from data already present: assignment rows carry `updated_at`, so "correspondence assigned since the last extraction" is a query, not a counter that can drift. This is the same reasoning as deriving `last_activity_at` in [home-overview §8.2](addendum-home-overview.md).

### 6.2 The scheduler pass

The scheduler already runs every minute (`rate(1 minute)`, `SchedulerService.Tick`). It gains a pass that enqueues extraction for due projects.

A project is **due** when it has at least one assignment newer than `last_extracted_at`, and either:

- **quiet** — the newest such assignment is at least **1 minute** old, or
- **ceiling** — the oldest such assignment is at least **5 minutes** old.

One query returns the due project ids, membership-irrelevant (this is system work, not a user view). Assignments here means all three carriers: thread assignments, message overrides, and manual items.

Correspondence marked not-project-related ([not-relevant §5.1](addendum-triage-not-relevant.md)) has no project and therefore cannot make one due.

### 6.3 Why the scheduler rather than the job itself

`ScheduledFor` on a pending job is **not** a "do not run before" gate — the worker fires on the job item write, and `RetryNotBefore` is the only claim-time gate, currently set solely by retry backoff. Deferring via the queue would mean either a new store mutation or re-enqueueing from inside a job that still holds its own lock.

Deriving due-ness on a tick avoids all of that, is idempotent, and self-heals: a missed tick simply runs the next minute.

### 6.4 Coalescing

Extraction is enqueued with the lock scope `project` keyed by project id, so a chain already pending or running for a project blocks a second. A burst of fifty assignments produces one extraction run.

---

## 7. Issue extraction

### 7.1 A third candidate kind

`domaininterpretations.CandidateKind` gains `issue`. The interpret prompt is extended to emit it, and must draw the distinction explicitly, because it is the distinction the operator is also missing:

- **fact** — a value that is currently true ("duty is 90 kW")
- **decision** — a choice that was made ("proceed with 90 kW")
- **issue** — an open question or piece of work ("the P-03 seal is leaking")

An issue candidate carries a title, an optional opening note, and evidence refs.

### 7.2 Reconcile applies it

Reconcile gains an issue branch:

| Situation | Outcome |
| --------- | ------- |
| No open issue with a similar title | Create `open`, `source = llm`, evidence attached as issue items |
| An open issue already matches | `reinforce` — attach the evidence to it, create nothing |

Matching reuses the normalisation already used for decision statements. Resolved issues are not reopened by extraction; a recurrence creates a new issue rather than disturbing a closed one.

### 7.3 Discarding

`POST /api/issues/{id}/discard` sets status `discarded`.

`discarded` is a new status value rather than a reuse of `resolved`: resolving means the work is done, discarding means it should never have been raised. Conflating them corrupts the signal for whether extraction is useful.

This widens a CHECK constraint, which on DSQL means dropping an unnamed constraint by its generated name ([aurora-dsql §3.2](addendum-aurora-dsql.md)) — the hazard that broke PR #9 twice. **Therefore:** `discarded_at TIMESTAMPTZ` is added instead, additive, matching [not-relevant §4.1](addendum-triage-not-relevant.md). A discarded issue is one with `discarded_at` set; it is excluded from open issues, attention, and the overview counts.

---

## 8. UI contract (`web/`)

### 8.1 The verbs go

**Interpret** and **Reconcile** buttons are removed, and interpretations are no longer rendered.

They are replaced by a status line — *"Reviewed 4 minutes ago"*, or *"Reviewing…"* while a chain is in flight — with a **Check now** action for impatience, which enqueues the chain immediately.

### 8.2 One review surface

Everything awaiting the operator on this project collapses into a single **Needs your confirmation** panel at the top: proposed fact versions, proposed decisions, open contradictions. Each row states what it is, what it would change, and what it came from, with confirm and reject inline.

Today these are scattered across two tabs and mixed with the things that need no attention.

### 8.3 Provenance everywhere

Every fact, decision and issue shows what produced it — *"from 3 messages"*, linking to the evidence — and whether it came from a person or the model. With extraction now automatic, "where did this come from?" becomes the first question, and it should be answered structurally rather than in documentation.

### 8.4 Teaching the model

Empty states carry the definition rather than an apology:

> **Facts** are values that are currently true about this project. They are extracted from correspondence filed here, and supersede each other as things change.

The same for decisions, issues and contradictions. The **Current position** panel states that it is derived from confirmed facts and accepted decisions.

### 8.5 Structure

`ProjectDetail.tsx` is 1,814 lines and issues 11 queries. The renovation splits it per mode and collapses the fetching, as Home was.

---

## 9. Slice exit criteria

**R1.** Assigning correspondence enqueues one extraction chain; the assign response does not wait for an LLM call. A fact for a new subject appears **active** without any operator action. A proposed supersession appears for an incompatible value. A failing extraction is a `failed` run with a message, not silence.

**R2.** Fifty assignments in one burst produce one extraction run. A project assigned continuously for ten minutes extracts at the five-minute ceiling rather than waiting for quiet. A project with no new correspondence is never enqueued.

**R3.** A thread describing a problem produces an open issue with its evidence attached. The same problem restated attaches evidence to the existing issue rather than creating a second. Discarding removes it from open issues and from attention, and it is not recreated by the next extraction.

**R4.** The words "interpret", "reconcile" and "interpretation" do not appear in the UI. Everything awaiting confirmation on a project is in one place. Every fact, decision and issue shows its evidence count and source.

---

## 10. Testing

- Chain: assignment enqueues `[interpret_project, reconcile_project]` once, with the project id; the assign path issues no LLM call.
- Auto-apply: new subject → active fact; compatible value → evidence attached and no new version; incompatible + high confidence → proposed, not active; incompatible + low confidence → open contradiction and no winner.
- Decisions stay proposed (§2.1), asserted explicitly so the asymmetry cannot be "tidied" away.
- Debounce: due when quiet for a minute; due at the ceiling under continuous assignment; not due without new correspondence; not due from correspondence marked not-project-related; one chain per project while one is pending.
- `last_extracted_at` advances on success, and does **not** advance on failure, so a failed run retries rather than skipping the window.
- Issues: created from a candidate; a duplicate reinforces; resolved issues are not reopened; discarded issues stay discarded across a later extraction and leave attention.
- Contract coverage on both adapters; query-count guards must not regress.
- DSQL lints cover the new migration.

---

## 11. Risks

| Risk | Mitigation |
| ---- | ---------- |
| Auto-applied facts are wrong | They are versioned with evidence and supersedable; §8.3 makes provenance visible so a bad one is traceable. Only the safe outcomes auto-apply (§2.1). |
| Auto-created issues become noise | Duplicates reinforce rather than accumulate; discard is one action; §10 asserts a discarded issue is not recreated. Revisit if the discard rate is high — that is the signal extraction is wrong, which is why `discarded` is distinct from `resolved`. |
| The confirmation panel becomes a second triage queue | Only supersede, new decisions and contradictions land there. If it still grows, the lever is the confidence threshold for supersede, not more auto-apply. |
| Extraction cost rises with volume | Debounce coalesces bursts; the chain is chunked by the existing registry boundaries. |
| A project is never extracted because the tick keeps missing it | Due-ness is derived, not a flag, so a missed tick self-heals. `last_extracted_at` only advances on success. |
