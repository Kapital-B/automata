import { useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Decision, FactDetail } from "@/lib/auth";
import { Provenance, TeachingEmpty } from "./shared";
import { DEFINITIONS } from "./definitions";

/**
 * What this project holds to be true. Proposals are not shown here — they wait
 * in the confirmation panel (spec §8.2), so this mode is the settled position
 * plus the history behind it.
 */
export function PositionMode({
  facts,
  factsLoading,
  decisions,
  decisionsLoading,
  onAddFact,
  onAddDecision,
}: {
  facts: FactDetail[];
  factsLoading: boolean;
  decisions: Decision[];
  decisionsLoading: boolean;
  onAddFact: () => void;
  onAddDecision: () => void;
}) {
  const [expandedFactID, setExpandedFactID] = useState<string | null>(null);
  const settledDecisions = decisions.filter((d) => d.status !== "proposed");

  return (
    <div className="space-y-8" role="tabpanel" aria-label="Position">
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" onClick={onAddFact}>
          Add fact
        </Button>
        <Button variant="outline" size="sm" onClick={onAddDecision}>
          Add decision
        </Button>
      </div>

      <div className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Facts
        </h2>
        {factsLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : facts.length === 0 ? (
          <TeachingEmpty>{DEFINITIONS.facts}</TeachingEmpty>
        ) : (
          <ul className="space-y-3">
            {facts.map((fact) => (
              <FactRow
                key={fact.id}
                fact={fact}
                expanded={expandedFactID === fact.id}
                onToggle={() =>
                  setExpandedFactID((cur) => (cur === fact.id ? null : fact.id))
                }
              />
            ))}
          </ul>
        )}
      </div>

      <div className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Decisions
        </h2>
        {decisionsLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : settledDecisions.length === 0 ? (
          <TeachingEmpty>{DEFINITIONS.decisions}</TeachingEmpty>
        ) : (
          <ul className="space-y-3">
            {settledDecisions.map((d) => (
              <li key={d.id} className="space-y-1 text-sm">
                <p className="text-xs uppercase tracking-wider text-muted-foreground">
                  {d.status}
                </p>
                <p>{d.statement}</p>
                <Provenance evidenceCount={d.evidence?.length ?? 0} source={d.source} />
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function FactRow({
  fact,
  expanded,
  onToggle,
}: {
  fact: FactDetail;
  expanded: boolean;
  onToggle: () => void;
}) {
  const active = fact.versions.find((v) => v.status === "active");
  const history = fact.versions.filter(
    (v) => v.status === "superseded" || v.status === "rejected",
  );
  const display = active ?? fact.versions.find((v) => v.status === "proposed");

  return (
    <li className="text-sm">
      <button type="button" className="w-full text-left hover:underline" onClick={onToggle}>
        <span className="font-medium">{fact.label}</span>
        <span className="mt-0.5 block text-xs text-muted-foreground">
          {display
            ? `${display.value_text}${display.unit ? ` ${display.unit}` : ""} · ${display.status}`
            : fact.subject_key}
        </span>
      </button>
      {display ? (
        <Provenance
          className="mt-0.5 block text-xs text-muted-foreground"
          evidenceCount={display.evidence?.length ?? 0}
          source={display.source}
        />
      ) : null}
      {expanded && history.length > 0 ? (
        <ul className="mt-2 space-y-1 border-l border-border/60 pl-2 text-xs text-muted-foreground">
          {history.map((v) => (
            <li key={v.id}>
              {v.status}: {v.value_text}
              {v.unit ? ` ${v.unit}` : ""}
              {v.evidence?.length ? ` · ${v.evidence.length} evidence` : ""}
            </li>
          ))}
        </ul>
      ) : null}
    </li>
  );
}
