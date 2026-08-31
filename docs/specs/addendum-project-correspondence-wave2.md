# Technical Specification: Project Correspondence Wave 2

**Status:** Draft  
**Parent PRD:** [Project correspondence Wave 2](../prds/addendum-project-correspondence-wave2.md)  
**Prerequisite:** [Wave 1 spec](addendum-project-correspondence-wave1.md)  
**Related PRD:** [Initial PRD](../prds/initial.md) · [Wave 1 product addendum](../prds/addendum-project-correspondence.md)  
**Last updated:** 2026-08-22  

This document specifies schema, APIs, jobs, LLM contracts, and UI for **Wave 2** (slices 2a–2e). The Wave 2 PRD is authoritative for product intent. This spec is authoritative for implementation invariants.

Wave 1 mail invariants still apply: `account_id` on mail-derived rows, `(account_id, provider_message_id)` uniqueness, explicit send confirmation, allowlisted forwarding, Contact ≠ Profile, no Cascade-delete of contacts/issues/facts when messages are deleted.

---

## 1. Scope

Wave 2 delivers:

- **Facts** with version history, evidence links, and supersession;
- **Decisions** with evidence and status;
- **Interpretations** (provisional candidates from correspondence);
- **Reconcile** outcomes including **contradictions**;
- **Needs My Input** derived attention API + UI chips;
- **Ask Project AI** grounded Q&A with citations.

Slices (ship in order):

| Slice | Name |
| ----- | ---- |
| **2a** | Facts + versions + Current position UI |
| **2b** | Interpret candidates from project correspondence |
| **2c** | Reconcile + contradictions |
| **2d** | Decisions + Needs My Input |
| **2e** | Ask Project AI |

---

## 2. Non-goals

- Invites UI, ACL enforcement, `contact_profile_links` writes, Path B.
- Live Slack / Teams / WhatsApp / SMS / call ingest.
- Replacing Wave 1 timeline/issues or mail action_items.
- In-place mutation of fact/decision values.
- Mixing two mail `account_id`s in one LLM call.

---

## 3. Package layout

Extend the Wave 1 bounded context:

```text
svc/internal/
  domain/
    facts/
    decisions/
    interpretations/   # optional; may live under facts
  application/
    facts/
    decisions/
    reconcile/
    projectai/
```

