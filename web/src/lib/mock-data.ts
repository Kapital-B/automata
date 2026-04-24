// Mock data shaped to the spec's data model. Used everywhere until backend is wired.

export type AccountKind = "work" | "personal";
export type ConnectionStatus = "connected" | "expired" | "error";

export interface Account {
  id: string;
  label: string;
  primaryEmail: string;
  kind: AccountKind;
  status: ConnectionStatus;
  lastSyncedAt: string;
  colorVar: "acct-1" | "acct-2" | "acct-3" | "acct-4";
}

export type Category = "important" | "finance" | "newsletter" | "spam" | "personal" | "other";

export interface Message {
  id: string;
  accountId: string;
  from: { name: string; address: string };
  subject: string;
  preview: string;
  body: string;
  receivedAt: string;
  category: Category;
  unread: boolean;
  hasAttachments?: boolean;
  needsReply?: boolean;
}

export interface ActionItem {
  id: string;
  accountId: string;
  messageId: string;
  text: string;
  due?: string;
}

export interface FYIItem {
  id: string;
  accountId: string;
  messageId: string;
  text: string;
}

export interface SummarySnapshot {
  generatedAt: string;
  windowStart: string;
  windowEnd: string;
  model: string;
  runId: string;
  actionItems: ActionItem[];
  fyi: FYIItem[];
}

export interface ForwardRule {
  id: string;
  accountId: string;
  name: string;
  enabled: boolean;
  conditionSummary: string;
  forwardTo: string;
  lastRun?: string;
  matchesLast7d: number;
}

export interface Draft {
  id: string;
  accountId: string;
  messageId: string;
  toName: string;
  toEmail: string;
  subject: string;
  body: string;
  generatedAt: string;
  model: string;
  status: "ready" | "edited" | "sent";
}

export interface JobRun {
  id: string;
  accountId: string | null;
  jobType: "sync" | "summarize" | "categorize" | "forward_rules" | "draft_suggest";
  trigger: "schedule" | "api";
  status: "success" | "failed" | "running";
  startedAt: string;
  finishedAt?: string;
  meta?: Record<string, string | number>;
}

export const accounts: Account[] = [
  {
    id: "acc-work",
    label: "Work",
    primaryEmail: "elena.park@northwind.com",
    kind: "work",
    status: "connected",
    lastSyncedAt: new Date(Date.now() - 3 * 60 * 1000).toISOString(),
    colorVar: "acct-1",
  },
  {
    id: "acc-personal",
    label: "Personal",
    primaryEmail: "elena.park@outlook.com",
    kind: "personal",
    status: "connected",
    lastSyncedAt: new Date(Date.now() - 11 * 60 * 1000).toISOString(),
    colorVar: "acct-2",
  },
  {
    id: "acc-side",
    label: "Side project",
    primaryEmail: "hello@parkstudio.co",
    kind: "personal",
    status: "expired",
    lastSyncedAt: new Date(Date.now() - 26 * 60 * 60 * 1000).toISOString(),
    colorVar: "acct-3",
  },
];

const now = Date.now();
const h = (n: number) => new Date(now - n * 3600 * 1000).toISOString();

