# Addendum: DynamoDB vs relational store (access-pattern review)

**Status:** Draft  
**Parent:** [AWS deployment (Heimdall pattern)](aws-deployment.md)  
**Related:** [Technical specification](initial.md)  
**Last updated:** 2026-08-29

Heimdall hosts product state in DynamoDB. This addendum records whether Automata could do the same. **Conclusion: no, not as the product store.** The access patterns are relational (joins, computed views, multi-entity transactions). DynamoDB would require denormalizing those views at write time and rewriting every repository. Aurora DSQL remains the product-data target; DynamoDB is deliberately used only for the job control plane defined in [aws-deployment.md](aws-deployment.md).

---

## 1. Method

Reviewed:

- Driven ports in `svc/internal/application/ports/driven/persistence.go` (~20 repository interfaces, ~45 persisted entity types).
- SQLite adapters, especially `ListMessages`, `ListProjectTimeline`, `ListUnassigned`, `EffectiveAssignment`, `MergeContacts`, `ListContacts`, `ReassignMessageCategories`, `ListDueSchedules`, fact versioning, and `CreateUserWithHomeOrg`.
- Application aggregations that are not tables: `attention.Service.ForUser`, reconcile, project AI.

Compared against what DynamoDB does well (GetItem/Query by known PK/SK, conditional writes, sparse GSIs) and against Heimdall’s actual usage (job/config/auth documents, stream-triggered workers, no product-graph joins).

---

## 2. Patterns that *would* fit DynamoDB

These are PK/SK or single-GSI lookups. They are **not** the reason to reject DynamoDB.

| Pattern | Current SQL | DynamoDB analogue |
| ------- | ----------- | ----------------- |
| Get user / account / message / project / fact by id | PK + ownership join | Composite key with owner in PK, or condition on attribute |
| List accounts / connector accounts for a user | `WHERE user_id = ?` | `PK = USER#id`, `SK begins_with ACCOUNT#` |
| Upsert mail by Graph id | `UNIQUE (account_id, provider_message_id)` | `PK = ACCOUNT#id`, `SK = MSG#providerId` |
| OAuth state consume | insert / select+delete | GetItem + DeleteItem (or TransactWrite) |
| Refresh session consume | `UNIQUE (token_hash)` then delete | `PK = SESSION#hash`, conditional delete |
| Due schedule tick | `enabled AND next_run_at <= now` | Sparse GSI on `next_run_at` |
| Claim `job_runs` pending | `UPDATE … WHERE status = pending` | Conditional `UpdateItem` + GSI on status |

Worker state and due-job recovery are **more** DynamoDB-native than relational queue claims. The AWS plan therefore adopts the hybrid deliberately: DynamoDB Streams for bounded job continuations, Aurora DSQL for product data. Job uniqueness uses conditional lock items and lifecycle transitions use revision/attempt fencing; do not build a `SKIP LOCKED` queue.

---

## 3. Patterns that do **not** fit without denormalization

These are the product’s hot paths. SQL evaluates them at read time. DynamoDB cannot.

### 3.1 Inbox (`ListMessages`)

`messages` ⋈ `accounts` (authz) ⋈ `message_categories` (LLM source) ⋈ `category_definitions` (slug filter) plus an `EXISTS` over **message override vs thread assignment** for `project_id`. Filters combine: account, category slug, `received_at`, summary/forward unseen, project. Ordered `received_at DESC` with limit/offset.

**DDB implication:** store a denormalized inbox item that already contains `category_slug`, `project_id`, and seen flags. **Every** sync, categorize, assign, and “mark seen” must rewrite that item (and possibly secondary SKs for each filter). Four optional filters mean either many GSIs or filter-in-memory after a time-ordered query.

### 3.2 Project timeline (`ListProjectTimeline`)

Union of:

1. Mail rows whose **effective** assignment is the project (same CASE join as inbox).
2. Manual items for the project.
3. Connector (Slack) messages for the project.

Then **N+1** contact lookups and issue-id lookups per row, in-memory sort by `occurred_at`, then offset/limit. `UnassignedToIssue` filters after the union.

**DDB implication:** a `TIMELINE#{projectId}#{ts}` collection written by mail sync, paste, Slack sync, **and rewritten when a thread is reassigned**. Reassignment of a conversation can touch every message in the thread.

### 3.3 Unassigned queue (`ListUnassigned` / `CountUnassignedSummary`)

Loads up to **2000** recent messages, runs `EffectiveAssignment` **per message** (override then thread), merges unassigned/provisional manuals, sorts in memory, paginates. The nav badge **reuses the full list** and counts.

