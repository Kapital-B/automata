/** What each schedulable job does, in words a person picks from. */
export const jobInfo: Record<string, { label: string; description: string }> = {
  sync: { label: "Sync mail", description: "Fetch new mail from the mailbox." },
  resolve_contacts: { label: "Update contacts", description: "Add senders and recipients to People." },
  categorize: { label: "Categorise", description: "Sort new mail into categories." },
  assign_projects: { label: "File to projects", description: "Suggest a project for new correspondence." },
  summarize: { label: "Summarise", description: "Refresh the inbox summary." },
  forward_rules: { label: "Run forwarding rules", description: "Forward mail that matches a rule that is on." },
};

/** The server's order; used until the server says otherwise. */
export const defaultAvailableJobs = [
  "sync",
  "resolve_contacts",
  "categorize",
  "assign_projects",
  "summarize",
  "forward_rules",
];

export function jobLabel(job: string): string {
  return jobInfo[job]?.label ?? job;
}

export const intervalPresets: { minutes: number; label: string }[] = [
  { minutes: 5, label: "Every 5 minutes" },
  { minutes: 15, label: "Every 15 minutes" },
  { minutes: 30, label: "Every 30 minutes" },
  { minutes: 60, label: "Every hour" },
  { minutes: 120, label: "Every 2 hours" },
  { minutes: 360, label: "Every 6 hours" },
  { minutes: 720, label: "Every 12 hours" },
  { minutes: 1440, label: "Once a day" },
];

export function intervalLabel(minutes: number): string {
  const preset = intervalPresets.find((p) => p.minutes === minutes);
  if (preset) return preset.label;
  if (minutes % 60 === 0) return `Every ${minutes / 60} hours`;
  return `Every ${minutes} minutes`;
}

/** "in 12 min", "in 3 h", "overdue" — for a time in the future. */
export function untilTime(iso?: string, now = Date.now()): string {
  if (!iso) return "";
  const mins = Math.round((new Date(iso).getTime() - now) / 60000);
  if (mins <= 0) return "due now";
  if (mins < 60) return `in ${mins} min`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `in ${hours} h`;
  return `in ${Math.round(hours / 24)} d`;
}
