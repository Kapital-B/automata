# PRD Addendum: Project Correspondence Wave 2

**Status:** Draft  
**Owner:** Product (single-user / self-hosted context)  
**Last updated:** 2026-08-22  
**Parent PRD:** [docs/prds/initial.md](initial.md)  
**Prerequisite:** [Project correspondence intelligence (Wave 1)](addendum-project-correspondence.md)  
**Companion spec:** [docs/specs/addendum-project-correspondence-wave2.md](../specs/addendum-project-correspondence-wave2.md)  
**Source vision:** [AI Project Correspondence Agent](AI%20Project%20Correspondence%20Agent.pdf) (August 2026)

This addendum freezes **Wave 2**: turning Wave 1’s project timeline and issue trails into a **user-correctable current-position engine**. It does not replace the parent PRD or Wave 1. Mail provenance, explicit send approval, Contact ≠ Profile, and “evidence is immutable” remain non-negotiable.

The PRD is authoritative for product intent. The companion spec is authoritative for schema, APIs, jobs, and invariants.

---

## 1. Summary

Wave 1 answers: **“What correspondence belongs on this project, and which issue trail groups it?”**

Wave 2 adds: **“What is currently true on this project, what changed, what contradicts, and what needs me — given my role?”**

The organising unit remains the **Project**. Wave 1 objects (**Contact**, **Issue**, timeline, Unassigned) stay. Wave 2 introduces first-class **Facts** and **Decisions**, a durable **supersession / contradiction** model, a **two-stage interpret → reconcile** pipeline, **role-aware Needs My Input**, and **Ask Project AI** over structured project state (not free-form chat over raw mail alone).

Wave 2 still ships as **Path A**: one operator, no invites UI, no shared ACL product. Contact and Profile remain distinct; Wave 2 must not invent auto-link or auto-join.

---

## 2. Relationship to Wave 1 and the rest of the product

| Surface | Role after Wave 2 |
| ------- | ----------------- |
| Assistant home | Gains project-aware attention chips when facts / Needs My Input produce real counts. Still not a replacement for the project timeline. |
| Today | May surface role-aware Needs My Input items that are project-derived, alongside existing mail action items. |
| Inbox / Unassigned / People / Projects / Issues | Unchanged as Wave 1 surfaces; project page gains **Current position**, **Facts**, **Decisions**, and reconcile prompts. |
| Ask Project AI (new) | Q&A grounded in facts, decisions, issue trails, and cited evidence — not inventing positions without citations. |
| Drafts, Rules, Runs, Accounts | Unchanged. |

Wave 1 issue `current_position_note` remains a **label**. It must not become a silent write-through to the fact register. Facts are their own aggregate with history.

---

## 3. Goals

1. Persist **Facts**: current-valid project assertions with version history, evidence links, and explicit supersession.
2. Persist **Decisions**: agreed outcomes (who, when, what) distinct from transient facts, with evidence.
3. Run a **two-stage pipeline**: (a) **interpret** new correspondence into candidate claims; (b) **reconcile** candidates against existing facts/decisions (confirm, supersede, flag contradiction, or leave provisional).
4. Detect and surface **contradictions** without silently overwriting either claim.
5. Deliver **Needs My Input** as a role-aware attention list derived from project membership role / discipline / scope plus issue assignee and awaiting_input — not a boolean on mail alone.
6. Ship **Ask Project AI** over structured project state with mandatory evidence citations.
7. Keep every AI write **provisional until user confirmation** unless confidence and policy explicitly allow auto-apply for narrow, reversible cases (spec-defined).

---

## 4. Non-goals (Wave 2)

- Invites, org switcher, project ACL enforcement, contact↔profile **writes**, Path B multi-user product.
- Live Slack / Teams / WhatsApp / SMS / call / meeting ingest (still manual paste + mail). That remains Wave 3.
- Replacing Assistant, Today, Inbox, drafts, or rules.
- Turning Issues into a generic task / RFI / ERP module.
- Autonomous send or any relaxation of explicit confirmation.
- Using the latest message as “the truth” without a fact version and evidence.
- Storing facts only as free text on the issue note.