This is already a SQL-shaped scan. DynamoDB would not make it cheaper unless we **maintain an UNASSIGNED collection at write time** (insert on sync if unassigned, delete on assign).

### 3.4 Effective assignment

Not a table: message override wins; else thread assignment `(account_id, conversation_id)`; else unassigned. Used by inbox project filter, timeline, unassigned, auto-assign (`ListMessagesNeedingAssign`), sibling-thread lookup.

**DDB implication:** denormalize `project_id` / `assignment_status` onto each message at write time, or accept two extra GetItems per message (override + thread) on every list.

### 3.5 Contacts

- List with `LIKE '%q%'` across display name, company, and identities (contains search). DynamoDB has no such operator; OpenSearch or a client-side scan of the org’s contacts would be required.
- `SuggestMerges`: same display name, no shared email identity (`NOT EXISTS` join).
- `MergeContacts`: **one SQL transaction** moving identities and `correspondence_participants`, deleting duplicates, setting `merged_into_contact_id`. DynamoDB `TransactWriteItems` is capped at **100 items**. A contact on many threads exceeds that; merge must become a paginated workflow with partial-failure semantics the product does not have today.

### 3.6 Attention (`GET /api/attention`)

Not a query: for **each** home-org project, load member + issues + facts + decisions + contradictions, then append open mail action items. Fine for a single operator with few projects; a DynamoDB implementation either fans out Queries or maintains a denormalized attention item on every confirm/reject/resolve.

### 3.7 Bulk category rewrite

`ReassignMessageCategories` is one `UPDATE … WHERE category_id = ?`. Unbounded mailbox. DynamoDB: Query a GSI, then update every message item (batches of 25 / transactions of 100).

### 3.8 Fact / decision lineage

`UNIQUE (project_id, subject_key)`, **partial unique** “one active fact version”, supersede pointers, evidence rows, contradiction sides. Uniqueness is application-enforced on DynamoDB. Reconcile writes facts, versions, contradictions, and job_runs together — a graph mutation, not a document replace.

---

## 4. Constraints DynamoDB does not give you for free

Copied from migrations / `migrate.go` (not exhaustive):

| Constraint | Role |
| ---------- | ---- |
| `users.email` unique | Login |
| `(provider, provider_subject)` unique | OAuth link |
| `auth_sessions.token_hash` unique | Refresh rotate |
| `(account_id, provider_message_id)` | Idempotent sync |
| `(user_id, slug)` categories | Vocabulary |
| `(organisation_id, kind, value_normalized)` | Contact identity |
| `(organisation_id, code)` projects | Human codes |
| `(account_id, conversation_id)` thread assignment | One assignment per thread |
| `(project_id, subject_key)` facts | Lineage |
| one `fact_versions.status = 'active'` per fact | Current position |
| `(connector_account_id, provider_event_id)` | Slack idempotency |

On DynamoDB, “unique” means “this primary key exists”. Partial uniques and uniqueness on a non-key attribute need a dedicated item used as a lock, plus careful transactions.

`ON DELETE CASCADE` (delete account → messages, tokens, assignments) becomes a manual fan-out.

---

## 5. Why Heimdall’s DynamoDB pattern does not transfer

Heimdall tables are **documents keyed the way they are read**: jobs, auth, queries, config, aggregation runs. Async work is a **stream on those documents**. There is no “list mail in a project with category and unseen flags”.

Automata’s UI invents **read-time combinations** of edges (message ↔ category ↔ project ↔ contact ↔ issue). That is the definition of a relational workload. Copying Heimdall’s Terraform DynamoDB module would only honestly cover `job_runs`, `oauth_states`, and `auth_sessions`.

---

## 6. Hybrid store

**Jobs in DynamoDB, product graph in Aurora DSQL/Postgres:** matches each engine and is now the selected AWS design. It does add two backup/IAM stories and cross-store `run_id` provenance, so [aws-deployment.md §4](aws-deployment.md#4-job-rearchitecture-heimdall-dynamodb-loop) defines strict ownership, idempotent product writes, deterministic schedule IDs, conditional lock items, and atomic DynamoDB chain handoff. Sessions and OAuth state remain relational unless separately decided.

---

## 7. Decision

| Option | Feasible? | Recommendation |
| ------ | --------- | -------------- |
| DynamoDB as system of record for all Automata data | Only after denormalizing inbox, timeline, unassigned, attention and rewriting ports | **Do not** |
| DynamoDB for the job control plane only | Yes | **Selected** — bounded Lambda continuations and run history |
| Aurora DSQL/Postgres for product data | Yes | **Selected** — accounts/messages/projects/facts |

A move of **product data** to DynamoDB would still be a domain-model change, not a hosting shortcut. The narrow jobs-table use does not change the relational product model.
