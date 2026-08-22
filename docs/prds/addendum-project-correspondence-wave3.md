# PRD Addendum: Project Correspondence Wave 3

**Status:** Draft  
**Owner:** Product (single-user / self-hosted context evolving toward multi-operator)  
**Last updated:** 2026-08-22  
**Parent PRD:** [docs/prds/initial.md](initial.md)  
**Prerequisites:** [Wave 1](addendum-project-correspondence.md), [Wave 2](addendum-project-correspondence-wave2.md)  
**Companion spec:** [docs/specs/addendum-project-correspondence-wave3.md](../specs/addendum-project-correspondence-wave3.md)  
**Source vision:** [AI Project Correspondence Agent](AI%20Project%20Correspondence%20Agent.pdf) (August 2026)

This addendum freezes **Wave 3**: bringing **live non-email correspondence** into the same project memory Wave 1–2 already trust for mail + paste, and **enabling Path B** so a second profile can join a project without collapsing Contact into Profile. It does not replace prior addenda. Evidence immutability, explicit send approval, Contact ≠ Profile, and no silent fact overwrite remain non-negotiable.

The PRD is authoritative for product intent. The companion spec is authoritative for connectors, Path B APIs, schema, jobs, and invariants.

---

## 1. Summary

Wave 1 answers: **“What correspondence belongs on this project?”**  
Wave 2 answers: **“What is currently true, what contradicts, and what needs me?”**

Wave 3 answers: **“Can the trail and current position stay correct when the real work happens off email — and when another operator joins?”**

Two product thrusts, shipped as slices:

1. **Live connectors** — Slack / Teams / WhatsApp / SMS / call & meeting transcripts / document revision events map into the existing correspondence model (`messages` or `manual_items`-shaped rows with stable provider ids, account/connector provenance, project assignment, interpret → reconcile). Paste remains the fallback; it is no longer the only non-mail path.
2. **Path B collaboration** — Invite a profile onto a project (membership). Optionally **link** that profile to an existing Contact after accept. No auto-join on email match. No dual-link without merge.

Wave 3 still does **not** turn Automata into Slack, ERP, or a generic task system. The organising unit remains the **Project**; Facts / Decisions / Issues stay the memory layer.

---

## 2. Relationship to Waves 1–2 and the rest of the product

| Surface | Role after Wave 3 |
| ------- | ----------------- |
| Project timeline | Shows mail **and** live connector items with source badges; same assignment / Unassigned rules. |
| Current position / Facts / Decisions / Contradictions / Ask | Unchanged contracts; fed by richer evidence from connectors. |
| Needs My Input | Gains connector-derived reasons where useful; still derived, not a parallel boolean store. |
| Accounts / Connectors | Email remains first; Slack / Teams / etc. appear as real connectors when implemented (assistant “soon” placeholders retire only when live). |
| People | Contact↔profile **link** UI appears after invite accept; merge-first rule stays. |
| Projects | Member invite / accept / role fields become multi-profile; ACL remains project-membership based (home-org Path A still works for solo operators). |
| Assistant / Today / Inbox / Drafts / Rules | Unchanged safety model; Today may list Path B attention for the signed-in profile. |

---

## 3. Goals

1. Ingest **live** Slack, Teams, WhatsApp (and optionally SMS) conversations into project timelines with durable provider ids and connector provenance.
2. Ingest **transcripts** (call / meeting) and **document revision** events as correspondence-shaped evidence (not as free-floating files without a trail row).
3. Reuse Wave 1 assignment + Wave 2 interpret / reconcile / Ask — connectors must not invent a second memory model.
4. Enable **Path B**: invite profile → accept → project member; optional confirmed **contact↔profile link**.
5. Keep Contact ≠ Profile: signup email match never grants membership or writes a link.
6. Preserve operator control: connector sync is explainable; medium-confidence assignment and all durable fact/decision commits stay user-correctable.
7. Retire “paste-only” as the sole story for Teams/WhatsApp once the matching connector ships — paste remains available forever.

---

## 4. Non-goals (Wave 3)

- Becoming a full Slack/Teams client, WhatsApp Business suite, or dialer.
- Autonomous send / post to external channels without explicit confirmation (parent safety unchanged).
- Org-wide ACL products beyond **project membership** (no fine-grained field-level ACL in Wave 3).
- Replacing Issues with RFI/ERP modules.
- Migrating Contacts into Users or auto-linking on first email sighting.
- Multi-region SaaS tenancy / billing (still self-hosted / single-deployment assumptions unless a later addendum).
- Reworking Wave 2 fact versioning or contradiction semantics.

---

## 5. Design principles

### 5.1 One correspondence model

Every connector produces **evidence rows** the timeline already understands: stable id, occurred_at, title/snippet/body, participants as Contacts, optional `account_id` / `connector_id`, project assignment. Prefer extending existing tables over parallel “slack_messages” silos that Ask/Interpret cannot see.

### 5.2 Connector = account-like provenance

Each connected workspace/channel identity is scoped like mail accounts today: sync jobs set connector provenance; LLM calls must not mix two connectors (or a connector + unrelated mail account) in one prompt unless the spec defines a deliberate, labeled multi-source pack for Ask.

### 5.3 Invite ≠ link

| Step | Effect |
| ---- | ------ |
| Invite accepted | Profile becomes **project member** (role/discipline/scope editable). |
| Link confirmed | That profile’s **Contact** in the org is associated (≤1 contact per profile per org). |
| Email match alone | Suggestion only. |

### 5.4 Paste stays honest

If a connector is disconnected or out of scope (e.g. personal WhatsApp), paste continues. Pasted items are not rewritten into fake provider messages.

