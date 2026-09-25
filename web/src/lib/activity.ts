import type { ActivityItem, ActivityKind } from "@/lib/auth";

/** Human label for each event kind, phrased as what happened. */
const KIND_LABELS: Record<ActivityKind, string> = {
  decision_proposed: "Decision proposed",
  decision_accepted: "Decision accepted",
  decision_withdrawn: "Decision withdrawn",
  fact_recorded: "Fact recorded",
  fact_superseded: "Fact superseded",
  contradiction_opened: "Contradiction opened",
  contradiction_resolved: "Contradiction resolved",
  issue_opened: "Issue opened",
  issue_resolved: "Issue resolved",
};

export function activityLabel(kind: string): string {
  return KIND_LABELS[kind as ActivityKind] ?? kind.split("_").join(" ");
}

/**
 * Events that need the operator's eye are worth marking. A contradiction
 * opening is not the same class of news as a fact being recorded.
 */
export function isAdverse(kind: string): boolean {
  return kind === "contradiction_opened";
}

/** Deep link to the object the event refers to. */
export function activityHref(item: ActivityItem): string {
  switch (item.ref_type) {
    case "issue":
      return `/projects/${item.project_id}/issues/${item.ref_id}`;
    case "fact_version":
    case "decision":
      return `/projects/${item.project_id}#position`;
    case "contradiction":
      return `/projects/${item.project_id}#needs-you`;
    default:
      return `/projects/${item.project_id}`;
  }
}

export type ActivityGroup = {
  /** Stable key for React, independent of locale formatting. */
  key: string;
  label: string;
  items: ActivityItem[];
};

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/**
 * Groups events into calendar days, newest first, labelling the two most
 * recent relative to `now` so the feed reads as a timeline rather than a log.
 *
 * Input is assumed newest-first, as the API returns it; grouping preserves
 * that order within each day.
 */
export function groupActivityByDay(items: ActivityItem[], now: Date = new Date()): ActivityGroup[] {
  const today = startOfDay(now).getTime();
  const yesterday = today - 24 * 60 * 60 * 1000;

  const groups: ActivityGroup[] = [];
  const index = new Map<string, ActivityGroup>();

  for (const item of items) {
    const at = new Date(item.occurred_at);
    if (Number.isNaN(at.getTime())) continue;
    const day = startOfDay(at);
    const key = day.toISOString().slice(0, 10);
    let group = index.get(key);
    if (!group) {
      let label: string;
      if (day.getTime() === today) label = "Today";
      else if (day.getTime() === yesterday) label = "Yesterday";
      else {
        label = day.toLocaleDateString(undefined, {
          weekday: "short",
          day: "numeric",
          month: "short",
        });
      }
      group = { key, label, items: [] };
      index.set(key, group);
      groups.push(group);
    }
    group.items.push(item);
  }
  return groups;
}
