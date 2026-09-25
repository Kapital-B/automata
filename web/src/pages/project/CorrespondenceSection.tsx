import { useId, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, ClipboardPaste, Hash, Mail, MessagesSquare } from "lucide-react";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { relativeTime, type UiAccount } from "@/lib/accounts";
import { cn } from "@/lib/utils";
import type { IssueListItem, TimelineItem } from "@/lib/auth";
import { SECTION, selectClass } from "./format";

export type TimelineFilterState = {
  source: "all" | "mail" | "manual" | "slack";
  unassignedToIssue: boolean;
};

const SOURCES: { id: TimelineFilterState["source"]; label: string }[] = [
  { id: "all", label: "All" },
  { id: "mail", label: "Mail" },
  { id: "slack", label: "Slack" },
  { id: "manual", label: "Pasted" },
];

const sourceIcon = { mail: Mail, slack: Hash, manual: ClipboardPaste } as const;

/**
 * The project's history of record, newest first. It sits below the position
 * and the issues because it is what those were built from, not what the
 * operator usually came to see.
 */
export function CorrespondenceSection({
  items,
  loading,
  filters,
  onFiltersChange,
  projectID,
  issues,
  accountFor,
  attaching,
  onAttach,
  onCreateIssue,
  onRecordFact,
}: {
  items: TimelineItem[];
  loading: boolean;
  filters: TimelineFilterState;
  onFiltersChange: (next: TimelineFilterState) => void;
  projectID: string;
  issues: IssueListItem[];
  accountFor: (accountID?: string) => UiAccount | undefined;
  attaching: boolean;
  onAttach: (issueID: string, item: TimelineItem) => void;
  onCreateIssue: (item: TimelineItem) => void;
  onRecordFact: (item: TimelineItem) => void;
}) {
  return (
    <section id={SECTION.correspondence} aria-labelledby="correspondence-heading" className="scroll-mt-20 space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="correspondence-heading" className="font-display text-xl font-medium">
            Correspondence
          </h2>
          <p className="text-sm text-muted-foreground">Mail, Slack and pasted notes filed to this project, newest first.</p>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="radiogroup" aria-label="Source" className="inline-flex rounded-md border border-border p-0.5">
          {SOURCES.map((s) => (
            <button
              key={s.id}
              type="button"
              role="radio"
              aria-checked={filters.source === s.id}
              onClick={() => onFiltersChange({ ...filters, source: s.id })}
              className={cn(
                "rounded px-3 py-1.5 text-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                filters.source === s.id ? "bg-secondary font-medium" : "text-muted-foreground hover:text-foreground",
              )}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="trail-unassigned"
            checked={filters.unassignedToIssue}
            onCheckedChange={(v) => onFiltersChange({ ...filters, unassignedToIssue: v })}
          />
          <Label htmlFor="trail-unassigned" className="text-sm font-normal">
            Not on an issue
          </Label>
        </div>
      </div>

      {loading ? (
        <div aria-label="Loading correspondence" className="surface-card space-y-3 p-4">
          <Skeleton className="h-4 w-72" />
          <Skeleton className="h-3 w-96 max-w-full" />
          <Skeleton className="h-4 w-64" />
        </div>
      ) : items.length === 0 ? (
        <div className="surface-card flex flex-col items-center gap-2 px-6 py-10 text-center">
          <MessagesSquare aria-hidden="true" className="h-7 w-7 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            {filters.source !== "all" || filters.unassignedToIssue
              ? "Nothing matches these filters."
              : "No correspondence yet. File mail to this project from Triage, bind a Slack channel, or paste a note."}
          </p>
        </div>
      ) : (
        <ol aria-label="Correspondence" className="surface-card divide-y divide-border/70 overflow-hidden">
          {items.map((item) => (
            <CorrespondenceRow
              key={
                item.message_id ??
                item.manual_item_id ??
                item.connector_message_id ??
                `${item.source}-${item.occurred_at}-${item.title}`
              }
              item={item}
              account={accountFor(item.account_id)}
              projectID={projectID}
              issues={issues}
              attaching={attaching}
              onAttach={(issueID) => onAttach(issueID, item)}
              onCreateIssue={() => onCreateIssue(item)}
              onRecordFact={() => onRecordFact(item)}
            />
          ))}
        </ol>
      )}
    </section>
  );
}

function CorrespondenceRow({
  item,
  account,
  projectID,
  issues,
  attaching,
  onAttach,
  onCreateIssue,
  onRecordFact,
}: {
  item: TimelineItem;
  account: UiAccount | undefined;
  projectID: string;
  issues: IssueListItem[];
  attaching: boolean;
  onAttach: (issueID: string) => void;
  onCreateIssue: () => void;
  onRecordFact: () => void;
}) {
  const [showBody, setShowBody] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [attachIssueID, setAttachIssueID] = useState("");
  const actionsID = useId();
  const when = useMemo(() => new Date(item.occurred_at), [item.occurred_at]);
  const Icon = sourceIcon[item.source] ?? Mail;
  const people = item.contacts.map((c) => c.display_name).filter(Boolean).join(", ");
  const title = item.title || (item.source === "mail" ? "(no subject)" : "(untitled)");

  return (
    <li className="px-4 py-3">
      <div className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-secondary"
        >
          <Icon className="h-4 w-4 text-muted-foreground" />
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-baseline justify-between gap-x-3">
            {item.source === "mail" && item.message_id && item.account_id ? (
              <Link
                to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
                className="min-w-0 truncate font-medium hover:underline focus-visible:underline focus-visible:outline-none"
              >
                {title}
              </Link>
            ) : (
              <p className="min-w-0 truncate font-medium">{title}</p>
            )}
            <time dateTime={item.occurred_at} title={when.toLocaleString()} className="shrink-0 text-xs text-muted-foreground">
              {relativeTime(item.occurred_at)}
            </time>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            {item.source === "mail" && <AccountBadge account={account} />}
            {item.source === "slack" && item.channel && <span>#{item.channel.replace(/^#/, "")}</span>}
            {item.source === "manual" && <span>Pasted{item.channel ? ` from ${item.channel}` : ""}</span>}
            {people && <span className="truncate">{people}</span>}
            {item.issue_id && (
              <Link to={`/projects/${projectID}/issues/${item.issue_id}`} className="font-medium text-primary hover:underline">
                On an issue
              </Link>
            )}
          </div>
          {item.snippet && <p className="line-clamp-2 text-sm text-foreground/85">{item.snippet}</p>}
          {showBody && item.body_text && (
            <pre className="whitespace-pre-wrap rounded-md bg-muted/40 p-3 font-sans text-sm">{item.body_text}</pre>
          )}
          <div className="flex flex-wrap items-center gap-1 pt-0.5">
            {(item.source === "manual" || item.source === "slack") && item.body_text && (
              <Button size="sm" variant="ghost" className="h-8 px-2 text-muted-foreground" onClick={() => setShowBody((v) => !v)}>
                {showBody ? "Hide full text" : "Show full text"}
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              className="h-8 px-2 text-muted-foreground"
              aria-expanded={showActions}
              aria-controls={actionsID}
              onClick={() => setShowActions((v) => !v)}
            >
              <ChevronDown
                aria-hidden="true"
                className={cn("mr-1 h-3.5 w-3.5 transition-transform motion-reduce:transition-none", showActions && "rotate-180")}
              />
              Actions
            </Button>
          </div>
          {showActions && (
            <div id={actionsID} className="flex flex-wrap items-end gap-2 rounded-md border border-border bg-muted/30 p-3">
              {!item.issue_id && issues.length > 0 && (
                <div className="flex min-w-[14rem] flex-1 items-end gap-2">
                  <div className="flex-1 space-y-1">
                    <Label htmlFor={`${actionsID}-issue`} className="text-xs">
                      Attach to issue
                    </Label>
                    <select
                      id={`${actionsID}-issue`}
                      className={selectClass}
                      value={attachIssueID}
                      onChange={(e) => setAttachIssueID(e.target.value)}
                    >
                      <option value="">Choose an issue…</option>
                      {issues.map((iss) => (
                        <option key={iss.id} value={iss.id}>
                          {iss.title}
                        </option>
                      ))}
                    </select>
                  </div>
                  <Button size="sm" className="h-10" disabled={!attachIssueID || attaching} onClick={() => onAttach(attachIssueID)}>
                    Attach
                  </Button>
                </div>
              )}
              {!item.issue_id && (
                <Button size="sm" variant="outline" className="h-10" onClick={onCreateIssue}>
                  New issue from this
                </Button>
              )}
              <Button size="sm" variant="outline" className="h-10" onClick={onRecordFact}>
                Record a fact from this
              </Button>
            </div>
          )}
        </div>
      </div>
    </li>
  );
}
