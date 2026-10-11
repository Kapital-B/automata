# R0: Contract and Dependency Inventory

**Snapshot:** 2026-10-11. See [baseline and validation](r0-domain-owned-ports-baseline.md) and [the RFC](rfc-domain-owned-ports.md).

This is a historical source snapshot, captured with Go AST parsing. Source links identify the definitions at R0; method signatures remain authoritative in those files. It is not a second set of port definitions. Candidate domain owners are migration planning inputs, not a final package split.

## 1. Application-owned contracts

All 44 dependency contracts below are outside domain: 37 in `application/ports/driven`, five named interfaces in application services, and two inline interfaces on `SyncService`. There are 42 named contracts in total. Method counts include embedded interface entries, where present.

The signature dependency column lists referenced named types other than primitives, `context.Context`, `time.Time`, and UUID. Container/pointer shapes are preserved in the linked signatures. The next section expands dependencies inside the central package so types can move with their interfaces.

| Contract/source | Methods | Signature types | Candidate domain owner |
| --- | ---: | --- | --- |
| [jobs.ChainEnqueuer](../../svc/internal/application/jobs/scheduler.go#L28) | 1 | `JobPayload`, `JobRecord` | `domain/jobs` |
| [jobs.EffectAuditReconciler](../../svc/internal/application/jobs/scheduler.go#L24) | 1 | `EffectRecord` | `domain/jobs` |
| [jobs.ScheduleCASRepository](../../svc/internal/application/jobs/scheduler.go#L20) | 1 | — | `domain/jobs` |
| [messages.SyncService.Assign](../../svc/internal/application/messages/sync.go#L26) | 1 | — | `domain/projects` |
| [messages.SyncService.Resolve](../../svc/internal/application/messages/sync.go#L23) | 1 | — | `domain/contacts` |
| [overview.AttentionCounts](../../svc/internal/application/overview/service.go#L51) | 1 | — | `domain/attention` |
| [AccountRepository](../../svc/internal/application/ports/driven/persistence.go#L355) | 8 | `AccountRow` | `domain/accounts` |
| [AssignmentRepository](../../svc/internal/application/ports/driven/persistence.go#L1027) | 14 | `AssignCandidateFilter`, `AssignmentRow`, `EffectiveAssignment`, `MessageRow`, `UnassignedItem`, `UnassignedListFilter`, `UnassignedSummary` | `domain/projects` |
| [AuthSessionRepository](../../svc/internal/application/ports/driven/sessions.go#L11) | 2 | — | `domain/auth` |
| [ConnectorRepository](../../svc/internal/application/ports/driven/persistence.go#L367) | 14 | `ConnectorAccountRow`, `ConnectorBindingRow`, `ConnectorMessageRow` | `domain/connectors` |
| [ContactRepository](../../svc/internal/application/ports/driven/persistence.go#L496) | 15 | `ContactIdentityRow`, `ContactListFilter`, `ContactRow`, `CorrespondenceParticipantRow` | `domain/contacts` |
| [ContradictionRepository](../../svc/internal/application/ports/driven/persistence.go#L962) | 6 | `ContradictionRow`, `ContradictionSideRow` | `domain/contradictions` |
| [DecisionRepository](../../svc/internal/application/ports/driven/persistence.go#L1000) | 7 | `DecisionEvidenceRow`, `DecisionRow` | `domain/decisions` |
| [FactRepository](../../svc/internal/application/ports/driven/persistence.go#L882) | 16 | `FactEvidenceRow`, `FactRow`, `FactVersionRow` | `domain/facts` |
| [ForwardRepository](../../svc/internal/application/ports/driven/persistence.go#L1103) | 13 | `ForwardActivityRow`, `ForwardAllowlistRow`, `ForwardAuditRow`, `ForwardCandidateCursor`, `ForwardRuleRow`, `ForwardRuleStats`, `ManualForwardAuditRow`, `MessageRow` | `domain/messages` |
| [GoogleOAuth](../../svc/internal/application/ports/driven/google.go#L6) | 2 | `GoogleTokenResult` | `domain/auth` |
| [InterpretationRepository](../../svc/internal/application/ports/driven/persistence.go#L930) | 6 | `InterpretationRow`, `InterpretationSourceRow` | `domain/interpretations` |
| [IssueRepository](../../svc/internal/application/ports/driven/persistence.go#L818) | 12 | `IssueItemRow`, `IssueLink`, `IssueRow` | `domain/issues` |
| [JobEnqueuer](../../svc/internal/application/ports/driven/jobs.go#L193) | 1 | `CreateJobInput`, `JobRecord` | `domain/jobs` |
| [JobExecutionPort](../../svc/internal/application/ports/driven/jobs.go#L204) | 2 | `CreateJobInput`, `JobExecutor`, `JobRecord` | `domain/jobs` |
| [JobExecutor](../../svc/internal/application/ports/driven/jobs.go#L198) | 2 | `ChunkResult`, `RunContext` | `domain/jobs` |
| [JobRunRepository](../../svc/internal/application/ports/driven/persistence.go#L1052) | 6 | `JobRunListFilter`, `JobRunRow` | `domain/jobs` |
| [JobStore](../../svc/internal/application/ports/driven/jobs.go#L167) | 19 | `ClaimEffectInput`, `CreateJobInput`, `EffectRecord`, `JobCursor`, `JobListFilter`, `JobListPage`, `JobProgress`, `JobRecord` | `domain/jobs` |
| [LLMClient](../../svc/internal/application/ports/driven/llm.go#L37) | 1 | `LLMMessage`, `LLMResponse` | `domain/ai` |
| [LLMClientWithOptions](../../svc/internal/application/ports/driven/llm.go#L41) | 2 | `LLMClient`, `LLMMessage`, `LLMRequestOptions`, `LLMResponse` | `domain/ai` |
| [LLMUsageRepository](../../svc/internal/application/ports/driven/llm_usage.go#L40) | 2 | `LLMUsageRow`, `LLMUsageTotal` | `domain/ai` |
| [MailProvider](../../svc/internal/application/ports/driven/mailbox.go#L104) | 2 | `AccountRow`, `Mailbox`, `MailboxCapabilities` | `domain/accounts` |
| [Mailbox](../../svc/internal/application/ports/driven/mailbox.go#L92) | 5 | `MailChangePage`, `MailboxCapabilities` | `domain/accounts` |
| [ManualItemRepository](../../svc/internal/application/ports/driven/persistence.go#L1011) | 5 | `ManualItemRow` | `domain/projects` |
| [MessageRepository](../../svc/internal/application/ports/driven/persistence.go#L385) | 17 | `CategoryDefinitionRow`, `MessageCategoryRow`, `MessageListFilter`, `MessageRow` | `domain/messages` |
| [MessageSearchRepository](../../svc/internal/application/ports/driven/message_search.go#L36) | 1 | `MessageSearchFilter`, `MessageSearchHit` | `domain/messages` |
| [MicrosoftOAuth](../../svc/internal/application/ports/driven/microsoft.go#L17) | 3 | `TokenPair`, `domain/accounts.MsAccountKind` | `domain/accounts` |
| [OAuthMailConnector](../../svc/internal/application/ports/driven/mailbox.go#L129) | 2 | `ConnectOptions`, `ConnectedMailbox` | `domain/accounts` |
| [OAuthStateRepository](../../svc/internal/application/ports/driven/persistence.go#L412) | 3 | — | `domain/auth` |
| [OrganisationRepository](../../svc/internal/application/ports/driven/persistence.go#L440) | 1 | `OrganisationRow` | `domain/organisations` |
| [PasswordMailConnector](../../svc/internal/application/ports/driven/mailbox.go#L175) | 1 | `ConnectedMailbox`, `PasswordConnectRequest` | `domain/accounts` |
| [ProjectRepository](../../svc/internal/application/ports/driven/persistence.go#L618) | 15 | `ActivityFilter`, `ActivityItem`, `AttentionRow`, `DueProject`, `OverviewCounts`, `OverviewProject`, `ProjectListFilter`, `ProjectMemberRow`, `ProjectRow` | `domain/projects` |
| [ScheduleRepository](../../svc/internal/application/ports/driven/persistence.go#L1096) | 4 | `ScheduleChainRow` | `domain/jobs` |
| [SlackClient](../../svc/internal/application/ports/driven/slack.go#L38) | 4 | `SlackConversation`, `SlackHistoryPage`, `SlackOAuthResult` | `domain/connectors` |
| [SummaryRepository](../../svc/internal/application/ports/driven/persistence.go#L1063) | 24 | `ActionItemRow`, `DraftSuggestionRow`, `FYIRow`, `ProjectTodoRow`, `SendAttemptRow`, `SummaryJobChunkRow`, `SummarySettingsRow`, `SummarySnapshotRow` | `domain/messages` |
| [TimelineRepository](../../svc/internal/application/ports/driven/persistence.go#L1022) | 1 | `TimelineFilter`, `TimelineItem` | `domain/projects` |
| [TokenVault](../../svc/internal/application/ports/driven/microsoft.go#L24) | 2 | — | `domain/accounts` |
| [UserRepository](../../svc/internal/application/ports/driven/persistence.go#L419) | 7 | — | `domain/auth` |
| [projectai.AttentionProjects](../../svc/internal/application/projectai/service.go#L49) | 1 | — | `domain/attention` |

`MicrosoftOAuth` is also consumed by sign-in; its initial ownership must account for that use rather than introduce an auth/accounts import cycle. `TokenVault` may be shared between account and connector flows. Attention interfaces and the two sync hooks are application collaboration contracts, implemented by application services. The scheduler CAS capability is optional today; moving it must preserve the fallback.

## 2. Central signature and supporting types

The central package declares 98 non-interface types, including private usage-scope types. This table records their immediate named dependencies; following the rows gives the transitive type closure. Blank dependency cells mean the definition uses only primitive/approved standard/value types. Interface references lead back to §1.

| Type/source | Shape | Named dependencies |
| --- | --- | --- |
| [AccountRow](../../svc/internal/application/ports/driven/persistence.go#L12) | struct | `domain/accounts.MsAccountKind` |
| [ActionItemRow](../../svc/internal/application/ports/driven/persistence.go#L191) | struct | — |
| [ActivityFilter](../../svc/internal/application/ports/driven/persistence.go#L581) | struct | — |
| [ActivityItem](../../svc/internal/application/ports/driven/persistence.go#L567) | struct | — |
| [AssignCandidateFilter](../../svc/internal/application/ports/driven/persistence.go#L461) | struct | — |
| [AssignmentRow](../../svc/internal/application/ports/driven/persistence.go#L649) | struct | — |
| [AttentionRow](../../svc/internal/application/ports/driven/persistence.go#L549) | struct | — |
| [CategoryDefinitionRow](../../svc/internal/application/ports/driven/persistence.go#L97) | struct | — |
| [ChunkResult](../../svc/internal/application/ports/driven/jobs.go#L145) | struct | `JobCursor`, `JobProgress` |
| [ClaimEffectInput](../../svc/internal/application/ports/driven/jobs.go#L137) | struct | — |
| [ConnectOptions](../../svc/internal/application/ports/driven/mailbox.go#L114) | struct | `domain/accounts.MsAccountKind` |
| [ConnectRejectedError](../../svc/internal/application/ports/driven/mailbox.go#L160) | struct | — |
| [ConnectedMailbox](../../svc/internal/application/ports/driven/mailbox.go#L119) | struct | `domain/accounts.MsAccountKind` |
| [ConnectorAccountRow](../../svc/internal/application/ports/driven/persistence.go#L29) | struct | — |
| [ConnectorBindingRow](../../svc/internal/application/ports/driven/persistence.go#L44) | struct | — |
| [ConnectorMessageRow](../../svc/internal/application/ports/driven/persistence.go#L57) | struct | — |
| [ContactIdentityRow](../../svc/internal/application/ports/driven/persistence.go#L468) | struct | — |
| [ContactListFilter](../../svc/internal/application/ports/driven/persistence.go#L489) | struct | — |
| [ContactRow](../../svc/internal/application/ports/driven/persistence.go#L445) | struct | — |
| [ContradictionRow](../../svc/internal/application/ports/driven/persistence.go#L940) | struct | — |
| [ContradictionSideRow](../../svc/internal/application/ports/driven/persistence.go#L954) | struct | — |
| [CorrespondenceParticipantRow](../../svc/internal/application/ports/driven/persistence.go#L479) | struct | — |
| [CreateJobInput](../../svc/internal/application/ports/driven/jobs.go#L92) | struct | `JobPayload` |
| [DecisionEvidenceRow](../../svc/internal/application/ports/driven/persistence.go#L991) | struct | — |
| [DecisionRow](../../svc/internal/application/ports/driven/persistence.go#L972) | struct | — |
| [DraftSuggestionRow](../../svc/internal/application/ports/driven/persistence.go#L225) | struct | — |
| [DueProject](../../svc/internal/application/ports/driven/persistence.go#L560) | struct | — |
| [EffectRecord](../../svc/internal/application/ports/driven/jobs.go#L125) | struct | — |
| [EffectiveAssignment](../../svc/internal/application/ports/driven/persistence.go#L670) | struct | — |
| [FYIRow](../../svc/internal/application/ports/driven/persistence.go#L215) | struct | — |
| [FactEvidenceRow](../../svc/internal/application/ports/driven/persistence.go#L873) | struct | — |
| [FactRow](../../svc/internal/application/ports/driven/persistence.go#L840) | struct | — |
| [FactVersionRow](../../svc/internal/application/ports/driven/persistence.go#L852) | struct | — |
| [ForwardActivityRow](../../svc/internal/application/ports/driven/persistence.go#L335) | struct | `ForwardAuditRow` |
| [ForwardAllowlistRow](../../svc/internal/application/ports/driven/persistence.go#L269) | struct | — |
| [ForwardAuditRow](../../svc/internal/application/ports/driven/persistence.go#L297) | struct | — |
| [ForwardCandidateCursor](../../svc/internal/application/ports/driven/persistence.go#L318) | struct | — |
| [ForwardRuleRow](../../svc/internal/application/ports/driven/persistence.go#L276) | struct | — |
| [ForwardRuleStats](../../svc/internal/application/ports/driven/persistence.go#L324) | struct | — |
| [GoogleTokenResult](../../svc/internal/application/ports/driven/google.go#L12) | struct | — |
| [InterpretationRow](../../svc/internal/application/ports/driven/persistence.go#L906) | struct | — |
| [InterpretationSourceRow](../../svc/internal/application/ports/driven/persistence.go#L921) | struct | — |
| [IssueItemRow](../../svc/internal/application/ports/driven/persistence.go#L801) | struct | — |
| [IssueLink](../../svc/internal/application/ports/driven/persistence.go#L810) | struct | — |
| [IssueRow](../../svc/internal/application/ports/driven/persistence.go#L777) | struct | — |
| [JobCursor](../../svc/internal/application/ports/driven/jobs.go#L37) | struct | — |
| [JobListFilter](../../svc/internal/application/ports/driven/jobs.go#L111) | struct | — |
| [JobListPage](../../svc/internal/application/ports/driven/jobs.go#L120) | struct | `JobRecord` |
| [JobPayload](../../svc/internal/application/ports/driven/jobs.go#L48) | struct | — |
| [JobProgress](../../svc/internal/application/ports/driven/jobs.go#L42) | struct | — |
| [JobRecord](../../svc/internal/application/ports/driven/jobs.go#L58) | struct | `JobCursor`, `JobPayload`, `JobProgress` |
| [JobRunListFilter](../../svc/internal/application/ports/driven/persistence.go#L153) | struct | — |
| [JobRunRow](../../svc/internal/application/ports/driven/persistence.go#L137) | struct | — |
| [LLMMessage](../../svc/internal/application/ports/driven/llm.go#L5) | struct | — |
| [LLMRequestOptions](../../svc/internal/application/ports/driven/llm.go#L10) | struct | — |
| [LLMResponse](../../svc/internal/application/ports/driven/llm.go#L31) | struct | `LLMUsage` |
| [LLMUsage](../../svc/internal/application/ports/driven/llm.go#L21) | struct | — |
| [LLMUsageRow](../../svc/internal/application/ports/driven/llm_usage.go#L13) | struct | — |
| [LLMUsageTotal](../../svc/internal/application/ports/driven/llm_usage.go#L30) | struct | — |
| [MailChangePage](../../svc/internal/application/ports/driven/mailbox.go#L84) | struct | `MailMessage` |
| [MailMessage](../../svc/internal/application/ports/driven/mailbox.go#L58) | struct | `MailRecipient` |
| [MailRecipient](../../svc/internal/application/ports/driven/mailbox.go#L75) | struct | — |
| [MailServer](../../svc/internal/application/ports/driven/mailbox.go#L137) | struct | — |
| [MailboxCapabilities](../../svc/internal/application/ports/driven/mailbox.go#L40) | struct | — |
| [ManualForwardAuditRow](../../svc/internal/application/ports/driven/persistence.go#L342) | struct | — |
| [ManualItemRow](../../svc/internal/application/ports/driven/persistence.go#L727) | struct | — |
| [MessageCategoryRow](../../svc/internal/application/ports/driven/persistence.go#L108) | struct | — |
| [MessageListFilter](../../svc/internal/application/ports/driven/persistence.go#L120) | struct | — |
| [MessageRow](../../svc/internal/application/ports/driven/persistence.go#L74) | struct | — |
| [MessageSearchFilter](../../svc/internal/application/ports/driven/message_search.go#L11) | struct | — |
| [MessageSearchHit](../../svc/internal/application/ports/driven/message_search.go#L25) | struct | — |
| [OrganisationRow](../../svc/internal/application/ports/driven/persistence.go#L432) | struct | — |
| [OverviewCounts](../../svc/internal/application/ports/driven/persistence.go#L590) | struct | — |
| [OverviewProject](../../svc/internal/application/ports/driven/persistence.go#L598) | struct | — |
| [PasswordConnectRequest](../../svc/internal/application/ports/driven/mailbox.go#L144) | struct | `MailServer` |
| [ProjectListFilter](../../svc/internal/application/ports/driven/persistence.go#L609) | struct | — |
| [ProjectMemberRow](../../svc/internal/application/ports/driven/persistence.go#L533) | struct | — |
| [ProjectRow](../../svc/internal/application/ports/driven/persistence.go#L516) | struct | — |
| [ProjectTodoRow](../../svc/internal/application/ports/driven/persistence.go#L208) | struct | `ActionItemRow` |
| [RunContext](../../svc/internal/application/ports/driven/jobs.go#L154) | struct | `JobCursor`, `JobPayload` |
| [ScheduleChainRow](../../svc/internal/application/ports/driven/persistence.go#L255) | struct | — |
| [SendAttemptRow](../../svc/internal/application/ports/driven/persistence.go#L243) | struct | — |
| [SlackConversation](../../svc/internal/application/ports/driven/slack.go#L18) | struct | — |
| [SlackHistoryPage](../../svc/internal/application/ports/driven/slack.go#L32) | struct | `SlackMessage` |
| [SlackMessage](../../svc/internal/application/ports/driven/slack.go#L24) | struct | — |
| [SlackOAuthResult](../../svc/internal/application/ports/driven/slack.go#L9) | struct | — |
| [SummaryJobChunkRow](../../svc/internal/application/ports/driven/persistence.go#L179) | struct | — |
| [SummarySettingsRow](../../svc/internal/application/ports/driven/persistence.go#L160) | struct | — |
| [SummarySnapshotRow](../../svc/internal/application/ports/driven/persistence.go#L168) | struct | — |
| [TimelineContact](../../svc/internal/application/ports/driven/persistence.go#L744) | struct | — |
| [TimelineFilter](../../svc/internal/application/ports/driven/persistence.go#L769) | struct | — |
| [TimelineItem](../../svc/internal/application/ports/driven/persistence.go#L751) | struct | `TimelineContact` |
| [TokenPair](../../svc/internal/application/ports/driven/microsoft.go#L10) | struct | — |
| [UnassignedItem](../../svc/internal/application/ports/driven/persistence.go#L688) | struct | — |
| [UnassignedListFilter](../../svc/internal/application/ports/driven/persistence.go#L709) | struct | — |
| [UnassignedSummary](../../svc/internal/application/ports/driven/persistence.go#L719) | struct | — |
| [usageScope](../../svc/internal/application/ports/driven/llm_usage.go#L49) | struct | — |
| [usageScopeKey](../../svc/internal/application/ports/driven/llm_usage.go#L47) | struct | — |

Keep current representations during R1. Do not turn a relocated domain type into an alias back to application. Preserve custom error identity and helper behavior: `ChatCompletion`, `WithUsageScope`, `UsageScopeFrom`, `MailTooLarge`, mailbox error variables, job status constants/errors, and `ConnectRejectedError` methods need explicit moves/forwarders, not just type aliases.

## 3. Interfaces that are not domain migration debt

| Interface/source | Responsibility |
| --- | --- | --- |
| [postgres.rowScanner](../../svc/internal/adapters/outbound/persistence/postgres/repo.go#L48) | Adapter-local SQL scan helper; retain in adapter. |
| [sqlite.rowScanner](../../svc/internal/adapters/outbound/persistence/sqlite/repo.go#L628) | Adapter-local SQL scan helper; retain in adapter. |
| [persistencetest.Repository](../../svc/internal/adapters/outbound/persistencetest/suite.go#L17) | Integration-suite aggregate; retain as test support. |
| [composition.repository](../../svc/internal/composition/app.go#L54) | Aggregate for composition wiring; retain in composition. |

## 4. Dependency exceptions

The architecture check records 41 originating-package/dependency pairs: 39 direct imports and two transitive SQL dependencies. Thirteen are assigned to R1, ten to R2, and eighteen to R3. These are package-level pairs, not counts of files or import statements. Domain has no exceptions.

The authoritative removable entries, reasons, and phases are in [migration_exceptions.json](../../svc/internal/architecture/testdata/migration_exceptions.json). Removing a violation requires deleting its exception in the same change.

| Originating package | Forbidden dependency | Reachability | Remove by |
| --- | --- | --- | --- |
| `internal/adapters/inbound/asynq` | `internal/application/connectors` | direct | R3 |
| `internal/adapters/inbound/asynq` | `internal/application/messages` | direct | R3 |
| `internal/adapters/inbound/asynq` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/inbound/http` | `internal/adapters/outbound/security` | direct | R2 |
| `internal/adapters/inbound/http` | `internal/application/accounts` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/attention` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/auth` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/connectors` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/contacts` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/decisions` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/facts` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/interpret` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/issues` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/jobs` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/messages` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/overview` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/inbound/http` | `internal/application/projectai` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/projects` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/reconcile` | direct | R3 |
| `internal/adapters/inbound/http` | `internal/application/todos` | direct | R3 |
| `internal/adapters/outbound/dynamodbjobs` | `internal/application/joblistcursor` | direct | R2 |
| `internal/adapters/outbound/dynamodbjobs` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/google` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/imapmail` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/llm` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/mailmime` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/memoryjobs` | `internal/application/joblistcursor` | direct | R2 |
| `internal/adapters/outbound/memoryjobs` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/microsoft` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/persistence/postgres` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/persistence/sqlite` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/persistence/sqlkit` | `internal/application/ports/driven` | direct | R1 |
| `internal/adapters/outbound/slack` | `internal/application/ports/driven` | direct | R1 |
| `internal/application/auth` | `database/sql` | direct | R2 |
| `internal/application/auth` | `golang.org/x/crypto/bcrypt` | direct | R2 |
| `internal/application/auth` | `internal/adapters/outbound/security` | direct | R2 |
| `internal/application/facts` | `database/sql` | direct | R2 |
| `internal/application/interpret` | `database/sql` | direct | R2 |
| `internal/application/jobs` | `database/sql` | transitive | R2 |
| `internal/application/reconcile` | `database/sql` | transitive | R2 |

`application/jobs` reaches SQL through its interpretation executor/service; `application/reconcile` reaches SQL through the facts service. The check follows Automata-owned imports and records the originating package so a new caller cannot inherit an existing exception.

## 5. HTTP dependency fields and direct calls

`http.Handlers` declares twelve repository fields, plus `JobStore`, `JobEnqueuer`, and a concrete Asynq `JobQueue`. Unused repository fields still belong in the R3 removal inventory.

| Field | Current type |
| --- | --- |
| `Accounts` | `driven.AccountRepository` |
| `Assignments` | `driven.AssignmentRepository` |
| `Contacts` | `driven.ContactRepository` |
| `Forwards` | `driven.ForwardRepository` |
| `Issues` | `driven.IssueRepository` |
| `JobEnqueuer` | `driven.JobEnqueuer` |
| `JobQueue` | `*asynqadapter.QueueClient` |
| `JobRuns` | `driven.JobRunRepository` |
| `JobStore` | `driven.JobStore` |
| `Messages` | `driven.MessageRepository` |
| `OAuthStates` | `driven.OAuthStateRepository` |
| `Projects` | `driven.ProjectRepository` |
| `Schedules` | `driven.ScheduleRepository` |
| `Summaries` | `driven.SummaryRepository` |
| `Users` | `driven.UserRepository` |

AST call-site inspection finds 66 direct calls across 40 functions: 54 repository calls, three job-store calls, three enqueuer calls, and six concrete queue calls. This covers explicit `h.Field.Method(...)` calls, not dynamic aliases or business semantics. Calls through concrete service fields are additional R3 coupling, captured by import exceptions.

| HTTP function | Dependency operation | Call site |
| --- | --- | --- |
| `me` | `Users.GetUserByID` | [internal/adapters/inbound/http/auth_handlers.go:198](../../svc/internal/adapters/inbound/http/auth_handlers.go#L198) |
| `me` | `Users.GetHomeOrganisationID` | [internal/adapters/inbound/http/auth_handlers.go:205](../../svc/internal/adapters/inbound/http/auth_handlers.go#L205) |
| `syncConnector` | `JobEnqueuer.Enqueue` | [internal/adapters/inbound/http/connectors.go:227](../../svc/internal/adapters/inbound/http/connectors.go#L227) |
| `syncConnector` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/connectors.go:252](../../svc/internal/adapters/inbound/http/connectors.go#L252) |
| `syncConnector` | `JobQueue.EnqueueSyncSlack` | [internal/adapters/inbound/http/connectors.go:259](../../svc/internal/adapters/inbound/http/connectors.go#L259) |
| `syncConnector` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/connectors.go:266](../../svc/internal/adapters/inbound/http/connectors.go#L266) |
| `extractProject` | `JobEnqueuer.Enqueue` | [internal/adapters/inbound/http/extract.go:45](../../svc/internal/adapters/inbound/http/extract.go#L45) |
| `putForwardAllowlist` | `Forwards.ReplaceForwardAllowlist` | [internal/adapters/inbound/http/forward_rules.go:124](../../svc/internal/adapters/inbound/http/forward_rules.go#L124) |
| `listForwardRules` | `Forwards.ListForwardRules` | [internal/adapters/inbound/http/forward_rules.go:146](../../svc/internal/adapters/inbound/http/forward_rules.go#L146) |
| `listForwardRules` | `Forwards.ListForwardAllowlist` | [internal/adapters/inbound/http/forward_rules.go:151](../../svc/internal/adapters/inbound/http/forward_rules.go#L151) |
| `listForwardRules` | `Forwards.ForwardRuleStats` | [internal/adapters/inbound/http/forward_rules.go:158](../../svc/internal/adapters/inbound/http/forward_rules.go#L158) |
| `createForwardRule` | `Forwards.CreateForwardRule` | [internal/adapters/inbound/http/forward_rules.go:231](../../svc/internal/adapters/inbound/http/forward_rules.go#L231) |
| `updateForwardRule` | `Forwards.UpdateForwardRule` | [internal/adapters/inbound/http/forward_rules.go:268](../../svc/internal/adapters/inbound/http/forward_rules.go#L268) |
| `deleteForwardRule` | `Forwards.DeleteForwardRule` | [internal/adapters/inbound/http/forward_rules.go:282](../../svc/internal/adapters/inbound/http/forward_rules.go#L282) |
| `forwardRuleActivity` | `Forwards.ListForwardActivity` | [internal/adapters/inbound/http/forward_rules.go:306](../../svc/internal/adapters/inbound/http/forward_rules.go#L306) |
| `validRule` | `Forwards.ListForwardAllowlist` | [internal/adapters/inbound/http/forward_rules.go:73](../../svc/internal/adapters/inbound/http/forward_rules.go#L73) |
| `getForwardAllowlist` | `Forwards.ListForwardAllowlist` | [internal/adapters/inbound/http/forward_rules.go:90](../../svc/internal/adapters/inbound/http/forward_rules.go#L90) |
| `rescanUnassigned` | `Accounts.ListAccounts` | [internal/adapters/inbound/http/projects.go:859](../../svc/internal/adapters/inbound/http/projects.go#L859) |
| `listCategories` | `Messages.ListCategoryDefinitions` | [internal/adapters/inbound/http/router.go:1024](../../svc/internal/adapters/inbound/http/router.go#L1024) |
| `createCategory` | `Messages.CreateCategoryDefinition` | [internal/adapters/inbound/http/router.go:1105](../../svc/internal/adapters/inbound/http/router.go#L1105) |
| `updateCategory` | `Messages.GetCategoryDefinitionByID` | [internal/adapters/inbound/http/router.go:1130](../../svc/internal/adapters/inbound/http/router.go#L1130) |
| `updateCategory` | `Messages.UpdateCategoryDefinition` | [internal/adapters/inbound/http/router.go:1145](../../svc/internal/adapters/inbound/http/router.go#L1145) |
| `deleteCategory` | `Messages.GetCategoryDefinitionByID` | [internal/adapters/inbound/http/router.go:1160](../../svc/internal/adapters/inbound/http/router.go#L1160) |
| `deleteCategory` | `Messages.CountMessageCategoriesByCategory` | [internal/adapters/inbound/http/router.go:1172](../../svc/internal/adapters/inbound/http/router.go#L1172) |
| `deleteCategory` | `Messages.GetCategoryDefinitionByID` | [internal/adapters/inbound/http/router.go:1188](../../svc/internal/adapters/inbound/http/router.go#L1188) |
| `deleteCategory` | `Messages.ReassignMessageCategories` | [internal/adapters/inbound/http/router.go:1198](../../svc/internal/adapters/inbound/http/router.go#L1198) |
| `deleteCategory` | `Messages.DeleteCategoryDefinition` | [internal/adapters/inbound/http/router.go:1204](../../svc/internal/adapters/inbound/http/router.go#L1204) |
| `categorizeAccount` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/router.go:1276](../../svc/internal/adapters/inbound/http/router.go#L1276) |
| `categorizeAccount` | `JobQueue.EnqueueCategorize` | [internal/adapters/inbound/http/router.go:1281](../../svc/internal/adapters/inbound/http/router.go#L1281) |
| `categorizeAccount` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/router.go:1291](../../svc/internal/adapters/inbound/http/router.go#L1291) |
| `refreshSummary` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/router.go:1348](../../svc/internal/adapters/inbound/http/router.go#L1348) |
| `refreshSummary` | `JobQueue.EnqueueSummarize` | [internal/adapters/inbound/http/router.go:1353](../../svc/internal/adapters/inbound/http/router.go#L1353) |
| `refreshSummary` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/router.go:1362](../../svc/internal/adapters/inbound/http/router.go#L1362) |
| `generateDrafts` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/router.go:1444](../../svc/internal/adapters/inbound/http/router.go#L1444) |
| `generateDrafts` | `JobQueue.EnqueueDraftSuggest` | [internal/adapters/inbound/http/router.go:1449](../../svc/internal/adapters/inbound/http/router.go#L1449) |
| `generateDrafts` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/router.go:1460](../../svc/internal/adapters/inbound/http/router.go#L1460) |
| `listSummaries` | `Summaries.ListSummarySnapshots` | [internal/adapters/inbound/http/router.go:1482](../../svc/internal/adapters/inbound/http/router.go#L1482) |
| `listSummaries` | `Summaries.ListOpenActionItems` | [internal/adapters/inbound/http/router.go:1496](../../svc/internal/adapters/inbound/http/router.go#L1496) |
| `listSummaries` | `Summaries.ListOpenFYI` | [internal/adapters/inbound/http/router.go:1501](../../svc/internal/adapters/inbound/http/router.go#L1501) |
| `markActionItemDone` | `Summaries.MarkActionItemDone` | [internal/adapters/inbound/http/router.go:1570](../../svc/internal/adapters/inbound/http/router.go#L1570) |
| `dismissFYI` | `Summaries.DeleteFYI` | [internal/adapters/inbound/http/router.go:1587](../../svc/internal/adapters/inbound/http/router.go#L1587) |
| `getSummarySettings` | `Summaries.GetSummarySettings` | [internal/adapters/inbound/http/router.go:1599](../../svc/internal/adapters/inbound/http/router.go#L1599) |
| `updateSummarySettings` | `Summaries.UpsertSummarySettings` | [internal/adapters/inbound/http/router.go:1644](../../svc/internal/adapters/inbound/http/router.go#L1644) |
| `getSchedules` | `Schedules.ListSchedulesByUser` | [internal/adapters/inbound/http/router.go:1656](../../svc/internal/adapters/inbound/http/router.go#L1656) |
| `updateSchedules` | `Schedules.ReplaceSchedulesByUser` | [internal/adapters/inbound/http/router.go:1757](../../svc/internal/adapters/inbound/http/router.go#L1757) |
| `listRuns` | `JobStore.List` | [internal/adapters/inbound/http/router.go:1785](../../svc/internal/adapters/inbound/http/router.go#L1785) |
| `listRuns` | `JobRuns.ListJobRuns` | [internal/adapters/inbound/http/router.go:1839](../../svc/internal/adapters/inbound/http/router.go#L1839) |
| `getRun` | `JobStore.Get` | [internal/adapters/inbound/http/router.go:1860](../../svc/internal/adapters/inbound/http/router.go#L1860) |
| `getRun` | `JobRuns.GetJobRun` | [internal/adapters/inbound/http/router.go:1883](../../svc/internal/adapters/inbound/http/router.go#L1883) |
| `cancelRun` | `JobStore.RequestCancel` | [internal/adapters/inbound/http/router.go:1906](../../svc/internal/adapters/inbound/http/router.go#L1906) |
| `authorizeAccount` | `Accounts.GetAccount` | [internal/adapters/inbound/http/router.go:2009](../../svc/internal/adapters/inbound/http/router.go#L2009) |
| `enqueueAccountJob` | `JobEnqueuer.Enqueue` | [internal/adapters/inbound/http/router.go:2020](../../svc/internal/adapters/inbound/http/router.go#L2020) |
| `jobStoreItem` | `Accounts.GetAccount` | [internal/adapters/inbound/http/router.go:2049](../../svc/internal/adapters/inbound/http/router.go#L2049) |
| `listAccounts` | `Accounts.ListAccounts` | [internal/adapters/inbound/http/router.go:243](../../svc/internal/adapters/inbound/http/router.go#L243) |
| `getAccount` | `Accounts.GetAccount` | [internal/adapters/inbound/http/router.go:462](../../svc/internal/adapters/inbound/http/router.go#L462) |
| `syncAccount` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/router.go:557](../../svc/internal/adapters/inbound/http/router.go#L557) |
| `syncAccount` | `JobQueue.EnqueueSync` | [internal/adapters/inbound/http/router.go:562](../../svc/internal/adapters/inbound/http/router.go#L562) |
| `syncAccount` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/router.go:571](../../svc/internal/adapters/inbound/http/router.go#L571) |
| `listMessages` | `Messages.ListMessages` | [internal/adapters/inbound/http/router.go:653](../../svc/internal/adapters/inbound/http/router.go#L653) |
| `listMessages` | `Assignments.EffectiveProjectIDsForMessages` | [internal/adapters/inbound/http/router.go:661](../../svc/internal/adapters/inbound/http/router.go#L661) |
| `getMessage` | `Messages.GetMessage` | [internal/adapters/inbound/http/router.go:769](../../svc/internal/adapters/inbound/http/router.go#L769) |
| `listDrafts` | `Summaries.ListDraftSuggestions` | [internal/adapters/inbound/http/router.go:849](../../svc/internal/adapters/inbound/http/router.go#L849) |
| `runForwardRules` | `JobRuns.InsertJobRun` | [internal/adapters/inbound/http/router.go:906](../../svc/internal/adapters/inbound/http/router.go#L906) |
| `runForwardRules` | `JobQueue.EnqueueForwardRules` | [internal/adapters/inbound/http/router.go:907](../../svc/internal/adapters/inbound/http/router.go#L907) |
| `runForwardRules` | `JobRuns.UpdateJobRunStatus` | [internal/adapters/inbound/http/router.go:912](../../svc/internal/adapters/inbound/http/router.go#L912) |
| `listDraftAttempts` | `Summaries.ListSendAttemptsByDraft` | [internal/adapters/inbound/http/router.go:941](../../svc/internal/adapters/inbound/http/router.go#L941) |

## 6. Explicit SQL absence returns

AST inspection finds 32 explicit `sql.ErrNoRows` return statements across fourteen repository methods. It also finds 121 selector references in production source, including internal adapter recognition and five application references. Raw query errors can propagate without an explicit sentinel expression, so this is not an exhaustive proof of all absent-record paths. See the baseline behavior matrix for those cases and boolean/nil/idempotent conventions.

| Method | Explicit returned shape(s) | Evidence |
| --- | --- | --- |
| `GetHomeOrganisationID` | `return uuid.Nil, sql.ErrNoRows` | [postgres:2152](../../svc/internal/adapters/outbound/persistence/postgres/repo.go#L2152), [postgres:2157](../../svc/internal/adapters/outbound/persistence/postgres/repo.go#L2157), [sqlite:116](../../svc/internal/adapters/outbound/persistence/sqlite/users.go#L116), [sqlite:122](../../svc/internal/adapters/outbound/persistence/sqlite/users.go#L122) |
| `GetMessageIDByProvider` | `return uuid.Nil, sql.ErrNoRows` | [postgres:604](../../svc/internal/adapters/outbound/persistence/postgres/repo.go#L604), [sqlite:315](../../svc/internal/adapters/outbound/persistence/sqlite/repo.go#L315) |
| `GetUserByEmail` | `return uuid.Nil, nil, sql.ErrNoRows` | [postgres:2126](../../svc/internal/adapters/outbound/persistence/postgres/repo.go#L2126), [sqlite:87](../../svc/internal/adapters/outbound/persistence/sqlite/users.go#L87) |
| `MergeContacts` | `return sql.ErrNoRows` | [postgres:286](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L286), [postgres:293](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L293), [sqlite:334](../../svc/internal/adapters/outbound/persistence/sqlite/contacts.go#L334), [sqlite:343](../../svc/internal/adapters/outbound/persistence/sqlite/contacts.go#L343) |
| `RemoveDecisionEvidence` | `return sql.ErrNoRows` | [postgres:2585](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2585), [sqlite:155](../../svc/internal/adapters/outbound/persistence/sqlite/decisions.go#L155) |
| `RemoveFactEvidence` | `return sql.ErrNoRows` | [postgres:2162](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2162), [sqlite:249](../../svc/internal/adapters/outbound/persistence/sqlite/facts.go#L249) |
| `RemoveIssueItem` | `return sql.ErrNoRows` | [postgres:1790](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L1790), [sqlite:117](../../svc/internal/adapters/outbound/persistence/sqlite/issues.go#L117) |
| `UpdateContradiction` | `return sql.ErrNoRows` | [postgres:2446](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2446), [sqlite:111](../../svc/internal/adapters/outbound/persistence/sqlite/contradictions.go#L111) |
| `UpdateDecision` | `return sql.ErrNoRows` | [postgres:2562](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2562), [sqlite:124](../../svc/internal/adapters/outbound/persistence/sqlite/decisions.go#L124) |
| `UpdateFact` | `return sql.ErrNoRows` | [postgres:1998](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L1998), [sqlite:40](../../svc/internal/adapters/outbound/persistence/sqlite/facts.go#L40) |
| `UpdateFactVersion` | `return sql.ErrNoRows` | [postgres:2126](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2126), [sqlite:214](../../svc/internal/adapters/outbound/persistence/sqlite/facts.go#L214) |
| `UpdateInterpretationStatus` | `return sql.ErrNoRows` | [postgres:2321](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L2321), [sqlite:99](../../svc/internal/adapters/outbound/persistence/sqlite/interpretations.go#L99) |
| `UpdateIssue` | `return sql.ErrNoRows` | [postgres:1767](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L1767), [sqlite:86](../../svc/internal/adapters/outbound/persistence/sqlite/issues.go#L86) |
| `UpdateManualItemAssignment` | `return sql.ErrNoRows` | [postgres:1483](../../svc/internal/adapters/outbound/persistence/postgres/domains.go#L1483), [sqlite:80](../../svc/internal/adapters/outbound/persistence/sqlite/manual.go#L80) |
