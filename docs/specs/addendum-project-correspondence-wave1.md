# Technical Specification: Project Correspondence Wave 1

**Status:** Draft  
**Parent PRD:** [Project correspondence intelligence](../prds/addendum-project-correspondence.md)  
**Related PRD:** [Initial PRD](../prds/initial.md)  
**Related spec:** [Initial technical specification](initial.md)  
**Last updated:** 2026-08-21  

This document specifies schema, APIs, jobs, and UI for **Wave 1** (slices 1a–1d). The PRD is authoritative for product intent. This spec is authoritative for implementation invariants.

Parent mail invariants still apply: `account_id` on mail-derived rows, `(account_id, provider_message_id)` uniqueness, explicit send confirmation, allowlisted forwarding, no secrets in logs.

---

## 1. Scope

Wave 1 delivers:

- an **organisation** created at profile signup;
- **Contacts** in that organisation’s address book, bootstrapped from mail;
- **Projects** with a required structured **code**;
- **thread-unit project assignment** with **per-message override**;
- a dedicated **Unassigned** page;
- **manual correspondence** paste onto a project or Unassigned;
- **Issues** with a trail and an assignee that is a **profile** or a **contact**;
- schema for **project members (profiles)** vs **participants (contacts)** and for a **contact↔profile link** that Wave 1 does not write.

Slices (must ship in order):

| Slice | Name |
| ----- | ---- |
| **1a** | Organisation at signup, contacts, identities, People UI, merge |
| **1b** | Projects, members, thread assignment, Unassigned page |
| **1c** | Project timeline, manual ingest |
| **1d** | Issues |

---

## 2. Non-goals

- Facts, decisions, supersession, two-stage reconcile, role-aware Needs My Input, Ask Project AI.
- Invites UI, ACL enforcement, contact↔profile **writes**, auto-join or auto-link on signup.
- Live Slack / Teams / WhatsApp / SMS / call ingest.
- Replacing Assistant, Today, Inbox, Drafts, Rules, Runs, or Accounts.
- Storing project as a `category_definitions` slug.
- Cascading contact or issue deletion when a message is deleted.

---

## 3. Package layout

New bounded context beside `accounts` and `messages`. Domain must not import HTTP, SQL, Graph, or LLM adapters.

```text
svc/internal/
  domain/
    organisations/
    contacts/
    projects/
    issues/
  application/
    organisations/
    contacts/
    projects/
    issues/
    ports/          # add repository ports here (or keep driven/ as today)
```

HTTP handlers in `adapters/inbound/http` call application services. Sync continues to own Graph I/O; after a successful message upsert it calls contact resolution and (1b+) assignment use cases for **that account only**.

`web/` adds routes and nav; it does not invent assignment rules. Effective project for a message is always computed by `svc/`.

---

## 4. Data model

SQLite (then Postgres-compatible types). UTC timestamps as ISO-8601 text, consistent with existing migrations. UUIDs as text.

**Profile** = existing `users` row. Do not create a second user table.

### 4.1 `users` (alter)

| Column | Type | Notes |
| ------ | ---- | ----- |
| `home_organisation_id` | UUID FK `organisations(id)` nullable during migrate | **NOT NULL** after backfill. The organisation created at signup. |

A profile may later join other organisations via `organisation_members`. Wave 1 APIs always use `home_organisation_id`. Do **not** add `UNIQUE (user_id)` on `organisation_members` (that would block Path B).

### 4.2 `organisations`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `name` | text | Default `"Personal"` at signup; user-renamable later (not required in Wave 1 UI). |
| `created_at` / `updated_at` | timestamptz | |

### 4.3 `organisation_members`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `user_id` | UUID FK `users` | Profile with access to this org. |
| `org_role` | text | `owner` \| `member`. Signup inserts `owner`. |
| `created_at` | timestamptz | |

**Unique:** `(organisation_id, user_id)`.

