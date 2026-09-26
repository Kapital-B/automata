const DAY = 24 * 60 * 60 * 1000;

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/**
 * A due date in words, relative to today: "Overdue", "Due today", "Due
 * tomorrow", or the weekday and date. Returns undefined when there is none.
 */
export function dueLabel(dueAt: string | undefined, now: Date = new Date()): { text: string; overdue: boolean } | undefined {
  if (!dueAt) return undefined;
  const due = new Date(dueAt);
  if (Number.isNaN(due.getTime())) return undefined;
  const days = Math.round((startOfDay(due) - startOfDay(now)) / DAY);
  if (days < 0) return { text: days === -1 ? "Overdue since yesterday" : `Overdue by ${-days} days`, overdue: true };
  if (days === 0) return { text: "Due today", overdue: false };
  if (days === 1) return { text: "Due tomorrow", overdue: false };
  return {
    text: `Due ${due.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" })}`,
    overdue: false,
  };
}

export function inboxHref(messageID: string, accountID: string): string {
  return `/inbox?message_id=${encodeURIComponent(messageID)}&account_id=${encodeURIComponent(accountID)}`;
}
