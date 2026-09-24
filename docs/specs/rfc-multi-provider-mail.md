# RFC: Multi-Provider Mail

**Status:** Implemented (R1–R4) on `feat/project-renovation` — see §11 for where the build differs from the plan
**Related spec:** [Aurora DSQL](addendum-aurora-dsql.md) §3 (limits), [Redis/Asynq jobs](addendum-redis-asynq-jobs.md), [Project correspondence Wave 1](addendum-project-correspondence-wave1.md) §7 (assignment)
**Last updated:** 2026-09-24

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

Forward rules are an existing, automated feature. Silently giving them worse behaviour on a new provider is exactly the failure mode this codebase keeps getting bitten by: something that looks like it ran, did not do what the operator assumed, and said nothing.

**Decision: forward rules are supported on every provider.** Dropping them where the server cannot do the work would make a rule mean different things on different accounts, which is worse than the fallback. A provider without server-side forward implements it by fetching the original MIME and re-sending — so the feature is portable and the *implementation* is what varies, not the contract.

Capability declaration (§4) is therefore not a way to disable the feature. It exists so the difference is visible: the operator can see which accounts re-send rather than forward server-side, and the fallback can be bounded by size and fail loudly instead of streaming an unbounded attachment through a worker.

---

## 3. Non-goals

- **POP3.** No server-side state, no stable ids, no folders, no removal signal, and destructive retrieval by default. Everything downstream — delta sync, thread assignment, removal handling — has no meaning. If someone genuinely needs POP, it should be a one-way importer, not a mailbox.
- **EWS, and on-premises Exchange as a dedicated adapter.** §5.3 argues this out: the blocker is network reachability rather than protocol, and IMAP already covers most of the population.
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

### 5.3 Exchange: already supported, or out of reach

"Exchange" is three different things, and separating them dissolves most of the question:

- **Exchange Online** — the hosted mail in Microsoft 365. Already supported; it is what Graph talks to. Nothing to build.
- **Hybrid** — some mailboxes online, some on-premises, one identity. Graph reaches the online ones.
- **Exchange Server on-premises** — the self-hosted product. This is the only case that is actually missing.

For the on-premises case the access protocols are EWS (SOAP/XML, rich: server-side forward, folders, change notifications), IMAP/SMTP (usually available, sometimes disabled by the administrator), ActiveSync, or MAPI/RPC (proprietary, not realistic).

**Recommendation: do not build EWS.** Three reasons, in the order they matter:

1. **Reachability is the blocker, not the protocol.** An on-premises Exchange server is usually not exposed to the public internet. A cloud-hosted Automata cannot open a connection to it at all without a VPN, tunnel or on-premises relay. A correct EWS adapter does not help if the TCP connection never establishes, and that infrastructure is a far larger project than the adapter.
2. **IMAP subsumes most of it.** Most on-premises deployments can enable IMAP/SMTP, so they arrive through an adapter we want for everyone else anyway.
3. **EWS is on a deprecation path for the hosted case.** Microsoft has been retiring it for Exchange Online — the date should be confirmed, it is around now — so it is a SOAP API with NTLM/Kerberos and Autodiscover, serving a narrowing population.

If a real customer appears with an on-premises mailbox that IMAP cannot reach, EWS is a later adapter behind the same port and the network question gets answered first. The port is what keeps that deferrable.

### 5.4 Gmail and Google Workspace are the same mailbox

They are not two integrations. A Workspace account's mail **is** Gmail: same API, same endpoints, same scopes. One adapter serves both, and the RFC means both wherever it says Google.

What differs is consent and administration, which is where the cost actually sits:

| | Consumer Gmail | Google Workspace |
| --- | --- | --- |
| Connect flow | Per-user OAuth | Per-user OAuth, or domain-wide delegation via a service account |
| Who can block it | Nobody | The Workspace admin, through API access controls |
| App verification | Full public verification | An internal-use app in one domain can be trusted by its own admin |

**Gmail read scopes are restricted.** `gmail.readonly` and `gmail.modify` are restricted scopes: Google requires app verification, and for restricted scopes a periodic third-party security assessment when serving users outside your own domain. That is a schedulable prerequisite with a lead time and a price, not a code task — and it can gate launch long after the adapter works.

