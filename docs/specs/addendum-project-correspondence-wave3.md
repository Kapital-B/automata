# Spec Addendum: Project Correspondence Wave 3

**Status:** Draft  
**Last updated:** 2026-08-22  
**PRD:** [docs/prds/addendum-project-correspondence-wave3.md](../prds/addendum-project-correspondence-wave3.md)  
**Prerequisites:** [Wave 1 spec](addendum-project-correspondence-wave1.md), [Wave 2 spec](addendum-project-correspondence-wave2.md)

This spec freezes implementation shape for Wave 3: **live connectors** into the existing correspondence model, and **Path B** invites / membership / contact↔profile links. Product intent lives in the PRD.

---

## 1. Scope

### In scope

- Project invites, accept, member listing; authz by project membership (multi-profile).
- `contact_profile_links` **writes** (confirm after accept); merge-first uniqueness.
- Connector accounts (Slack, Teams, …) with OAuth/token vault patterns parallel to mail `accounts`.
- Ingest pipelines → timeline rows → assignment → interpret / reconcile / Ask (reuse Wave 2 services).
- Transcript and document-revision event shapes as correspondence.

### Out of scope

- Fine-grained ACL beyond project membership.
- Auto membership or auto link on email sighting.
- Outbound Slack/Teams posts (unless a named slice adds confirm-gated send).
- Full DMS / file vault product.

---

## 2. Architecture notes

Keep hexagonal layering. Suggested packages:

```text
application/
  invites/
  links/          # contact↔profile
  connectors/     # shared sync orchestration
adapters/outbound/
  slack/
  teams/          # or microsoft graph channel APIs
  whatsapp/       # optional / bridge
```

Jobs: extend `job_runs` CHECK with connector sync types (e.g. `sync_slack`, `sync_teams`, `sync_whatsapp`, `ingest_transcript`, `ingest_doc_revision`) via the existing table-rebuild migration pattern.

Home-org listing remains default for Path A. Path B authorization: **project membership** (not “any profile in org”) for project-scoped reads/writes. Spec APIs must check `GetProjectMember` for the caller on project routes once a project has members beyond the creator (creator bootstrapped as owner/member in Wave 1).

---

## 3. Schema

### 3.1 `project_invites`

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `project_id` | UUID FK | |
| `email` | text | Invite target (normalized). |
| `invited_by_user_id` | UUID FK | |
| `role` | text | Seed member role. |
| `token_hash` | text | Opaque accept token (store hash only). |
| `status` | text | `pending\|accepted\|revoked\|expired`. |
| `expires_at` | timestamptz | |
| `accepted_by_user_id` | UUID nullable | |
| `accepted_at` | timestamptz nullable | |
| `created_at` / `updated_at` | timestamptz | |

Unique: `(project_id, email)` where status = pending (partial).

### 3.2 `contact_profile_links`

Wave 1 reserved the table. Wave 3 enables writes:

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `organisation_id` | UUID FK | |
| `contact_id` | UUID FK | Unique per org with profile. |
| `user_id` | UUID FK | Profile. Unique per org. |
| `confirmed_at` | timestamptz | |
| `confirmed_by_user_id` | UUID | |
| `created_at` | timestamptz | |

Constraints: one contact per profile per org; one profile per contact per org. Creating a second link requires merge or unlink.

### 3.3 `connector_accounts`

Parallel to mail `accounts` for non-mail providers:

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `user_id` | UUID FK | Owning profile (Path A vault). |
| `provider` | text | `slack\|teams\|whatsapp\|sms\|…` |
| `label` | text | |
| `external_tenant_id` | text nullable | Workspace / tenant. |
| `connection_status` | text | `connected\|error\|disconnected` |
| `scopes_json` | text | |
| `vault_ref` | … | Encrypted tokens (same vault approach as mail). |
| `created_at` / `updated_at` | timestamptz | |

### 3.4 `connector_bindings`

Maps a provider channel/conversation into Automata:

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | UUID PK | |
| `connector_account_id` | UUID FK | |
| `organisation_id` | UUID FK | |
| `external_channel_id` | text | |
| `project_id` | UUID FK nullable | Null → Unassigned queue until bound. |
| `label` | text | |
| `created_at` / `updated_at` | timestamptz | |

### 3.5 Correspondence rows

Prefer **one** of:

**Option A (recommended):** generalize `manual_items` into `correspondence_items` with `source_kind = paste\|slack\|teams\|…` and nullable `connector_account_id` + `provider_event_id`, **or**

**Option B:** keep `manual_items` for paste; add `connector_messages` with the same timeline projection fields Wave 1 already unions.

Either way, timeline repository must return a unified DTO. Unique `(connector_account_id, provider_event_id)`.

### 3.6 Transcript / revision payloads

Store body/snippet on the correspondence row; optional `meta_json` for `{ "doc_id", "revision", "filename" }` or `{ "meeting_id", "duration_s" }`. Attachments tables may link blobs later; Wave 3 minimum is event + text.

---

## 4. Domain rules

### 4.1 Invite accept

1. Valid pending token, not expired.
2. Accepting profile email should match invite email (allow admin override only if product later says so — default: must match).
3. Upsert `project_members` with invite role.
4. Mark invite accepted.
5. Do **not** create `contact_profile_links` here.

