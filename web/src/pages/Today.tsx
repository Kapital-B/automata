import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { summary, accounts, getAccount, getMessage, relativeTime } from "@/lib/mock-data";
import { ArrowUpRight, RefreshCw, CheckCircle2, Info } from "lucide-react";
import { Link } from "react-router-dom";
import type { AccountFilter } from "@/components/AppShell";

interface Props {
  accountFilter: AccountFilter;
}

export default function TodayPage({ accountFilter }: Props) {
  const filterFn = <T extends { accountId: string }>(arr: T[]) =>
    accountFilter === "all" ? arr : arr.filter((x) => x.accountId === accountFilter);

  const actionItems = filterFn(summary.actionItems);
  const fyi = filterFn(summary.fyi);

  // Group action items by account for clear provenance
  const grouped = accounts
    .map((a) => ({
      account: a,
      items: actionItems.filter((i) => i.accountId === a.id),
    }))
    .filter((g) => g.items.length > 0);

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow={`Daily summary · ${new Date(summary.generatedAt).toLocaleDateString(undefined, {
          weekday: "long",
          month: "long",
          day: "numeric",
        })}`}
        title="What needs your attention today."
        description={`Generated ${relativeTime(summary.generatedAt)} from ${
          accountFilter === "all" ? `${accounts.length} accounts` : getAccount(accountFilter)?.label
        } · model: ${summary.model}`}
        actions={
          <>
            <Button variant="outline" size="sm">
              <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" /> Mark all reviewed
            </Button>
            <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90">
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" /> Refresh summary
            </Button>
          </>
        }
      />

      {/* Stats strip */}
      <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {[
          { label: "Action items", value: actionItems.length, tone: "text-foreground" },
          { label: "FYI", value: fyi.length, tone: "text-foreground" },
          { label: "Drafts ready", value: 3, tone: "text-foreground" },
          { label: "Run id", value: summary.runId.split("_").pop(), tone: "font-mono text-sm text-muted-foreground" },
        ].map((s) => (
          <div key={s.label} className="surface-card px-4 py-3">
            <p className="text-[10px] uppercase tracking-widest text-muted-foreground">
              {s.label}
            </p>
            <p className={`mt-1 font-display text-2xl font-medium ${s.tone}`}>{s.value}</p>
          </div>
        ))}
      </section>

      {/* Action items, grouped by account so provenance is unambiguous */}
      <section className="space-y-6">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">Action items</h2>
          <p className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            {actionItems.length} open
          </p>
        </div>

        {grouped.length === 0 ? (
          <div className="surface-card flex items-center gap-3 px-4 py-6 text-muted-foreground">
            <CheckCircle2 className="h-5 w-5 text-success" />
            Nothing pending. Inbox zero, summary-zero.
          </div>
        ) : (
          grouped.map(({ account, items }) => (
            <div key={account.id} className="space-y-2">
              <AccountBadge account={account} showEmail size="sm" />
              <ul className="surface-card divide-y divide-border/70 overflow-hidden">
                {items.map((item) => {
                  const msg = getMessage(item.messageId);
                  return (
                    <li key={item.id} className="group flex items-start gap-4 px-5 py-4 transition hover:bg-secondary/40">
                      <div className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-foreground/70" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm leading-snug text-foreground">{item.text}</p>
                        {msg && (
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            From {msg.from.name} · "{msg.subject}"
                          </p>
                        )}
                      </div>
                      {item.due && (
                        <span className="rounded-full border border-border bg-background px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-foreground/80">
                          {item.due}
                        </span>
                      )}
                      <Link
                        to="/inbox"
                        className="opacity-0 transition group-hover:opacity-100"
                        aria-label="Open message"
                      >
                        <ArrowUpRight className="h-4 w-4 text-muted-foreground" />
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))
        )}
      </section>

      {/* FYI */}
      <section className="space-y-4">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">For your awareness</h2>
          <p className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            {fyi.length} items
          </p>
        </div>
        <ul className="grid gap-3 md:grid-cols-2">
          {fyi.map((item) => (
            <li key={item.id} className="surface-card flex items-start gap-3 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="text-sm leading-snug">{item.text}</p>
                <div className="mt-2">
                  <AccountBadge account={getAccount(item.accountId)} />
                </div>
              </div>
            </li>
          ))}
        </ul>
      </section>

      <footer className="hairline pt-6 text-xs text-muted-foreground">
        Window: {new Date(summary.windowStart).toLocaleString()} → {new Date(summary.windowEnd).toLocaleString()}
        <span className="mx-2">·</span>
        Run <span className="font-mono">{summary.runId}</span>
      </footer>
    </div>
  );
}
