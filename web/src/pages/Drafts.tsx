import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { drafts, getAccount, relativeTime } from "@/lib/mock-data";
import { Send, Trash2, Sparkles } from "lucide-react";
import { useState } from "react";
import { cn } from "@/lib/utils";
import type { AccountFilter } from "@/components/AppShell";

interface Props {
  accountFilter: AccountFilter;
}

export default function DraftsPage({ accountFilter }: Props) {
  const visible = drafts.filter(
    (d) => accountFilter === "all" || d.accountId === accountFilter
  );
  const [selectedId, setSelectedId] = useState<string>(visible[0]?.id ?? drafts[0].id);
  const draft = drafts.find((d) => d.id === selectedId) ?? visible[0];

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Assistant"
        title="Drafts"
        description="AI-generated reply drafts. Stored locally — sending is always an explicit action."
      />

      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {visible.map((d) => {
            const isSel = draft?.id === d.id;
            return (
              <li
                key={d.id}
                onClick={() => setSelectedId(d.id)}
                className={cn(
                  "cursor-pointer px-4 py-3 transition",
                  isSel ? "bg-secondary/70" : "hover:bg-secondary/40"
                )}
              >
                <div className="flex items-center justify-between">
                  <AccountBadge account={getAccount(d.accountId)} />
                  <span className="text-[11px] text-muted-foreground">
                    {relativeTime(d.generatedAt)}
                  </span>
                </div>
                <p className="mt-1.5 truncate text-sm font-medium">{d.toName}</p>
                <p className="truncate text-sm text-foreground/85">{d.subject}</p>
                <div className="mt-2 flex items-center gap-2 text-[10px] uppercase tracking-wider">
                  <Sparkles className="h-3 w-3 text-accent" />
                  <span className="text-muted-foreground">{d.status}</span>
                </div>
              </li>
            );
          })}
        </ul>

        {draft && (
          <div className="surface-card overflow-hidden">
            <div className="border-b border-border/70 px-6 py-4">
              <div className="flex items-center justify-between">
                <AccountBadge account={getAccount(draft.accountId)} showEmail />
                <span className="text-xs text-muted-foreground font-mono">
                  {draft.model}
                </span>
              </div>
              <h2 className="mt-3 font-display text-xl font-medium">{draft.subject}</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                To <span className="text-foreground/85">{draft.toName}</span>{" "}
                &lt;{draft.toEmail}&gt;
              </p>
            </div>
            <textarea
              defaultValue={draft.body}
              className="min-h-[280px] w-full resize-none bg-transparent px-6 py-5 text-sm leading-relaxed text-foreground focus:outline-none"
            />
            <div className="flex items-center justify-between border-t border-border/70 bg-secondary/40 px-6 py-3">
              <p className="text-xs text-muted-foreground">
                Send goes through <span className="font-medium text-foreground/85">{getAccount(draft.accountId)?.primaryEmail}</span>. Recorded in audit log.
              </p>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="ghost" className="text-muted-foreground hover:text-destructive">
                  <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Discard
                </Button>
                <Button size="sm" variant="outline">Save</Button>
                <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90">
                  <Send className="mr-1.5 h-3.5 w-3.5" /> Send
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
