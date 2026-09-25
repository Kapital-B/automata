import type { JobRun } from "@/lib/auth";
import { jobInfo } from "@/lib/scheduleJobs";

// Jobs outside the schedulable set still appear on the Runs page.
const extraJobLabels: Record<string, string> = {
  sync_slack: "Sync Slack",
  draft_suggest: "Draft replies",
  "auto-draft": "Draft replies",
  interpret_project: "Interpret project",
  reconcile_project: "Reconcile project",
  project_ai: "Project review",
};

export function jobTypeLabel(jobType: string): string {
  return jobInfo[jobType]?.label ?? extraJobLabels[jobType] ?? humanize(jobType);
}

/** Every job type a run can have, labelled, for the filter. */
export function jobTypeOptions(): { value: string; label: string }[] {
  const types = [...Object.keys(jobInfo), ...Object.keys(extraJobLabels).filter((t) => t !== "auto-draft")];
  return types.map((value) => ({ value, label: jobTypeLabel(value) })).sort((a, b) => a.label.localeCompare(b.label));
}

export function humanize(key: string): string {
  const s = key.replace(/[_-]+/g, " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function isActiveRun(run: JobRun): boolean {
  return run.status === "pending" || run.status === "running";
}

/** "4s", "2m 10s", "1h 5m"; empty when the run has not finished. */
export function runDuration(run: JobRun, now = Date.now()): string {
  if (!run.started_at) return "";
  const end = run.finished_at ? new Date(run.finished_at).getTime() : isActiveRun(run) ? now : NaN;
  const secs = Math.round((end - new Date(run.started_at).getTime()) / 1000);
  if (!Number.isFinite(secs) || secs < 0) return "";
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ${secs % 60}s`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
}

// Bookkeeping, not results: kept out of the summary line.
const internalKeys = new Set(["queued", "chain_started_at", "chain_id", "schema_version", "step_index", "remaining_jobs"]);

/**
 * One line saying what a run did, from its metadata. Counts that are zero are
 * left out; a run that recorded nothing says so rather than showing a dash.
 */
export function runSummary(run: JobRun): string {
  const meta = run.meta_json ?? {};
  if (typeof meta.drafts_generated === "number") {
    const seen = typeof meta.action_items_seen === "number" ? ` · ${meta.action_items_seen} seen` : "";
    return `${meta.drafts_generated} drafts generated${seen}`;
  }
  const processed = meta.processed_messages;
  const total = meta.total_messages;
  if (typeof processed === "number" && typeof total === "number" && total > 0) {
    return `${processed}/${total} processed`;
  }
  const counts = Object.entries(meta)
    .filter(([k, v]) => !internalKeys.has(k) && typeof v === "number" && v !== 0)
    .map(([k, v]) => `${v} ${humanize(k).toLowerCase()}`);
  if (counts.length > 0) return counts.join(" · ");
  if (isActiveRun(run)) return run.status === "pending" ? "Waiting to start" : "In progress";
  if (run.status === "success") return "Nothing to do";
  return "";
}

/** The metadata worth showing in a run's details, as label/value pairs. */
export function runDetails(run: JobRun): { label: string; value: string }[] {
  return Object.entries(run.meta_json ?? {})
    .filter(([k, v]) => k !== "queued" && v !== null && v !== undefined && typeof v !== "object")
    .map(([k, v]) => ({ label: humanize(k), value: String(v) }));
}
