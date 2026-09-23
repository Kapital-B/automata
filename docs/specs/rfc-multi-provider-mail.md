# RFC: Multi-Provider Mail

**Status:** Draft — for discussion, not yet scheduled
**Related spec:** [Aurora DSQL](addendum-aurora-dsql.md) §3 (limits), [Redis/Asynq jobs](addendum-redis-asynq-jobs.md), [Project correspondence Wave 1](addendum-project-correspondence-wave1.md) §7 (assignment)
**Last updated:** 2026-09-23

Automata reads exactly one kind of mailbox: a Microsoft 365 account, through Graph. Every layer above the adapter assumes it. This RFC proposes what has to change to add Google, IMAP/SMTP, and on-premises Exchange, and argues for where the seam belongs.

---

## 1. What is actually Microsoft-specific today

Not as much as it looks, and not where you would expect.

**The provider column is already open.** `accounts.provider` is `TEXT NOT NULL DEFAULT 'm365'` with no CHECK, so new values cost nothing. `AssignService`, the job registry, triage, and the whole project domain key off `account_id` and never read the provider.

**The blocker is one column.** `accounts.ms_account_kind` is `NOT NULL CHECK (ms_account_kind IN ('work', 'personal'))` — an unnamed CHECK on a NOT NULL column. A Gmail or IMAP account has no meaningful value for it. §5.1 deals with this; it is the single most awkward part of the change.

The domain already disagrees with that constraint: `accounts.KindCommon` exists in Go and would be rejected by the database. Nothing constructs it today, so it has never failed.

**The port is a vendor API, not a capability.** `driven.MicrosoftGraph` is eight methods that mix reading, sending, and Graph's own id resolution:

| Method | What it really is |
| ------ | ----------------- |
| `ListInboxDelta` | incremental read, Graph's cursor shape |
| `GetMessageBody` | fetch one message |
| `SendMail` | outbound, new message |
| `ReplyToMessage` | outbound, server-side threading |
| `ForwardMessage` | outbound, **server-side, preserves attachments without downloading** |
| `ResolveGraphMessageID` | Graph immutable-id quirk |

Four services depend on it directly: `SyncService`, `DraftService`, `ForwardRulesService`, `AccountService`, plus `manual_forward` and the job executors.

**Token handling assumes one shape.** `token_ciphertext` holds a blob whose payload is `(MsAccountKind, refreshToken)`, and the refresh path is `OAuth.RefreshAccessToken(ctx, kind, refresh)`. IMAP has no refresh token; it has a password that never expires, which is a different security problem.

**Sync state assumes one cursor.** `account_sync_state.delta_link` is a Graph delta link. There is already an unused `cursor_json` column beside it, written as NULL and never read — free space for provider-specific state.

**Google exists but only for sign-in.** `driven.GoogleOAuth` returns a subject and an email for login. It has no mail scopes and no client. Adding Gmail is not "finishing" it; it is a new adapter that happens to share an OAuth registration.

---

## 2. The thing that does not generalise

Providers differ most where it matters most: **forwarding**.

`ForwardMessage` is a Graph server-side operation. The original message, its formatting, and its attachments never touch our infrastructure. Gmail, IMAP and POP have no equivalent — forwarding means fetching the full MIME, re-wrapping it, and sending it ourselves.

That is not a smaller version of the same feature. It changes:

- **Cost and latency** — a 20 MB attachment becomes a download and an upload.
- **Deliverability** — we become the sending party. SPF and DKIM now describe us, not the original sender, and forwarded mail is more likely to be filtered.
- **Data exposure** — attachment bytes pass through the worker, which they currently never do.
- **Size limits** — SMTP servers reject what Graph would have forwarded happily.

Forward rules are an existing, automated feature. Silently giving them worse behaviour on a new provider is exactly the failure mode this codebase keeps getting bitten by: something that looks like it ran, did not do what the operator assumed, and said nothing. §4 proposes capability declaration for this reason.

---

## 3. Non-goals

- **POP3.** No server-side state, no stable ids, no folders, no removal signal, and destructive retrieval by default. Everything downstream — delta sync, thread assignment, removal handling — has no meaning. If someone genuinely needs POP, it should be a one-way importer, not a mailbox.
- **Calendar, contacts-as-directory, or files** from any provider. This is about mail.
- **Outbound-only accounts** (an SMTP relay with no mailbox). Plausible later, but it is not an account in the sense the rest of the system means.
- **Migrating existing Microsoft accounts** to a different code path than the one they use today. R1 is a refactor that must not change their behaviour.
- **Per-folder sync.** Inbox only, as now. Labels and folders are a separate piece of work.