### 4.4 `contacts`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | Address book owner. |
| `display_name` | text | |
| `company` | text nullable | |
| `merged_into_contact_id` | UUID FK `contacts` nullable | Set on the losing row after merge. |
| `created_at` / `updated_at` | timestamptz | |

List APIs omit rows where `merged_into_contact_id IS NOT NULL`.

**No `user_id` column.**

### 4.5 `contact_identities`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | Denormalised; must match `contacts.organisation_id`. |
| `contact_id` | UUID FK | |
| `kind` | text | `email` \| `phone` \| `display_name_hint`. (`slack`, `teams` reserved; unused in Wave 1.) |
| `value_normalized` | text | Email: lowercase trim. Phone: E.164 if parseable, else digits-only. |
| `value_raw` | text | Original. |
| `created_at` | timestamptz | |

**Unique:** `(organisation_id, kind, value_normalized)`.

### 4.6 `contact_profile_links` (Wave 1: table only)

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | The **contact’s** org (your address book). |
| `contact_id` | UUID FK | |
| `user_id` | UUID FK `users` | The other person’s profile. |
| `created_by_user_id` | UUID FK `users` | Who confirmed the link. |
| `created_at` | timestamptz | |

**Unique:** `contact_id` (one link per contact).  
**Unique:** `(organisation_id, user_id)` (one profile per address book).

Wave 1 **must not** insert into this table (no HTTP write, no signup trigger, no invite-accept trigger). Application layer should still enforce: if a write is ever attempted and another contact in the same org is already linked to that `user_id`, reject with “merge contacts first.”

