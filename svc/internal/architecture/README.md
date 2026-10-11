# Architecture checks

These tests enforce the migration rules in [the domain-owned-ports RFC](../../../docs/specs/rfc-domain-owned-ports.md). The [R0 baseline](../../../docs/specs/r0-domain-owned-ports-baseline.md) records the original contracts, dependencies, behaviors, and test results.

Run from `svc/`:

```sh
go test ./internal/architecture
```

Backend CI already runs `go test ./...`, which includes this package. No separate CI job or infrastructure dependency is required.

## What is checked

- Go AST parsing separates production imports from test imports and scans source variants regardless of platform/build tags.
- Domain production and domain tests may depend only on domain packages, permitted standard-library packages, and the UUID value library. Domain has no migration exceptions.
- Application may depend on domain/application packages, but not infrastructure implementations. Domain/application imports of SQL or HTTP packages are rejected; other third-party imports are rejected except UUID.
- Inbound adapters may depend on domain and inbound/shared adapter helpers. Outbound adapters may depend on domain and outbound/shared adapter helpers. Composition, configuration, and entrypoints keep their wiring responsibilities.
- Dependencies are followed through Automata-owned production packages. A new caller cannot inherit an existing package's permission to reach infrastructure. Third-party libraries' private import graphs are not traversed.
- Named application interfaces and nonempty inline struct-field interfaces must be defined in domain. The completed migration has no ownership or dependency exceptions. A final check rejects application compatibility aliases and forwarded domain symbols, and the old `application/ports` package must remain absent.
- Every exception identifies one originating package/dependency or one contract, a reason, and its removal phase. New violations, duplicate/invalid exceptions, and stale exceptions fail the check.

The source check supplements Go compilation: it does not replace compiler detection of package cycles, inspect business semantics, detect every possible form of anonymous interface, or prove that DTOs are mapped correctly. HTTP and workers invoke domain-owned inbound ports; application services implement orchestration, and outbound adapters implement persistence/security/queue ports. Compile-time assertions and adapter contract suites complement the source checks.

## Test rules

Non-domain `_test.go` files may wire multiple layers for integration setup. Their imports never enter the production dependency graph. Domain test files follow domain isolation rules.

Three existing helper packages contain integration suites in ordinary `.go` files: `jobstoretest`, `mailboxtest`, and `persistencetest` under `internal/adapters/outbound`. These exact packages get test-support rules; production cannot import them, even through another package. A new directory ending in `test` receives no automatic exemption.

## Migrating an exception

When a refactor removes a dependency or moves a contract, delete the corresponding entry from [migration_exceptions.json](testdata/migration_exceptions.json) in the same change. The check deliberately fails on obsolete entries so permissions cannot accumulate.

The exception file is a record of R0 debt, not an automatic update mechanism. Do not regenerate it to approve new violations. Preserve the rules while implementing R1–R4, and remove each exception by its recorded phase. The baseline documentation is a historical snapshot; it does not need to be rewritten when an exception is removed.