### 4.2 Link confirm

1. Caller is the profile being linked (or project owner — pick one in implementation; default: self-link only).
2. Contact in same org; email identity intersects profile email (suggestion strength).
3. Refuse if contact or profile already linked in org.
4. Write link row with `confirmed_at`.

### 4.3 Connector sync

1. Fetch new events since cursor; upsert by provider id.
2. Resolve/create Contacts from participant identities (Wave 1 bootstrap rules).
3. If binding has `project_id`, assign; else Unassigned.
4. Best-effort interpret (Wave 2 hook) — never commit facts.

### 4.4 Authz

- Project GET/PATCH/timeline/facts/… : caller must be member (Path A creator already is).
- Org-level People/Contacts: home-org as today; linked profiles do not expose other orgs.

---

## 5. Application services

| Service | Responsibility |
| ------- | -------------- |
| `invites.Service` | Create/revoke/list/accept |
| `links.Service` | Suggest + confirm contact↔profile |
| `connectors.Service` | OAuth start/callback, list, disconnect |
| `connectors.SyncService` | Provider fetch → upsert → assign |
| Existing Wave 2 | interpret, reconcile, attention, projectai — consume new evidence |

---

## 6. HTTP API (snake_case)

### 6.1 Invites (3a)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/projects/{id}/invites` | `{ "email", "role?" }` |
| `GET /api/projects/{id}/invites` | List pending/recent. |
| `POST /api/invites/{id}/revoke` | Revoke. |
| `POST /api/invites/accept` | `{ "token" }` → membership. |
| `GET /api/projects/{id}/members` | Profiles on project. |

### 6.2 Links (3b)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/contacts/{id}/link-suggestion` | Optional match to current profile. |
| `POST /api/contacts/{id}/link` | Confirm link to current profile. |
| `DELETE /api/contacts/{id}/link` | Unlink. |

### 6.3 Connectors (3c–3e)

| Method & path | Purpose |
| ------------- | ------- |
| `GET /api/connectors` | List connector accounts. |
| `POST /api/connectors` | Start OAuth `{ "provider" }`. |
| `GET /api/connectors/callback` | Provider redirect. |
| `DELETE /api/connectors/{id}` | Disconnect. |
| `POST /api/connectors/{id}/sync` | Enqueue sync → `job_run_id`. |
| `GET/POST /api/connectors/{id}/bindings` | Channel ↔ project bindings. |

### 6.4 Ingest helpers (3f)

| Method & path | Purpose |
| ------------- | ------- |
| `POST /api/projects/{id}/transcripts` | Manual or connector-fed transcript create. |
| `POST /api/projects/{id}/doc-revisions` | Revision event create. |

Ask / attention / facts APIs unchanged.

---

## 7. Jobs

| `job_type` | Trigger | Notes |
| ---------- | ------- | ----- |
| `sync_teams` / `sync_slack` / … | API / schedule | Cursor in `meta_json` |
| `ingest_transcript` | API | Optional LLM cleanup; still evidence-first |
| `ingest_doc_revision` | API / webhook | |

Failures must not delete prior correspondence. Partial sync resumes via cursor.

---

## 8. UI contract (`web/`)

| Element | Slice |
| ------- | ----- |
| Project **Members / Invite** panel | 3a |
| People **Link to my profile** | 3b |
| Accounts/Connectors cards live (not “soon”) | 3c–3e |
| Timeline source badges for connectors | 3c+ |
| Transcript / revision compose or ingest affordance | 3f |

Assistant connector placeholders flip to live only when the matching provider is configured.

---

## 9. Slice exit criteria (testable)

**3a.** Second user accepts invite; `GET` project timeline succeeds for them; fails for a third unrelated user.

**3b.** Link confirm writes one row; second contact for same profile rejected until merge.

**3c.** Teams message with unique provider id appears on DC01 timeline without paste; re-sync is idempotent.

**3d.** Slack message likewise; interpret produces a pending candidate grounded in that item.

**3e.** At least one WhatsApp/SMS path delivers a durable item with provider id (or documented bridge with same invariants).

**3f.** Doc revision event cited by Ask after an operator-confirmed fact update that lists it as evidence.

---

## 10. Testing

- Invite token single-use; expired/revoked rejected.
- Membership required for project-scoped Wave 2 APIs once Path B is enabled in the build (keep Path A creator path green).
- Link uniqueness; merge-first.
- Connector upsert idempotent on provider id.
- Sync does not cross-mix connector + unrelated mail account in one interpret call.
- No handler auto-writes `contact_profile_links` on signup or invite accept.
- Wave 2 `wave2_test.go` exit criteria still pass with connector-sourced manual/correspondence fixtures.

---

## 11. Mapping to Wave 3 PRD

| PRD topic | Spec |
| --------- | ---- |
| Path B invite | `project_invites` + members API |
| Contact↔profile | `contact_profile_links` writes |
| Live connectors | `connector_accounts` + bindings + sync jobs |
| One memory model | Unified timeline DTO → Wave 2 pipeline |
| Transcripts / revisions | Correspondence events + meta_json |

---

*Implementation PRs should name the slice (3a–3f) and this spec. Do not land auto-link or silent fact activation under Wave 3.*
