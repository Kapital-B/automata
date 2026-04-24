import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { runs, getAccount, relativeTime } from "@/lib/mock-data";
import { CheckCircle2, XCircle, Clock } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AccountFilter } from "@/components/AppShell";

interface Props {
  accountFilter: AccountFilter;
}

const statusIcon = {
  success: <CheckCircle2 className="h-3.5 w-3.5 text-success" />,
  failed: <XCircle className="h-3.5 w-3.5 text-destructive" />,
  running: <Clock className="h-3.5 w-3.5 text-accent" />,
};

export default function RunsPage({ accountFilter }: Props) {
  const visible = runs.filter(
    (r) => accountFilter === "all" || r.accountId === accountFilter || r.accountId === null
  );

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Audit"
        title="Job runs"
        description="Every sync, summarize, categorize, forward, and draft pipeline records what it did, against which account, and when."
      />

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
                  <span className="font-mono text-xs text-foreground/85">{r.jobType}</span>
                </td>
                <td className="px-4 py-3">
                  <AccountBadge account={getAccount(r.accountId ?? "")} />
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
                    {statusIcon[r.status]}
                    {r.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-xs text-muted-foreground">
                  {relativeTime(r.startedAt)}
                </td>
                <td className="px-4 py-3">
                  <span className="font-mono text-[11px] text-muted-foreground">
                    {r.meta
                      ? Object.entries(r.meta)
                          .map(([k, v]) => `${k}=${v}`)
                          .join(" · ")
                      : "—"}
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