---

## 5. Design principles

### 5.1 Evidence stays immutable

Interpretation and reconciliation attach **new rows**. They never edit stored body text, timestamps, provider ids, or attachments.

### 5.2 Interpret ≠ commit

LLM (or rule) output produces **candidates** (`source = llm|rule`, usually `status = provisional`). Committing a fact or decision is a separate user act (or a narrowly allowlisted auto-apply).

### 5.3 Supersession is explicit

When 90 kW replaces 75 kW, both versions remain. The older version is marked superseded with `superseded_at`, `superseded_by_fact_version_id` (or equivalent), and evidence for the change. Silent overwrite is forbidden.

### 5.4 Contradiction is first-class

Two live claims that conflict must produce a **contradiction** record (or equivalent open conflict on the project), not a hidden pick of the newer timestamp.

### 5.5 Role-aware attention is derived

`awaiting_me` on issues (Wave 1) stays. Wave 2 adds Needs My Input items that can also come from: provisional facts needing confirm, open contradictions in the operator’s discipline, decisions awaiting the assignee, and scope matches on `project_members.role` / `discipline` / `current_scope`. Do not store a parallel boolean “for me” that diverges from assignee and role.

### 5.6 Ask Project AI must cite

Answers must reference fact ids, decision ids, issue ids, and/or correspondence evidence. “Because the model said so” is a product failure.

---

## 6. Domain model (product-level)

### 6.1 Fact

A **Fact** is a current (or historical) project assertion.

Examples:

- Pump P-03 duty = 90 kW (current).
- Pump P-03 duty = 75 kW (superseded 14 Aug, evidence: WhatsApp approval + Rev C mail).

Required product properties:

- Belongs to one **project** (and optionally linked to one **issue**).
- Has a stable **subject** / key (e.g. `pump.p03.duty_kw`) and a **value** (typed or structured JSON — spec chooses representation).
- Has **versions**: each version has value, status (`proposed` | `active` | `superseded` | `rejected`), evidence links, `source`, `confidence`, timestamps, optional author (profile).
- Exactly one **active** version per subject per project (unless the conflict model allows zero while a contradiction is open — prefer zero active + open contradiction over two actives).

### 6.2 Decision

A **Decision** records an agreed course of action or approval, distinct from a measurable fact.

Example: “Proceed with 90 kW; update M-402 to Rev C” — decided by contact X on date Y, evidence Z.

Properties:

- Belongs to a project; optional issue link.
- Statement, status (`proposed` | `accepted` | `superseded` | `withdrawn`).
- Optional decision-maker as **Contact** or **Profile** (same XOR rule as issue assignee).
- Evidence links; never overwrite the statement in place — supersede with a new row/version.

### 6.3 Interpretation candidate

An **Interpretation** is a proposed claim extracted from one or more correspondence items (often after sync, paste, or issue attach).

- Links to source message(s) and/or manual item(s).
- Proposes fact and/or decision payloads.
- Does not mutate facts until reconcile + confirm.

### 6.4 Contradiction

A **Contradiction** ties two (or more) incompatible active/proposed claims.

- Status: `open` | `resolved`.
- Resolution paths: supersede A with B, reject A, reject B, merge subjects, or user note that both stand in different scopes (rare; must be explicit).

### 6.5 Needs My Input (derived attention)

Not a single table that replaces issues. A **query / materialised view** over:

- issues with `awaiting_input` and assignee = me (profile);
- provisional facts/decisions on projects where I am a member;
- open contradictions on my projects, optionally filtered by discipline/scope;
- existing mail `action_items` whose source message is on the project (Wave 1 bridge — still not migrated away).

### 6.6 Ask Project AI session