export const messages: Message[] = [
  {
    id: "m1", accountId: "acc-work",
    from: { name: "Priya Natarajan", address: "priya@northwind.com" },
    subject: "Q2 roadmap review — your input needed by Friday",
    preview: "Hi Elena — pulling the roadmap doc together for Monday's exec sync. Could you add your team's…",
    body: "Hi Elena — pulling the roadmap doc together for Monday's exec sync. Could you add your team's three priorities and any risks to the shared deck by EOD Friday? Happy to jump on a 15 min call if easier.\n\nThanks,\nPriya",
    receivedAt: h(0.4), category: "important", unread: true, needsReply: true,
  },
  {
    id: "m2", accountId: "acc-work",
    from: { name: "Stripe", address: "receipts@stripe.com" },
    subject: "Your March invoice is available — $4,218.00",
    preview: "Your monthly invoice for March 2026 is now available in your Stripe dashboard…",
    body: "Your monthly invoice for March 2026 is now available. Total charged: $4,218.00.",
    receivedAt: h(2), category: "finance", unread: true,
  },
  {
    id: "m3", accountId: "acc-personal",
    from: { name: "Marco Levi", address: "marco.l@gmail.com" },
    subject: "Dinner Saturday?",
    preview: "Hey! A few of us are doing the new Sicilian place at 8pm Saturday. You in?",
    body: "Hey! A few of us are doing the new Sicilian place at 8pm Saturday. You in? Need to confirm by tomorrow for the reservation.",
    receivedAt: h(5), category: "personal", unread: true, needsReply: true,
  },
  {
    id: "m4", accountId: "acc-personal",
    from: { name: "Chase", address: "alerts@chase.com" },
    subject: "Statement available — checking •• 4421",
    preview: "Your March statement for account ending in 4421 is now available…",
    body: "Your March statement is available. Closing balance: $12,940.21.",
    receivedAt: h(7), category: "finance", unread: false,
  },
  {
    id: "m5", accountId: "acc-work",
    from: { name: "GitHub", address: "noreply@github.com" },
    subject: "[northwind/api] PR #4821 needs your review",
    preview: "Aakash requested your review on the new auth refactor.",
    body: "Aakash requested your review on the new auth refactor. CI is green.",
    receivedAt: h(9), category: "important", unread: true, needsReply: true,
  },
  {
    id: "m6", accountId: "acc-work",
    from: { name: "Notion", address: "team@notion.so" },
    subject: "Weekly digest — 14 pages updated in your workspace",
    preview: "Your team made 14 updates this week. Top contributors…",
    body: "Your team made 14 updates this week.",
    receivedAt: h(14), category: "newsletter", unread: false,
  },
  {
    id: "m7", accountId: "acc-personal",
    from: { name: "Verge", address: "newsletter@theverge.com" },
    subject: "The Vergecast: AI agents are here, kind of",
    preview: "This week we talk about the new wave of consumer AI agents…",
    body: "This week we talk about consumer AI agents.",
    receivedAt: h(18), category: "newsletter", unread: false,
  },
  {
    id: "m8", accountId: "acc-work",
    from: { name: "Daniela Rossi", address: "daniela@northwind.com" },
    subject: "Re: Vendor contract — clauses 4 & 7",
    preview: "Legal flagged two changes in the latest redline. See attached.",
    body: "Legal flagged two changes in the latest redline. See attached. Need your sign-off before we counter.",
    receivedAt: h(22), category: "important", unread: false, hasAttachments: true, needsReply: true,
  },
];

export const summary: SummarySnapshot = {
  generatedAt: new Date(now - 12 * 60 * 1000).toISOString(),
  windowStart: h(24),
  windowEnd: new Date(now).toISOString(),
  model: "llama-3.1-8b-instruct (LM Studio)",
  runId: "run_2026-04-24_0612",
  actionItems: [
    { id: "a1", accountId: "acc-work", messageId: "m1", text: "Add your team's three Q2 priorities to the exec deck.", due: "Fri" },
    { id: "a2", accountId: "acc-work", messageId: "m5", text: "Review GitHub PR #4821 (auth refactor).", due: "Today" },
    { id: "a3", accountId: "acc-work", messageId: "m8", text: "Sign off on vendor contract clauses 4 & 7.", due: "Tomorrow" },
    { id: "a4", accountId: "acc-personal", messageId: "m3", text: "Confirm dinner reservation for Saturday 8pm.", due: "Tomorrow" },
  ],
  fyi: [
    { id: "f1", accountId: "acc-work", messageId: "m2", text: "March Stripe invoice posted — $4,218.00." },
    { id: "f2", accountId: "acc-personal", messageId: "m4", text: "Chase March statement ready (•• 4421)." },
    { id: "f3", accountId: "acc-work", messageId: "m6", text: "Notion workspace activity up 22% week over week." },
  ],
};

