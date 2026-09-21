import { Link } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { AttentionResult, IssueListItem } from "@/lib/auth";
import { Provenance, TeachingEmpty } from "./shared";
import { DEFINITIONS } from "./definitions";

/** Open work on this project: what needs someone, and the issues themselves. */
export function OpenMode({
  attention,
  attentionLoading,
  issues,
  issuesLoading,
  projectID,
  onNewIssue,
  onDiscard,
  discarding,
}: {
  attention?: AttentionResult;
  attentionLoading: boolean;
  issues: IssueListItem[];
  issuesLoading: boolean;
  projectID: string;
  onNewIssue: () => void;
  onDiscard: (issueID: string) => void;
  discarding: boolean;
}) {
  return (
    <div className="space-y-8" role="tabpanel" aria-label="Open">
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" onClick={onNewIssue}>
          New issue
        </Button>
      </div>

      <div className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Needs attention
        </h2>
        {attentionLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : (attention?.items ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">Nothing project-scoped waiting on you.</p>
        ) : (
          <ul className="divide-y divide-border/70 border-y border-border/70">
            {(attention?.items ?? []).map((item) => (
              <li key={item.id} className="py-2.5 text-sm">
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">
                  {item.why_me.replace(/_/g, " ")}
                </p>
                <p className="mt-0.5 font-medium">{item.title}</p>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Issues
        </h2>
        {issuesLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : issues.length === 0 ? (
          <TeachingEmpty>{DEFINITIONS.issues}</TeachingEmpty>
        ) : (
          <ul className="divide-y divide-border/70 border-y border-border/70">
            {issues.map((iss) => (
              <li key={iss.id} className="space-y-1 py-3">
                <Link
                  to={`/projects/${projectID}/issues/${iss.id}`}
                  className="block text-sm hover:underline"
                >
                  <span className="font-medium">{iss.title}</span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">
                    {iss.assignee_label ?? "Unassigned"} · {iss.status}
                    {iss.awaiting_me ? " · awaiting you" : ""}
                  </span>
                </Link>
                <div className="flex flex-wrap items-center gap-2">
                  <Provenance evidenceCount={iss.item_count} source={iss.source} />
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 text-xs"
                    disabled={discarding}
                    title="This should never have been raised"
                    onClick={() => onDiscard(iss.id)}
                  >
                    Discard
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
