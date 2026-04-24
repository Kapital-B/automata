import { useMemo, useState } from "react";
import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { CategoryPill } from "@/components/CategoryPill";
import { messages, getAccount, relativeTime, Category } from "@/lib/mock-data";
import { Paperclip, Reply, Forward, Archive } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import type { AccountFilter } from "@/components/AppShell";

interface Props {
  accountFilter: AccountFilter;
}

const categoryFilters: (Category | "all")[] = [
  "all", "important", "finance", "personal", "newsletter", "spam",
];

export default function InboxPage({ accountFilter }: Props) {
  const [cat, setCat] = useState<Category | "all">("all");
  const [selectedId, setSelectedId] = useState<string>(messages[0].id);

  const filtered = useMemo(
    () =>
      messages.filter(
        (m) =>
          (accountFilter === "all" || m.accountId === accountFilter) &&
          (cat === "all" || m.category === cat)
      ),
    [accountFilter, cat]
  );

  const selected = messages.find((m) => m.id === selectedId) ?? filtered[0];
  const selAccount = selected ? getAccount(selected.accountId) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Mailbox"
        title="Inbox"
        description="Per-account messages with LLM-assigned categories. Provenance preserved on every row."
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        {categoryFilters.map((c) => (
          <button
            key={c}
            onClick={() => setCat(c)}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition",
              cat === c
                ? "border-foreground bg-foreground text-background"
                : "border-border bg-card text-muted-foreground hover:border-foreground/40 hover:text-foreground"
            )}
          >
            {c}
          </button>
        ))}
        <span className="ml-auto text-xs text-muted-foreground">
          {filtered.length} {filtered.length === 1 ? "message" : "messages"}
        </span>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        {/* List */}
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {filtered.map((m) => {
            const acct = getAccount(m.accountId);
            const isSel = selected?.id === m.id;
            return (
              <li
                key={m.id}
                onClick={() => setSelectedId(m.id)}
                className={cn(
                  "cursor-pointer px-4 py-3 transition",
                  isSel ? "bg-secondary/70" : "hover:bg-secondary/40"
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <AccountBadge account={acct} />
                  <span className="text-[11px] text-muted-foreground">
                    {relativeTime(m.receivedAt)}
                  </span>
                </div>
                <p
                  className={cn(
                    "mt-1.5 truncate text-sm",
                    m.unread ? "font-semibold text-foreground" : "text-foreground/90"
                  )}
                >
                  {m.from.name}
                </p>
                <p className="truncate text-sm text-foreground/85">{m.subject}</p>
                <p className="mt-0.5 truncate text-xs text-muted-foreground">{m.preview}</p>
                <div className="mt-2 flex items-center gap-2">
                  <CategoryPill category={m.category} />
                  {m.hasAttachments && (
                    <Paperclip className="h-3 w-3 text-muted-foreground" />
                  )}
                  {m.needsReply && (
                    <span className="text-[10px] font-medium uppercase tracking-wider text-accent">
                      reply suggested
                    </span>
                  )}
                </div>
              </li>
            );
          })}
        </ul>

        {/* Detail */}
        {selected && (
          <article className="surface-card flex flex-col">
            <div className="flex items-center justify-between border-b border-border/70 px-5 py-3">
              <AccountBadge account={selAccount} showEmail />
              <div className="flex items-center gap-1">
                <Button size="sm" variant="ghost"><Reply className="mr-1.5 h-3.5 w-3.5" />Reply</Button>
                <Button size="sm" variant="ghost"><Forward className="mr-1.5 h-3.5 w-3.5" />Forward</Button>
                <Button size="sm" variant="ghost"><Archive className="h-3.5 w-3.5" /></Button>
              </div>
            </div>
            <div className="px-6 py-5">
              <div className="flex items-start justify-between gap-3">
                <h2 className="font-display text-2xl font-medium leading-snug">
                  {selected.subject}
                </h2>
                <CategoryPill category={selected.category} />
              </div>
              <p className="mt-2 text-sm text-muted-foreground">
                <span className="font-medium text-foreground/80">{selected.from.name}</span>{" "}
                &lt;{selected.from.address}&gt; · {relativeTime(selected.receivedAt)}
              </p>
            </div>
            <div className="hairline" />
            <div className="prose prose-sm max-w-none whitespace-pre-wrap px-6 py-5 text-foreground/90">
              {selected.body}
            </div>
            {selected.needsReply && (
              <div className="border-t border-border/70 bg-secondary/40 px-6 py-4">
                <p className="text-[10px] font-mono uppercase tracking-widest text-muted-foreground">
                  AI suggestion
                </p>
                <p className="mt-1 text-sm">A draft reply is waiting in the Drafts area.</p>
                <Button size="sm" className="mt-3 bg-foreground text-background hover:bg-foreground/90">
                  Open draft
                </Button>
              </div>
            )}
          </article>
        )}
      </div>
    </div>
  );
}
