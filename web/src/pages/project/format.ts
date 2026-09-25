export type ItemRef = { message_id?: string; manual_item_id?: string };

export const selectClass =
  "h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** A fact's identifier from its label: facts with the same one are versions. */
export function factKeyFor(label: string): string {
  return label
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

export function formatValue(value: string, unit?: string | null): string {
  return unit ? `${value} ${unit}` : value;
}

/** Section anchors, used for deep links from Home and activity. */
export const SECTION = {
  needsYou: "needs-you",
  position: "position",
  issues: "issues",
  correspondence: "correspondence",
} as const;

// Old links used ?mode=; they still land on the right section.
export function sectionForLegacyMode(mode: string | null): string | null {
  if (mode === "position") return SECTION.position;
  if (mode === "open") return SECTION.needsYou;
  if (mode === "trail") return SECTION.correspondence;
  return null;
}
