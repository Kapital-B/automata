# Addendum: Aurora DSQL as the hosted store

**Status:** Draft  
**Parent:** [AWS deployment (Heimdall pattern)](aws-deployment.md)  
**Related:** [DynamoDB access-pattern review](addendum-dynamodb-access-patterns.md)  
**Last updated:** 2026-08-29

This addendum records whether **Amazon Aurora DSQL** can host Automata’s relational model. **Conclusion: yes**, if we add a PostgreSQL-dialect adapter (not a SQLite port) and design around DSQL’s transaction and index limits. DSQL is the preferred hosted store for cost (pay-per-DPU, no VPC/NAT). It is **not** a drop-in for Aurora PostgreSQL.

Sources: [SQL compatibility](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility.html), [data types](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-data-types.html), [quotas](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html), [CREATE INDEX](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-index-syntax-support.html), [concurrency](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-concurrency-control.html), [release notes](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/release-notes.html) (FKs 2026-08-26, JSONB 2026-06-08, expression indexes 2026-08-13). Older blog posts that say “no FKs / no JSONB” are stale.

---

## 1. Verdict

| Question | Answer |
| -------- | ------ |
| Can DSQL run Automata’s access patterns (joins, inbox, timeline, assignments)? | **Yes** — standard SELECT/JOIN/CTE/EXISTS. |
| Can we keep FKs, CHECK enums, unique natural keys, `ON CONFLICT`? | **Yes** (FKs as of Aug 2026; `INSERT … ON CONFLICT` supported). |
| Can Lambda reach DSQL without NAT? | **Yes** — public endpoint + IAM auth (Go connector). Available in **eu-west-2**. |
| Drop-in from SQLite? | **No** — new adapter, Postgres types, `$1` placeholders, `CREATE INDEX ASYNC`. |
| Drop-in from Aurora PostgreSQL? | **No** — no partial indexes, OCC instead of `SKIP LOCKED`, 3,000-row / 5-minute / 10 MiB write caps. |

---

## 2. What already fits

- **Types we need:** `uuid`, `timestamptz`, `boolean`, `text`/`varchar`, `bytea` (tokens; **not** indexable — we do not index ciphertext), `json`/`jsonb` with compression (message payloads, product metadata). JSON/JSONB **cannot be index keys**. Job control fields (`connector_account_id`, `remaining_jobs`, cursors) live on **DynamoDB**, not in DSQL jsonb.
- **DDL/DML:** `CREATE TABLE`, CHECK, UNIQUE, FK including `ON DELETE CASCADE` / `SET NULL`, `INSERT … ON CONFLICT`, UPDATE/DELETE, views.
- **Queries:** inner/outer joins, `WITH`, `UNION`, window functions, `LIKE`/`ILIKE`. Expression indexes (`lower(title)`) exist as of 13 Aug 2026 — useful for case-insensitive email/name uniqueness instead of SQLite `COLLATE NOCASE`.
- **Sync today:** `SyncInbox` does **not** wrap Graph I/O in one SQL transaction; `database/sql` auto-commits each `UpsertMessage`. The **3,000-row mutation cap therefore does not bind a large delta sync** unless we later wrap the loop in `BeginTx`.
- **Idle cost:** DPU = 0 when idle. A 1-minute **scheduler** that only touches DSQL for due schedules / OAuth GC is cheap DPU. Job execution state lives in **DynamoDB**; workers open DSQL only while running a chunk.

---

## 3. Limits that must be designed in

### 3.1 Per-transaction caps (non-negotiable)

| Cap | Value | Automata risk |
| --- | ----- | ------------- |
| Mutated rows | **3,000** | `ON DELETE CASCADE` when deleting an account with a large mailbox; `ReassignMessageCategories` (one `UPDATE`); `MergeContacts` (identities + participants); `InsertActionItems` (all rows in one tx today) |
| Write set size | **10 MiB** | Unlikely for our rows |
| Transaction age | **5 minutes** | Only if Graph/LLM work is inside a SQL transaction — **do not do that** |
| Connection life | **60 minutes** | Lambda pool: recycle ~55 min |
| Query memory | **128 MiB** | Unassigned “scan 2000 messages” is OK; do not add huge analytical queries |

**Rule:** any bulk mutate (delete account, reassign category, merge, batch inserts) **chunks at ≤ ~2,500 rows** and commits between chunks. Document that CASCADE on `accounts` must not be relied on for large mailboxes — delete messages in batches in the application, then delete the account.

Cascading FK actions **count toward the 3,000-row limit**.

### 3.2 No partial indexes

`CREATE INDEX` has **no `WHERE` clause**. Affected Automata indexes:

