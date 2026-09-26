import { Link } from "react-router-dom";
import { AlertTriangle, CheckCircle2, ChevronRight, FileQuestion, Inbox, Scale } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import type { ConfirmationRow } from "@/hooks/useProjectDetailData";
import type { AttentionItem } from "@/lib/auth";
import { Provenance } from "./shared";
import { SECTION } from "./format";

export type ConfirmationActions = {
  confirmFact: (versionID: string, supersedesVersionID?: string) => void;
  rejectFact: (versionID: string) => void;
  confirmDecision: (decisionID: string) => void;
  withdrawDecision: (decisionID: string) => void;
  resolveContradiction: (
    id: string,
    resolution: "supersede" | "reject_a" | "reject_b" | "note",
    keepFactVersionID?: string,
  ) => void;
  busy: boolean;
};

const KIND: Record<ConfirmationRow["kind"], { label: string; icon: typeof Scale }> = {
  fact: { label: "Changed value", icon: FileQuestion },
  decision: { label: "Proposed decision", icon: CheckCircle2 },
  contradiction: { label: "Sources disagree", icon: Scale },
};

// These attention reasons are the confirmation rows themselves; listing them
// twice would make the queue look longer than it is. To-dos from mail have
// their own section, shared with the whole project.
const CONFIRMATION_REASONS = new Set(["provisional_fact", "provisional_decision", "open_contradiction", "mail_action_item"]);

function attentionLabel(why: string): string {
  switch (why) {
    case "issue_assignee":
      return "Assigned to you";
    case "member_role":
      return "In your role";
    default:
      return why.replace(/_/g, " ");
  }
}

function attentionHref(item: AttentionItem, projectID: string): string | undefined {
  if (item.ref_type === "issue") return `/projects/${projectID}/issues/${item.ref_id}`;
  if (item.message_id && item.account_id) {
    return `/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`;
  }
  return undefined;
}

/**
 * The top of the page: everything waiting on the operator, and nothing else.
 * Model proposals are confirmed in place; other items link to where they are
 * handled. When the queue is empty it says so in one line and gets out of
 * the way.
 */
export function NeedsYou({
  rows,
  attention,
  loading,
  projectID,
  actions,
}: {
  rows: ConfirmationRow[];
  attention: AttentionItem[];
  loading: boolean;
  projectID: string;
  actions: ConfirmationActions;
}) {
  const other = attention.filter((a) => !CONFIRMATION_REASONS.has(a.why_me));
  const total = rows.length + other.length;

  return (
    <section id={SECTION.needsYou} aria-labelledby="needs-you-heading" className="scroll-mt-20 space-y-3">
      <div className="flex items-baseline gap-2">
        <h2 id="needs-you-heading" className="font-display text-xl font-medium">
          Needs you
        </h2>
        {!loading && total > 0 && (
          <span className="rounded-full bg-warning/15 px-2 py-0.5 text-xs font-medium text-foreground">{total}</span>
        )}
      </div>

      {loading ? (
        <div aria-label="Loading" className="surface-card space-y-2 p-4">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-4 w-48" />
        </div>
      ) : total === 0 ? (
        <p className="surface-card flex items-center gap-2 px-4 py-3 text-sm text-muted-foreground">
          <CheckCircle2 aria-hidden="true" className="h-4 w-4 text-success" />
          Nothing is waiting on you. New values the model is sure of are applied on their own.
        </p>
      ) : (
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {rows.map((row) => {
            const kind = KIND[row.kind];
            const Icon = kind.icon;
            return (
              <li key={`${row.kind}-${row.id}`} className="flex flex-wrap items-start gap-3 px-4 py-3">
                <Icon aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="text-xs text-muted-foreground">{kind.label}</p>
                  <p className="font-medium">{row.what}</p>
                  <p className="text-sm text-muted-foreground">{row.change}</p>
                  <Provenance evidenceCount={row.evidenceCount} source={row.source} />
                </div>
                <div className="flex flex-wrap gap-2 sm:ml-auto">
                  <RowActions row={row} actions={actions} />
                </div>
              </li>
            );
          })}
          {other.map((item) => {
            const href = attentionHref(item, projectID);
            const body = (
              <>
                {item.message_id ? (
                  <Inbox aria-hidden="true" className="h-4 w-4 shrink-0 text-muted-foreground" />
                ) : (
                  <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0 flex-1">
                  <p className="text-xs text-muted-foreground">{attentionLabel(item.why_me)}</p>
                  <p className="truncate font-medium">{item.title}</p>
                </div>
                {href && <ChevronRight aria-hidden="true" className="h-4 w-4 shrink-0 text-muted-foreground" />}
              </>
            );
            return (
              <li key={item.id}>
                {href ? (
                  <Link
                    to={href}
                    className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  >
                    {body}
                  </Link>
                ) : (
                  <div className="flex items-center gap-3 px-4 py-3">{body}</div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function RowActions({ row, actions }: { row: ConfirmationRow; actions: ConfirmationActions }) {
  if (row.kind === "fact" && row.version) {
    const versionID = row.version.id;
    return (
      <>
        <Button size="sm" disabled={actions.busy} onClick={() => actions.confirmFact(versionID, row.activeVersionID)}>
          Confirm
        </Button>
        <Button size="sm" variant="outline" disabled={actions.busy} onClick={() => actions.rejectFact(versionID)}>
          Reject
        </Button>
      </>
    );
  }
  if (row.kind === "decision" && row.decision) {
    const decisionID = row.decision.id;
    return (
      <>
        <Button size="sm" disabled={actions.busy} onClick={() => actions.confirmDecision(decisionID)}>
          Accept
        </Button>
        <Button size="sm" variant="outline" disabled={actions.busy} onClick={() => actions.withdrawDecision(decisionID)}>
          Withdraw
        </Button>
      </>
    );
  }
  if (row.kind === "contradiction" && row.contradiction) {
    const sides = row.contradiction.sides ?? [];
    const proposed = sides.length >= 2 ? sides[1]?.fact_version_id : sides[0]?.fact_version_id;
    const id = row.contradiction.id;
    return (
      <>
        <Button size="sm" disabled={actions.busy || !proposed} onClick={() => actions.resolveContradiction(id, "supersede", proposed)}>
          Keep new value
        </Button>
        <Button size="sm" variant="outline" disabled={actions.busy} onClick={() => actions.resolveContradiction(id, "reject_b")}>
          Keep current
        </Button>
        <Button size="sm" variant="ghost" disabled={actions.busy} onClick={() => actions.resolveContradiction(id, "note")}>
          Just note it
        </Button>
      </>
    );
  }
  return null;
}
