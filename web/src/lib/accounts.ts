import type { MailAccount } from "@/lib/auth";

export type AccountColorVar = "acct-1" | "acct-2" | "acct-3" | "acct-4";

const colorVars: AccountColorVar[] = ["acct-1", "acct-2", "acct-3", "acct-4"];

export type UiAccount = {
  id: string;
  label: string;
  primaryEmail: string;
  kind: "work" | "personal" | "common";
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
    kind: row.ms_account_kind,
    status: row.connection_status,
    lastSyncedAt: row.last_synced_at,
    lastError: row.last_error,
    colorVar: colorVars[idx % colorVars.length],
  }));
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
