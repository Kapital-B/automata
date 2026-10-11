# R0: Domain-Owned Ports Refactoring Baseline

**Status:** Complete

**Recorded:** 2026-10-11

**Related:** [Refactoring RFC](rfc-domain-owned-ports.md), [contract/dependency inventory](r0-domain-owned-ports-inventory.md), [before/after test evidence](r0-domain-owned-ports-test-results.json), [architecture check](../../svc/internal/architecture/README.md)

R0 establishes the existing behavior and architecture debt before moving contracts or implementations. Production behavior has not been refactored. The new check protects domain isolation and prevents the documented debt from growing; it does not declare the service compliant with the final architecture.

## 1. Snapshot and scope

The baseline is the working tree at commit `7c06bca5666ff69af771734f1754cc18965ba08c`, including the pre-existing `svc/README.md` edits and untracked `svc/cmd/seeddev/` tooling. Those changes were preserved and the CLI's tests were included. The refactoring RFC was also present as an untracked document. This is a working-tree baseline, not a claim that every tested file is in that commit.

The source inventory covers production Go files in `svc/internal/`; the architecture check also covers `svc/cmd/` and domain test imports. It parses Go ASTs and separates test imports from production imports. Source variants are checked even when their build tags do not match the current platform. Generated Terraform artifacts and frontend code are outside the architecture inventory.

| Baseline measure | Result |
| --- | ---: |
| Domain packages | 10; standard-library imports only |
| Application-owned dependency contracts | 44: 37 central, five other named, two inline |
| Named non-interface definitions in `application/ports/driven` | 98, including private supporting types |
| Package/dependency violations under target rules | 41: 39 direct, two transitive |
| Import exceptions assigned to R1 / R2 / R3 | 13 / 10 / 18 |
| Domain exceptions | 0 |
| Repository fields on `http.Handlers` | 12, plus `JobStore`, `JobEnqueuer`, and concrete `JobQueue` |
| Direct HTTP dependency calls | 66 across 40 functions |
| Explicit SQL absence return statements | 32 across 14 repository methods |

The [inventory](r0-domain-owned-ports-inventory.md) lists each contract and its signature dependencies, all central types, the exception pairs, every direct HTTP call site, and explicit SQL-error returns. Counts describe source structure; they are not coverage percentages.

## 2. Dependency baseline and enforcement

[TestServiceArchitecture](../../svc/internal/architecture/architecture_test.go) enforces the target dependency rules against [exact migration exceptions](../../svc/internal/architecture/testdata/migration_exceptions.json). Existing debt is allowed only for its recorded originating package/dependency or contract; each exception includes a reason and removal phase.

Domain production and tests have no outward exceptions. Application and adapter rules already reject newly introduced boundaries. The checker follows reachable imports through Automata-owned production packages, so an allowed application-to-application dependency cannot hide a new infrastructure dependency. It deliberately does not apply Automata's layers to the internal dependencies of the permitted UUID library.

The two inherited violations are `application/jobs` reaching `database/sql` through interpretation and `application/reconcile` reaching it through facts. They are distinct originating-package exceptions, not a blanket permission for all application code to use SQL.

Named application interfaces and nonempty inline struct-field interfaces also have an ownership baseline. New contracts in these forms must live in domain. During R1, aliases pointing into domain can replace old definitions; the associated ownership exceptions must then be removed. New violations, invalid/duplicate entries, and obsolete exceptions fail the test.

Non-domain test files may construct multi-layer integration fixtures. The existing `jobstoretest`, `mailboxtest`, and `persistencetest` packages are explicitly classified as test support despite having ordinary `.go` files. Production imports of those packages are forbidden. This does not exempt their neighboring production adapters or arbitrary future packages ending in `test`.

Regression cases verify layer boundaries, namespace lookalikes, transitive dependencies/cycles, domain/test-support exceptions, stale/duplicate entries, build variants, named/inline contracts, and production/test separation. Backend CI already runs `go test ./...`, so the check is included without changing deployment workflows.

The check does not freeze method signatures, prove business logic placement, or detect every possible anonymous-interface form. The HTTP operation inventory and behavior matrix below are explicit migration obligations, not automated claims of correctness.

## 3. Helper and implementation seams

