import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ConfirmationRow } from "@/hooks/useProjectDetailData";
import { Provenance, TeachingEmpty } from "./shared";
import { DEFINITIONS } from "./definitions";

const KIND_LABEL: Record<ConfirmationRow["kind"], string> = {
  fact: "Fact",
  decision: "Decision",
  contradiction: "Contradiction",
};

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

/**
 * Everything awaiting the operator on this project, in one place (spec §8.2).
 * These used to be scattered across two tabs and mixed in with the things that
 * need no attention, which is why the auto-applied majority was indistinguish-
 * able from the handful that actually needed a decision.
 */
export function NeedsConfirmation({
  rows,
  loading,
  actions,
}: {
  rows: ConfirmationRow[];
  loading: boolean;
  actions: ConfirmationActions;
}) {
  return (
    <section aria-label="Needs your confirmation" className="space-y-3">
      <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
        Needs your confirmation
      </h2>
      {loading ? (
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <TeachingEmpty>{DEFINITIONS.confirmation}</TeachingEmpty>
      ) : (
        <ul className="divide-y divide-border/70 border-y border-border/70">
          {rows.map((row) => (
            <li key={`${row.kind}-${row.id}`} className="space-y-1.5 py-3">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground">
                {KIND_LABEL[row.kind]}
              </p>
              <p className="text-sm font-medium">{row.what}</p>
              <p className="text-xs text-muted-foreground">{row.change}</p>
              <Provenance evidenceCount={row.evidenceCount} source={row.source} />
              <div className="flex flex-wrap gap-1.5 pt-1">
                <RowActions row={row} actions={actions} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function RowActions({
  row,
  actions,
}: {
  row: ConfirmationRow;
  actions: ConfirmationActions;
}) {
  if (row.kind === "fact" && row.version) {
    const versionID = row.version.id;
    const supersedes = row.activeVersionID;
    return (
      <>
        <Button
          size="sm"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.confirmFact(versionID, supersedes)}
        >
          Confirm
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.rejectFact(versionID)}
        >
          Reject
        </Button>
      </>
    );
  }

  if (row.kind === "decision" && row.decision) {
    const decisionID = row.decision.id;
    return (
      <>
        <Button
          size="sm"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.confirmDecision(decisionID)}
        >
          Accept
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.withdrawDecision(decisionID)}
        >
          Withdraw
        </Button>
      </>
    );
  }

  if (row.kind === "contradiction" && row.contradiction) {
    const sides = row.contradiction.sides ?? [];
    const proposed =
      sides.length >= 2 ? sides[1]?.fact_version_id : sides[0]?.fact_version_id;
    const id = row.contradiction.id;
    return (
      <>
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={actions.busy || !proposed}
          onClick={() => actions.resolveContradiction(id, "supersede", proposed)}
        >
          Keep proposed
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.resolveContradiction(id, "reject_b")}
        >
          Reject proposed
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="h-7 text-xs"
          disabled={actions.busy}
          onClick={() => actions.resolveContradiction(id, "note")}
        >
          Note only
        </Button>
      </>
    );
  }

  return null;
}
