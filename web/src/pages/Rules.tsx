import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { rules, allowlist, getAccount, relativeTime } from "@/lib/mock-data";
import { Plus, ShieldCheck, X } from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";

interface Props {
  accountFilter: AccountFilter;
}

export default function RulesPage({ accountFilter }: Props) {
  const visible = rules.filter(
    (r) => accountFilter === "all" || r.accountId === accountFilter
  );

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Automation"
        title="Forwarding rules"
        description="Rules can only forward to addresses on your allowlist. Each execution is recorded against the originating account."
        actions={
          <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90">
            <Plus className="mr-1.5 h-3.5 w-3.5" /> New rule
          </Button>
        }
      />

      <ul className="space-y-3">
        {visible.map((r) => (
          <li key={r.id} className="surface-card p-5">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <h3 className="font-display text-lg font-medium">{r.name}</h3>
                  {!r.enabled && (
                    <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                      paused
                    </span>
                  )}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                  <AccountBadge account={getAccount(r.accountId)} />
                  <span>·</span>
                  <span>
                    Forwards to{" "}
                    <span className="font-mono text-foreground/80">{r.forwardTo}</span>
                  </span>
                </div>
                <div className="mt-3 rounded-md bg-secondary/60 px-3 py-2 font-mono text-xs text-foreground/80">
                  {r.conditionSummary}
                </div>
                <p className="mt-3 text-xs text-muted-foreground">
                  {r.matchesLast7d} matches in the last 7 days
                  {r.lastRun && <> · last run {relativeTime(r.lastRun)}</>}
                </p>
              </div>
              <Switch checked={r.enabled} />
            </div>
          </li>
        ))}
      </ul>

      <section className="space-y-4">
        <div className="flex items-end justify-between">
          <div>
            <h2 className="font-display text-2xl">Allowlist</h2>
            <p className="text-sm text-muted-foreground">
              Forwarding destinations are restricted to these addresses.
            </p>
          </div>
          <Button size="sm" variant="outline">
            <ShieldCheck className="mr-1.5 h-3.5 w-3.5" /> Add address
          </Button>
        </div>
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {allowlist.map((email) => (
            <li key={email} className="flex items-center justify-between px-4 py-3">
              <span className="font-mono text-sm">{email}</span>
              <button className="text-muted-foreground transition hover:text-destructive">
                <X className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
