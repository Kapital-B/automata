import { ChevronLeft, Forward, Loader2, Paperclip, Reply } from "lucide-react";
import { Link } from "react-router-dom";
import { AccountBadge } from "@/components/AccountBadge";
import { CategoryPill } from "@/components/CategoryPill";
import { ContactAvatar } from "@/components/ContactAvatar";
import { ProjectAssignControl } from "@/components/ProjectAssignControl";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { relativeTime, type UiAccount } from "@/lib/accounts";
import type { MessageItem } from "@/lib/auth";
import { EmailBody } from "./EmailBody";
import { senderLines } from "./format";

/**
 * One message: who sent it and when, what to do with it (draft a reply,
 * forward, file to a project), then the message itself. Actions are visible
 * buttons rather than a menu, since there are only two.
 */
export function MessageDetail({
  message,
  account,
  categoryName,
  body,
  bodyLoading,
  refreshingHtml,
  draftID,
  draftPending,
  onCreateDraft,
  onForward,
  onBack,
}: {
  message: MessageItem;
  account?: UiAccount;
  categoryName?: string;
  body?: string;
  bodyLoading: boolean;
  refreshingHtml: boolean;
  draftID?: string;
  draftPending: boolean;
  onCreateDraft: () => void;
  onForward: () => void;
  /** Present on narrow screens, where the message replaces the list. */
  onBack?: () => void;
}) {
  const from = senderLines(message.from_json);
  const received = new Date(message.received_at);

  return (
    <article aria-labelledby="message-subject" className="surface-card min-w-0 overflow-hidden">
      {onBack && (
        <div className="border-b border-border/70 px-2 py-1">
          <Button type="button" variant="ghost" className="h-10 gap-1 px-2 text-muted-foreground" onClick={onBack}>
            <ChevronLeft aria-hidden="true" className="h-4 w-4" />
            Messages
          </Button>
        </div>
      )}

      <header className="space-y-4 px-4 py-4 sm:px-6 sm:py-5">
        <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <h2
            id="message-subject"
            className="min-w-0 font-display text-xl font-medium leading-snug [overflow-wrap:anywhere] sm:text-2xl"
          >
            {message.subject || "(no subject)"}
          </h2>
          <div className="flex shrink-0 gap-2">
            {draftID ? (
              <Button asChild size="sm" variant="outline" className="h-9">
                <Link to={`/drafts?draft_id=${encodeURIComponent(draftID)}`}>
                  <Reply aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                  Open draft
                </Link>
              </Button>
            ) : (
              <Button size="sm" variant="outline" className="h-9" disabled={draftPending} onClick={onCreateDraft}>
                {draftPending ? (
                  <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Reply aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                )}
                {draftPending ? "Drafting…" : "Draft reply"}
              </Button>
            )}
            <Button size="sm" variant="outline" className="h-9" onClick={onForward}>
              <Forward aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
              Forward
            </Button>
          </div>
        </div>

        <div className="flex min-w-0 items-center gap-3">
          <ContactAvatar name={from.primary} email={message.from_json?.address} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{from.primary}</p>
            <p className="truncate text-xs text-muted-foreground">
              {from.secondary && <span className="font-mono">{from.secondary} · </span>}
              <time dateTime={message.received_at} title={received.toLocaleString()}>
                {relativeTime(message.received_at)}
              </time>
            </p>
          </div>
        </div>

        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <AccountBadge account={account} showEmail className="max-w-full" />
          <CategoryPill category={message.category_slug ?? "uncategorized"} label={categoryName} />
          {message.has_attachments && (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <Paperclip aria-hidden="true" className="h-3.5 w-3.5" />
              Attachments
            </span>
          )}
        </div>
      </header>

      <div className="border-t border-border/70 bg-muted/20 px-4 py-3 sm:px-6">
        <ProjectAssignControl
          key={message.id}
          messageID={message.id}
          hasConversation={Boolean(message.conversation_id)}
          currentProjectID={message.project_id}
        />
      </div>

      {refreshingHtml && (
        <p role="status" className="border-t border-border/70 bg-secondary/40 px-4 py-2 text-xs text-muted-foreground sm:px-6">
          Fetching the formatted version of this email…
        </p>
      )}
      <div className="border-t border-border/70">
        {bodyLoading ? (
          <div aria-label="Loading message" className="space-y-2 px-4 py-5 sm:px-6">
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        ) : (
          <EmailBody body={body ?? ""} />
        )}
      </div>
    </article>
  );
}