export const rules: ForwardRule[] = [
  {
    id: "r1", accountId: "acc-work", name: "Forward invoices to bookkeeper",
    enabled: true,
    conditionSummary: "From contains 'invoice' OR subject matches /receipt|invoice/i",
    forwardTo: "books@parkstudio.co",
    lastRun: h(3), matchesLast7d: 11,
  },
  {
    id: "r2", accountId: "acc-personal", name: "Send shipping updates to family inbox",
    enabled: true,
    conditionSummary: "From in {amazon.com, ups.com, fedex.com}",
    forwardTo: "household@parkstudio.co",
    lastRun: h(20), matchesLast7d: 4,
  },
  {
    id: "r3", accountId: "acc-work", name: "Forward calendar invites (paused)",
    enabled: false,
    conditionSummary: "Has .ics attachment AND from internal domain",
    forwardTo: "elena.park@outlook.com",
    matchesLast7d: 0,
  },
];

export const allowlist = [
  "books@parkstudio.co",
  "household@parkstudio.co",
  "elena.park@outlook.com",
  "archive@parkstudio.co",
];

export const drafts: Draft[] = [
  {
    id: "d1", accountId: "acc-work", messageId: "m1",
    toName: "Priya Natarajan", toEmail: "priya@northwind.com",
    subject: "Re: Q2 roadmap review — your input needed by Friday",
    body: "Hi Priya,\n\nThanks for the heads up. I'll get our three Q2 priorities and the top two risks into the deck by Thursday EOD so you have a buffer day. No call needed — I'll flag in the doc if anything is ambiguous.\n\nElena",
    generatedAt: h(0.2), model: "llama-3.1-8b-instruct", status: "ready",
  },
  {
    id: "d2", accountId: "acc-personal", messageId: "m3",
    toName: "Marco Levi", toEmail: "marco.l@gmail.com",
    subject: "Re: Dinner Saturday?",
    body: "Marco — I'm in for Saturday at 8. Want me to handle the wine?\n\nElena",
    generatedAt: h(0.3), model: "llama-3.1-8b-instruct", status: "ready",
  },
  {
    id: "d3", accountId: "acc-work", messageId: "m8",
    toName: "Daniela Rossi", toEmail: "daniela@northwind.com",
    subject: "Re: Vendor contract — clauses 4 & 7",
    body: "Daniela,\n\nLooked at the redline. I'm fine with clause 4 as edited; for clause 7 I'd like to keep the 30-day cure period rather than 14. Happy to jump on a quick call to align before we counter.\n\nElena",
    generatedAt: h(0.5), model: "llama-3.1-8b-instruct", status: "edited",
  },
];

export const runs: JobRun[] = [
  { id: "j1", accountId: "acc-work", jobType: "sync", trigger: "schedule", status: "success", startedAt: h(0.1), finishedAt: h(0.05), meta: { fetched: 12, model: "—" } },
  { id: "j2", accountId: null, jobType: "summarize", trigger: "api", status: "success", startedAt: h(0.25), finishedAt: h(0.2), meta: { accounts: 2, items: 7 } },
  { id: "j3", accountId: "acc-personal", jobType: "categorize", trigger: "schedule", status: "success", startedAt: h(0.4), finishedAt: h(0.38), meta: { messages: 8 } },
  { id: "j4", accountId: "acc-work", jobType: "forward_rules", trigger: "schedule", status: "success", startedAt: h(3), finishedAt: h(2.99), meta: { matched: 1, forwarded: 1 } },
  { id: "j5", accountId: "acc-side", jobType: "sync", trigger: "schedule", status: "failed", startedAt: h(6), finishedAt: h(5.99), meta: { error: "token expired" } },
  { id: "j6", accountId: "acc-personal", jobType: "sync", trigger: "schedule", status: "success", startedAt: h(11), finishedAt: h(10.98), meta: { fetched: 4 } },
  { id: "j7", accountId: "acc-work", jobType: "draft_suggest", trigger: "schedule", status: "success", startedAt: h(0.2), finishedAt: h(0.18), meta: { drafts: 2 } },
];

export function getAccount(id: string | null | undefined): Account | undefined {
  if (!id) return undefined;
  return accounts.find((a) => a.id === id);
}

export function getMessage(id: string): Message | undefined {
  return messages.find((m) => m.id === id);
}

export function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.round(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const hr = Math.round(m / 60);
  if (hr < 24) return `${hr}h ago`;
  const d = Math.round(hr / 24);
  return `${d}d ago`;
}
