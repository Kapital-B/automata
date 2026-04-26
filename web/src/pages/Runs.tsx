import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { listRuns } from "@/lib/auth";
import { relativeTime } from "@/lib/accounts";
import { CheckCircle2, XCircle, Clock, Ban } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useQuery } from "@tanstack/react-query";

interface Props {
  accountFilter: AccountFilter;
}

const statusIcon = {
  success: <CheckCircle2 className="h-3.5 w-3.5 text-success" />,
  failed: <XCircle className="h-3.5 w-3.5 text-destructive" />,
  running: <Clock className="h-3.5 w-3.5 text-accent" />,
  pending: <Clock className="h-3.5 w-3.5 text-muted-foreground" />,
  cancelled: <Ban className="h-3.5 w-3.5 text-muted-foreground" />,
};

export default function RunsPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const runsQuery = useQuery({
    queryKey: ["runs", accessToken, accountFilter],
    queryFn: () =>
      listRuns(accessToken!, {
        accountId: accountFilter === "all" ? undefined : accountFilter,
      }),
    enabled: Boolean(accessToken),
    refetchInterval: (query) => {
      const rows = (query.state.data ?? []) as Array<{ status?: string }>;
      return rows.some((r) => r.status === "pending" || r.status === "running") ? 2000 : false;
    },
  });
  const visible =
    runsQuery.data?.filter(
      (r) => accountFilter === "all" || r.account_id === accountFilter || !r.account_id,
    ) ?? [];

  const getAccount = (accountID?: string) => accounts.find((account) => account.id === accountID);

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Audit"
        title="Job runs"
        description="Every sync, summarize, categorize, forward, and draft pipeline records what it did, against which account, and when."
      />

      {runsQuery.isLoading && (
        <div className="surface-card p-4 text-sm text-muted-foreground">Loading runs...</div>
      )}
      {runsQuery.isError && (
        <div className="surface-card p-4 text-sm text-destructive">
          Could not load runs: {runsQuery.error instanceof Error ? runsQuery.error.message : "unknown error"}
        </div>
      )}
      {!runsQuery.isLoading && !runsQuery.isError && visible.length === 0 && (
        <div className="surface-card p-4 text-sm text-muted-foreground">No job runs yet.</div>
      )}

      <div className="surface-card overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-secondary/60 text-left text-xs uppercase tracking-wider text-muted-foreground">
            <tr>
              <th className="px-4 py-2.5 font-medium">Job</th>
              <th className="px-4 py-2.5 font-medium">Account</th>
              <th className="px-4 py-2.5 font-medium">Trigger</th>
              <th className="px-4 py-2.5 font-medium">Status</th>
              <th className="px-4 py-2.5 font-medium">Started</th>
              <th className="px-4 py-2.5 font-medium">Result</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/70">
            {visible.map((r) => (
              <tr key={r.id} className="hover:bg-secondary/40">
                <td className="px-4 py-3">
                  <span className="font-mono text-xs text-foreground/85">{r.job_type}</span>
                </td>
                <td className="px-4 py-3">
                  <AccountBadge account={getAccount(r.account_id)} />
                </td>
                <td className="px-4 py-3 text-xs text-muted-foreground">{r.trigger}</td>
                <td className="px-4 py-3">
                  <span
                    className={cn(
                      "inline-flex items-center gap-1.5 text-xs font-medium",
                      r.status === "success" && "text-success",
                      r.status === "failed" && "text-destructive",
                      r.status === "running" && "text-accent"
                    )}
                  >
                    {statusIcon[r.status] ?? <Clock className="h-3.5 w-3.5 text-muted-foreground" />}
                    {r.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-xs text-muted-foreground">
                  {relativeTime(r.started_at)}
                </td>
                <td className="px-4 py-3">
                  <span className="font-mono text-[11px] text-muted-foreground">
                    {renderRunResult(r.meta_json)}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function renderRunResult(meta: Record<string, unknown>) {
  const draftsGenerated = typeof meta.drafts_generated === "number" ? meta.drafts_generated : undefined;
  const actionItemsSeen = typeof meta.action_items_seen === "number" ? meta.action_items_seen : undefined;
  if (typeof draftsGenerated === "number") {
    const extra = typeof actionItemsSeen === "number" ? ` · ${actionItemsSeen} seen` : "";
    return `${draftsGenerated} drafts generated${extra}`;
  }
  if (typeof meta.chain_started_at === "string") {
    return `chain started ${relativeTime(meta.chain_started_at)}`;
  }
  const processed = typeof meta.processed_messages === "number" ? meta.processed_messages : undefined;
  const total = typeof meta.total_messages === "number" ? meta.total_messages : undefined;
  if (typeof processed === "number" && typeof total === "number" && total > 0) {
    return `${processed}/${total} processed`;
  }
  const entries = Object.entries(meta ?? {});
  if (entries.length === 0) return "—";
  return entries.map(([k, v]) => `${k}=${v}`).join(" · ");
}