| Current seam | Baseline responsibility | Migration obligation |
| --- | --- | --- |
| [`application/ports/driven`](../../svc/internal/application/ports/driven) | Contracts, models, enums, sentinels, LLM dispatch and usage-scope helpers | R1 moves canonical definitions into domain; functions and variables need deliberate forwarding, not type aliases alone |
| [`application/accounts/opener.go`](../../svc/internal/application/accounts/opener.go) | Orchestrates encrypted credentials, provider selection, rotation persistence and expired status | Keep orchestration in application; consumers eventually receive suitable domain-owned contracts |
| [`application/auth/service.go`](../../svc/internal/application/auth/service.go) | Calls security JWT signing, bcrypt and SQL absence checks | R2 injects token/password ports and translates storage absence |
| [`application/auth/msclaims.go`](../../svc/internal/application/auth/msclaims.go) | Maps Microsoft token claims to the existing subject/email outcome | R2 moves provider mapping behind the identity capability without changing identity selection |
| [`application/auth/tokens.go`](../../svc/internal/application/auth/tokens.go) | Standard-library opaque refresh-token generation/hashing | No outward import violation; preserve refresh rotation and hash compatibility |
| [`application/joblistcursor`](../../svc/internal/application/joblistcursor) | Cursor encoding/signing, imported by both outbound job stores | R2 moves storage/transport encoding to an outbound helper and preserves existing cursor bytes/scoping |
| [`application/jobkit`](../../svc/internal/application/jobkit) | Application job coordination helpers | Retain orchestration; relocate domain-only types/policy where identified in R1/R3 |
| [`application/jobs/registry.go`](../../svc/internal/application/jobs/registry.go) | Job metadata and schedule-chain normalization invoked from HTTP | R3 puts schedule use cases behind a contract; preserve supported jobs/order and validation |
| [`application/messages/forward_conditions.go`](../../svc/internal/application/messages/forward_conditions.go) | Forward-rule normalization/evaluation consumed by services and HTTP | R3 extracts reusable policy into domain, leaving repository orchestration in application |
| [`adapters/inbound/asynq/jobs.go`](../../svc/internal/adapters/inbound/asynq/jobs.go) | Mixes a concrete queue producer and task consumption | R3 separates outbound producer from inbound consumer and removes concrete queue wiring from HTTP |
| [`composition/app.go`](../../svc/internal/composition/app.go) | Builds providers/repositories/services and exposes runtime wiring | Retain the composition root; update bindings as contracts move |

The four additional named interfaces in SQL adapters, composition, and test support are technical scan/wiring/test aggregates. They are recorded separately in the inventory and are not business ports to move into domain.

## 4. Missing-record and idempotency behavior

The service currently mixes error, nil-result, boolean and no-op conventions. R2 must translate SQL implementation details while preserving each operation's semantics. Storage failures must not be reclassified as absence. This matrix records inspected behavior; it does not imply that every branch already has a dedicated test.

| Operation(s) | Current absence/outcome convention | Behavior to preserve |
| --- | --- | --- |
| `UserRepository.GetUserByEmail` | `uuid.Nil`, nil hash, `sql.ErrNoRows` for no user; a present user may legitimately have a nil password hash | Registration treats absence as available email; login treats absence/nil hash as invalid credentials; external login may create/link an identity |
| `GetUserByID`, `GetHomeOrganisationID` | Missing user propagates SQL absence; missing home organisation also reports `sql.ErrNoRows` | Preserve application/HTTP outcome while replacing the implementation-specific sentinel |
| `MessageRepository.GetMessageIDByProvider` | `uuid.Nil`, `sql.ErrNoRows` | Distinguish absent provider message from query failure |
| `GetAccount`, `GetMessage`, `GetFact`, `GetInterpretation` | Nil result with nil error when the scoped record is absent; `GetAccount` also returns nil credential bytes | Keep scoped absence/non-disclosure and each caller's existing missing-record response |
| `UserRepository.FindIdentity` | `uuid.Nil`, `ok=false`, nil error | A missing identity is an expected lookup result |
| `AuthSessionRepository.ConsumeAuthSession` | `ok=false`, nil error for missing, consumed or expired token | Single-use refresh tokens remain single-use; successful consumption remains atomic |
| `OAuthStateRepository.TakeOAuthState` | `ok=false`, nil error for unavailable state | Preserve single-use/expiry handling and invalid-state responses |
| `MergeContacts` | Missing source/survivor reports SQL absence; other ownership/merge conflicts have their own errors | Do not collapse absence and conflicting identities/organisation into one outcome |
| `UpdateManualItemAssignment`, `UpdateIssue`, `UpdateFact`, `UpdateFactVersion`, `UpdateContradiction`, `UpdateDecision` | A zero affected-row count reports `sql.ErrNoRows` | Keep update failure distinguishable from a successful write |
| `RemoveFactEvidence`, `RemoveIssueItem`, `RemoveDecisionEvidence`, `UpdateInterpretationStatus` | Zero affected rows reports `sql.ErrNoRows` | `facts.RemoveEvidence` and `interpret.Dismiss` currently translate this into their application `ErrNotFound`; preserve API behavior |
| `DeleteCategoryDefinition`, `DeleteAccount` | Missing scoped records do not become SQL absence errors; a successful no-op delete returns nil | Do not make currently idempotent deletes fail simply because errors are standardized |
| Repository list operations | No matches produce an empty result, potentially a nil slice, with nil error | Preserve response serialization through existing view/HTTP mappings |
| `ScheduleCASRepository.MarkScheduleExecutedIfDue` | No matching schedule/time returns `false`, nil error | This is a conditional-write outcome; the scheduler retains its optional CAS/fallback behavior |
| Job-store `Get`, `GetByID`, `GetEffect` | Missing records report existing `driven.ErrJobNotFound`; scoped reads conceal another user's job | Preserve domain-like sentinels during relocation; do not convert them to SQL absence |
| Job-store conditional writes and effect claims | Existing conflict/lock/effect sentinels distinguish failed preconditions from absence | Preserve fencing, idempotency, lease recovery and crash-window behavior |

