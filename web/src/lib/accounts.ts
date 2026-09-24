import type { MailAccount, MailCapabilities } from "@/lib/auth";

export type AccountColorVar = "acct-1" | "acct-2" | "acct-3" | "acct-4";

const colorVars: AccountColorVar[] = ["acct-1", "acct-2", "acct-3", "acct-4"];

export type UiAccount = {
  id: string;
  label: string;
  primaryEmail: string;
  provider: string;
  /** Only meaningful when provider is m365. */
  kind: "work" | "personal" | "common";
  capabilities?: MailCapabilities;
  status: "connected" | "error" | "expired";
  lastSyncedAt?: string;
  lastError?: string;
  colorVar: AccountColorVar;
};

export function mapAccountsForUi(rows: MailAccount[]): UiAccount[] {
  return rows.map((row, idx) => ({
    id: row.id,
    label: row.label || row.primary_email || `Account ${idx + 1}`,
    primaryEmail: row.primary_email,
    provider: row.provider || "m365",
    kind: row.ms_account_kind,
    capabilities: row.capabilities,
    status: row.connection_status,
    lastSyncedAt: row.last_synced_at,
    lastError: row.last_error,
    colorVar: colorVars[idx % colorVars.length],
  }));
}

export function providerLabel(provider: string): string {
  switch (provider) {
    case "m365":
      return "Microsoft";
    case "google":
      return "Google";
    case "imap":
      return "IMAP";
    default:
      return provider;
  }
}

/** The plain-language limits of an account's provider, for the account card. */
export function capabilityNotes(caps?: MailCapabilities): string[] {
  if (!caps) return [];
  const notes: string[] = [];
  if (!caps.server_side_forward) {
    notes.push("Forwards are sent as a new message with the original attached, from this mailbox.");
  }
  if (!caps.reports_removals) {
    notes.push("Mail deleted or moved out of the inbox on the server stays listed here.");
  }
  return notes;
}

export function relativeTime(iso?: string): string {
  if (!iso) {
    return "never";
  }
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}