A project-scoped Q&A surface that:

- retrieves structured facts, decisions, issue summaries, and cited correspondence snippets;
- returns an answer with citations;
- may propose new interpretations but must not auto-commit facts.

---

## 7. Two-stage pipeline

### Stage A — Interpret

Trigger when new correspondence lands on a project (mail assign, manual paste, issue attach), or when the user runs “Interpret new items.”

Input: one account’s mail items and/or manuals on that project (same single-`account_id` rule as Wave 1 LLM calls for mail).

Output: zero or more **interpretation candidates** (JSON schema in the companion spec).

### Stage B — Reconcile

For each candidate, against the project’s active facts/decisions:

| Outcome | Meaning |
| ------- | ------- |
| **Confirm new** | No prior subject → provisional fact/decision ready for user confirm. |
| **Supersede** | Same subject, incompatible value → propose new active version + mark prior superseded (provisional until confirm). |
| **Reinforce** | Same subject, compatible value → optional confidence bump / extra evidence link; no new “truth.” |
| **Contradiction** | Competing live claims without a safe supersede → open contradiction; do not pick a winner. |
| **Ignore** | Noise / not a durable claim. |

User confirmation is the default commit gate. Auto-apply is only for high-confidence reinforce / exact duplicate evidence attach if the spec allowlists it.

---

## 8. User-facing capabilities (Wave 2)

### 8.1 Current position on the project

Project page gains a **Current position** strip (or panel): active facts and recent decisions for the project, each with evidence deep-links. This is the product answer to “what is true,” not the chronological timeline (which remains dominant for history).

### 8.2 Facts UI

- List active facts; expand history (superseded versions).
- Confirm / reject provisional facts.
- Manually add or edit a fact (creates a new version; never silent overwrite).
- Link / unlink evidence.

### 8.3 Decisions UI

- List decisions; confirm provisional; record manual decisions.
- Supersede or withdraw with reason + evidence.

### 8.4 Contradictions UI

- Open conflicts with both sides and evidence.
- Resolve via supersede / reject / note.

### 8.5 Needs My Input

- Project filter and/or Assistant / Today chip with **real counts**.
- Items explain **why me** (assignee, role, discipline, provisional confirm, contradiction).

### 8.6 Ask Project AI

- Entry from project page.
- Answer + citations; “propose fact” CTA that goes through confirm, never silent write.

### 8.7 Issue integration

- Issue trail may show related facts/decisions.
- Issue status may be suggested from reconcile (e.g. proposed_resolution) but remains user-correctable.
- Do not delete Wave 1 assignee XOR or derived `awaiting_me`.

---

## 9. AI in Wave 2

Allowed:

- Constrained JSON for interpret and reconcile (schemas in spec).
- Ask Project AI grounded on structured retrieval + cited snippets.
- Optional auto-apply only where the spec lists reversible, high-confidence cases.

Forbidden:

- Overwriting fact values in place.
- Writing `contact_profile_links` or memberships.
- Mixing multiple mail `account_id`s in one interpret call.
- Answering Ask Project AI without citations when structured state exists.

---

## 10. Phased delivery

Each slice should be shippable. Do not start Ask Project AI before facts exist. Do not start Path B here.

| Slice | Name | Delivers | Exit criteria |
| ----- | ---- | -------- | ------------- |
| **2a** | Facts + versions | Fact CRUD, evidence links, supersession, project Current position UI | 75 kW stored; 90 kW confirmed as active; 75 kW visible as superseded with evidence |
| **2b** | Interpret | Candidates from new project correspondence; provisional only | Pasting/assigning items produces candidates the user can confirm or dismiss |
| **2c** | Reconcile + contradictions | Stage B outcomes; contradiction objects + resolve UI | Conflicting claims open a contradiction instead of silent overwrite |
| **2d** | Decisions + Needs My Input | Decision register; role-aware attention list / chips | Operator sees “needs me” items with why-me reason; decisions have evidence |
| **2e** | Ask Project AI | Project Q&A with citations over facts/decisions/trails | “What is Pump P-03 duty?” answers 90 kW and cites evidence; does not invent unaudited facts |