| Current SQLite | DSQL approach |
| -------------- | ------------- |
| `UNIQUE (fact_id) WHERE status = 'active'` | Generated stored column: `active_fact_id = CASE WHEN status = 'active' THEN fact_id END`, `UNIQUE` on that column. Default **NULLS DISTINCT** allows many non-active versions. |
| Unique `(contact_id, role, message_id) WHERE message_id IS NOT NULL` | Unique on those columns; NULLs are distinct so multiple manual-only rows remain valid. |
| `UNIQUE (message_id) WHERE message_id IS NOT NULL` on `issue_items` | Unique on `message_id` (multiple NULLs OK). |
| `idx_messages_account_summary_unseen … WHERE summary_seen_at IS NULL` | Full index `(account_id, summary_seen_at)` or `(account_id)` and filter in SQL. |

### 3.3 Optimistic concurrency (no `SKIP LOCKED`)

`SELECT FOR UPDATE` does **not** block; conflicts abort at commit with **SQLSTATE 40001**. Retry product updates that hit `40001`.

**Jobs are not claimed in DSQL.** Lease, status, and cursor live on the DynamoDB job item (Heimdall stream loop in [aws-deployment.md §4](aws-deployment.md#4-job-rearchitecture-heimdall-dynamodb-loop)). Do **not** build a `SKIP LOCKED` / `UPDATE job_runs … pending` queue in DSQL.

### 3.4 Other gaps (non-blocking)

- **No extensions** (no `pgcrypto`, `pg_trgm`). AES-GCM stays in Go (`security.NewAESGCMVault`). Contact search stays `ILIKE`; one organisation is small.
- **No PL/pgSQL / triggers.** All logic already in application services.
- **Indexes:** `CREATE [UNIQUE] INDEX ASYNC` only; migrations must wait (`sys.wait_for_job`).
- **jsonb not indexable.** Job payload fields that used to live in `job_runs.meta_json` (e.g. `connector_account_id`) belong on the **DynamoDB job item**, not as jsonb index keys in DSQL.
- **text/jsonb ~1 MiB compressed per value; row 2 MiB.** Truncate or store elsewhere if a Graph body exceeds this.
- **Max 24 indexes per table.** `messages` stays well under this if we do not naively copy every SQLite partial index as a full one.

---

## 4. Runtime on Lambda

- Connect with the **AWS DSQL Go connector** (IAM token, TLS). Instantiate a small pool (1–3) **outside** the handler; set max connection lifetime **&lt; 60 minutes**.
- **Do not put API/scheduler/worker Lambdas in a VPC** solely for the database. Keep Graph, Entra, Google, Slack, and LLM on the public internet.
- Auth: disable `AUTH_DEFAULT_USER_ID` in AWS (unchanged from the parent spec).

---

## 5. Local and tests

| Environment | Store |
| ----------- | ----- |
| Unit tests | SQLite (fast) and/or Postgres via testcontainers |
| Local AWS-parity | Floci (Lambda, DynamoDB Streams, EventBridge) + **Postgres 16** in Automata Compose — DSQL is not emulated; same Postgres dialect adapter as AWS |
| AWS | **Aurora DSQL** |
| CI compatibility | Common + Postgres migrations/repositories on every PR; mandatory dev-DSQL migrations and representative repository/OCC tests before promotion |

Split migrations into `common`, `postgres`, and `dsql`; vanilla Postgres must never receive `CREATE INDEX ASYNC` or `sys.wait_for_job`. Do not compile SQLite `PRAGMA` / `INSERT OR IGNORE` / `COLLATE NOCASE` into the hosted path.

---

## 6. Terraform / CI delta vs parent spec

Replace “Aurora Serverless v2 / RDS in VPC / RDS Proxy” with:

- `aws_dsql_cluster` in **eu-west-2**, AWS provider **>= 6.15, < 7** (DSQL exists from 5.100; 6.15 fixes deletion-protection behaviour used by the plan)
- IAM `dsql:DbConnect` on API, scheduler, and worker; migration Lambda alone gets `dsql:DbConnectAdmin`
- DynamoDB `automata-jobs` + Streams for the Heimdall job loop ([aws-deployment.md §4](aws-deployment.md#4-job-rearchitecture-heimdall-dynamodb-loop)); no Redis/SQS for jobs
- Cluster ARN/endpoint, region, and database role in environment variables; no long-lived `DATABASE_URL` password secret for DSQL
- DSQL deletion protection plus daily full-cluster AWS Backup plan and tested restore before prod
- **No** NAT Gateway, **no** RDS security groups, **no** RDS Proxy for the database

Scheduler EventBridge `rate(1 minute)` stays for due schedules / GC / watchdog only. Workers are DynamoDB stream-triggered and only burn DPU while executing a chunk against product tables.

---

## 7. Decision

**Use Aurora DSQL as the hosted system of record for product data.** Use **DynamoDB for the job control plane** (Heimdall stream loop), not for accounts/messages/projects ([addendum-dynamodb-access-patterns.md](addendum-dynamodb-access-patterns.md)). Treat provisioned RDS / Serverless v2 as a product-data fallback only if DSQL’s 3,000-row cap or missing partial indexes prove unworkable — that is an engineering spike risk, not the default plan.