---

## 4. Proposal: a capability-declaring mailbox port

Replace direct `MicrosoftGraph` use with a `driven.Mailbox` port, obtained per account from a registry keyed on `provider`.

The important design choice is that this is **not** a lowest-common-denominator interface. Reducing every provider to what IMAP can do would throw away Graph's server-side forward, which is the better behaviour. Instead the port declares what it supports:

```go
type MailboxCapabilities struct {
    IncrementalSync     bool // false means full re-list each run
    ServerSideForward   bool // false means fetch MIME and re-send
    ServerSideReply     bool
    ReportsRemovals     bool // provider tells us when a message leaves the folder
    StableMessageIDs    bool
}

type Mailbox interface {
    Capabilities() MailboxCapabilities
    ListChanges(ctx context.Context, cursor MailCursor, pageSize int) (*MailChangePage, error)
    GetMessage(ctx context.Context, providerMessageID string) (*MailMessage, error)
    GetRawMessage(ctx context.Context, providerMessageID string) (io.ReadCloser, error)
    Send(ctx context.Context, msg OutboundMessage) error
    Reply(ctx context.Context, providerMessageID string, body string) error
    Forward(ctx context.Context, providerMessageID, to, comment string) error
}
```

Four things about this shape:

**Credentials live behind the port, not in front of it.** Today every caller passes `accessToken`, which forces every caller to know about OAuth refresh. A `Mailbox` is constructed for an account with its credentials already resolved, so `SyncService` stops knowing what a refresh token is.

**`GetRawMessage` is what makes portable forwarding possible.** It is the fetch half of the fallback in §2, and Graph can implement it too, so the fallback is testable against the provider we understand best.

**`Forward` is always present, but `ServerSideForward` says how it will behave.** A provider without it implements `Forward` via `GetRawMessage` + `Send`. Callers that care — forward rules, which run unattended — can read the capability and warn. Callers that do not can just call it.

**`MailChangePage` must carry removals explicitly**, not as an empty message. A Graph delta tombstone is an id plus `@removed`; a Gmail history record has `messagesDeleted`; IMAP has `EXPUNGE`. All three mean "this left the folder", and the September 2026 incident came from decoding one of them as a message with every field empty and writing it over a good row. The contract should say: a change record is either an **upsert with a full payload** or a **removal**, never a partial, and the sync service must reject anything else.

---

## 5. Decisions needed

### 5.1 What to do about `ms_account_kind`

It is `NOT NULL` with an unnamed `CHECK` limiting it to two Microsoft-specific values. Per the precedent set in `005_project_extraction.sql`, widening an unnamed CHECK is what broke the DSQL deploys on PR #9 twice, and dropping one by its generated name is not something we want to rely on.

Three options:

| Option | Cost | Honesty |
| ------ | ---- | ------- |
| **A. Placeholder** — write `'work'` for every non-Microsoft account, document that the column is meaningless unless `provider = 'm365'` | Near zero; additive `provider_config_json` beside it | Poor: the data says something untrue |
| **B. New `mail_accounts` table**, migrate reads, leave `accounts` behind | High: every join in Inbox, triage, sync and projects | Good |
| **C. Drop NOT NULL, leave the CHECK** — `ALTER COLUMN ... DROP NOT NULL` is a column attribute, not a named-constraint drop | Unknown on DSQL; needs verification before it is chosen | Good, if it works |

**Recommendation: verify C, fall back to A.** C is the small honest change if DSQL accepts it, and we cannot find out from CI — there is no DSQL cluster there, only the grammar lints in `migrate/dsql_lint_test.go`. That verification is a prerequisite task, not an implementation detail, and it should happen against the dev cluster before this RFC is scheduled.

If we land on A, it needs a test asserting no non-`m365` code path ever reads the column, or the lie will eventually be believed.

### 5.2 Where send credentials live for IMAP

IMAP and SMTP are different servers with, usually, the same credentials — but not always, and they have separate hosts, ports and TLS modes. An account therefore has up to two transports.

Proposal: one `accounts` row, with `provider_config_json` holding both endpoint descriptions, and one credential envelope in `token_ciphertext` that can carry either an OAuth refresh token or a username/password pair. A tagged envelope, so the decoder cannot mistake one for the other.

