/** Provenance and teaching pieces shared by the project modes (spec §8.3–8.4). */

/** Turns a stored source value into something an operator can read. */
function sourceLabel(source: string | undefined): string {
  switch (source) {
    case "llm":
      return "from the model";
    case "rule":
      return "from a rule";
    default:
      return "from a person";
  }
}

function evidenceLabel(count: number): string {
  if (count <= 0) return "no evidence yet";
  return `from ${count} message${count === 1 ? "" : "s"}`;
}

/**
 * With extraction now automatic, "where did this come from?" is the first
 * question about any fact, decision or issue, so it is answered on the row
 * itself rather than in documentation.
 */
export function Provenance({
  evidenceCount,
  source,
  className,
}: {
  evidenceCount: number;
  source?: string;
  className?: string;
}) {
  return (
    <span className={className ?? "text-xs text-muted-foreground"}>
      {evidenceLabel(evidenceCount)} · {sourceLabel(source)}
    </span>
  );
}

/** An empty state that explains the object rather than apologising for it. */
export function TeachingEmpty({ children }: { children: React.ReactNode }) {
  return <p className="max-w-prose py-2 text-sm text-muted-foreground">{children}</p>;
}