The OAuth consent screen has a **user type**, set per client, and it decides who can connect before any verification happens:

| | Internal | External, Testing | External, Production (verified) |
| --- | --- | --- | --- |
| Who can connect | Accounts in one Workspace domain | Up to 100 named test users — any Google account, personal Gmail included | Anyone |
| Verification required | No | No | Yes, plus the security assessment for restricted scopes |
| Unverified-app warning shown | No | Yes (click-through, same shape as Microsoft's) | No |
| Refresh token lifetime | Normal | **Expires after 7 days** | Normal |

**Decision: internal-use first.** R2's OAuth client is Internal, scoped to one Workspace domain whose administrator trusts it directly. That avoids the verification burden entirely and gives the stable, long-lived tokens a background sync product needs — the alternative, External-Testing, technically reaches personal Gmail today but re-issues a token every 7 days, which is a maintenance burden dressed up as a feature.

The consequence has to be stated plainly, because it is a product limit and not a technical one: **an Internal client can only connect mailboxes inside its own Workspace domain.** A personal `@gmail.com` address, or a client's mailbox in someone else's domain, cannot use it at all — not "will work with warnings," genuinely cannot. If connecting external mailboxes matters sooner than expected, verification becomes the critical path and should start in parallel with R1 rather than after R2. §5.4.1 below is a narrower stopgap for personal Gmail specifically, ahead of that.

**Verification cost, so it can be budgeted rather than discovered.** The review itself is free; the cost is the mandatory third-party security assessment for restricted scopes, tiered by Google on user count and scope risk:

| Tier | What it is | Rough cost |
| --- | --- | --- |
| Tier 1 | Self-assessment via an automated scanner | Often free to a few hundred dollars |
| Tier 2 | Manual review by an approved third party | Roughly low-to-mid hundreds to a few thousand |
| Tier 3 | Full penetration test | Tens of thousands |

A product this size requesting Gmail read/modify plausibly lands at Tier 2, not Tier 3, but Google assigns the tier — confirm before this becomes a budget line, and note it recurs (annual reassessment), not a one-time fee. Contrast: Microsoft's equivalent, Publisher Verification, is free and administrative — Microsoft Partner Network enrollment plus domain verification, no paid security review for standard `Mail.Read`/`Mail.Send` scopes. That asymmetry is why Microsoft's unverified-app experience already feels more permissive than Google's; it structurally is.

#### 5.4.1 App Passwords via the IMAP adapter: an immediate stopgap for personal Gmail

Gmail also speaks IMAP/SMTP (`imap.gmail.com:993`, `smtp.gmail.com:465`), which R3 already covers. This looks like a way to sidestep Google's OAuth verification question entirely, and for one specific credential type, it is.

Plain username/password auth is gone; two things can stand in:

- **An app password** — a 16-character, per-app credential generated in the account's security settings once 2FA is on. No OAuth consent screen, no Google review, no 7-day expiry: it is an ordinary IMAP/SMTP credential, indistinguishable to the adapter from Fastmail or Zoho. This is genuinely outside the verification question above.
- **OAuth2 over IMAP (XOAUTH2)** — Google also accepts an OAuth token as IMAP/SMTP credentials, but the scope it requires, `https://mail.google.com/`, is Google's full-access mail scope — broader than the granular `gmail.readonly`/`gmail.modify` the dedicated adapter would request. This path still hits verification, at the same tier or worse, so it buys nothing.

An app password is a real, if partial, answer: R3's generic adapter can connect a personal Gmail account today, with no dependency on R2's OAuth client or its Internal/domain limit. It is not a substitute for R2. It is a manual step the user performs outside the product (no "Connect with Google" button), and **a Workspace admin can disable app passwords org-wide** — plausibly the same admin whose trust R2's Internal client depends on — so it may not even be available for the deployment R2 targets. Treat it as covering the gap for personal accounts specifically, ahead of verification, not as an alternative sequencing for R2.

**Domain-wide delegation is a different connect flow**, not a variant of the OAuth one: a service account reads many mailboxes without per-user consent. Worth supporting eventually for org-wide deployments; out of scope for R2, which does per-user OAuth only.

### 5.5 Google mail connect is a separate flow from Google sign-in

They already are for Microsoft, and the reason is structural rather than cosmetic. Sign-in inserts OAuth state with **no user** (`flowAuthMicrosoft`, `flowAuthGoogle`, `userID = nil`) because nobody is logged in yet; it identifies exactly one person. Mailbox connect inserts state **bound to a user** (`oauthFlowM365Mail`, `&userID`) because someone already logged in is attaching a mailbox, and may attach several.

Google mail follows the same shape: its own flow constant (`google_mail`), its own OAuth client, its own scopes.

`driven.GoogleOAuth` stays as it is — it serves sign-in, returns a subject and an email, and has no mail scopes. The mail client is a new adapter that happens to talk to the same vendor. Sharing one registration would couple the consent screen and every future scope change to the login path, and would make "sign in with Google" ask for mailbox access, which is both worse for the user and harder to get verified.

**Decision: separate OAuth client, separate flow, multiple Google mailboxes per user** — the same arrangement Microsoft already has.

### 5.6 Whether IMAP idle/push is in scope

No, initially. Polling on the existing scheduler tick is consistent with how Graph is synced today. IMAP IDLE needs a long-lived connection, which does not fit a Lambda worker; it would need a different execution model and should be argued separately.

---

## 6. Slices

| Slice | Name | Delivers |
| ----- | ---- | -------- |
| **R0** | Two spikes, no code | §5.1 against the dev cluster (does DSQL accept `DROP NOT NULL`?) and §5.4's current Google verification requirement. Both have lead times and both can change the shape of what follows. |
| **R1** | Extract the port | `driven.Mailbox` + registry; Graph becomes one implementation; **no behaviour change**, no new provider |
| **R2** | Google | Gmail adapter, own OAuth client and `google_mail` flow, `historyId` cursor, forward via the MIME fallback. Internal-use in one Workspace domain; several Google mailboxes per user, as Microsoft already allows. |
| **R3** | IMAP/SMTP | Adapter, credential envelope, connection settings UI with a well-known-host table. Answers the on-premises Exchange question as a side effect. |
| **R4** | Capability-aware UI | Features a provider cannot do are visibly unavailable, not silently degraded |

Google leads because it is the largest addressable population after Microsoft 365 and the cleanest second implementation: OAuth we partly have, a real incremental cursor, and a REST API. IMAP follows because one adapter covers everything else — Fastmail, Zoho, hosted cPanel mail, Proton via bridge, and on-premises Exchange with IMAP enabled.

The order also derisks the port deliberately. Gmail stresses *different cursor, no server-side forward*; IMAP stresses *no OAuth, two transports, weak ids*. If `driven.Mailbox` survives both, it will survive EWS should anyone ever need it.

R1 is the whole bet. If the port is right, R2 and R3 are adapters and a settings form. If it is wrong, every slice after it pays for it.

---

## 7. Exit criteria

**R1.** Microsoft accounts behave exactly as before: same delta cursor, same forward path, same failure messages. `MicrosoftGraph` is gone from every application service. A contract test suite runs against the Graph adapter and a fake, and both pass the same assertions.

**R2.** A Google account syncs, categorises, files to projects, and resolves contacts using the same jobs as a Microsoft account, with no Gmail-specific branch above the adapter. Two Google mailboxes can be connected to one user alongside a Microsoft one, and signing in with Google does not connect a mailbox. A forward rule on a Google account delivers, with the re-send path visible to the operator.

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
| Internal-use Google means external mailboxes cannot connect via R2 at all | Stated as a product limit in §5.4, not discovered later; personal Gmail specifically has the app-password stopgap (§5.4.1); if other external mailboxes are needed sooner, verification starts in parallel with R1 |
| Forwarded mail is larger than the sending server will accept | Bound the fallback by size, fail the rule loudly, and surface it per account (§2) |
| Per-provider quirks leak upward over time | The capability struct is the pressure valve; anything that cannot be expressed as a capability is a signal the port is wrong |

---

## 10. Open questions

1. Does DSQL accept `ALTER COLUMN ... DROP NOT NULL`? (R0) — no longer blocking: the build took option A (§11), so this only matters if the placeholder is ever removed.
2. What does Google currently require for restricted Gmail scopes? Still open for the day R2 goes beyond internal use; internal-first (§5.4) does not need it.

Answered since the first draft, and left here so the reasoning is traceable:

- **On-premises Exchange** — not worth a dedicated adapter (§5.3).
- **Gmail OAuth registration** — separate client and flow from sign-in, so a user can attach several Google mailboxes (§5.5).
- **Forward rules without server-side forward** — supported everywhere via the MIME fallback, with the difference surfaced rather than the feature withdrawn (§2).
- **Size ceiling for the MIME fallback** — 25 MB of raw message, the same for every provider and just under Gmail's own send limit. Above it a forward is a permanent failure (`ErrMailTooLarge`): the rule's effect is recorded as rejected and not retried. An SMTP server's advertised `SIZE`, or a 552 at end of data, is treated the same way.

---

## 11. As built

What shipped, slice by slice, and where it departs from the sections above.

**R0 was not needed.** Both spikes existed to decide migrations; the build needed none.

- **`ms_account_kind`: option A.** Non-Microsoft rows carry `work` as a placeholder that nothing outside the Microsoft adapter reads (`placeholderMsAccountKind` in the accounts service). The API still returns the column; the UI shows it only for `m365` accounts.
- **No `google_mail` flow.** `oauth_states.flow` is also an unnamed CHECK over a fixed list — the same trap as PR #9 — so every mailbox connect reuses flow `m365_mail`, with the provider carried in the server-side state payload. The separation §5.5 cares about, sign-in versus mailbox connect, is still enforced by the flow; the provider in the payload was written by us, not supplied by the client.
- **No `provider_config_json` (§5.2).** The accounts table has no such column. IMAP server settings ride in the encrypted credential next to the password, which is the only place they are read. Credentials are disambiguated by the `provider` column (which has no CHECK) and, for Google and IMAP, a `type` tag inside the blob; the Microsoft blob keeps its original untagged shape so existing rows decode unchanged.

**R1.** `driven.Mailbox` with declared capabilities, `driven.MailProvider` to open one from a stored credential, and `appaccounts.MailboxOpener` as the single place that decrypts, picks the adapter, persists rotated credentials, and marks an account `expired` when the provider rejects its credentials outright (`ErrCredentialsRejected`). Services never see a token. The shared contract suite (`mailboxtest`) runs against every adapter; since R3 it also checks that resuming with no new mail returns nothing.

**R2.** Gmail over the REST API: `format=full` to sync, `format=raw` only to reply or forward, history replay for increments with a 404 reported as an expired cursor, forward as the original attached `message/rfc822`. Enabled only when `GOOGLE_MAIL_CLIENT_ID` and its secret are configured; otherwise Google is simply not offered.

**R3.** One IMAP/SMTP adapter, always available. Implicit TLS or STARTTLS only. Both servers are logged into before anything is stored, and failures come back as a 422 naming the cause (`driven.ConnectRejectedError`). Sync walks INBOX by UID under a `UIDVALIDITY`-scoped cursor; a changed `UIDVALIDITY` is an expired cursor. Message ids are the `Message-ID` header, which survives renumbering, with a UID fallback for mail that has none. Removals are not reported (`ReportsRemovals: false`). Every operation dials, works and logs out; nothing is held open across a worker chunk. Tests run the contract against real go-imap and go-smtp servers over TLS in-process, so this provider has CI coverage without a container.

**R4.** Providers declare capabilities without opening a mailbox. `GET /api/accounts/providers` tells the UI what this deployment can connect; the account list carries each account's capabilities. The connect dialog starts from a provider choice, the IMAP form fills servers from well-known presets, expired accounts reconnect through their own provider's flow (the server restores the existing account rather than adding one), and a forward rule on an account without server-side forward warns and asks before it is saved.

**Not built, as planned:** EWS (§5.3), POP3, IMAP IDLE (§5.6), outbound-only accounts. Sent-folder copies for IMAP are also not made: a forward or reply submitted over SMTP appears in the provider's Sent folder only where the provider does that itself (Gmail does; most others do not).

