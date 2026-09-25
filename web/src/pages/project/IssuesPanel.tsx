import { Link } from "react-router-dom";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmAction, Tag } from "@/components/ConnectionCard";
import type { IssueListItem } from "@/lib/auth";
import { DEFINITIONS } from "./definitions";
import { Provenance } from "./shared";
import { SECTION } from "./format";

function statusText(issue: IssueListItem): string {
  if (issue.status === "awaiting_input") return "Awaiting input";
  return "Open";
}

export function IssuesPanel({
  issues,
  loading,
  projectID,
  onNewIssue,
  onDiscard,
  discarding,
}: {
  issues: IssueListItem[];
  loading: boolean;
  projectID: string;
  onNewIssue: () => void;
  onDiscard: (issueID: string) => void;
  discarding: boolean;
}) {
  // Yours first, then the rest, newest first.
  const sorted = [...issues].sort(
    (a, b) => Number(b.awaiting_me) - Number(a.awaiting_me) || b.updated_at.localeCompare(a.updated_at),
  );
  return (
    <section id={SECTION.issues} aria-labelledby="issues-heading" className="scroll-mt-20 space-y-3">
      <div className="flex items-end justify-between gap-2">
        <h2 id="issues-heading" className="font-display text-xl font-medium">
          Open issues{!loading && issues.length > 0 ? <span className="ml-2 text-base text-muted-foreground">{issues.length}</span> : null}
        </h2>
        <Button size="sm" variant="outline" onClick={onNewIssue}>
          <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> New issue
        </Button>
      </div>
      <div className="surface-card overflow-hidden">
        {loading ? (
          <div aria-label="Loading issues" className="space-y-2 p-4">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-4 w-40" />
          </div>
        ) : sorted.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground">{DEFINITIONS.issues}</p>
        ) : (
          <ul className="divide-y divide-border/70">
            {sorted.map((iss) => (
              <li key={iss.id} className="flex items-start gap-2 px-4 py-3">
                <Link
                  to={`/projects/${projectID}/issues/${iss.id}`}
                  className="min-w-0 flex-1 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="font-medium hover:underline">{iss.title}</span>
                    {iss.awaiting_me && <Tag>Awaiting you</Tag>}
                  </span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">
                    {statusText(iss)} · {iss.assignee_label ?? "Unassigned"}
                  </span>
                  <Provenance className="block text-xs text-muted-foreground" evidenceCount={iss.item_count} source={iss.source} />
                </Link>
                <ConfirmAction
                  title={`Discard “${iss.title}”?`}
                  description="Use this when the issue should never have been raised. It leaves the list; its correspondence stays on the project."
                  confirmLabel="Discard issue"
                  destructive
                  onConfirm={() => onDiscard(iss.id)}
                  trigger={(open) => (
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 shrink-0 text-muted-foreground"
                      aria-label={`Discard ${iss.title}`}
                      title="Discard"
                      disabled={discarding}
                      onClick={open}
                    >
                      <X aria-hidden="true" className="h-4 w-4" />
                    </Button>
                  )}
                />
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
