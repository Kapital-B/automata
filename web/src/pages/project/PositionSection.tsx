import { useState } from "react";
import { ChevronDown, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import type { CurrentPosition, Decision, FactDetail } from "@/lib/auth";
import { Provenance } from "./shared";
import { DEFINITIONS } from "./definitions";
import { SECTION, formatValue } from "./format";

/**
 * What the project holds to be true: current values and accepted decisions,
 * each with where it came from. Proposals are not here; they wait in Needs
 * you until confirmed, then appear here.
 */
export function PositionSection({
  position,
  facts,
  decisions,
  loading,
  onAddFact,
  onAddDecision,
}: {
  position?: CurrentPosition;
  facts: FactDetail[];
  decisions: Decision[];
  loading: boolean;
  onAddFact: () => void;
  onAddDecision: () => void;
}) {
  const current = position?.facts ?? [];
  const accepted = position?.decisions ?? [];
  const byVersion = new Map(facts.flatMap((f) => f.versions.map((v) => [v.id, f] as const)));
  const history = (versionID: string) =>
    (byVersion.get(versionID)?.versions ?? []).filter((v) => v.status === "superseded" || v.status === "rejected");
  const withdrawn = decisions.filter((d) => d.status === "withdrawn" || d.status === "superseded");

  return (
    <section id={SECTION.position} aria-labelledby="position-heading" className="scroll-mt-20 space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 id="position-heading" className="font-display text-xl font-medium">
            Current position
          </h2>
          <p className="text-sm text-muted-foreground">{DEFINITIONS.position}</p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={onAddFact}>
            <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Record fact
          </Button>
          <Button size="sm" variant="outline" onClick={onAddDecision}>
            <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Record decision
          </Button>
        </div>
      </div>

      <div className="surface-card overflow-hidden">
        {loading ? (
          <div aria-label="Loading position" className="space-y-2 p-4">
            <Skeleton className="h-4 w-72" />
            <Skeleton className="h-4 w-56" />
            <Skeleton className="h-4 w-64" />
          </div>
        ) : current.length === 0 && accepted.length === 0 ? (
          <div className="max-w-prose space-y-2 p-4 text-sm text-muted-foreground">
            <p>{DEFINITIONS.facts}</p>
            <p>{DEFINITIONS.decisions}</p>
          </div>
        ) : (
          <>
            {current.length > 0 && (
              <div>
                <h3 className="border-b border-border/70 bg-muted/30 px-4 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Facts
                </h3>
                <ul aria-label="Facts" className="divide-y divide-border/70">
                  {current.map((f) => (
                    <FactRow
                      key={f.version_id}
                      label={f.label}
                      value={formatValue(f.value_text, f.unit)}
                      evidenceCount={f.evidence_count}
                      source={byVersion.get(f.version_id)?.versions.find((v) => v.id === f.version_id)?.source}
                      history={history(f.version_id).map((v) => ({
                        id: v.id,
                        text: formatValue(v.value_text, v.unit),
                        status: v.status,
                      }))}
                    />
                  ))}
                </ul>
              </div>
            )}
            {accepted.length > 0 && (
              <div className={cn(current.length > 0 && "border-t border-border/70")}>
                <h3 className="border-b border-border/70 bg-muted/30 px-4 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Decisions
                </h3>
                <ul className="divide-y divide-border/70">
                  {accepted.map((d) => (
                    <li key={d.decision_id} className="space-y-0.5 px-4 py-3">
                      <p className="text-sm font-medium">{d.statement}</p>
                      <p className="text-xs text-muted-foreground">
                        {d.decided_at && <>Decided {new Date(d.decided_at).toLocaleDateString()} · </>}
                        <Provenance
                          className=""
                          evidenceCount={d.evidence_count}
                          source={decisions.find((x) => x.id === d.decision_id)?.source}
                        />
                      </p>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
      </div>
      {withdrawn.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {withdrawn.length} earlier {withdrawn.length === 1 ? "decision was" : "decisions were"} withdrawn or replaced.
        </p>
      )}
    </section>
  );
}

function FactRow({
  label,
  value,
  evidenceCount,
  source,
  history,
}: {
  label: string;
  value: string;
  evidenceCount: number;
  source?: string;
  history: { id: string; text: string; status: string }[];
}) {
  const [open, setOpen] = useState(false);
  return (
    <li className="px-4 py-3">
      <div className="grid gap-x-6 gap-y-1 sm:grid-cols-[minmax(0,1fr)_auto]">
        <span className="text-sm text-muted-foreground">{label}</span>
        <span className="text-sm font-medium tabular-nums sm:text-right">{value}</span>
      </div>
      <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
        <Provenance evidenceCount={evidenceCount} source={source} />
        {history.length > 0 && (
          <button
            type="button"
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
            className="inline-flex items-center gap-1 rounded-sm text-xs text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ChevronDown
              aria-hidden="true"
              className={cn("h-3 w-3 transition-transform motion-reduce:transition-none", open && "rotate-180")}
            />
            {history.length} earlier {history.length === 1 ? "value" : "values"}
          </button>
        )}
      </div>
      {open && (
        <ul className="mt-2 space-y-1 border-l-2 border-border pl-3 text-xs text-muted-foreground">
          {history.map((v) => (
            <li key={v.id}>
              <span className="line-through decoration-muted-foreground/50">{v.text}</span>{" "}
              <span>({v.status === "rejected" ? "rejected" : "replaced"})</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
