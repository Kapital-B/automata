import type { UnassignedItem } from "@/lib/auth";

export type TriageProject = { id: string; name: string; code: string };

/** Stable identity for a queue row. Mail rows are thread units. */
export function itemKey(item: UnassignedItem): string {
  return item.kind === "manual"
    ? `manual:${item.manual_item_id ?? ""}`
    : `message:${item.message_id ?? ""}`;
}

export function itemID(item: UnassignedItem): string {
  return (item.kind === "manual" ? item.manual_item_id : item.message_id) ?? "";
}

export function headlineFor(item: UnassignedItem): string {
  return item.kind === "manual"
    ? item.title || item.subject || "(untitled)"
    : item.subject || "(no subject)";
}

/**
 * Turns a raw scorer reason into something an operator can read.
 * The backend stores tokens like `code:DC01+sender_domain:acme.com`.
 */
export function explainReason(
  reason: string | undefined,
  projects: TriageProject[],
): string {
  if (!reason) return "";
  const codeOf = (code: string) => {
    const p = projects.find((x) => x.code.toUpperCase() === code.toUpperCase());
    return p ? `${p.code} · ${p.name}` : code;
  };
  const parts = reason.split("+").map((raw) => {
    const token = raw.trim();
    if (token.startsWith("code:")) return `mentions ${codeOf(token.slice(5))}`;
    if (token.startsWith("name_or_keyword:")) {
      return `matches ${codeOf(token.slice(16))} by name or keyword`;
    }
    if (token.startsWith("sender_domain:")) return `sender at ${token.slice(14)} files here`;
    if (token === "participant_overlap") return "shares people with this project";
    if (token === "thread_sibling") return "same thread as filed mail";
    if (token === "user_assign") return "assigned by you";
    return token;
  });
  return parts.join(", ");
}

/**
 * Shared options for the unassigned-summary badge.
 *
 * The badge is mounted on every page and its query counts the whole triage
 * queue, so it must not refetch on every navigation or window focus.
 */
export const unassignedSummaryQueryOptions = {
  staleTime: 30_000,
  refetchOnWindowFocus: false,
} as const;
