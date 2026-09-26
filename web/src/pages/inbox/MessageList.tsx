import { Inbox as InboxIcon, Paperclip } from "lucide-react";
import { Link } from "react-router-dom";
import { AccountBadge } from "@/components/AccountBadge";
import { CategoryPill } from "@/components/CategoryPill";
import { ContactAvatar } from "@/components/ContactAvatar";
import { ListSkeleton } from "@/components/ListSkeleton";
import { Button } from "@/components/ui/button";
import { relativeTime, type UiAccount } from "@/lib/accounts";
import type { MessageItem } from "@/lib/auth";
import { cn } from "@/lib/utils";
import { senderLines } from "./format";

/**
 * The message list. Every row is a button, so the list works from the
 * keyboard, and every text line truncates inside a min-width-0 column, so a
 * long subject can never push the page past the edge of a phone screen.
 */
export function MessageList({
  messages,
  selectedID,
  onSelect,
  accountFor,
  showAccount,
  projectCode,
  categoryName,
  loading,
  error,
  filtered,
  hasAccounts,
  onClearFilters,
  hasMore,
  loadingMore,
  loadMoreFailed,
  onLoadMore,
}: {
  messages: MessageItem[];
  selectedID?: string;
  onSelect: (id: string) => void;
  accountFor: (id: string) => UiAccount | undefined;
  /** Show which account each message is in; pointless when one is chosen. */
  showAccount: boolean;
  projectCode: (id?: string) => string | undefined;
  categoryName: (slug?: string) => string | undefined;
  loading: boolean;
  error?: string;
  filtered: boolean;
  hasAccounts: boolean;
  onClearFilters: () => void;
  hasMore: boolean;
  loadingMore: boolean;
  loadMoreFailed: boolean;
  onLoadMore: () => void;
}) {
  if (loading) return <ListSkeleton label="Loading messages" rows={8} />;
  if (error) {
    return (
      <div role="alert" className="surface-card p-5 text-sm text-destructive">
        Could not load messages: {error}
      </div>
    );
  }
  if (messages.length === 0) {
    return (
      <div className="surface-card flex flex-col items-center gap-3 px-6 py-12 text-center">
        <InboxIcon aria-hidden="true" className="h-8 w-8 text-muted-foreground" />
        {filtered ? (
          <>
            <p className="text-sm text-muted-foreground">No messages match these filters.</p>
            <Button size="sm" variant="outline" onClick={onClearFilters}>
              Clear filters
            </Button>
          </>
        ) : hasAccounts ? (
          <p className="max-w-sm text-sm text-muted-foreground">
            No mail yet. New messages appear here after your mailboxes sync.
          </p>
        ) : (
          <>
            <p className="font-medium">No mailbox connected</p>
            <Button asChild size="sm">
              <Link to="/accounts">Connect a mailbox</Link>
            </Button>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-3">
      <ul aria-label="Messages" className="surface-card divide-y divide-border/70 overflow-hidden">
        {messages.map((m) => {
          const from = senderLines(m.from_json);
          const selected = m.id === selectedID;
          const code = projectCode(m.project_id);
          return (
            <li key={m.id} className="min-w-0">
              <button
                type="button"
                aria-current={selected ? "true" : undefined}
                onClick={() => onSelect(m.id)}
                className={cn(
                  "flex w-full min-w-0 items-start gap-3 px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                  selected ? "bg-secondary shadow-[inset_3px_0_0_hsl(var(--primary))]" : "hover:bg-muted/50",
                )}
              >
                <ContactAvatar name={from.primary} email={m.from_json?.address} className="mt-0.5" />
                <div className="min-w-0 flex-1 space-y-0.5">
                  <div className="flex min-w-0 items-baseline justify-between gap-2">
                    <p className="min-w-0 truncate text-sm font-medium">{from.primary}</p>
                    <time dateTime={m.received_at} className="shrink-0 text-xs text-muted-foreground">
                      {relativeTime(m.received_at)}
                    </time>
                  </div>
                  <p className="truncate text-sm text-foreground/90">{m.subject || "(no subject)"}</p>
                  {m.preview && <p className="truncate text-xs text-muted-foreground">{m.preview}</p>}
                  <div className="flex min-w-0 flex-wrap items-center gap-1.5 pt-1">
                    {showAccount && <AccountBadge account={accountFor(m.account_id)} className="max-w-[12rem]" />}
                    {code && (
                      <span className="rounded border border-border bg-secondary px-1.5 font-mono text-[10px] font-medium">
                        {code}
                      </span>
                    )}
                    <CategoryPill
                      category={m.category_slug ?? "uncategorized"}
                      label={categoryName(m.category_slug)}
                    />
                    {m.has_attachments && (
                      <Paperclip aria-label="Has attachments" className="h-3.5 w-3.5 text-muted-foreground" />
                    )}
                  </div>
                </div>
              </button>
            </li>
          );
        })}
      </ul>
      {(hasMore || loadingMore || loadMoreFailed) && (
        <div className="flex flex-col items-center gap-2">
          <Button type="button" size="sm" variant="outline" disabled={!hasMore || loadingMore} onClick={onLoadMore}>
            {loadingMore ? "Loading…" : "Load more"}
          </Button>
          {loadMoreFailed && (
            <p role="alert" className="text-xs text-destructive">
              Could not load more messages. Try again.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