HTTP handlers remain thin. On the hosted/Floci path, `interpret_project` and `reconcile_project` use the bounded DynamoDB job registry in [aws-deployment.md §4.4](aws-deployment.md#44-job-contract-keys-and-registry); `project_ai` remains synchronous request/response audit through `JobExecutionPort`. Asynq/relational `job_runs` applies only to the legacy local implementation.

---

## 4. Data model

SQLite-first (Postgres-compatible types). UTC ISO-8601 text. UUIDs as text.

### 4.1 `facts`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | Stable fact identity (subject lineage). |
| `organisation_id` | UUID FK | Cascade with org. |
| `project_id` | UUID FK | Cascade with project. |
| `issue_id` | UUID FK nullable | `ON DELETE SET NULL`. |
| `subject_key` | text | Normalised key, e.g. `pump.p03.duty_kw`. Unique per `(project_id, subject_key)`. |
| `label` | text | Human title, e.g. “Pump P-03 duty”. |
| `created_at` / `updated_at` | timestamptz | |

### 4.2 `fact_versions`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `fact_id` | UUID FK | `ON DELETE CASCADE`. |
| `status` | text | CHECK `proposed\|active\|superseded\|rejected`. |
| `value_json` | text | Structured value (number/string/object). |
| `value_text` | text | Display denorm / search. |
| `unit` | text nullable | Optional (`kW`, …). |
| `source` | text | CHECK `user\|rule\|llm`. |
| `confidence` | real nullable | |
| `interpretation_id` | UUID nullable | FK when created from interpret. |
| `supersedes_version_id` | UUID nullable | Prior version this replaces. |
| `superseded_by_version_id` | UUID nullable | Set when this version is superseded. |
| `superseded_at` | timestamptz nullable | |
| `created_by_user_id` | UUID nullable | Profile who confirmed/created. |
| `created_at` | timestamptz | |

Logical invariant: at most one `status = active` per `fact_id`. SQLite/Postgres may use a partial unique index; hosted DSQL uses generated `active_fact_id = CASE WHEN status = 'active' THEN fact_id END` plus a UNIQUE constraint, as defined in [addendum-aurora-dsql.md](addendum-aurora-dsql.md).

### 4.3 `fact_evidence`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `fact_version_id` | UUID FK | `ON DELETE CASCADE`. |
| `message_id` | UUID nullable | `ON DELETE CASCADE` (link only). |
| `manual_item_id` | UUID nullable | `ON DELETE CASCADE` (link only). |
| `added_at` | timestamptz | |
| CHECK exactly one of message/manual | | Same pattern as `issue_items`. |

### 4.4 `decisions`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `project_id` | UUID FK | |
| `issue_id` | UUID FK nullable | |
| `statement` | text | |
| `status` | text | CHECK `proposed\|accepted\|superseded\|withdrawn`. |
| `decided_at` | timestamptz nullable | |
| `assignee_user_id` | UUID nullable | XOR with contact. |
| `assignee_contact_id` | UUID nullable | |
| `source` | text | `user\|rule\|llm`. |
| `confidence` | real nullable | |
| `supersedes_decision_id` | UUID nullable | |
| `created_by_user_id` | UUID nullable | |
| `created_at` / `updated_at` | timestamptz | |
| CHECK not both assignees | | Same as issues. |

### 4.5 `decision_evidence`

Same shape as `fact_evidence` (message XOR manual, CASCADE link only).

### 4.6 `interpretations`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `project_id` | UUID FK | |
| `account_id` | UUID nullable | Set when mail-bound; null for manual-only runs. |
| `run_id` | UUID nullable | Job provenance id; no DSQL FK because hosted run state is in DynamoDB. |
| `status` | text | CHECK `pending\|accepted\|dismissed\|expired`. |
| `payload_json` | text | Candidate fact/decision proposals (schema §9). |
| `confidence` | real nullable | |
| `reason` | text | |
| `created_at` / `updated_at` | timestamptz | |

### 4.7 `interpretation_sources`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `interpretation_id` | UUID FK | CASCADE. |
| `message_id` / `manual_item_id` | UUID nullable | Exactly one; CASCADE link only. |

### 4.8 `contradictions`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `project_id` | UUID FK | |
| `status` | text | CHECK `open\|resolved`. |
| `summary` | text | |
| `resolution_note` | text nullable | |
| `resolved_at` | timestamptz nullable | |
| `resolved_by_user_id` | UUID nullable | |
| `created_at` / `updated_at` | timestamptz | |

### 4.9 `contradiction_sides`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `contradiction_id` | UUID FK | CASCADE. |
| `fact_version_id` | UUID nullable | |
| `decision_id` | UUID nullable | |
| CHECK at least one side ref | | Prefer fact_version or decision. |

### 4.10 Indexes

- `facts (project_id)`, unique `(project_id, subject_key)`
- `fact_versions (fact_id, status)`
- `decisions (project_id, status)`
- `interpretations (project_id, status)`
- `contradictions (project_id, status)`

---

## 5. Domain rules

### 5.1 Confirm fact version

Transition `proposed → active`:

1. If another active version exists on the same fact and the new value is incompatible → either require `supersedes_version_id` or open a contradiction (do not activate both).
2. On supersede: set prior `active → superseded`, fill `superseded_by_version_id` / `superseded_at`.
3. Attach evidence rows (may already exist from interpretation).

### 5.2 Subject keys

- Lowercase dotted identifiers: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$` (spec implementation may tighten).
- LLM proposes `subject_key` + `label`; user may edit label freely; changing key on an active fact is a merge/migrate operation (out of early slices — prefer new fact + supersede narrative).

### 5.3 Assignee XOR

Decisions reuse issue assignee XOR (profile **or** contact, not both).

### 5.4 Derived Needs My Input

API builds a list of attention items; persistence is optional cache. Each item includes `why_me`: `issue_assignee` | `member_role` | `provisional_fact` | `provisional_decision` | `open_contradiction` | `mail_action_item`.

---

## 6. Application services

| Service | Responsibility |
| ------- | -------------- |
| `facts.Service` | List/get/create/confirm/reject/supersede; evidence add/remove |
| `decisions.Service` | CRUD-ish confirm/withdraw/supersede; evidence |
| `interpret.Service` | Build candidates from timeline / new items; persist interpretations |
| `reconcile.Service` | Apply Stage B; open contradictions; propose supersessions |
| `attention.Service` | Needs My Input aggregation |
| `projectai.Service` | Retrieve structured context + LLM answer with citations |

Home-org scoping only (Wave 1 Path A).

---

## 7. HTTP API

Snake_case JSON. Auth as today.

### 7.1 Facts (2a)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects/{id}/facts` | Active (+ optional `?include=history\|proposed`). |
| `POST /api/projects/{id}/facts` | Manual create (`subject_key`, `label`, `value`, evidence?). Creates fact + `proposed` or `active` if `confirm=true`. |
| `GET /api/facts/{id}` | Fact + versions + evidence. |
| `POST /api/fact-versions/{id}/confirm` | Activate (with supersede rules). |
| `POST /api/fact-versions/{id}/reject` | Reject proposed. |
| `POST /api/fact-versions/{id}/evidence` | Attach message/manual. |
| `DELETE /api/fact-versions/{id}/evidence/{evidence_id}` | Detach link. |

### 7.2 Interpret / reconcile (2b–2c)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/projects/{id}/interpret` | `{ "account_id?", "message_ids?", "manual_item_ids?", "async?" }` → interpretations. |
| `GET /api/projects/{id}/interpretations` | List `pending`. |
| `POST /api/interpretations/{id}/dismiss` | Dismiss. |
| `POST /api/projects/{id}/reconcile` | Run Stage B over pending interpretations (or specified ids). |
| `GET /api/projects/{id}/contradictions` | List. |
| `POST /api/contradictions/{id}/resolve` | `{ "resolution": "supersede\|reject_a\|reject_b\|note", ... }`. |

### 7.3 Decisions (2d)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects/{id}/decisions` | List. |
| `POST /api/projects/{id}/decisions` | Create / propose. |
| `POST /api/decisions/{id}/confirm` | Accept. |
| `POST /api/decisions/{id}/withdraw` | Withdraw. |
| `PATCH /api/decisions/{id}` | Limited fields while proposed. |

### 7.4 Needs My Input (2d)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/attention` | `{ items: [...], counts }` across home org. |
| `GET /api/projects/{id}/attention` | Project-scoped. |

### 7.5 Ask Project AI (2e)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/projects/{id}/ask` | `{ "question": "..." }` → `{ "answer", "citations": [...], "proposed_interpretation_id?" }`. |

Citations reference `fact_version_id`, `decision_id`, `issue_id`, `message_id`, and/or `manual_item_id`.

### 7.6 Current position (2a+)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects/{id}/current-position` | Compact active facts + recent accepted decisions for the project header strip. |

---

## 8. Jobs

| job_type | Trigger | Notes |
| -------- | ------- | ----- |
| `interpret_project` | After assign/paste/issue-attach; or API | Set `account_id` when mail included |
| `reconcile_project` | After interpret success; or API | Must not destroy evidence on failure |
| `project_ai` | Ask API async mode (optional) | Persist answer artifact if useful for debug |

Failures must not roll back correspondence. Retry only through the fenced/idempotent policy registered for the job type in the AWS deployment plan.

Register payload schema, chunk boundary, cursor, and retry/effect policy when adding a type. Do not extend a hosted relational `job_runs` CHECK; DynamoDB is the hosted run store.

---

## 9. LLM contracts

Reuse parse+retry (JSON-only, fence strip) from categorize / issue suggest.

### 9.1 Interpret (single object or array under `candidates`)

```json
{
  "schema_version": 1,
  "project_id": "uuid",
  "candidates": [
    {
      "kind": "fact",
      "subject_key": "pump.p03.duty_kw",
      "label": "Pump P-03 duty",
      "value": { "amount": 90, "unit": "kW" },
      "message_ids": [],
      "manual_item_ids": [],
      "confidence": 0.0,
      "reason": ""
    },
    {
      "kind": "decision",
      "statement": "Proceed with 90 kW and update M-402 to Rev C",
      "message_ids": [],
      "manual_item_ids": [],
      "confidence": 0.0,
      "reason": ""
    }
  ]
}
```

All mail `message_ids` in one call must share one `account_id`.

### 9.2 Reconcile assist (optional LLM)

Input: candidate + active facts for same/near subject keys. Output:

```json
{
  "schema_version": 1,
  "outcome": "confirm_new|supersede|reinforce|contradiction|ignore",
  "supersedes_fact_version_id": "",
  "confidence": 0.0,
  "reason": ""
}
```

Deterministic rules may short-circuit before LLM (exact subject + numeric equality → reinforce; exact subject + numeric inequality → supersede or contradiction based on confidence / language cues).

### 9.3 Ask Project AI

System: answer only from provided structured context + snippets. Response:

```json
{
  "schema_version": 1,
  "answer": "",
  "citations": [
    { "type": "fact_version|decision|issue|message|manual_item", "id": "uuid" }
  ],
  "confidence": 0.0
}
```

If context is insufficient: say so; do not invent facts.

---

## 10. UI contract (`web/`)

Keep Wave 1 routes. Add:

| Route / element | Slice |
| --------------- | ----- |
| Project **Current position** strip | 2a |
| `/projects/:id/facts` or panel | 2a |
| Interpretations inbox on project (pending confirm) | 2b |
| Contradictions list / resolve | 2c |
| Decisions rail or section | 2d |
| Nav/Assistant **Needs my input** chip | 2d |
| Project **Ask** panel | 2e |

Timeline remains visually dominant. Current position is a strip, not a card wall that replaces the trail.

---

## 11. Slice exit criteria

**2a.** On DC01, fact duty 75 kW exists historically; after confirm, 90 kW is the sole active version; 75 kW appears superseded with evidence; deleting a source message leaves the fact.

**2b.** New paste/mail on the project yields at least one pending interpretation the user can dismiss without creating a fact.

**2c.** A conflicting claim opens an `open` contradiction; resolve via supersede updates versions correctly.

**2d.** Decisions can be confirmed with evidence; `GET /api/attention` returns items with `why_me`; counts are real.

**2e.** Ask “What is Pump P-03 duty?” returns 90 kW with ≥1 citation to the active fact version (and preferably correspondence).

---

## 12. Testing

- Unique `(project_id, subject_key)`; at most one active version.
- Confirm with supersede flips prior version status atomically.
- Evidence CASCADE removes links only.
- Interpret rejects mixed `account_id` message sets.
- Reconcile does not activate two conflicting versions.
- Attention isolation by home org.
- Ask Project AI returns citations or an explicit insufficiency message.
- No handler inserts `contact_profile_links`.

---

## 13. Mapping to Wave 2 PRD

| PRD topic | Spec |
| --------- | ---- |
| Facts with history | `facts` + `fact_versions` + `fact_evidence` |
| Decisions | `decisions` + `decision_evidence` |
| Two-stage pipeline | `interpretations` + reconcile service/jobs |
| Contradictions | `contradictions` + sides |
| Needs My Input | `attention` APIs (derived) |
| Ask Project AI | `POST .../ask` + citation schema |
| No silent overwrite | version status machine + contradiction |

---

*Implementation PRs should name the slice (2a–2e) and this spec. Do not land Path B, live non-email connectors, or fact in-place mutation under Wave 2.*