### 5.3 How much to invest in Exchange

"Exchange" is three different things:

- **Exchange Online** — already covered by Graph. Nothing to do.
- **Exchange on-premises, modern** — IMAP/SMTP with OAuth or basic auth. Covered by the IMAP adapter, given basic auth support.
- **Exchange on-premises via EWS** — a distinct SOAP API, and the only way to get server-side forward and reliable change notifications on old deployments.

**Recommendation: treat EWS as a separate, later adapter** behind the same `Mailbox` port, and only if a real user needs it. The port is what makes that decision deferrable, which is the main argument for doing R1 before anything else.

### 5.4 Whether IMAP idle/push is in scope

No, initially. Polling on the existing scheduler tick is consistent with how Graph is synced today. IMAP IDLE needs a long-lived connection, which does not fit a Lambda worker; it would need a different execution model and should be argued separately.

---

## 6. Slices

| Slice | Name | Delivers |
| ----- | ---- | -------- |
| **R0** | DSQL constraint spike | Answers §5.1 against the dev cluster. Blocks everything else. |
| **R1** | Extract the port | `driven.Mailbox` + registry; Graph becomes one implementation; **no behaviour change**, no new provider |
| **R2** | Google | Gmail adapter, OAuth scopes, `historyId` cursor; sign-in and mail share a registration but not a code path |
| **R3** | IMAP/SMTP | Adapter, credential envelope, connection settings UI with a well-known-host table |
| **R4** | Capability-aware UI | Features a provider cannot do are visibly unavailable, not silently degraded |
| **R5** | EWS | Only on demand |

R1 is the whole bet. If the port is right, R2 and R3 are adapters and a settings form. If it is wrong, every slice after it pays for it.

---

## 7. Exit criteria

**R1.** Microsoft accounts behave exactly as before: same delta cursor, same forward path, same failure messages. `MicrosoftGraph` is gone from every application service. A contract test suite runs against the Graph adapter and a fake, and both pass the same assertions.

**R2.** A Gmail account syncs, categorises, files to projects, and resolves contacts using the same jobs as a Microsoft account, with no Gmail-specific branch above the adapter. Forwarding works via the MIME fallback and says so.

**R3.** An IMAP account can be added with host, port and TLS mode; a wrong password fails at connect time with a message naming the cause, not on the first sync.

**R4.** An operator can see, per account, which features are available, and a forward rule on an account without server-side forward warns before it is saved.

---

## 8. Testing

The contract suite is the deliverable that matters. Each adapter must pass the same assertions:

- A removal is reported as a removal, and never as a message with empty fields.
- A partial change record is rejected rather than written.
- A cursor survives a round trip and resumes where it left off.
- `GetRawMessage` round-trips a message with an attachment.
- `Forward` delivers, whether server-side or via the fallback.
- Refreshing credentials mid-sync does not lose the page in flight.

IMAP and Gmail need a recorded-fixture or containerised server in CI. The lesson from the DynamoDB job store — where the contract tests skip unless an endpoint is configured, and the store that runs in every deployment is the one with no coverage — applies directly. **A provider without CI coverage should not ship.**

---

## 9. Risks

| Risk | Mitigation |
| ---- | ---------- |
| The port is wrong and R2/R3 fight it | R1 ships with Graph only and no new provider, so the port is proven against the one implementation we understand before anything depends on it |
| Forwarded mail from non-Graph providers lands in spam | §4 capability flag plus an explicit warning; measure before enabling forward rules by default on those accounts |
| IMAP passwords are a higher-value secret than refresh tokens | Same vault, but they do not expire and cannot be scoped; consider requiring app passwords and refusing plaintext-auth servers |
| Attachment bytes now transit the worker | Bound the fallback by size and fail loudly above it rather than streaming unbounded data through a Lambda |
| `ms_account_kind` blocks the migration | R0 answers this before any code is written |
| Per-provider quirks leak upward over time | The capability struct is the pressure valve; anything that cannot be expressed as a capability is a signal the port is wrong |

---

## 10. Open questions

1. Does DSQL accept `ALTER COLUMN ... DROP NOT NULL`? (R0)
2. Is there a real user waiting on on-premises Exchange, or is it hypothetical? It changes whether EWS is worth designing for at all.
3. Should a Gmail account reuse the existing Google sign-in registration, or a separate OAuth client? Sharing couples consent screens and scope changes to login.
4. Do forward rules stay enabled by default on providers without server-side forward, or opt-in per account?
