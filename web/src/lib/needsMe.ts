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

export function whyMeLabel(whyMe: string): string {
  return WHY_ME_LABELS[whyMe] ?? whyMe.replaceAll("_", " ");
}

export function attentionHref(item: AttentionItem): string {
  if (item.ref_type === "issue" && item.project_id && item.ref_id) {
    return `/projects/${item.project_id}/issues/${item.ref_id}`;
  }
  if (item.project_id) {
    return `/projects/${item.project_id}`;
  }
  return "/projects";
}

export function mergeNeedsMeRows(
  attention: AttentionItem[],
  actionItems: SummaryActionItem[],
): NeedsMeRow[] {
  const fromAttention: NeedsMeRow[] = attention.map((item) => ({
    id: item.id,
    whyMe: item.why_me,
    whyMeLabel: whyMeLabel(item.why_me),
    title: item.title,
    href: attentionHref(item),
    projectLabel: item.project_name || undefined,
    kind: "attention",
  }));

  const fromMail: NeedsMeRow[] = actionItems.map((item) => ({
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