**Wave 2 is done** when the core acceptance scenario below works on Path A with mail + paste only.

**Wave 3** (separate): live non-email connectors; enabling Path B (invite + optional contact↔profile link) — see [Wave 3 PRD](addendum-project-correspondence-wave3.md) and [Wave 3 spec](../specs/addendum-project-correspondence-wave3.md).

---

## 11. Core acceptance scenario (Wave 2)

Continues Wave 1’s DC01 / Pump P-03 trail.

1. Correspondence already on DC01 states 75 kW, then later 90 kW approval and Rev C drawing (Wave 1 trail intact).
2. Interpret + reconcile propose: fact `pump.p03.duty_kw` = 90 kW superseding 75 kW.
3. Operator confirms. Current position shows **90 kW**. History shows **75 kW superseded** with evidence links to the earlier mail and the approval items.
4. If a later item claims 80 kW without clear supersession language, the system opens a **contradiction** rather than overwriting 90 kW.
5. Needs My Input surfaces the contradiction and/or awaiting confirmations for the Mechanical Engineer member role.
6. Ask Project AI: “What is the current duty for Pump P-03?” → **90 kW** with citations to the active fact and underlying correspondence.

---

## 12. Success criteria

- Facts and decisions are first-class, evidence-linked, and versioned.
- Supersession and contradiction are explicit and user-visible.
- Interpret never auto-commits durable state without policy + (usually) confirm.
- Needs My Input is explainable and role-aware without collapsing Contact into Profile.
- Ask Project AI cites structured state and evidence.
- Wave 1 timelines, issues, Unassigned, People, and mail automation still work.
- No invites/ACL/live-connectors shipped under the Wave 2 label.

---

## 13. Invariants (additive to Wave 1)

1. Fact/decision commits are append-only versions (or equivalent), never in-place value mutation.
2. At most one **active** fact version per subject per project (or zero while conflict is open).
3. Deleting a message removes evidence **links** only; facts/decisions/issues/contacts remain.
4. Interpret/reconcile/Ask Project AI jobs set `account_id` when mail-bound; do not mix accounts in one LLM call.
5. Contact ≠ Profile; no Wave 2 writes to `contact_profile_links`.
6. Parent safety rules unchanged (no send without confirm; no off-allowlist forward; no secrets in logs).

---

## 14. Resolved decisions (Wave 2)

| Topic | Decision |
| ----- | -------- |
| Fact vs issue note | Facts are a separate register. Issue `current_position_note` stays a label. |
| Fact vs decision | Separate objects. Approvals / “proceed with…” are decisions; measurable assertions are facts. |
| Commit gate | Default: user confirm. Spec may allowlist narrow auto-apply (e.g. duplicate evidence attach). |
| Conflict | Prefer open **contradiction** over picking newest timestamp. |
| Needs My Input | Derived attention, not a replacement for issues or mail action items. |
| Ask Project AI | Grounded Q&A with citations; may propose interpretations; must not silent-commit. |
| Path B | Still out of Wave 2. |

---

## 15. What this addendum does not change

- Wave 1 organisation-at-signup, contacts, projects, thread assignment, Unassigned, manuals, issues.
- Parent mail phases, AI-first assistant phases for mailbox attention (except additive chips).
- Local/OpenAI-compatible LLM hosting assumptions.

It **does** change the product’s destination: Automata becomes the operator’s **project memory with a correctable current position**, not only a classified mailbox and trail.

---

*Implementation PRs should name the slice (2a–2e) and [the Wave 2 spec](../specs/addendum-project-correspondence-wave2.md). Do not land invites, live non-email connectors, or contact↔profile writes in Wave 2.*