### 5.5 Wave 2 memory stays authoritative

Connectors feed interpret → reconcile. They do not write active facts directly.

---

## 6. Product surfaces

### 6.1 Connector onboarding

Accounts (or a Connectors section) gains first-class cards for Slack / Teams / WhatsApp / SMS as they ship. Each shows connection status, last sync, and scopes (workspaces, channels, chats) the operator allowed.

### 6.2 Project timeline

Live items appear beside mail and paste with clear source labels. Deep-link back to the provider when possible; otherwise show stored body/snippet.

### 6.3 Invites

Project settings: invite by email → pending invite → accept as logged-in profile → member row. Inviter sees pending/expired. Invitee lands on the project with Needs My Input empty until role/assignment warrants items.

### 6.4 Contact ↔ profile link

After accept, if a Contact shares an email identity with the profile, suggest link. User confirms. If two Contacts would claim the same profile, require merge first (Wave 1 rule).

### 6.5 Document revisions & transcripts

Revision events and transcripts appear as timeline items (and optional issue attachments). Extracted claims still go through interpret/reconcile — a PDF upload is not an instant active fact.

---

## 7. Phased delivery

Each slice should be shippable. Prefer one real connector before Path B polish, but Path B schema/API can land early because Wave 1 already reserved the shape.

| Slice | Name | Delivers | Exit criteria |
| ----- | ---- | -------- | ------------- |
| **3a** | Path B invites + membership | Invite/accept APIs + UI; second profile on a project; role fields editable | Invitee opens DC01 timeline; cannot see other orgs’ projects |
| **3b** | Contact↔profile link | Confirm link after accept; merge-first enforcement | Linked contact shows on People; dual-link blocked |
| **3c** | Teams live ingest | Connect Microsoft Teams (or Graph channel messages); sync into timeline; assign / Unassigned | A Teams channel message on DC01 appears without paste |
| **3d** | Slack live ingest | Slack workspace OAuth + channel/DM sync into same model | Slack item on DC01 timeline; interpret can propose a fact candidate |
| **3e** | WhatsApp / SMS (one first) | At least one mobile messaging connector or documented bridge; paste remains fallback | Mobile message appears as correspondence with provider id |
| **3f** | Transcripts + doc revisions | Meeting/call transcript ingest + drawing/doc revision events | Rev D event on trail; Ask can cite it after reconcile confirms related fact |

Order note: **3a–3b** unlock multi-operator demos on mail+paste alone. **3c–3f** deepen evidence. Do not block connector work on Path B if staffing prefers the reverse — but do not ship a connector that bypasses Contacts/assignment.

---

## 8. Core acceptance scenario (Wave 3)

Continues DC01 / Pump P-03.

1. Wave 2 state: active duty **90 kW** (or whatever was confirmed), history intact.
2. A **Teams** (or Slack) message in the project channel: “Vendor drawing Rev D — duty now 95 kW pending approval.”
3. Item lands on DC01 timeline **without paste**; Unassigned/assign rules still apply if channel mapping is ambiguous.
4. Interpret → reconcile opens a **contradiction** or proposed supersede vs 90 kW; operator resolves.
5. Operator **invites** a colleague; colleague accepts and becomes member; Needs My Input can show the open contradiction for their role.
6. Colleague confirms **contact↔profile link** for their existing Contact row.
7. Ask Project AI for either member: “What is Pump P-03 duty?” returns the post-resolution value with citations including the live connector item when it was evidence.

---

## 9. Success criteria

- At least two live non-email sources (recommend Teams + Slack) feed the same timeline model as mail/paste.
- Path B: second profile can join a project via invite; Contact≠Profile preserved; link is explicit.
- Wave 2 facts/decisions/contradictions/Ask still pass their exit criteria with connector-sourced evidence.
- No connector posts outbound without explicit user confirmation.
- Solo Path A operators remain first-class (invites optional).

---

## 10. Invariants (additive)

1. Connector rows carry connector provenance analogous to `account_id` for mail.
2. Provider message/event ids are unique per connector identity.
3. Invite-accept writes **membership only**; link is a separate confirm.
4. ≤1 contact↔profile link per profile per organisation; merge before dual claim.
5. Connectors never silent-activate fact versions.
6. Parent send/forward safety unchanged; outbound connector actions require confirm + audit.
7. Deleting a provider message removes **links** only; project memory remains.

---

## 11. Resolved decisions (Wave 3)

| Topic | Decision |
| ----- | -------- |
| Paste vs live | Live preferred when connected; paste never removed. |
| Which connector first | Spec may sequence Teams (Graph adjacency) then Slack; WhatsApp may be bridge/partner. |
| Path B timing | First-class Wave 3 slices (3a–3b), not deferred again. |
| ACL depth | Project membership is the authz unit; no document-level ACL yet. |
| Outbound chat | Out of Wave 3 unless a later slice explicitly adds “propose reply in Slack/Teams” with confirm. |
| Doc vault | Revisions as correspondence events first; full DMS is out of scope. |

---

## 12. What this addendum does not change

- Wave 1–2 schema meaning for facts, decisions, issues, contacts, assignment.
- Mail sync, drafts, rules, categorization.
- LLM hosting assumptions (OpenAI-compatible / local).

It **does** change the product’s reach: Automata becomes the operator’s **multi-channel project memory**, shareable with invited colleagues, still correctable and citation-backed.

---

*Implementation PRs should name the slice (3a–3f) and [the Wave 3 spec](../specs/addendum-project-correspondence-wave3.md). Do not land auto-link-on-email or silent fact writes under Wave 3.*