### 4.7 `projects`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `name` | text | Free text, e.g. `Cooling Upgrade`. |
| `code` | text | Structured; see [§6](#6-project-codes). Stored uppercase. |
| `description` | text nullable | |
| `client` | text nullable | |
| `keywords_json` | jsonb | Array of strings for medium-confidence match. |
| `archived_at` | timestamptz nullable | |
| `created_at` / `updated_at` | timestamptz | |

**Unique:** `(organisation_id, code)`.

### 4.8 `project_members` (profile access)

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `project_id` | UUID FK | |
| `user_id` | UUID FK `users` | |
| `role` | text | e.g. `Mechanical Engineer`. |
| `discipline` | text nullable | |
| `responsibilities` | text nullable | |
| `current_scope` | text nullable | |
| `approval_authority` | text nullable | |
| `out_of_scope` | text nullable | |
| `created_at` / `updated_at` | timestamptz | |

**Unique:** `(project_id, user_id)`.

Wave 1 inserts one row for the creating profile. No API to add a second member.

### 4.9 `project_participants` (contacts, no access)

| Column | Type | Notes |
| ------ | ---- | ----- |
| `project_id` | UUID FK | |
| `contact_id` | UUID FK | |
| `first_seen_at` | timestamptz | |

**Unique:** `(project_id, contact_id)`. Upsert when a committed assignment includes that contact as a participant.

### 4.10 Mail columns (alter `messages`)

Add, if not already present:

| Column | Type | Notes |
| ------ | ---- | ----- |
| `to_json` | jsonb | Array of `{ "name", "address" }`. Empty array if unknown. |
| `cc_json` | jsonb | Same. |

`from_json` stays `{ "name", "address" }`. Sync must populate `to_json` / `cc_json` when Graph provides them; otherwise `[]`.

Do **not** copy message bodies into a second correspondence table.

### 4.11 `thread_assignments`

Assignment unit for mail with a non-empty `conversation_id`.

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `account_id` | UUID FK `accounts` | Provenance. |
| `conversation_id` | text | Graph conversation id. |
| `project_id` | UUID FK nullable | Null = thread explicitly has no project (rare; usually delete the row). |
| `status` | text | `committed` \| `provisional`. |
| `confidence` | real nullable | |
| `reason` | text | Shown in UI. |
| `source` | text | `user` \| `rule` \| `llm`. |
| `run_id` | UUID FK `job_runs` nullable | |
| `assigned_by_user_id` | UUID FK `users` nullable | |
| `created_at` / `updated_at` | timestamptz | |

**Unique:** `(account_id, conversation_id)`.

### 4.12 `message_assignment_overrides`

Per-message override of the thread assignment.

| Column | Type | Notes |
| ------ | ---- | ----- |
| `message_id` | UUID PK FK `messages` | |
| `organisation_id` | UUID FK | |
| `account_id` | UUID FK | Denormalised; must match `messages.account_id`. |
| `project_id` | UUID FK nullable | Null = this message is Unassigned even if the thread is assigned. |
| `status` | text | `committed` \| `provisional`. |
| `confidence` | real nullable | |
| `reason` | text | |
| `source` | text | `user` \| `rule` \| `llm`. |
| `run_id` | UUID FK nullable | |
| `assigned_by_user_id` | UUID FK nullable | |
| `created_at` / `updated_at` | timestamptz | |

### 4.13 `manual_items`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `channel` | text | `whatsapp` \| `teams` \| `sms` \| `call` \| `meeting` \| `note`. |
| `occurred_at` | timestamptz | User-supplied; required. |
| `title` | text | |
| `body_text` | text | Immutable after insert (PRD: evidence). |
| `project_id` | UUID FK nullable | Per-item assignment (no thread). |
| `assignment_status` | text | `committed` \| `provisional` \| `unassigned`. |
| `assignment_reason` | text nullable | |
| `assignment_source` | text nullable | `user` \| `rule` \| `llm`. |
| `created_by_user_id` | UUID FK | Audit. |
| `created_at` | timestamptz | |

No `account_id`. Not a fake Graph message.

### 4.14 `correspondence_participants`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `contact_id` | UUID FK | |
| `role` | text | `from` \| `to` \| `cc` \| `participant`. |
| `message_id` | UUID FK nullable | |
| `manual_item_id` | UUID FK nullable | |

**Check:** exactly one of `message_id`, `manual_item_id`.  
**Unique:** `(contact_id, role, message_id)` where message set; `(contact_id, role, manual_item_id)` where manual set.

On message delete: delete these rows (`ON DELETE CASCADE` from `messages` / `manual_items`). **Do not** delete the contact.

### 4.15 `issues`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `project_id` | UUID FK | |
| `title` | text | |
| `current_position_note` | text | Label only; not a fact register. |
| `status` | text | `open` \| `awaiting_input` \| `resolved`. |
| `assignee_user_id` | UUID FK nullable | Project **member** profile. |
| `assignee_contact_id` | UUID FK nullable | Address-book contact. |
| `created_at` / `updated_at` | timestamptz | |

**Check:** not both assignee columns set. Wave 1 default: `assignee_user_id` = current user (must be a `project_members` row).

### 4.16 `issue_items`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `issue_id` | UUID FK | |
| `message_id` | UUID FK nullable `ON DELETE CASCADE` | Removes the *link*, not the issue. |
| `manual_item_id` | UUID FK nullable `ON DELETE CASCADE` | |
| `added_at` | timestamptz | |

**Check:** exactly one source. **Unique** per source id. Default: one primary issue per item; Wave 1 API rejects a second issue for the same item.

### 4.17 `job_runs.job_type`

Extend the CHECK (or equivalent) to include:

- `resolve_contacts`
- `assign_projects`

Existing types unchanged.

---

## 5. Organisation at signup

`auth.Service.Register` and any OAuth user-create path must, in **one transaction**:

1. Insert `users`.
2. Insert `organisations` (`name = 'Personal'`).
3. Set `users.home_organisation_id`.
4. Insert `organisation_members` (`org_role = owner`).

If org insert fails, user insert must roll back.

**Backfill:** for every existing `users` row with null `home_organisation_id`, create an organisation and owner membership, then set the FK. Dev seed user `a0000001-…` included.

Wave 1 authorisation: the caller may only read/write rows whose `organisation_id = home_organisation_id` (or, for mail, `accounts.user_id` is the caller). Do not implement a general ACL engine.

---

## 6. Project codes

- Required on create.
- Normalize: trim, uppercase.
- Validate: `^[A-Z][A-Z0-9]{1,7}$` (2–8 chars). Examples: `DC01`, `HVAC2`, `PLANT`.
- Unique per `organisation_id`.
- High-confidence auto-assign: whole-token match of `code` in subject or body (Unicode letter/digit boundaries; case-insensitive compare against stored uppercase code).
- Medium: case-insensitive substring match of `name` or any `keywords_json` entry in subject (prefer subject over body).
- Drawing-style tokens (`M-402`) are **not** project codes.

---

## 7. Effective project assignment

For a **mail message** `M` on `account_id` with `conversation_id = C`:

1. If a `message_assignment_overrides` row exists for `M`, that is the effective assignment (`project_id` may be null → Unassigned).
2. Else if `C` is non-empty and a `thread_assignments` row exists for `(account_id, C)`, that is the effective assignment.
3. Else Unassigned.

For a message with empty/null `conversation_id`, only an override row (treated as the sole assignment) applies; the assign-thread API is rejected (`conversation_required`).

For a **manual item**, `manual_items.project_id` + `assignment_status` are the effective assignment.

**Assign thread** (default in UI for mail): upsert `thread_assignments`. Do not delete existing overrides.

**Assign this message only:** upsert `message_assignment_overrides`.

**Clear override:** delete the override row so the thread assignment applies again.

**Reassign thread:** update `thread_assignments`; overrides stay.

User correction always `source = user`, `status = committed`. Rule/LLM writes `provisional` unless confidence ≥ 0.9 **and** the match is a project **code** token (then `committed`).

---

## 8. Contact resolution

Run after sync upserts messages (same process or chained job `resolve_contacts`, `account_id` + `run_id` on the job).

For each new/updated message:

1. Parse `from_json`, `to_json`, `cc_json` into `(kind=email, address, name)`.
2. Skip empty addresses.
3. `value_normalized` = lowercase trim.
4. If an identity exists in this organisation: reuse `contact_id`; update `display_name` only if the contact name is empty and the payload has a name.
5. Else create `contacts` + `contact_identities`.
6. Upsert `correspondence_participants`.

**Exact-email reuse is automatic** (same org). Suggesting a merge of two contacts that do not share an email (name similarity, etc.) is Wave 1-optional; if implemented, require confirmation. **Never** create `contact_profile_links`.

**Merge API:** move all identities and participant rows to the surviving contact; set `merged_into_contact_id` on the loser; fail if either contact is already profile-linked (Wave 1: none are). Identity unique violations → reject.

---

## 9. Auto-assign (1b)

Job `assign_projects` after contacts, per account, only for messages whose effective assignment is Unassigned (no override and no thread row).

Order:

1. If any sibling in the same `(account_id, conversation_id)` already has a **committed** effective project, copy that onto `thread_assignments` as `source = rule`, `reason = thread_sibling`, `status = committed`. Do not overwrite overrides.
2. Else if subject/body matches exactly one project **code** token → `thread_assignments` `committed`, `source = rule`.
3. Else if matches exactly one project by name/keyword → `provisional`.
4. Else leave Unassigned.
5. Ambiguous (two codes) → Unassigned, do not guess.

Manual items are not auto-assigned in Wave 1 unless pasted onto a project (then `committed` / `user`).

---

## 10. Retention

| Object | Wave 1 policy |
| ------ | ------------- |
| Messages / mail bodies | No new purge. If a later spec expires mail, it must **not** `CASCADE` contacts or issues. |
| `thread_assignments` / overrides | Deleted with the account or when the user clears assignment. Override rows `ON DELETE CASCADE` from `messages`. Thread rows are **not** deleted because one message expired. |
| Contacts, identities | Kept until merge-hide or user delete. No TTL. User delete of a contact is explicit and blocked if it is an issue assignee or the only identity on open issues — product may allow delete and null the assignee; spec: **restrict** delete while `assignee_contact_id` or `issue_items` still need the contact, or null assignee and keep the issue. Prefer **restrict** for Wave 1. |
| Issues | Kept until user archive/delete. Message delete removes `issue_items` links only. |
| Manual items | User delete removes the item and its links; does not delete contacts. |
| Organisations | Deleted only if the profile is deleted (future); Wave 1 has no org-delete UI. |

Assistant 90-day conversation retention is unrelated and unchanged.

---

## 11. HTTP API

JSON, existing auth (`Authorization` bearer / session). All list items that are mail-derived include `account_id` and `account_label`. Organisation is implied by the caller; do not take `organisation_id` from the client.

Errors: existing `{ "error": { "code", "message" } }` shape.

### 11.1 Contacts (1a)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/contacts` | List. Query: `q`, `limit`, `offset`. Exclude merged. |
| `GET /api/contacts/{id}` | Detail, identities, recent message ids. |
| `POST /api/contacts` | Manual create: `display_name`, optional `company`, optional identities. |
| `POST /api/contacts/{id}/identities` | Add email/phone. |
| `POST /api/contacts/{survivor_id}/merge` | Body: `{ "source_contact_id" }`. |

No `POST /api/contact-profile-links` in Wave 1.

### 11.2 Projects (1b)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects` | List (hide archived by default). |
| `GET /api/unassigned/summary` | Counts for nav badge (`unassigned`, `provisional`). |
| `POST /api/projects` | `{ "name", "code", "description?", "client?", "keywords?", "member"?: { role, discipline, … } }`. Creates project + member row. |
| `GET /api/projects/{id}` | Header fields + member (self). |
| `PATCH /api/projects/{id}` | Name, keywords, archive. Code immutable after create (avoids silent re-match). |
| `PATCH /api/projects/{id}/member` | Self member role/scope fields. |

### 11.3 Assignment (1b)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/unassigned` | Effective Unassigned + provisional. Query: `status=unassigned\|provisional\|all`, paginate. Each row: kind `message` \| `manual`, ids, subject, from contact, account badge, thread id, reason if provisional. |
| `POST /api/messages/{id}/project-assignment` | Body: `{ "project_id": uuid\|null, "scope": "thread"\|"message", "status?": "committed" }`. Default `scope=thread`. `null` project + committed → Unassigned. |
| `DELETE /api/messages/{id}/project-assignment/override` | Drop override only. |
| `POST /api/manual-items/{id}/project-assignment` | `{ "project_id": uuid\|null }`. |

`GET /api/messages` may take `project_id` or `unassigned=true` as a **secondary** filter; Unassigned UX must not depend on Inbox.

### 11.4 Timeline and manual (1c)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects/{id}/timeline` | Unified items by `occurred_at` desc. Query: `source=mail\|manual\|all`, `unassigned_to_issue=true`, cursor pagination. Each item: source, timestamp, contacts, snippet, `account_id?`, `message_id?`, `manual_item_id?`, `issue_id?`. |
| `POST /api/manual-items` | `{ "channel", "occurred_at", "title", "body_text", "project_id?", "participant_contact_ids?" }`. If `project_id` omitted, Unassigned. |

### 11.5 Issues (1d)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/projects/{id}/issues` | List. |
| `POST /api/projects/{id}/issues` | `{ "title", "current_position_note?", "assignee_user_id?", "assignee_contact_id?", "item_refs?" }`. Default assignee = caller. |
| `GET /api/issues/{id}` | Issue + trail items. |
| `PATCH /api/issues/{id}` | Title, note, status, assignee (one of user/contact/null). |
| `POST /api/issues/{id}/items` | `{ "message_id" }` or `{ "manual_item_id" }`. |
| `DELETE /api/issues/{id}/items/{item_id}` | Detach. |

Optional 1d: `POST /api/projects/{id}/issues/suggest` with LLM JSON `{ "title", "item_refs", "confidence" }` — user must confirm via `POST .../issues`.

### 11.6 Derived “awaiting me”

Not a stored status. API may set `awaiting_me: true` when `status = awaiting_input` and `assignee_user_id` is the caller.

---

## 12. Jobs

After `sync` succeeds for an account, enqueue (or inline for tests):

1. `resolve_contacts`
2. `assign_projects`

Same `run_id` chain or child runs with `account_id` set. Failures must not roll back stored messages (PRD: evidence survives AI/job failure). Retry via existing Asynq/`job_runs` behaviour.

Do not mix two `account_id`s in one LLM call if 1d suggestions are added.

---

## 13. UI contract (`web/`)

Keep Assistant `/`, Today, Inbox, Drafts, Rules, Runs, Accounts, Settings.

Add:

| Route | Slice |
| ----- | ----- |
| `/people` | 1a list |
| `/people/:id` | 1a detail / merge |
| `/projects` | 1b list + create |
| `/projects/:id` | 1c timeline-first project |
| `/unassigned` | 1b dedicated queue |
| `/projects/:id/issues/:issueId` | 1d trail (may be a panel on the project page) |

Nav: **Projects**, **People**, **Unassigned** (badge = unassigned+provisional count). Do not put Unassigned only under Inbox.

Project page: header (name, code, member role); timeline dominant; issues rail; paste control. Mail rows use `AccountBadge`. Assign control: default **This thread**; secondary **This message only**.

Unassigned page: sections **Needs confirmation** (provisional) and **Unassigned**; actions assign thread/message/manual; show `reason`.

People: contacts only. No profile directory. No “linked account” control.

---

## 14. LLM (optional, 1d only)

Allowed JSON (single object, existing parse+retry rules):

```json
{
  "schema_version": 1,
  "project_id": "uuid or empty",
  "issue_title": "",
  "message_ids": [],
  "confidence": 0.0,
  "reason": ""
}
```

All `message_ids` must belong to the same `account_id` as the run. Persist as `source = llm`, `status = provisional`. Never write facts or `contact_profile_links`.

---

## 15. Slice exit criteria

**1a.** New register creates `organisations` + owner membership. After sync, From addresses appear as contacts. Duplicate email in the same org reuses the contact. Merge moves identities. `GET /api/contacts` does not list other users. No link rows.

**1b.** Create `DC01`. Assign a thread; a second message in that `conversation_id` shows the same project without an override row. Override one message to another project or Unassigned; reassigning the thread leaves the override. `/unassigned` lists the rest. Contacts on the thread are participants, not `project_members`.

**1c.** Timeline shows Outlook items plus one pasted Teams/WhatsApp item in time order, with evidence links.

**1d.** Issue Pump P-03 exists; several items attached; assignee is the operator profile or a contact; deleting a source message leaves the issue; status is user-correctable.

---

## 16. Testing

- Signup transaction: user without org is impossible after Register.
- Backfill migration: seed user has `home_organisation_id`.
- Contact unique `(org, kind, email)`.
- Two orgs may share the same email on different contacts.
- Thread assign does not write N override rows.
- Override beats thread.
- Auto-assign does not set `project_members`.
- Message delete: contact remains; issue remains; override gone; participant rows gone.
- Assignee check: both contact and user set → reject.
- HTTP: cannot pass another user’s `organisation_id`.
- No handler inserts `contact_profile_links`.

---

## 17. Mapping to PRD frozen decisions

| PRD decision | Spec |
| ------------ | ---- |
| Thread unit + message override | `thread_assignments` + `message_assignment_overrides` |
| Org at signup | Register transaction + `users.home_organisation_id` |
| Link after accept | Table exists; no Wave 1 writes; unique contact and `(org, user_id)` |
| Merge before dual-link | Unique `(organisation_id, user_id)` on links; merge API |
| Structured codes | `^[A-Z][A-Z0-9]{1,7}$` |
| Unassigned page | `GET /api/unassigned` + `/unassigned` |
| Longer retention for contacts/issues | No TTL; no CASCADE from messages onto those aggregates |

---

*Implementation PRs should name the slice (1a–1d) and this spec. Do not land Facts, invites, or link writes in Wave 1.*