The inventory lists all fourteen methods with explicit SQL sentinel return expressions. `GetUserByID` illustrates why that list alone is insufficient: raw scan errors can propagate without explicitly naming `sql.ErrNoRows`. Update adapters and their consumers together, and check the existing contract suites for each migrated method.

## 5. Baseline validation

Both complete runs used Go `go1.27.0 linux/amd64`; the module declares Go `1.24.0`. Dependencies were available locally (`GOPROXY=off`, `GOSUMDB=off`). Tests were rerun with `-count=1`; these are not cached test results.

| Full service run | Passing packages | Packages without tests | Passing top-level tests | Passing test cases, including subtests | Failed/skipped tests |
| --- | ---: | ---: | ---: | ---: | ---: |
| Before R0 code | 33 | 24 | 302 | 455 | 0 / 0 |
| After R0 code | 34 | 24 | 309 | 489 | 0 / 0 |

The difference is the new architecture package: seven top-level tests and 34 cases including subtests. Go emits package-level `skip` events for packages with no test files; those are the 24 entries above, not skipped integration tests. [Recorded evidence](r0-domain-owned-ports-test-results.json) contains exact timestamps, per-package results and the environment.

Executed from `svc/`, with the local database variables below set:

```sh
go test -count=1 -p 2 -json ./...
```

| Area | Actual coverage |
| --- | --- |
| SQLite | Full existing repository contracts, migrations and regression tests |
| Postgres | Full existing contracts and timeline regression against the existing `automata-ci-pg` service on localhost port 55432; tests created and cleaned unique schemas |
| Memory jobs | Existing job-store contracts and lifecycle/crash-window checks |
| DynamoDB jobs | Existing job-store contracts, crash windows and completed-chain lock cleanup against a task-owned in-memory DynamoDB Local container on localhost port 18000 |
| Mailbox providers | Existing stub-based mailbox contracts; no live provider accounts contacted |
| Authentication, HTTP and application | Existing service/handler tests, including refresh/OAuth flows, paging, forwarding and executor idempotency |
| DSQL | Existing syntax/lint, DSN and local runtime-helper tests; no live Aurora DSQL cluster or deployed IAM/runtime migration validation |
| Architecture | Source boundaries and the new checker regression cases |

The first attempt was blocked by a full filesystem and produced no usable test result. Only ignored, regenerable Lambda build outputs (`.dist` and the four bootstrap archives) were cleared. Terraform state and provider installations were preserved. Temporary Go cache/build directories in `/dev/shm` kept subsequent compilation off the nearly full disk.

An intermediate Postgres-only run passed 442 cases and skipped the three DynamoDB integration tests. The complete before/after runs above replaced that incomplete coverage by enabling DynamoDB Local.

For reproduction, point `AUTOMATA_TEST_POSTGRES_DSN` at a disposable local Postgres database and `AUTOMATA_TEST_DDB_ENDPOINT` at DynamoDB Local. The exact local values used here are in the evidence JSON. The task-owned emulator was stopped after validation; the pre-existing Postgres service was left running. Live DSQL/deployment behavior remains outside this local baseline.

## 6. R0 exit and next phase

R0 is complete: contracts and signature dependencies are inventoried, helper seams and direct HTTP operations are recorded, missing-record behavior is documented, the full local baseline passes, and CI will prevent new structural violations through the architecture test.

R1 can now move canonical contract/type definitions context by context. Preserve current representations and behavior, remove ownership/import exceptions as their debt disappears, and rerun the same suites. No R1 relocation has been performed as part of this baseline.
