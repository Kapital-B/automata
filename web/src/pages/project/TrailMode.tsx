import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import type { UiAccount } from "@/lib/accounts";
import type { IssueListItem, TimelineItem } from "@/lib/auth";

export type TimelineFilterState = {
  source: "all" | "mail" | "manual" | "slack";
  unassignedToIssue: boolean;
};

const SOURCES = [
  { id: "all", label: "All" },
  { id: "mail", label: "Mail" },
  { id: "manual", label: "Manual" },
  { id: "slack", label: "Slack" },
] as const;

/** The trail stays the history of record — this mode is unchanged in kind. */
export function TrailMode({
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
  onAddFactEvidence,
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
  onAddFactEvidence: (item: TimelineItem) => void;
}) {
  return (
    <div className="space-y-4" role="tabpanel" aria-label="Trail">
      <div className="flex flex-wrap gap-2">
        {SOURCES.map((s) => (
          <Button
            key={s.id}
            size="sm"
            variant={filters.source === s.id ? "default" : "outline"}
            onClick={() => onFiltersChange({ ...filters, source: s.id })}
          >
            {s.label}
          </Button>
        ))}
        <Button
          size="sm"
          variant={filters.unassignedToIssue ? "default" : "outline"}
          onClick={() =>
            onFiltersChange({ ...filters, unassignedToIssue: !filters.unassignedToIssue })
          }
        >
          Unassigned to issue
        </Button>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading timeline…
        </div>
      ) : items.length === 0 ? (
        <p className="py-8 text-sm text-muted-foreground">
          No correspondence yet. Assign mail, sync Slack, or paste a Teams/WhatsApp note.
        </p>
      ) : (
        <ol className="divide-y divide-border/70 border-y border-border/70">
          {items.map((item) => (
            <TimelineRow
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
              onAddFactEvidence={() => onAddFactEvidence(item)}
            />
          ))}
        </ol>
      )}
    </div>
  );
}

function TimelineRow({
  item,
  account,
  projectID,
  issues,
  attaching,
  onAttach,
  onCreateIssue,
  onAddFactEvidence,
}: {
  item: TimelineItem;
  account: UiAccount | undefined;
  projectID: string;
  issues: IssueListItem[];
  attaching: boolean;
  onAttach: (issueID: string) => void;
  onCreateIssue: () => void;
  onAddFactEvidence: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [attachIssueID, setAttachIssueID] = useState("");
  const when = useMemo(() => {
    try {
      return new Date(item.occurred_at).toLocaleString();
    } catch {
      return item.occurred_at;
    }
  }, [item.occurred_at]);
  const contactLabel = item.contacts.map((c) => c.display_name).filter(Boolean).join(", ");

  return (
    <li className="space-y-2 py-4">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="uppercase tracking-wider">{item.source}</span>
        {item.channel ? <span>· {item.channel}</span> : null}
        <span>· {when}</span>
        {item.source === "mail" ? <AccountBadge account={account} /> : null}
        {item.source === "slack" && item.account_label ? (
          <span className="rounded border border-border/70 px-1.5 py-0.5">{item.account_label}</span>
        ) : null}
        {item.issue_id ? (
          <Link
            to={`/projects/${projectID}/issues/${item.issue_id}`}
            className="rounded border border-border/70 px-1.5 py-0.5 hover:underline"
          >
            On issue
          </Link>
        ) : null}
      </div>
      {item.source === "mail" && item.message_id && item.account_id ? (
        <Link
          to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
          className="font-medium hover:underline"
        >
          {item.title || "(no subject)"}
        </Link>
      ) : (
        <p className="font-medium">{item.title || "(untitled)"}</p>
      )}
      {contactLabel ? <p className="text-xs text-muted-foreground">{contactLabel}</p> : null}
      {item.snippet ? <p className="text-sm text-foreground/85">{item.snippet}</p> : null}
      {(item.source === "manual" || item.source === "slack") && item.body_text ? (
        <div>
          <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => setExpanded((v) => !v)}>
            {expanded ? "Hide full text" : item.source === "slack" ? "Show full message" : "Show full paste"}
          </Button>
          {expanded ? (
            <pre className="mt-2 whitespace-pre-wrap rounded-md bg-muted/40 p-3 text-xs">
              {item.body_text}
            </pre>
          ) : null}
        </div>
      ) : null}
      {!item.issue_id ? (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          {issues.length > 0 ? (
            <>
              <select
                aria-label="Attach to issue"
                className="h-8 rounded-md border border-input bg-background px-2 text-xs"
                value={attachIssueID}
                onChange={(e) => setAttachIssueID(e.target.value)}
              >
                <option value="">Attach to issue…</option>
                {issues.map((iss) => (
                  <option key={iss.id} value={iss.id}>
                    {iss.title}
                  </option>
                ))}
              </select>
              <Button
                size="sm"
                className="h-8"
                disabled={!attachIssueID || attaching}
                onClick={() => onAttach(attachIssueID)}
              >
                Attach
              </Button>
            </>
          ) : null}
          <Button size="sm" variant="outline" className="h-8" onClick={onCreateIssue}>
            New issue…
          </Button>
          <Button size="sm" variant="outline" className="h-8" onClick={onAddFactEvidence}>
            Add as fact…
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <Button size="sm" variant="outline" className="h-8" onClick={onAddFactEvidence}>
            Add as fact…
          </Button>
        </div>
      )}
    </li>
  );
}
