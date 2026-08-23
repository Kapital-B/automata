import type { AttentionItem, SummaryActionItem } from "@/lib/auth";

export type NeedsMeRow = {
  id: string;
  whyMe: string;
  whyMeLabel: string;
  title: string;
  href: string;
  projectLabel?: string;
  kind: "attention" | "mail";
  mailActionId?: string;
};

const WHY_ME_LABELS: Record<string, string> = {
  issue_assignee: "Assigned to you",
  member_role: "Needs your role",
  provisional_fact: "Confirm fact",
  provisional_decision: "Confirm decision",
  open_contradiction: "Contradiction",
  mail_action_item: "Mail action",
};

const WHY_ME_RANK: Record<string, number> = {
  open_contradiction: 0,
  provisional_decision: 1,
  provisional_fact: 2,
  issue_assignee: 3,
  member_role: 4,
  mail_action_item: 5,
};

const POSITION_WHY = new Set([
  "open_contradiction",
  "provisional_fact",
  "provisional_decision",
]);

const POSITION_REF = new Set(["contradiction", "fact_version", "decision"]);

export function whyMeLabel(whyMe: string): string {
  return WHY_ME_LABELS[whyMe] ?? whyMe.replaceAll("_", " ");
}

function mailHref(item: AttentionItem): string {
  if (item.message_id && item.account_id) {
    return `/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`;
  }
  return "/inbox";
}

/** Deep-link Home Needs me rows into the right project mode (U5) or Inbox. */
export function attentionHref(item: AttentionItem): string {
  if (item.why_me === "mail_action_item" || item.ref_type === "action_item") {
    return mailHref(item);
  }

  if (!item.project_id) {
    return "/projects";
  }

  if (item.ref_type === "issue" && item.ref_id) {
    return `/projects/${item.project_id}/issues/${item.ref_id}`;
  }

  if (POSITION_WHY.has(item.why_me) || POSITION_REF.has(item.ref_type)) {
    return `/projects/${item.project_id}?mode=position`;
  }

  if (
    item.why_me === "issue_assignee" ||
    item.why_me === "member_role" ||
    item.ref_type === "issue"
  ) {
    return `/projects/${item.project_id}?mode=open`;
  }

  return `/projects/${item.project_id}?mode=open`;
}

function rowFromAttention(item: AttentionItem): NeedsMeRow {
  const isMail = item.why_me === "mail_action_item" || item.ref_type === "action_item";
  return {
    id: item.id,
    whyMe: item.why_me,
    whyMeLabel: whyMeLabel(item.why_me),
    title: item.title,
    href: attentionHref(item),
    projectLabel: item.project_name || undefined,
    kind: isMail ? "mail" : "attention",
    mailActionId: isMail ? item.ref_id : undefined,
  };
}

/** Maps the merged GET /api/attention payload. Extra mail rows are ignored when already present. */
export function mergeNeedsMeRows(
  attention: AttentionItem[],
  actionItems: SummaryActionItem[] = [],
): NeedsMeRow[] {
  const fromAttention = attention.map(rowFromAttention);
  const seenMail = new Set(
    attention
      .filter((item) => item.why_me === "mail_action_item" || item.ref_type === "action_item")
      .map((item) => item.ref_id)
      .filter(Boolean),
  );

  const fromMail: NeedsMeRow[] = actionItems
    .filter((item) => !seenMail.has(item.id))
    .map((item) => ({
      id: `mail:${item.id}`,
      whyMe: "mail_action_item",
      whyMeLabel: whyMeLabel("mail_action_item"),
      title: item.text,
      href: `/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`,
      kind: "mail",
      mailActionId: item.id,
    }));

  return [...fromAttention, ...fromMail].sort((a, b) => {
    const ra = WHY_ME_RANK[a.whyMe] ?? 50;
    const rb = WHY_ME_RANK[b.whyMe] ?? 50;
    if (ra !== rb) return ra - rb;
    return a.title.localeCompare(b.title);
  });
}
