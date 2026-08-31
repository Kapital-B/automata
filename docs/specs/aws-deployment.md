# Technical Specification: AWS deployment (Heimdall pattern)

**Status:** Draft  
**Parent documents:** [Technical specification (initial.md)](initial.md) · [Aurora DSQL](addendum-aurora-dsql.md) · [DynamoDB access-pattern review](addendum-dynamodb-access-patterns.md)  
**Companion PRD:** [docs/prds/initial.md](../prds/initial.md)  
**Last updated:** 2026-08-29  
**Source of pattern:** [Heimdall](https://github.com/Yonda-Tax/heimdall) (`svc/terraform`, `web/terraform`, `.github/workflows`)

This document specifies how to host Automata on AWS using the **same infrastructure and CI pattern as Heimdall**, and how to **replace the long-running Asynq worker** with Heimdall’s **DynamoDB Streams self-triggering job loop** (plus a thin EventBridge scheduler). Product data stays on **Aurora DSQL**; the **job control plane** is **DynamoDB**. Product behaviour in [initial.md](initial.md) is unchanged; this document is the source of **hosting, packaging, job state-machine, and job-dispatch invariants** for AWS. It supersedes the Redis/Asynq and relational-`job_runs` recommendations in older addenda wherever they conflict.

---

## 1. Goals and non-goals

**Goals**

- Deploy **`svc/`** (Go API + background work) and **`web/`** (Vite SPA) independently, with the same Terraform/GitHub Actions shape Heimdall uses.
- Run the HTTP API on **Lambda + API Gateway** (AWS_PROXY catch-all), not a long-lived process.
- Run background work with Heimdall’s **DynamoDB Streams loop**: each job write/update triggers a **worker** Lambda that does **one bounded, restartable unit of work** (one page/batch or one chain transition), then conditionally writes the next state so the stream fires again.
- Use a short **scheduler** Lambda (EventBridge `rate(1 minute)`) only for due `schedule_chains`, OAuth GC, and stuck-job watchdog — **not** to execute Graph/LLM work.
- Persist **product** data in **Aurora DSQL**. Persist **jobs** in **DynamoDB** (same control-plane pattern as Heimdall). SQLite on Lambda is not viable for either.
- Keep **hexagonal ports**: HTTP, scheduler, and stream worker remain thin driving adapters into the same `internal/application` services.
- Ship **dev** and **prod** environments with OIDC-based GitHub deploys, Terraform plan comments on PRs, and environment-gated applies.
- Run **local** as close to AWS as practical: **Floci** emulates Lambda, API Gateway, DynamoDB (+ Streams), EventBridge, and Secrets Manager; **Postgres** stands in for DSQL (which Floci cannot emulate). Document the flow in the repo READMEs.

**Non-goals**

- Do **not** migrate product auth to Cognito. Automata already owns email/password, Microsoft, and Google sign-in plus JWT/refresh tokens. Heimdall’s Cognito wiring is a product difference, not a hosting requirement.
- Do **not** rewrite the **product** domain model onto DynamoDB. Accounts, messages, projects, facts, etc. stay relational on DSQL ([addendum-dynamodb-access-patterns.md](addendum-dynamodb-access-patterns.md)). DynamoDB is reserved for the **job system**.
- Do **not** keep Redis/Asynq (or SQS) as a required AWS job dependency. Job durability and fan-out live in DynamoDB + Streams, matching Heimdall.
- Do **not** treat “`go run` + SQLite + in-process jobs” as the AWS-parity local path. That may remain a fast unit-test / debug shortcut, but **day-to-day local behaviour must run through Floci** (§4.8, §6.7).

---

## 2. Current state

### 2.1 Automata today

| Concern | Current implementation |
| ------- | ---------------------- |
| Layout | Monorepo: `svc/` (Go 1.24, chi) and `web/` (Vite + React + TypeScript) |
| API process | `svc/cmd/server` — long-running `http.Server` on `LISTEN_ADDR` |
| Worker process | `svc/cmd/worker` — long-running [Asynq](https://github.com/hibiken/asynq) consumer on Redis |
| Scheduler | Goroutine inside `cmd/server`: ticker every **30s**, `schedulerService.Tick` lists due `schedule_chains` and enqueues the first job |
| OAuth GC | Goroutine inside `cmd/server`: ticker every **5m**, deletes expired `oauth_states` |
| Queue | Redis + Asynq. HTTP handlers insert `job_runs` as `pending`, then `JobQueue.Enqueue*` |
| Persistence | SQLite via `modernc.org/sqlite`, migrations embedded and applied on process start |
| Auth | App-issued JWT (`JWT_SECRET`) + hashed refresh tokens. Dev fallback: `AUTH_DEFAULT_USER_ID` when no Bearer header |
| Frontend | Talks to `VITE_API_BASE_URL`; stores tokens in `localStorage`; OAuth return via `DASHBOARD_BASE_URL` |
| Infra / CI | **None.** No `terraform/`, no `.github/workflows/` |
| Local extras | `docker-compose.yml` runs Redis 7 only |

Asynq consumes six types today: `sync`, `categorize`, `summarize`, `draft_suggest`, `forward_rules`, and `sync_slack`. Four more application paths currently create `job_runs` without Asynq: `assign_projects`, `project_ai`, `interpret_project`, and `reconcile_project`; the SQLite CHECK/spec also declares `resolve_contacts`. Schedule chains enqueue the first Asynq type, then `maybeEnqueueNext` inserts the next `job_runs` row and task. The target job registry in §4.4 must classify **all eleven declared** names; adding a job type without a registry entry is not allowed.

**Critical coupling:** Asynq task payloads carry fields that are **not first-class columns** on `job_runs` (`user_id`, `remaining_jobs`, `schedule_id`, `chain_started_at`, `recategorize`, `message_id`, `force`, `connector_account_id`). On AWS those fields live on the **DynamoDB job item** (payload / remaining_jobs / cursor). Do not leave them only in Redis.

### 2.2 Heimdall pattern (what we copy)

Heimdall is the same **svc + web monorepo** shape. Hosting is **not ECS**; it is Lambda-centric.

**Backend (`svc/terraform`)**

```
svc/terraform/
  modules/heimdall/          # Lambda, API Gateway, DynamoDB, IAM, EventBridge
  modules/heimdall_aws/      # Secrets Manager bootstrap for the AWS account
  envs/local|dev|prod/       # backend "s3", locals, module wiring
  scripts/bootstrap/         # DynamoDB lock table (applied once per account)
```

- One zip built in Terraform (`null_resource` + `archive_file`); **several Lambdas share the zip** and differ by **handler**.
- API Lambda behind **API Gateway REST**, `{proxy+}` + root `ANY` → `AWS_PROXY`.
- Stream / scheduled work are **separate functions** (DynamoDB streams; EventBridge `rate(5 minutes)` for integration snapshots).
- Optional **migrations Lambda** invoked with `aws_lambda_invocation` after deploy (`run_migrations_on_deploy`).
- State: S3 bucket + DynamoDB lock table. Dev and prod use **different AWS accounts** and **different state buckets**.
- Region for API: **eu-west-2**.

**Frontend (`web/terraform`)**

```
web/terraform/
  modules/heimdall_web/      # S3 (private) + CloudFront OAC + ACM + Route53 + build/sync
  envs/dev|prod/
  scripts/bootstrap/         # separate lock table `heimdall_web-lock-table`
```

- CloudFront ACM certificates live in **us-east-1**, so the **web Terraform provider region is us-east-1**.
- Custom DNS: `heimdall.{hosted_zone}` (dev: `heimdall.dev.yondatax.com`).
- Build happens **inside Terraform** (`npm run build` with `VITE_*` env), then `aws s3 sync` + CloudFront invalidation.
- SPA 404/403 → `/index.html`.

**GitHub Actions**

- Path-filtered workflows: `svc/**` → backend, `web/**` → frontend.
- `permissions: id-token: write` + `aws-actions/configure-aws-credentials` assuming `role/terraform-github-actions-role`.
- Jobs: **Test** (backend only) → **Plan** (matrix `dev`/`prod`) → **Apply (dev)** (PRs + `main`) → **Apply (prod)** (`main` only, GitHub Environment `prod`).
- Terraform **1.10.3**. Plan posted as a sticky PR comment.
- Dev apply uses GitHub Environment `dev`; prod uses `prod` (protection rules live in GitHub, not Terraform).

---

## 3. Target architecture

```mermaid
flowchart LR
  subgraph clients [Clients]
    Browser[Browser]
    IdP[Entra / Google]
  end

  subgraph edge [Edge]
    CF[CloudFront + ACM]
    S3[S3 web bucket]
    APIGW[API Gateway REST]
  end

  subgraph compute [Lambda]
    API[automata-api-handler]
    Scheduler[automata-scheduler-handler]
    Worker[automata-worker-handler]
    Migrate[automata-migrations-runner]
  end

  subgraph data [Data]
    DSQL[(Aurora DSQL product)]
    Jobs[(DynamoDB automata-jobs)]
    SM[Secrets Manager]
  end

  subgraph schedule [Schedule]
    EB[EventBridge rate 1 minute]
  end

  Browser --> CF
  CF --> S3
  Browser --> APIGW
  IdP --> APIGW
  APIGW --> API
  API --> DSQL
  API --> Jobs
  API --> SM
  EB --> Scheduler
  Scheduler --> DSQL
  Scheduler --> Jobs
  Jobs -->|DynamoDB Streams| Worker
  Worker --> DSQL
  Worker --> Jobs
  Worker --> SM
  Worker --> IdP
  Worker --> LLM[OpenAI-compatible LLM]
  Migrate --> DSQL
```

| Component | AWS resource | Notes |
| --------- | ------------ | ----- |
| SPA | S3 + CloudFront + ACM + Route53 | Copy Heimdall web module; bake `VITE_API_BASE_URL` at build time |
| HTTP API | Lambda (`provided.al2023`) + API Gateway REST `{proxy+}` | Chi router wrapped with `aws-lambda-go-api-proxy` |
| Scheduler | Lambda + EventBridge `rate(1 minute)` | Due `schedule_chains`, OAuth GC, stuck-job watchdog — **creates/resets DynamoDB job items only** |
| Worker | Lambda + DynamoDB Streams | Heimdall loop: **one unit of work per invocation**, then self-write to continue |
| Job control plane | DynamoDB table `automata-jobs` (+ stream) | Source of truth for job status, cursor, chain; replaces Redis/Asynq/SQS |
| Product database | **Aurora DSQL** (see [addendum-aurora-dsql.md](addendum-aurora-dsql.md)) | Accounts, messages, projects, facts, … — not jobs |
| Migrations | Lambda, invoked from Terraform on apply | Same pattern as Heimdall `migrations_runner` (DSQL schema only) |
| Secrets | Secrets Manager `automata/*` | Encryption key, JWT secret, OAuth client secrets, LLM key, DSQL endpoint |
| Outbound | Public internet from Lambda | Graph, Entra, Google, Slack, LLM — **no NAT** if Lambdas stay out of a VPC |
| Observability | Managed CloudWatch log groups, metrics, alarms/dashboard + stream-failure S3 | Structured JSON `slog`; optional OTLP later |

**Deliberate deviations from Heimdall**

| Heimdall | Automata |
| -------- | -------- |
| Python 3.13 zip via `uv pip install --target` | Go `provided.al2023` bootstrap binaries |
| DynamoDB for product **and** jobs | **DSQL for product**, **DynamoDB for jobs only** |
| No VPC | No VPC (DSQL IAM + public endpoint; DynamoDB public), same as Heimdall Lambdas |
| Cognito JWT verification | Existing app JWT; disable `AUTH_DEFAULT_USER_ID` in AWS |
| DynamoDB stream Lambda for async jobs | **Same pattern** (copy Heimdall’s stream wiring; Go worker) |
| Same zip, Python handler path | Separate Go bootstrap binaries per function (api / scheduler / worker / migrate) |

### 3.1 Architecture invariants

These are acceptance rules, not implementation suggestions:

1. **Canonical public run contract stays compatible:** `pending | running | success | failed | cancelled`, with `started_at` / `finished_at`, as defined by [initial.md](initial.md). Internal adapters must not leak alternative `in_progress` / `completed` names.
2. **One invocation is bounded:** no handler may traverse an unbounded mailbox, project, connector, or message set. Every streamed job type defines its batch/page boundary and durable cursor in §4.4.
3. **One lifecycle owner:** the stream worker owns status/cursor transitions for asynchronous jobs. Application services accept run context and return a chunk result; they do not update `JobRunRepository` themselves. Synchronous audited operations use the same `JobExecutionPort` from their API orchestration boundary.
4. **Every transition is conditional and fenced:** writes include expected `revision` and `attempt_id`. A timed-out invocation cannot commit after a watchdog starts a replacement attempt.
5. **Chain handoff is atomic inside DynamoDB:** completing a step, releasing/transferring locks, and creating the deterministic next step happen in one `TransactWriteItems`.
6. **Cross-store writes are retryable, not atomic:** DSQL product writes commit before the DynamoDB cursor/status update and must be idempotent by product key + `run_id` where appropriate. There is no pretend distributed transaction.
7. **Irreversible external effects declare semantics:** forwarding is at-most-once with an effect ledger and explicit `unknown` reconciliation state; it is never blindly retried.
8. **No scan-dependent API or recovery path:** every list, lock, and watchdog query has a documented key/GSI.

---

## 4. Job rearchitecture (Heimdall DynamoDB loop)

This is the largest application change. [addendum-redis-asynq-jobs.md](addendum-redis-asynq-jobs.md) remains valid for **local Asynq** until the new pipeline lands; **AWS must not depend on Redis or SQS**.

### 4.1 Why the current worker cannot ship as-is

`svc/cmd/worker` is a **long-running Asynq server**. Lambda is request-scoped: there is no process that “listens” on Redis. The scheduler and OAuth GC are **goroutines inside `cmd/server`**, which also disappear when the API becomes a Lambda.

A single EventBridge Lambda that both **finds** and **executes** all due work does not scale. SQS fan-out helps concurrency across jobs, but **within** one sync/categorize run (and across a schedule chain) we still need a way to **break work into many Lambda invocations** without a long-lived consumer.

Heimdall already solved that with a **DynamoDB Streams self-triggering loop** (`heimdall-jobs` → stream handler → process one page → write `pagination_token` / status → stream fires again). Automata copies that pattern for its job system.

### 4.2 Two stores, clear ownership

| Store | Owns | Does not own |
| ----- | ---- | ------------ |
| **Aurora DSQL** | Product graph: accounts, messages, categories, projects, facts, connectors, schedules metadata, OAuth rows, … | Job execution state / queue |
| **DynamoDB `automata-jobs`** | Job lifecycle and run history: status, payload, cursor, progress, chain, errors, lease/fence, active-job locks and irreversible-effect ledger | Message bodies or other product rows |

**`job_runs` today (SQLite):** on AWS and the Floci parity path, the **DynamoDB job item is the durable inbox** the UI and `GET /api/runs*` read. Provenance columns on DSQL rows (`run_id` on categories, summaries, …) keep the UUID but **drop the FK** to relational `job_runs`. The parity path must not keep a second SQLite lifecycle row; SQLite remains a unit-test adapter only.

Replace direct application-service use of `JobRunRepository` with two explicit ports:

- **`JobStore`** — DynamoDB persistence/state-machine operations: create, get/list, conditional start/advance/fail/cancel, atomic chain handoff, lock/effect records, and recovery queries.
- **`JobExecutionPort`** — orchestration boundary that supplies `{run_id, attempt_id, deadline}` to use cases and accepts `ChunkResult`; it owns lifecycle writes. Asynchronous implementation is the stream worker. A synchronous audited API operation may use the same port, but the underlying domain service must not write run state.

**No SQS.** Stream delivery is the fan-out. API-triggered work starts as soon as `PutItem` lands (no wait for the next EventBridge minute).

### 4.3 Target dispatch model (Heimdall loop)

| Lambda | Trigger | Responsibility |
| ------ | ------- | -------------- |
| **API** | API Gateway | Insert product rows in DSQL; **`PutItem` job `pending`** in DynamoDB (stream starts work) |
| **Scheduler** | EventBridge `rate(1 minute)` | OAuth GC (DSQL), deterministic due-schedule jobs, pending re-wake, fenced watchdog for stuck `running` |
| **Worker** | DynamoDB Streams (`batch_size = 1`, `NEW_AND_OLD_IMAGES`) | One unit of work; write next state on the **same** item (or create the next chain item) |

```mermaid
sequenceDiagram
  participant API as API Lambda
  participant DDB as DynamoDB jobs
  participant W as Worker Lambda
  participant DSQL as Aurora DSQL
  participant EB as EventBridge
  participant S as Scheduler Lambda

  API->>DDB: TransactWrite job=pending + optional active lock
  API-->>API: 202 job_id
  DDB-->>W: INSERT stream
  W->>DDB: conditional pending → running, attempt_id+lease (kick; return)
  DDB-->>W: MODIFY stream
  W->>DSQL: read/write product data for one chunk
  alt more chunks
    W->>DDB: conditional cursor+progress+revision (stay running)
    DDB-->>W: MODIFY → next chunk
  else job_type done, remaining_jobs non-empty
    W->>DDB: TransactWrite current=success + deterministic next=pending
    DDB-->>W: INSERT → next step
  else chain done
    W->>DDB: conditional status=success + release active lock
  end

  EB->>S: rate(1 minute)
  S->>DSQL: tick due schedule_chains
  S->>DDB: idempotent PutItem deterministic scheduled job
  S->>DSQL: conditionally advance matching next_run_at
  S->>DDB: re-wake old pending / fence stale running
```

**Unit of work (one Lambda invocation)** — pick exactly one:

1. **Kick:** conditionally transition `pending` → `running`, assign a fresh `attempt_id`, set `started_at` (once), `lease_until`, and increment `revision`; then **return**. The MODIFY runs the real work.
2. **Chunk:** process exactly the registered page/batch for `job_type` using the durable cursor. Commit DSQL writes, then conditionally advance cursor/progress/lease using the expected `revision` + `attempt_id`. If more remain, stay `running`.
3. **Chain step complete:** current `job_type` finished with no cursor left:
   - Use **one DynamoDB item per step** (matches today’s `job_runs` UX).
   - In one `TransactWriteItems`, conditionally set this item `success`, put the deterministic next item as `pending`, and transfer/release any active-job lock. The next ID is UUIDv5 (or equivalent deterministic hash) of `chain_id + step_index + job_type`.
4. **Terminal:** `success`, `failed`, or `cancelled`; handler skips terminal statuses. Terminal writes set `finished_at`, clear lease ownership, and release any active lock in the same transaction.

### 4.4 Job contract, keys, and registry

Canonical job item (`pk=JOB#<id>`, `sk=METADATA`):

```json
{
  "pk": "JOB#<uuid>",
  "sk": "METADATA",
  "entity_type": "job",
  "schema_version": 1,
  "job_id": "<uuid>",
  "job_type": "sync",
  "status": "pending|running|success|failed|cancelled",
  "user_id": "<uuid>",
  "account_id": "<uuid or null>",
  "trigger_kind": "api|schedule",
  "chain_id": "<uuid>",
  "step_index": 0,
  "remaining_jobs": ["categorize", "summarize"],
  "schedule_id": "<uuid or omit>",
  "scheduled_for": "<RFC3339 or omit>",
  "chain_started_at": "<RFC3339 or omit>",
  "cursor": { "kind": "graph_next_link|message_keyset|…", "value": "…" },
  "progress": { "processed": 0, "failed": 0, "detail": {} },
  "payload": {
    "connector_account_id": "<uuid or omit>",
    "message_id": "<uuid or omit>",
    "recategorize": false,
    "force": false,
    "time_window_start": "<RFC3339 or omit>",
    "time_window_end": "<RFC3339 or omit>"
  },
  "error_message": null,
  "error_count": 0,
  "cancel_requested_at": null,
  "retry_not_before": null,
  "revision": 0,
  "attempt_id": null,
  "lease_owner": null,
  "lease_until": "<RFC3339 or omit>",
  "wake_token": "<uuid>",
  "created_at": "<RFC3339>",
  "started_at": null,
  "updated_at": "<RFC3339>",
  "finished_at": null,
  "expires_at": "<epoch seconds, terminal items only>"
}
```

This replaces Asynq payloads **and** the need to put execution control in DSQL `job_runs.meta_json`. The API may omit internal fields (`revision`, leases, GSI keys) but preserves the existing public status/timestamp names. Payload decoding is versioned; unknown versions or job types fail terminally without calling product services.

#### 4.4.1 Required access patterns

No correctness path may scan the table:

| Access pattern | Key/index |
| -------------- | --------- |
| Get run by id | Base key `JOB#<id>` / `METADATA`, then verify `user_id` |
| `GET /api/runs` for a user, newest first | GSI1: `USER#<user_id>` / `CREATED#<created_at>#JOB#<id>` |
| Account run history | GSI2: `ACCOUNT#<account_id>` / `CREATED#<created_at>#JOB#<id>` |
| Watchdog / operations by active status | GSI3: `STATUS#<pending|running>` / `UPDATED#<updated_at>#JOB#<id>` |
| Effect recovery/audit convergence | GSI3: `EFFECTSTATE#<claimed|unknown|succeeded_pending_audit>` / `UPDATED#<updated_at>#EFFECT#<key>` |
| Account + job type history | GSI4: `ACCOUNT#<account_id>#TYPE#<job_type>` / `CREATED#<created_at>#JOB#<id>` |
| User + job type history | GSI5: `USER#<user_id>#TYPE#<job_type>` / `CREATED#<created_at>#JOB#<id>` |
| Delete/list forwarding effects for an account | Base query `ACCOUNT#<account_id>` / `begins_with(sk, "EFFECT#FORWARD#")` |
| One active sync | **Lock item**, not a GSI read: `MAILBOX#<account_id>` or `CONNECTOR#<connector_account_id>` / `LOCK#SYNC` |

If the status partition becomes hot, shard GSI3 deterministically (`STATUS#running#00..0f`) and query all shards in the scheduler. Do not add a uniqueness claim based on “query then put”: that races.
GSI3 is sparse: only active jobs and recoverable effects carry its key attributes; terminal jobs and settled effects remove them.

Create a sync job and its lock in one `TransactWriteItems`. Mail uses `MAILBOX#<account_id>` / `LOCK#SYNC`; connector sync uses `CONNECTOR#<connector_account_id>` / `LOCK#SYNC`:

- conditionally put the lock with `owner_job_id`, `owner_attempt_id`, `lease_until`, and `lock_revision`;
- put the job with `attribute_not_exists(pk)`;
- on a competing request, return the lock’s existing `owner_job_id`;
- initial pending lock lease is 5 minutes; the pending reconciler renews it only when the owner job is still pending;
- the kick transaction sets the same new `attempt_id` on job and lock;
- each chunk transaction advances job revision/cursor and renews the lock only if owner job/attempt/lock revision still match;
- the watchdog may replace an expired attempt only in a transaction that conditions both the observed job and lock revisions; direct API callers do not steal an expired lock and instead return/re-wake its owner;
- release or transfer the lock in the same transaction as terminal/chain state; orphan cleanup first proves the owner job is terminal/missing;
- TTL is cleanup only and is never the correctness mechanism.

The initial running lease is **960 seconds** (15-minute Lambda timeout + 60-second grace) and is refreshed after each chunk commit. A chunk must stop before the Lambda deadline; the lease is not a license to run for its full duration.

#### 4.4.2 Job type registry and chunk boundary

Create one typed registry in application code. It validates payload schema, execution mode, maximum chunk, cursor codec, retry class, and external-effect policy. Initial entries:

| Job type | Mode and one invocation | Cursor / idempotency |
| -------- | ----------------------- | -------------------- |
| `sync` | Streamed; one Microsoft Graph delta page, request `$top=100`, hard maximum 100 messages / 4 MiB response | Persist `nextLink` in DynamoDB; product upsert by `(account_id, provider_message_id)`; write account `deltaLink` only after the final page succeeds |
| `sync_slack` | Streamed; one Slack history page, `limit=200` | Provider cursor; upsert by `(connector_account_id, provider_event_id)` |
| `categorize` | Streamed; at most 25 messages and 25 LLM calls, with ≥60s state-write reserve | Fixed run cutoff + `(received_at,id)` keyset; one category per message, upsert on `message_id` with this `run_id` |
| `summarize` | Streamed; snapshot at most 240 messages. Map phase: one LLM batch (default 12, hard maximum 30); reduce phase: at most 20 partials | Fixed window + keyset; partial unique `(run_id, chunk_index)`, final snapshot unique `(run_id, account_id, window_start, window_end)` |
| `draft_suggest` | Streamed; exactly one `message_id` | Upsert one local draft with unique `(run_id, message_id)`; normalize legacy alias `auto-draft` at enqueue only |
| `forward_rules` | Streamed; at most 10 candidates / external calls, stopping with ≥60s reserve | Stable keyset plus at-most-once effect ledger described below |
| `resolve_contacts` | Streamed; at most 100 new/updated messages | Message keyset; identity/participant upserts by normalized product keys |
| `assign_projects` | Streamed; at most 25 messages | Stable keyset; assignment upsert is retry-safe |
| `interpret_project` | Streamed; one project context, maximum 40 source items and 60,000 prompt characters, one LLM call | Deterministic interpretation id from `(run_id, project_id, cutoff)` and unique/upsert on that tuple |
| `reconcile_project` | Streamed; at most 100 candidate interpretations/outcomes | Keyset by interpretation id; deterministic outcome key `(run_id, interpretation_id, candidate_index)`; product constraints + DSQL `40001` retry |
| `project_ai` | Synchronous audit; one LLM call, maximum 8 projects, per-project across limits 16 facts/8 decisions/8 issues/6 timeline items, 60,000 context characters, 20s LLM deadline | Run row written through `JobExecutionPort`; not stream-triggered unless its HTTP contract becomes async |

Registry tests must prove every name used by schedules, HTTP handlers, and `InsertJobRun` call sites is present. Future types cannot default to “process everything”.
Wave-3 names (`sync_teams`, `sync_whatsapp`, `ingest_transcript`, `ingest_doc_revision`) are **reserved, not implemented**. They cannot be added to schedules or migrations until the registry defines their bounded unit, cursor, and effect policy.
The canonical default mailbox chain is `sync → resolve_contacts → categorize → assign_projects → summarize → forward_rules`, omitting disabled steps. Schedule-specific chains may differ but are validated entirely against the registry before the first item is written.

`summarize` uses a bounded map/reduce state machine: candidate selection freezes at most 240 messages; each map invocation upserts one DSQL `summary_job_chunks` row by `(run_id, chunk_index)`. The configured map size is clamped to 12–30, so the 12-message minimum yields at most 20 partials. A final reduce invocation reads those at most 20 partials, writes the single summary/action/FYI result idempotently, then deletes the intermediates in a bounded cleanup. Requests exceeding 240 candidates are rejected or split into a separately specified follow-on run, never silently expanded. Every registry entry has a duplicate-after-DSQL-commit test that reruns the same cursor and asserts identical product state.

#### 4.4.3 Irreversible effects

Exactly-once delivery to Microsoft Graph is impossible. `forward_rules` therefore chooses **at-most-once**:

1. Before `ForwardMessage`, conditionally create `ACCOUNT#<account_id>` / `EFFECT#FORWARD#<message_id>#<rule_id>` with `state=claimed`, `job_id`, `attempt_id`, and timestamps. Populate GSI3 as `EFFECTSTATE#claimed` for recovery.
2. If token refresh/validation fails **before any request bytes can be sent**, conditionally remove/release the claim and return a retriable error.
3. Only the claim winner calls Graph. Classify the result:
   - confirmed 2xx → first persist `succeeded_pending_audit` with the sanitized audit fields needed for repair; idempotently upsert DSQL `forward_audit` on `(message_id, rule_id)` as `sent`; then settle the effect as `succeeded`;
   - definitive non-delivery response such as 429 → effect `retryable` plus job `pending/retry_not_before`; the scheduler re-wakes it after `Retry-After`, and a later attempt conditionally changes the effect back to `claimed`;
   - permanent 4xx rejection → effect `rejected`, idempotent DSQL audit `rejected`, terminal job failure;
   - timeout, connection loss after send, or ambiguous 5xx → effect `unknown`, idempotent DSQL audit `unknown`, no automatic retry.
4. Recovery never calls Graph:
   - stale `succeeded_pending_audit` backfills/upserts DSQL `sent`, then settles `succeeded`;
   - stale `claimed` becomes `unknown` and backfills/upserts DSQL `unknown`;
   - already `unknown` remains operator-visible until manually resolved.
5. Effect records have no short job TTL; retain them for at least the lifetime of the source message/rule. Account deletion queries the account partition by `begins_with(sk, "EFFECT#FORWARD#")` and deletes with `BatchWriteItem` batches of at most 25, independently retrying unprocessed keys.

LLM calls may repeat, but their resulting DSQL writes must be deterministic/upserted. Network/DSQL infrastructure errors are retriable **except when the forwarding classification above says delivery is ambiguous**. Malformed payloads and deterministic validation failures become terminal `failed`.

HTTP handlers and the scheduler **stop calling Asynq**. Introduce a driven port, e.g. `JobEnqueuer`:

| Implementation | When | Behaviour |
| -------------- | ---- | --------- |
| `dynamodbjobs.Store` | AWS and local Floci | Conditional job/lock transactions and indexed reads |
| `memoryjobs.Store` | Unit tests | Same state-machine conditions; no implicit auto-run unless the test asks |
| `sqlitejobs` | Legacy tests only | Not part of the AWS-parity composition root |
| `asynqadapter.QueueClient` | Optional short compatibility | Current Redis path; delete after cutover |

### 4.5 Scheduler Lambda (`svc/cmd/scheduler`)

Each EventBridge invocation, in order:

1. **Housekeeping** — delete expired `oauth_states` in DSQL.
2. **Scheduler tick** — move `schedulerService.Tick` out of `package main`; query at most **100 due rows** ordered by `next_run_at` per invocation. For each row with observed `scheduled_for = next_run_at`:
   1. derive `job_id` deterministically from `schedule_id + scheduled_for` and `chain_id` from the same occurrence;
   2. conditionally create the pending job/active lock in DynamoDB; an already-existing identical job counts as success;
   3. only then update DSQL with `WHERE id = $id AND next_run_at = $scheduled_for` to set `last_run_at` and the next occurrence;
   4. if the Lambda dies between steps 2 and 3, the next tick reuses the same ID and safely finishes step 3. Never advance DSQL before the job exists.
3. **Pending reconciler** — query GSI3 for old `pending` items whose `retry_not_before` is absent or due, then conditionally rotate `wake_token`/`updated_at`. For lock-backed jobs, the same transaction verifies `owner_job_id` and renews the pending lock lease. This creates a new stream record if the original wake-up was lost, aged past DynamoDB Streams’ 24-hour retention, or was deliberately deferred after provider throttling.
4. **Lease watchdog** — query stale `running` items. A conditional transaction requiring the observed `revision`, `attempt_id`, and expired `lease_until` moves the item to `pending`, clears lease ownership, increments `revision`, and assigns a new `wake_token`. It **does not clear the committed cursor**. The stale invocation is fenced from future writes.
5. **Effect reconciliation** — process the GSI3 states from §4.4.3: backfill `succeeded_pending_audit`, turn stale `claimed` into `unknown`, upsert matching DSQL audits, and alert on `unknown`. It never calls Graph.
6. **Exit** — timeout **60s** and stop new work with 10s remaining. Each due/pending/running/effect query processes at most **100 items** and at most one page per invocation; the next minute continues. Reserved concurrency **1**.

API-created jobs do **not** need the scheduler: `PutItem` alone starts the stream.

**Product rule for sync:** API and scheduler acquire `MAILBOX#<account_id>/LOCK#SYNC` for `sync` or `CONNECTOR#<connector_account_id>/LOCK#SYNC` for `sync_slack` in the same transaction that creates the job. Never use a GSI “check then put”. On conflict, skip or return/re-wake the lock’s `owner_job_id`.

### 4.6 Worker Lambda (`svc/cmd/worker`) — stream handler

Wire like Heimdall, with production failure controls: `aws_lambda_event_source_mapping` on `automata-jobs` stream, `batch_size = 1`, `starting_position = TRIM_HORIZON`, `stream_view_type = NEW_AND_OLD_IMAGES`. Filter in-handler (only job `sk=METADATA`, only `pending`/`running`); lock/effect records do not invoke product work. `TRIM_HORIZON` reduces lost wake-ups when a mapping is recreated; the pending reconciler covers records older than stream retention.

Each record:

1. **Ignore** terminal statuses and non-job keys.
2. **Kick** if `pending`: if `retry_not_before` is still in the future, return without writing and let the scheduler re-wake it. Otherwise conditionally set `running`, fresh `attempt_id`, lease owner/expiry, incremented revision, and `started_at` only if absent; return.
3. **Execute one registered unit** through `JobExecutionPort`, reading/writing **DSQL** product data. The service receives run/attempt/deadline context and returns `ChunkResult{NextCursor, ProgressDelta, Done}`. Leave at least 30 seconds for state persistence.
4. **Write next state conditionally** with `status=running AND attempt_id=:mine AND revision=:observed`:
   - More chunks → update monotonic cursor/progress/lease and increment revision; stream continues.
   - Step done → one `TransactWriteItems` marks current `success`, creates deterministic next `pending`, and transfers/releases lock.
   - Chain done → terminal `success` + `finished_at`, clear lease, release lock.
   - Cancellation requested → terminal `cancelled` at the next chunk boundary.
   - Deterministic application error → terminal `failed` with sanitized error.
   - Transient Graph throttling, DSQL `40001`, DSQL/network timeout, or AWS dependency error → return an error so the event source mapping retries. Respect `Retry-After`. For `forward_rules`, the stricter §4.4.3 classification overrides this rule: only proven pre-send/definitive non-delivery outcomes may retry.
5. **Idempotency:** commit DSQL first and advance DynamoDB second. If the second write fails, the same chunk may run again; every registry entry must prove its DSQL writes/effects tolerate that. A failed conditional state write means the invocation is stale and must stop without retrying the product effect.

**Concurrency**

- Scale with worker **reserved concurrency** (start **2–5**). Different jobs run in parallel; one job’s pages stay serialized by the self-write loop.
- Conditional revision/attempt fences, not stream ordering alone, serialize each job.
- Lambda timeout **15 minutes** (Heimdall stream handler uses that ceiling). Chunking is still mandatory for large mailboxes so one unit finishes comfortably under that cap.

**Event-source failure policy**

- `maximum_retry_attempts = 3`.
- `maximum_record_age_in_seconds = 3600`.
- `function_response_types = ["ReportBatchItemFailures"]`; `batch_size = 1` keeps isolation simple.
- `destination_config.on_failure` points to an encrypted, private S3 failure bucket with a lifecycle rule (90 days dev, 365 days prod). This S3 bucket is a replay/forensics destination, **not** a second job queue.
- Enable ESM `metrics_config = EventCount`. Grant the required S3 destination permissions and alarm on `OnFailureDestinationDeliveredEventCount > 0`, `DroppedEventCount > 0`, and `DestinationDeliveryFailures > 0`; do not alarm on daily S3 object-count storage metrics.
- Alarm on Lambda `Errors`, `Throttles`, and `Duration`, plus event-source `IteratorAge`; alert before iterator age approaches the 24-hour stream retention.
- Replay is an explicit operator tool that validates current job revision/status before rotating `wake_token`; never re-submit captured product effects blindly.

**Stuck `running`**

Scheduler watchdog uses the fenced transition in §4.5. A worker renews its lease only when committing a chunk; chunks must therefore finish comfortably before lease expiry. Lease expiry permits a replacement attempt but never permits the old attempt to commit.

### 4.7 API Lambda must not run jobs inline

Today, if `h.JobQueue == nil`, sync (and similar) runs **inside the HTTP request**. API Gateway timeout stays **~30s**. On AWS always `PutItem` pending and return **202** with `job_id`. Keep inline only behind `JOBS_INLINE=true` for local debugging.

Remove scheduler/OAuth GC goroutines from the API composition root on Lambda — they belong in **`cmd/scheduler`**.

`GET /api/runs` keeps its JSON-array response shape and supports exactly these query combinations:

- no filter → GSI1 (user);
- `job_type` only → GSI5 (user + type);
- `account_id` only → authorize that account in DSQL, then GSI2;
- `account_id + job_type` → authorize in DSQL, then GSI4;
- `limit` defaults to 50 and is clamped to 1–100;
- `cursor` is an authenticated opaque encoding of `LastEvaluatedKey + user_id + normalized filters`; return the next value in `X-Next-Cursor`;
- legacy `offset=0` is accepted during cutover; positive `offset` returns `400 cursor_required`, and the web client/tests move to `cursor`.

Unsupported combinations do not fall back to `Scan`. Every returned account item is checked against the authenticated user. `GET /api/runs/{id}` uses `GetItem`, returns 404 on missing/foreign ownership, and exposes the same shape:

```json
{
  "id": "<uuid>",
  "account_id": "<uuid or omitted>",
  "account_label": "<hydrated from DSQL or omitted>",
  "job_type": "sync",
  "trigger": "api|schedule",
  "status": "pending|running|success|failed|cancelled",
  "time_window_start": "<RFC3339 or omitted>",
  "time_window_end": "<RFC3339 or omitted>",
  "started_at": null,
  "finished_at": null,
  "error_message": "<sanitized or omitted>",
  "meta_json": {}
}
```

`started_at`/`finished_at` are RFC3339 strings when set and JSON `null` otherwise. Time windows are first-class job payload fields. `meta_json` is a compatibility projection of sanitized progress/result detail, never raw internal payload, cursor, tokens, lease, or effect data. `account_label` is batch-hydrated from authorized DSQL accounts after the DynamoDB query. Update the web `JobRun` type so `started_at`/`finished_at` are nullable.
Chi CORS must emit `Access-Control-Expose-Headers: X-Next-Cursor` so a browser SPA on CloudFront can read the pagination token.

`cancel` is a conditional DynamoDB transition. Pending jobs become terminal immediately; running jobs set `cancel_requested_at` and stop at the next chunk boundary. A terminal item gets `expires_at`: default **30 days in dev, 365 days in prod**, configurable. TTL applies only to terminal job history; lock/effect retention follows §4.4.1 and §4.4.3. DSQL provenance keeps `run_id` after expiry but has no FK. If permanent audit is later required, add a deliberate DSQL audit projection rather than reviving lifecycle dual writes.

### 4.8 Local behaviour after the cutover (Floci parity)

**Primary local path = same shape as AWS**, via [Floci](https://floci.io/) (same emulator yonda-local uses for Heimdall/Knox). Do not invent a second job system for laptops.

| Concern | AWS | Local (Floci) |
| ------- | --- | ------------- |
| HTTP API | Lambda + API Gateway | Same Lambdas/API Gateway on Floci (`http://localhost:4566`) |
| Jobs table + stream | DynamoDB + Streams | DynamoDB + Streams on Floci |
| Worker / scheduler | Stream + EventBridge Lambdas | Same functions deployed to Floci |
| Product DB | Aurora DSQL | **Postgres 16** in Compose (Postgres dialect adapter; DSQL is not emulated) |
| Secrets | Secrets Manager | Secrets Manager on Floci (or env injected by Terraform) |
| Redis / Asynq | Absent | Absent |

**Compose stack** (replace today’s Redis-only `docker-compose.yml`):

```yaml
services:
  floci:
    image: floci/floci:1.6.0   # pin; align with yonda-local when convenient
    ports: ["4566:4566"]
    volumes:
      - floci-data:/app/data
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      FLOCI_HOSTNAME: floci
      FLOCI_DEFAULT_REGION: us-east-1
      FLOCI_DEFAULT_ACCOUNT_ID: "000000000000"
      FLOCI_STORAGE_MODE: hybrid
      FLOCI_STORAGE_PERSISTENT_PATH: /app/data
      FLOCI_SERVICES_DOCKER_NETWORK: automata-local
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
      AWS_SESSION_TOKEN: test
    networks: [automata-local]
  postgres:
    image: postgres:16-alpine
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: automata
      POSTGRES_PASSWORD: automata
      POSTGRES_DB: automata
    networks: [automata-local]

networks:
  automata-local:
    name: automata-local

volumes:
  floci-data:
```

**Terraform `svc/terraform/envs/local`** (copy Heimdall’s local provider pattern):

- `backend "local" {}`
- Local region is **`us-east-1`** to match yonda-local/Floci defaults; AWS dev/prod remain `eu-west-2`.
- AWS provider: `access_key`/`secret_key` = `test`, service endpoints = `http://localhost:4566` for exactly the module resources (Lambda, IAM, CloudWatch, DynamoDB, API Gateway, EventBridge, Secrets Manager, S3, STS as needed).
- Module inputs inside Lambda containers use `aws_endpoint = "http://floci:4566"` and `database_url = "postgres://automata:automata@postgres:5432/automata?sslmode=disable"`.
- Floci and its spawned Lambda containers must join the named `automata-local` network (`FLOCI_SERVICES_DOCKER_NETWORK=automata-local`). `FLOCI_HOSTNAME=floci` gives the runtime API a resolvable host. `host.docker.internal` is only an explicit fallback via `FLOCI_SERVICES_LAMBDA_EXTRA_HOSTS=host.docker.internal:host-gateway`, not the default Linux path.
- **Omit DSQL resources** when `environment == "local"`; use Postgres connection string instead.
- Parameterize `lambda_architecture`. Local scripts default it from `uname -m` (`x86_64 → amd64`, `aarch64/arm64 → arm64`) with `LAMBDA_ARCH` override; real AWS uses the environment’s declared architecture (initially `amd64`). The archive architecture and `aws_lambda_function.architectures` must always match.

**Scripts** (Heimdall-style, at repo root or under `svc/`):

| Script | Purpose |
| ------ | ------- |
| `scripts/local_configure.sh` | First-time: copy `svc/.env.example` → `svc/local.env`, `web/.env.example` → `web/.env.local`, print next steps |
| `scripts/local_up.sh` | `docker compose up -d` (Floci + Postgres) |
| `scripts/local_deploy.sh` | `terraform apply` in `svc/terraform/envs/local`, invoke migrate Lambda, print API Gateway URL |
| `scripts/local_logs.sh` | `docker logs -f` on the Floci container (stream/worker logs live here) |

**Developer loop**

1. `./scripts/local_up.sh`
2. `./scripts/local_deploy.sh` after infra or Lambda code changes
3. Point `web` at the Floci API Gateway URL from Terraform output
4. Exercise sync → DynamoDB job → stream worker on Floci (same path as AWS)

**Allowed shortcuts (not the default README path)**

- `JOBS_INLINE=true` + `go run ./cmd/server` for debugging a single use case without Streams.
- SQLite **unit tests only**.
- In-process `HandleJob` after repository writes in tests that must not start Floci.

End state: **no Redis**. Floci is required for the documented local AWS-parity workflow.

### 4.9 Files to rework (application)

| Area | Change |
| ---- | ------ |
| `svc/internal/adapters/inbound/asynq/jobs.go` | Extract/retire payload aliases; dispatch through the typed registry rather than whole-job handlers |
| Application services that use `JobRunRepository` | Remove lifecycle writes; accept run context and return bounded `ChunkResult` or use synchronous `JobExecutionPort` orchestration |
| New DynamoDB `JobStore` + stream adapter | Conditional state machine, indexed reads, lock/effect items, atomic chain handoff, `lambda.Start` |
| New job registry | All eleven declared job names, payload schema version, mode, chunk/cursor codec, retry/effect policy |
| `svc/internal/adapters/inbound/http/router.go` (and connectors) | Enqueue via port; runs API uses user/account GSIs and opaque pagination |
| `svc/cmd/server/scheduler.go` | Move out of `package main`; share with **scheduler** Lambda |
| `svc/cmd/server/main.go` | No scheduler/GC goroutines in Lambda build; no Redis |
| New `svc/cmd/scheduler` | `lambda.Start` for EventBridge; deterministic schedule IDs, pending reconciler, fenced watchdog |
| `svc/cmd/worker/main.go` | `lambda.Start` for DynamoDB stream events; no long-running local consumer in the parity path |
| Provenance / migrations | `run_id` UUID without FK to DSQL `job_runs` (drop or stop using that table on AWS) |
| `svc/internal/configuration/config.go` | Drop Redis; add DSQL connector config, jobs/failure bucket names, retention, lease/chunk tuning, `JOBS_INLINE` |
| New `svc/cmd/api`, `svc/cmd/migrate` | API proxy + migrate-and-exit |

## 5. Persistence: SQLite → PostgreSQL

### 5.1 Why

Lambda has no durable local disk. SQLite in `/tmp` is lost on freeze/recycle and cannot be shared by API + worker + migrations. The schema is relational with FKs, unique keys, and JSON columns. **Hosted store: Aurora DSQL** ([addendum-aurora-dsql.md](addendum-aurora-dsql.md)) — PostgreSQL-compatible, billed per DPU, IAM from Lambda without VPC. DynamoDB is a poor fit for the product graph ([addendum-dynamodb-access-patterns.md](addendum-dynamodb-access-patterns.md)). Aurora PostgreSQL Serverless v2 remains a fallback if DSQL’s 3,000-row / 5-minute transaction caps or missing partial indexes block implementation.

### 5.2 Approach

1. Add a **Postgres-dialect adapter** (e.g. `…/persistence/postgres`) implementing the same `driven.*` ports. Hosted target is **DSQL**; local/CI can use vanilla Postgres. DSQL-specific DDL (generated-column uniques, `CREATE INDEX ASYNC`, chunked deletes) is specified in [addendum-aurora-dsql.md](addendum-aurora-dsql.md).
2. Keep SQLite **for unit tests**. Do not compile SQLite-only `PRAGMA` / `INSERT OR IGNORE` / `COLLATE NOCASE` into the AWS path.
3. Rewrite migrations as a **current-schema baseline plus forward migrations**, split by engine:
   - `migrations/common/`: tables, columns, constraints, and data changes accepted by both Postgres and DSQL;
   - `migrations/postgres/`: normal `CREATE INDEX` and local-only helpers;
   - `migrations/dsql/`: `CREATE INDEX ASYNC` followed by `sys.wait_for_job` and any DSQL generated-column workarounds.
   Vanilla Postgres must never receive DSQL-only SQL.
4. Apply migrations **only** from the migrations Lambda on deploy, never on API cold start. The runner records version/checksum and refuses divergent history.
5. Connection handling: small pool (`MaxOpenConns` 1–3) **outside** the Lambda handler; max lifetime **&lt; 60 minutes** (DSQL connection cap). IAM token via the AWS DSQL Go connector. Local Postgres uses `DATABASE_URL`; hosted DSQL uses cluster endpoint/identifier + region + database role, not a stored long-lived password URL. **No RDS Proxy.**
6. CI runs common + Postgres migrations and repository tests on Postgres. Promotion requires a dev-DSQL suite that runs the DSQL migration set plus representative joins, `ON CONFLICT`, generated uniques, `40001` retry, chunked deletes, and each high-risk repository write.

### 5.3 SQL dialect pitfalls

SQLite uses `?` placeholders, `TEXT` UUIDs, `INTEGER` booleans, `INSERT OR IGNORE`, and datetime strings. The hosted adapter uses `$1`, `UUID`, `BOOLEAN`, `TIMESTAMPTZ`, `JSONB`. Partial unique indexes must be emulated (generated columns / `NULLS DISTINCT`). Bulk mutates chunk at **&lt; 3,000 rows**. Job claim/queueing is **not** done in DSQL — that lives in DynamoDB (§4).

### 5.4 Network

Keep API, scheduler, worker, and migrate Lambdas **out of a VPC** so they can reach DSQL (IAM), DynamoDB, Secrets Manager, Graph, Entra, Google, Slack, and the LLM without NAT. This matches Heimdall’s Lambda networking (public AWS APIs). A VPC is **not** required for the first cut.

---

## 6. Terraform layout (copy Heimdall’s tree)

Create the same split so GitHub path filters work:

```
svc/terraform/
  modules/automata/
    main.tf              # tags
    variables.tf
    outputs.tf
    dsql.tf              # cluster, runtime/admin IAM split (skipped when environment=local)
    dynamodb.tf          # jobs, GSIs, TTL, PITR/deletion protection, stream
    storage.tf           # encrypted stream-failure S3 bucket + lifecycle
    backup.tf            # DSQL AWS Backup vault/plan/selection in prod
    observability.tf     # log groups, metric alarms, dashboard
    secrets.tf           # or a sibling module automata_aws like heimdall_aws
    lambda.tf            # build Go binaries, IAM, functions, EventBridge, stream mapping
    api_gateway.tf       # REST + {proxy+} AWS_PROXY — copy Heimdall resource-for-resource
  envs/local/            # Floci endpoints + local backend — required for AWS-parity local
    main.tf
    locals.tf
  envs/dev/
    main.tf              # backend "s3" { key = "dev/automata.tfstate" }
    locals.tf
  envs/prod/
    main.tf
    locals.tf
  scripts/bootstrap/
    bootstrap.tf         # aws_dynamodb_table automata-lock-table

web/terraform/
  modules/automata_web/
    main.tf
    variables.tf
    s3.tf                # private bucket + OAC policy
    cloudfront.tf        # SPA error routing
    acm.tf               # us-east-1 cert for automata.{zone}
    route_53.tf
    build.tf             # npm run build + sync web/dist + invalidation
  envs/dev/
  envs/prod/
  scripts/bootstrap/
    bootstrap.tf         # automata_web-lock-table
```

**`envs/local` is required**, not optional. It is how developers exercise the same Lambda / Streams / EventBridge wiring as AWS before touching a real account. DSQL is not available in Floci — the local env wires `DATABASE_URL` to Compose Postgres and skips `dsql.tf` resources (`count = var.environment == "local" ? 0 : 1`).

### 6.1 Lambda packaging (Go)

Heimdall’s `null_resource.build_app` installs a Python site-packages tree. Replace with a Go cross-compile:

```bash
cd ${path.module}/../../../
rm -rf dist && mkdir -p dist
for entrypoint in api scheduler worker migrate; do
  mkdir -p "dist/${entrypoint}"
  GOOS=linux GOARCH=${local.goarch} CGO_ENABLED=0 \
    go build -trimpath -o "dist/${entrypoint}/bootstrap" "./cmd/${entrypoint}"
done
```

Create **four archives**, each from one `dist/<entrypoint>/` directory. Every zip must contain an executable named **`bootstrap` at its root**; `provided.al2023` does not select `bootstrap-api` from the handler setting. Each function uses `runtime = "provided.al2023"`, `handler = "bootstrap"`, its own `source_code_hash`, and matching `architectures` (`x86_64` for Go `amd64`, `arm64` for Go `arm64`).

Trigger rebuild with `triggers = { always_run = timestamp() }` like Heimdall so GitHub apply always publishes the commit being deployed.

### 6.2 Lambda configuration

| Function | Timeout | Memory (starting point) | Trigger |
| -------- | ------- | ----------------------- | ------- |
| `automata-api-handler` | 30s | 1024–1792 MB | API Gateway |
| `automata-scheduler-handler` | 60s | 512–1024 MB | EventBridge `rate(1 minute)` |
| `automata-worker-handler` | 15 × 60 s | 1792 MB | DynamoDB Streams on `automata-jobs` (batch size 1) |
| `automata-migrations-runner` | 15 × 60 s | 1024 MB | `aws_lambda_invocation` on apply |

`aws_lambda_invocation` does not run on every apply by itself. Set `publish = true` on the migration function and make the invocation replace when either code or SQL changes:

```hcl
qualifier = aws_lambda_function.migrate.version
triggers = {
  migration_checksum = local.migrations_checksum # stable hash of common + selected engine files
  function_version   = aws_lambda_function.migrate.version
}
depends_on = [aws_lambda_function.migrate]
```

Compute `migrations_checksum` from sorted file names + file hashes, not `timestamp()`. The invocation payload names the expected engine and schema version; an unchanged apply is a no-op, while changed SQL/function code invokes the published version after deployment.

IAM is per function, not one shared broad policy:

- **API:** logs, read/write job items and lock transactions, read required secrets, `dsql:DbConnect`.
- **Scheduler:** logs, query/update jobs and locks/effects, read required secrets, `dsql:DbConnect`.
- **Worker:** logs, job/lock/effect transactions, stream read permissions, read required secrets, `dsql:DbConnect`, and write access to the stream-failure destination where required by the event-source configuration.
- **Migrate:** logs, migration-only secrets/config and **`dsql:DbConnectAdmin`**. Do not grant `DbConnectAdmin` to API/scheduler/worker. Migrations create/grant the least-privilege DSQL runtime role used by those functions.
- **No** VPC ENI, RDS Proxy, or SQS job-queue permissions. Terraform lock tables remain separate DynamoDB resources.

DynamoDB: table `automata-jobs-${env}` with `NEW_AND_OLD_IMAGES`, PAY_PER_REQUEST, TTL on `expires_at`, and the five GSIs in §4.4.1. Enable PITR and deletion protection in hosted environments (mandatory prod). Add the `on_demand_throughput` local conditional used by Heimdall if Floci rejects omitted GSI throughput metadata. Active uniqueness uses lock items, **not** a GSI.

Event source mapping:

```hcl
batch_size                         = 1
starting_position                  = "TRIM_HORIZON"
maximum_retry_attempts             = 3
maximum_record_age_in_seconds      = 3600
function_response_types            = ["ReportBatchItemFailures"]
metrics_config {
  metrics = ["EventCount"]
}
# destination_config.on_failure -> encrypted stream-failure S3 bucket
```

The failure bucket blocks public access, enables versioning and encryption, and expires objects after 90 days in dev / 365 days in prod. The web bucket separately enables versioning so a bad SPA sync can be rolled back.

EventBridge + permission: copy Heimdall’s snapshot schedule resources, target the **scheduler**, `schedule_expression = "rate(1 minute)"`. Scheduler reserved concurrency **1**. Worker reserved concurrency starts at **2–5**.

API Gateway: copy `svc/terraform/modules/heimdall/api_gateway.tf` (proxy resource, root ANY, stage = `var.environment`). CORS stays in chi (`CORS_ORIGINS` from env / Secrets) and explicitly exposes `X-Next-Cursor`.

### 6.3 Frontend module

Copy `web/terraform/modules/heimdall_web` with namespaced resources (`automata-web-oac`, bucket `automata-web-${env}-${account}-${region}`).

`build.tf` environment:

- `VITE_API_BASE_URL` = API Gateway invoke URL (or custom API domain if added later).
- No Cognito `VITE_COGNITO_*` unless we later add Cognito; Automata web only needs the API base URL today (`web/.env.example`).

`npm ci` / `npm run build` must run with `working_dir` = `web/` (Heimdall’s frontend workflow currently installs from **repo root** because Heimdall’s `package.json` lives there; Automata’s lives in `web/`). Automata’s Vite output is **`web/dist/`**; the S3 sync source must use that path rather than Heimdall’s repo-root `dist/`.

DNS: `automata.${hosted_zone_domain_name}` (or another hostname decided in §12). ACM in us-east-1, alias A record to CloudFront.

After the first backend apply, paste the API Gateway URL into `web/terraform/envs/{env}/locals.tf` `endpoint`, same manual step Heimdall uses today (`endpoint = "https://….execute-api.eu-west-2.amazonaws.com/dev"`).

### 6.4 VPC

**Not required** for DSQL. Omit NAT. Revisit only if a future dependency must live in private subnets.

### 6.5 State backend and bootstrap

Per AWS account, once:

```hcl
# svc/terraform/scripts/bootstrap
resource "aws_dynamodb_table" "lock_table" {
  name         = "automata-lock-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"
  attribute { name = "LockID"; type = "S" }
}
```

And `automata_web-lock-table` for the frontend.

`backend "s3"` keys:

- `dev/automata.tfstate` / `prod/automata.tfstate`
- `dev/automata_web.tfstate` / `prod/automata_web.tfstate`

Bucket names and account IDs are **not** copied from Heimdall (`terraform-states-c8cfa30f` / Yonda accounts). They belong to whichever AWS accounts host Automata (§12).

Terraform version: **1.10.3** to match Heimdall workflows. Deliberately use AWS provider **`>= 6.15, < 7.0`**: DSQL first appeared in 5.100.0, while 6.15 fixes `aws_dsql_cluster` deletion-protection/force-destroy behaviour needed here. Commit `.terraform.lock.hcl`. Declare and lock `hashicorp/archive` and `hashicorp/random` as well; do not inherit Heimdall’s older provider constraint blindly.

### 6.6 Secrets

Follow `svc/terraform/modules/heimdall_aws/secrets.tf`: create Secrets Manager entries with `REPLACE-ME` or `random_id` where safe.

| Secret | Purpose |
| ------ | ------- |
| `automata/ENCRYPTION_KEY` | 32-byte AES-GCM mailbox/connector token vault (must stay 32 bytes) |
| `automata/JWT_SECRET` | HS256 app tokens |
| `automata/MS_CLIENT_SECRET` | Entra |
| `automata/GOOGLE_CLIENT_SECRET` | optional |
| `automata/SLACK_CLIENT_SECRET` | optional |
| `automata/LLM_API_KEY` | optional |

Hosted DSQL does **not** use an `automata/DATABASE_URL` password secret. Cluster endpoint/identifier, region, database name, and runtime role are non-secret Lambda environment variables; the connector generates short-lived IAM auth. `DATABASE_URL` exists only for local/CI Postgres. Non-secret URLs, client IDs, CORS origins, `DASHBOARD_BASE_URL`, table/bucket names, and retention values come from `locals.tf`.

**Hosted must set** `AUTH_DEFAULT_USER_ID` unused / middleware must **reject** unauthenticated requests. The current default-user fallback is a local-dev footgun.

OAuth redirect URIs in Entra/Google/Slack must include the API Gateway (or custom) callback URLs for each environment.

### 6.7 Local Floci stack (`envs/local`)

Mirror Heimdall’s `svc/terraform/envs/local`:

```hcl
provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  s3_use_path_style           = true
  endpoints {
    lambda         = "http://localhost:4566"
    iam            = "http://localhost:4566"
    cloudwatch     = "http://localhost:4566"
    logs           = "http://localhost:4566"
    dynamodb       = "http://localhost:4566"
    apigateway     = "http://localhost:4566"
    events         = "http://localhost:4566"
    secretsmanager = "http://localhost:4566"
    s3             = "http://localhost:4566"
    sts            = "http://localhost:4566"
  }
}
```

- `backend "local" {}` (no S3 state for laptop applies).
- Local provider/ARN region is `us-east-1`; this intentionally differs from AWS dev/prod `eu-west-2`.
- Pass `aws_endpoint = "http://floci:4566"` into Lambdas; Floci injects the same reachable endpoint when `FLOCI_HOSTNAME=floci`.
- Pass `database_url = "postgres://automata:automata@postgres:5432/automata?sslmode=disable"` over the named Compose network.
- Set `FLOCI_SERVICES_DOCKER_NETWORK=automata-local`; both Floci and Postgres join it. Add `FLOCI_SERVICES_LAMBDA_DOCKER_HOST_OVERRIDE=floci` only if runtime API auto-detection fails.
- Output `api_gateway_url` in the Floci REST shape Heimdall uses (`http://localhost:4566/restapis/.../_user_request_/`).
- `scripts/local_deploy.sh` runs `terraform -chdir=svc/terraform/envs/local apply` then invokes the migrate Lambda.
- Verify the pinned Floci version supports every configured service in a smoke test. Secrets Manager is supported; if a Terraform resource is not emulated, guard that resource with `environment != "local"` and inject only a non-production local value.

Root `docker-compose.yml` must **drop Redis** and provide Floci + Postgres as in §4.8.

### 6.8 Backup, retention, and observability

These resources are part of the deployable module, not a post-launch note:

- **DSQL:** set `deletion_protection_enabled = true` in prod (`false` only for disposable local/dev as explicitly configured). Enable the Aurora DSQL service in `aws_backup_region_settings`; create an encrypted backup vault, daily full-backup plan, **35-day** retention, IAM backup role, and `aws_backup_selection` for the cluster ARN. DSQL backups are full backups, not PITR. Decide cross-Region copy in §12 before prod.
- **DynamoDB:** PITR and deletion protection enabled in prod; TTL only terminal jobs as defined in §4.7. Restore testing must include jobs-table ownership/index validation.
- **S3:** version web and failure buckets; block public access; use CloudFront OAC for web; lifecycle failure records per §6.2.
- **Logs:** explicitly create Lambda log groups before functions (avoid unmanaged defaults), encrypted where required, retained 14 days local/dev and 90 days prod.
- **Alarms/dashboard:** API Gateway 5xx/latency; each Lambda errors/throttles/duration; worker IteratorAge; ESM `OnFailureDestinationDeliveredEventCount`, `DroppedEventCount`, and `DestinationDeliveryFailures`; DynamoDB throttled/user errors; scheduler error or missing invocation; DSQL health/usage metrics available in the service. Route alarms to a configurable SNS/operations target.
- **Restore gate:** before production launch, restore a DSQL backup to a new cluster and a DynamoDB PITR copy in a non-prod account, run migrations/read smoke tests, record RTO/RPO, then destroy restored resources.

---

## 7. GitHub Action workflows (copy Heimdall)

Add:

```
.github/workflows/backend-ci.yml
.github/workflows/frontend-ci.yml
```

Mirror Heimdall’s job graph, with Automata-specific install/test steps.

### 7.1 Backend CI

**Triggers:** `push`/`pull_request` to `main`, paths `svc/**`.

**Permissions:** `id-token: write`, `contents: read`, `pull-requests: write`.

| Job | Notes |
| --- | ----- |
| `Test` | `actions/setup-go` from `svc/go.mod`; `go test ./...` in `svc`. No `uv`. |
| `plan` | Matrix `dev` / `prod`: setup Go; working-directory `svc/terraform/envs/${{ matrix.env.name }}`; OIDC role; Terraform 1.10.3; locked provider init, `fmt -check`, `validate`, `plan`; sticky comment |
| `apply-dev` | `needs: plan`; setup the same Go version/cache; `if: pull_request \|\| (push && main)`; GitHub Environment `dev`; `terraform apply -auto-approve` |
| `apply-prod` | `needs: plan`; setup the same Go version/cache; `if: github.ref == 'refs/heads/main'`; Environment `prod` |

Every Terraform job installs Go because build provisioners run during **apply** and plan/archive evaluation must remain deterministic. Runners are `ubuntu-latest` (amd64), matching the initial AWS `x86_64` architecture. Cache modules via `actions/setup-go`; run a packaging smoke that unzips each archive and verifies an executable root `bootstrap`. Fail if `.terraform.lock.hcl` changes unexpectedly.

OIDC: create `terraform-github-actions-role` (or Automata-specific name) in each AWS account with trust to `repo:OWNER/automata:…`. Do not reuse Heimdall’s account IDs unless Automata is intentionally hosted in those Yonda accounts.

### 7.2 Frontend CI

**Triggers:** paths `web/**`.

Same plan/apply-dev/apply-prod shape. Differences from Heimdall:

- `working-directory` for Terraform: `web/terraform/envs/${{ matrix.env.name }}`
- `aws-region` for the **role assumption** should be **us-east-1** (CloudFront/ACM), matching Heimdall frontend-ci
- Install: `working-directory: web` + `npm ci` (not repo root)
- Install Node in both plan and apply jobs because Terraform runs the Vite build; sync **`web/dist/`**
- Optional extra job: `npm test` / `npm run lint` before plan

`locals.endpoint` must be updated after the first API deploy or the baked SPA will point at the wrong API.

### 7.3 GitHub Environments

Create Environments `dev` and `prod` on the Automata repo. Put **prod** behind required reviewers if this should match Heimdall’s human gate. Secrets for OIDC are on the IAM trust, not in GitHub, except any non-AWS keys you choose not to put in Secrets Manager.

### 7.4 What not to copy

Heimdall’s `linear-check.yml` is Yonda-process specific (`Yonda-Tax/ci-templates`, team keys `YONDA,YE,Y`). Skip unless Automata adopts the same Linear gate.

---

## 8. Configuration mapping (env → AWS)

| Local env today | Hosted |
| --------------- | ------ |
| `LISTEN_ADDR` | unused (API Gateway) |
| `APP_PUBLIC_URL` | API Gateway invoke URL (no trailing slash) |
| `DATABASE_URL` | local/CI Postgres only; absent in hosted DSQL |
| `DSQL_CLUSTER_ENDPOINT`, `DSQL_REGION`, `DSQL_DATABASE_ROLE` | non-secret hosted connector configuration |
| `ENCRYPTION_KEY` / `JWT_SECRET` | Secrets Manager |
| `MS_*` / `GOOGLE_*` / `SLACK_*` | env + secrets; redirect URIs must be the hosted API |
| `LLM_*` | env + secret; hosted LLM must be reachable from Lambda (public HTTPS) |
| `REDIS_ADDR` / `ASYNQ_*` / `JOB_QUEUE_*` / `GLOBAL_MAX_CONCURRENT_JOBS` | **removed** on AWS/Floci |
| `JOBS_TABLE_NAME`, `JOBS_FAILURE_BUCKET`, `JOB_RETENTION_DAYS`, `JOB_LEASE_SECONDS` | DynamoDB job runtime |
| `LAMBDA_ARCH` | local build override; Terraform maps Go `amd64|arm64` to Lambda `x86_64|arm64` |
| `DASHBOARD_BASE_URL` | `https://automata.{zone}` |
| `CORS_ORIGINS` | that same origin (plus localhost if still desired on dev) |
| `AUTH_DEFAULT_USER_ID` | **disabled** |

Frontend: `VITE_API_BASE_URL` only, injected at Terraform build.

---

## 9. Implementation phases

### Phase 0 — Contracts, document reconciliation, and AWS prerequisites

1. Freeze the public run contract (`pending|running|success|failed|cancelled`; `finished_at`) and the DynamoDB schema/access patterns in §4.4.
2. Implement a compile-time/tested registry for all eleven declared run types; classify `project_ai` as synchronous audit and the remaining long work as streamed.
3. Mark [addendum-redis-asynq-jobs.md](addendum-redis-asynq-jobs.md) legacy for AWS/Floci; update DynamoDB and project-wave addenda so they defer job dispatch/lifecycle/provenance to this document. Remove stale `SKIP LOCKED`, Asynq, FK-to-`job_runs`, and SQLite CHECK-migration instructions for the hosted path.
4. Decide AWS account(s), `eu-west-2` backend region, `us-east-1` web provider, hosted zone, hostname, prod backup copy region, retention, and alarm destination.
5. Create S3 state buckets + bootstrap lock tables, GitHub OIDC provider/role, and future OAuth redirect registrations.

**Exit criterion:** no companion spec contradicts this plan, every current job/run producer is classified, and account/region/backup decisions are recorded.

### Phase 1 — PostgreSQL adapter and migration split

1. Add the Postgres-dialect repositories and the `common/postgres/dsql` migration split from §5.2.
2. Add Postgres 16 to Compose (Redis may coexist only until Phase 3 removes the legacy path), then run common + Postgres migrations and repository tests against it.
3. Implement the `cmd/migrate` runner with version/checksum history and backend selection.
4. Implement DSQL-safe chunked account deletion, category reassignment, merge, and batch inserts (&lt; 3,000 mutations per transaction).
5. Remove relational `job_runs` FK requirements from the Postgres/DSQL baseline; preserve UUID `run_id` provenance.

**Exit criterion:** product tests pass on Postgres, DSQL-only SQL is never sent to Postgres, and no hosted repository depends on SQLite syntax.

### Phase 2 — Fenced DynamoDB job engine and bounded use cases

1. Add `JobStore`, `JobExecutionPort`, the item/index schema, conditional transitions, lock/effect records, deterministic IDs, and atomic chain transaction.
2. Refactor all application services that write `JobRunRepository`; asynchronous services return `ChunkResult`, synchronous `project_ai` is wrapped at its API orchestration boundary.
3. Implement each registry boundary/cursor in §4.4.2. Start with `sync`, then `sync_slack`, LLM batches, forwarding, assignment, interpretation, and reconciliation.
4. Extract scheduler logic with deterministic scheduled occurrence IDs, DSQL compare-and-set advancement, pending re-wake, fenced lease watchdog, and effect reconciliation.
5. Change HTTP handlers to enqueue only and runs APIs to use keyed DynamoDB reads with opaque pagination.
6. Unit-test duplicate delivery after DSQL commit, stale-attempt fencing, lock races, schedule crash windows, atomic chain handoff, cancellation, TTL, and forwarding’s `unknown` outcome.

**Exit criterion:** all asynchronous types are bounded and pass the same state-machine suite against `memoryjobs`; no application service directly owns async run lifecycle.

### Phase 3 — Lambda entrypoints, Floci Terraform, and local runbook

1. Build shared composition roots plus `cmd/api`, `cmd/scheduler`, `cmd/worker`, and `cmd/migrate`.
2. Package four separate `provided.al2023` archives with root `bootstrap`; verify architecture mapping and executable bit.
3. Replace Redis Compose with the named-network Floci + Postgres stack. Implement `envs/local`, jobs/stream/failure bucket, four Lambdas, API Gateway, EventBridge, and local secrets; skip DSQL/backup resources.
4. Configure stream bounded retries, S3 failure destination, `TRIM_HORIZON`, and recovery metrics supported by Floci.
5. Finish `scripts/local_configure.sh`, `local_up.sh`, `local_deploy.sh`, `local_logs.sh`; update both READMEs and remove Redis as the default.
6. Smoke: health through API Gateway; sync reaches `success`; schedule chain runs; injected duplicate/stale records are no-ops; forced stream failure is captured; SPA uses the Terraform URL.

**Exit criterion:** a new developer follows README alone and runs the full API Gateway → DynamoDB Stream → worker → Postgres path without Redis, host-only DNS, or real AWS.

### Phase 4 — Real DSQL validation and backend dev deployment

1. Run `migrations/common` + `migrations/dsql` on a disposable dev DSQL cluster using the admin migration role.
2. Run the required DSQL repository/transaction suite from §5.2; do not promote based on Postgres compatibility alone.
3. Build backend Terraform: DSQL, jobs table/GSIs/PITR, failure S3, least-privilege IAM, four Lambdas, API Gateway, EventBridge, logs/alarms; no VPC/SQS queue.
4. Add backend CI with locked AWS `>=6.15,<7`, setup-go in apply jobs, archive smoke, plans, dev apply, and gated prod apply.
5. Apply dev, configure OAuth redirects, and run the Floci smoke flow against real AWS plus DSQL.

**Exit criterion:** dev DSQL migrations/repositories pass, runtime functions cannot use `DbConnectAdmin`, and a real chunk/chain completes with alarms and failure capture enabled.

### Phase 5 — Frontend Terraform + frontend CI

1. Adapt the Heimdall web module but build/sync **`web/dist/`**; enable bucket versioning.
2. Set `endpoint` from Phase 4 output and add frontend CI with Node available in apply jobs.
3. Deploy SPA; verify login, OAuth round-trip, run status vocabulary/pagination, cancellation, and mailbox sync.

### Phase 6 — Resilience, backup, and operational gate

1. Enable prod-equivalent DSQL AWS Backup plan in dev, DynamoDB PITR/deletion protection, log retention, dashboard, and alarm routing.
2. Restore DSQL and DynamoDB into isolated non-prod resources and record RTO/RPO.
3. Test Lambda timeout after DSQL commit, event-source retry exhaustion, S3 replay guard, 24-hour-lost-wakeup simulation, scheduler dual-write crash windows, expired lock takeover, and forwarding `unknown`.
4. Soak with representative mailbox sizes; tune chunk sizes, leases, reserved concurrency, and DSQL retry/backoff.

**Exit criterion:** restore drill and failure matrix pass; no failure mode duplicates forwarding or permanently strands a pending job.

### Phase 7 — Production

1. Create `envs/prod`, separate state/account if chosen, GitHub Environment protection, backup vault/plan, deletion protection, alarms, and runbook ownership.
2. Deploy backend, run migration/health/permissions smoke, then deploy frontend.
3. Monitor error/throttle/IteratorAge/DSQL usage through a rollback window; retain the previous SPA version and Lambda artifacts.
4. Revisit DSQL vs Aurora PostgreSQL only if measured transaction/index constraints fail the Phase 4/6 gates.

---

## 9.1 README deliverable (local Floci runbook)

Implementation must add (or replace) these sections in the repo READMEs.

### `svc/README.md` — Local development (Floci)

Document at least:

- Prerequisites: Docker Compose v2, Go, Terraform, Node
- `./scripts/local_configure.sh` — first-time env files
- `./scripts/local_up.sh` — Floci (`localhost:4566`, credentials `test`/`test`) + Postgres (`localhost:5432`, user/db/password `automata`)
- Named `automata-local` network: Lambda uses `floci:4566` and `postgres:5432`; do not require `host.docker.internal`
- `./scripts/local_deploy.sh` — detects/accepts `LAMBDA_ARCH`, Terraform apply to Floci + migrate, prints `api_gateway_url`
- `./scripts/local_logs.sh` — Floci container logs (stream worker / scheduler)
- Web: set `VITE_API_BASE_URL` to that `api_gateway_url`, then `npm run dev`
- Re-run `local_deploy` after Lambda or Terraform changes
- Expected run statuses: `pending → running → success`; troubleshooting for stream failure bucket and stale pending re-wake
- Explicitly mark `JOBS_INLINE=true go run ./cmd/server` as a **debug shortcut**, not the default path
- State that Redis/Asynq are **not** used on the Floci path

### `web/README.md`

Short pointer: for local API, start Floci via the svc scripts and set `VITE_API_BASE_URL` to Terraform `api_gateway_url` (not bare `http://localhost:8080` once Floci is default).

---

## 10. Testing and verification

| Layer | What |
| ----- | ---- |
| Registry/unit | Every current type registered; payload versioning; exact chunk cap/cursor round-trip; no unbounded default |
| State machine | Conditional kick, revision/attempt fencing, cancellation, deterministic next IDs, atomic chain+lock transaction, terminal TTL |
| Idempotency | Duplicate before/after DSQL commit for every registry entry; summary 240-message/20-partial bound; draft `(run_id,message_id)` uniqueness; `40001`; stale worker; lock races; schedule crash boundaries |
| Effects | Forward claim winner, 2xx→pending-audit crash/backfill, rejection, timeout→`unknown`, and proof that recovery/replay cannot call Graph again |
| Postgres | Common+Postgres migrations and all product repositories; chunk mutation cap tests |
| Floci smoke | Named network; health via API Gateway; sync 202 → `success`; schedule chain; failure S3 capture; stale pending re-wake |
| Package/Terraform | Four zips each contain executable root `bootstrap`; architecture match; `fmt`, validate, locked provider plan |
| Real DSQL | DSQL migration set, generated uniques, async-index wait, joins/upserts, OCC retry, chunked bulk mutation |
| AWS dev | IAM negative test (`DbConnectAdmin` denied to runtime), stream retry/IteratorAge alarms, OAuth and CloudFront SPA |
| Restore | Restore DSQL full backup and DynamoDB PITR into isolated resources; verify schema/ownership/queries and record RTO/RPO |

Browser verification after Phase 5: login, connect mailbox, trigger/cancel sync, paginate Runs, and observe canonical statuses — not only a screenshot of CloudFront.

---

## 11. Risks

| Risk | Mitigation |
| ---- | ---------- |
| Sync/LLM job > 15 minutes | Registry-enforced chunk + durable cursor; no “whole job” handler |
| Duplicate/stale stream delivery | Conditional revision + attempt fence on every transition; DSQL writes retry-safe |
| Poison record blocks a shard | Three retries, one-hour max age, S3 on-failure capture, alarms and guarded replay |
| Lost wake-up / mapping outage | `TRIM_HORIZON` plus scheduler re-wake for stale pending items |
| Stuck `running` | Expired-lease compare-and-set starts a new attempt without clearing cursor; old attempt is fenced |
| Duplicate scheduled occurrence | Deterministic occurrence ID; put job before conditional DSQL schedule advance |
| Concurrent syncs | Transactional account lock item; never query-then-put via GSI |
| Duplicate/uncertain forwarding | At-most-once effect claim; stale claims become visible `unknown`, never auto-retry |
| Dual-store complexity | DSQL product commit before DDB cursor; explicit idempotency; no distributed transaction claim |
| Floci networking/architecture drift | Named network/DNS, host-detected architecture, same module and failure smoke |
| SQLite dialect lock-in | Postgres/DSQL adapter + baseline migrations; do not run SQLite in Lambda |
| DSQL vs Postgres migration drift | Common/engine split and mandatory real-DSQL pre-promotion suite |
| DSQL 3,000-row / 5-minute caps | Chunk deletes and bulk updates; never wrap Graph/LLM in one SQL tx |
| DSQL missing partial indexes | Generated columns / unique on nullable keys ([addendum-aurora-dsql.md](addendum-aurora-dsql.md)) |
| Accidental data loss | DSQL deletion protection + daily AWS Backup; DynamoDB PITR/deletion protection; restore drill |
| Secrets / default user | Fail closed without JWT in AWS and Floci `local` |
| Provider/runtime packaging drift | Locked AWS provider >=6.15,<7; setup Go in apply; root-bootstrap archive test |
| Heimdall apply-on-PR | Copying apply-dev on every PR can deploy broken API to shared dev — same as Heimdall; use a dedicated Automata dev account |

---

## 12. Open decisions (block Terraform apply, not the pipeline PR)

Record answers in `svc/terraform/envs/*/locals.tf` when known:

1. **AWS accounts** — single account vs Heimdall-style split (dev `0848…` / prod `8249…` are Yonda; Automata needs its own or an explicit share).
2. **Domain** — hosted zone and hostname (`automata.dev.…` vs something else).
3. **LLM in AWS** — hosted OpenAI-compatible URL (cannot use `localhost:1234`).
4. **Custom API domain** — optional ACM + API Gateway custom domain so OAuth URLs stay stable; Heimdall currently uses raw `execute-api` URLs.
5. **Backup topology** — prod cross-Region copy destination and retention beyond the 35-day baseline.
6. **Operations target** — SNS/email/PagerDuty destination and owner for alarms and forwarding `unknown` reconciliation.
7. **Run retention** — confirm the 30-day dev / 365-day prod defaults and whether compliance requires a permanent DSQL audit projection.

---

## 13. References

- Heimdall backend workflow: `.github/workflows/backend-ci.yml`
- Heimdall frontend workflow: `.github/workflows/frontend-ci.yml`
- Heimdall API Gateway + Lambdas: `svc/terraform/modules/heimdall/api_gateway.tf`, `lambda.tf`
- Heimdall **job stream loop** + `svc/terraform/envs/local` + local deploy scripts (Floci/LocalStack pattern)
- Heimdall EventBridge snapshot rule: analogue of Automata’s **scheduler** only (not the worker)
- yonda-local Floci Compose: Floci on `:4566` ([yonda-local README](https://github.com/Yonda-Tax/yonda-local))
- Automata worker today: `svc/cmd/worker/main.go`, `svc/internal/adapters/inbound/asynq/jobs.go`
- Automata scheduler today: `svc/cmd/server/scheduler.go` (goroutine in `cmd/server/main.go`)
- Job run port today: sqlite repo (replaced by DynamoDB jobs on AWS/Floci)
- DSQL fit: [addendum-aurora-dsql.md](addendum-aurora-dsql.md)
- AWS Lambda DynamoDB Streams failure handling: bounded retry/record age + S3 on-failure destination
- AWS Backup for Aurora DSQL: full-cluster backup/restore (not PITR)
- Floci Lambda networking: `FLOCI_HOSTNAME` + `FLOCI_SERVICES_DOCKER_NETWORK`

---

*Implementation PRs should reference this document. Application PRs that still enqueue only to Redis, process an unbounded job, or write asynchronous lifecycle state from a domain service are not AWS-ready. After Phase 3, READMEs that still prescribe Redis as the default local path are incomplete.*
