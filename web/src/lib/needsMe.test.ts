import { describe, expect, it } from "vitest";
import type { AttentionItem, SummaryActionItem } from "@/lib/auth";
import { attentionHref, mergeNeedsMeRows, whyMeLabel } from "@/lib/needsMe";

describe("needsMe helpers", () => {
  it("labels why_me values for the operator", () => {
    expect(whyMeLabel("open_contradiction")).toBe("Contradiction");
    expect(whyMeLabel("mail_action_item")).toBe("Mail action");
  });

  it("deep-links issues and falls back to project", () => {
    expect(
      attentionHref({
        id: "issue:1",
        why_me: "issue_assignee",
        title: "Pump",
        project_id: "p1",
        ref_type: "issue",
        ref_id: "i1",
      }),
    ).toBe("/projects/p1/issues/i1");
    expect(
      attentionHref({
        id: "decision:1",
        why_me: "provisional_decision",
        title: "Confirm",
        project_id: "p1",
        ref_type: "decision",
        ref_id: "d1",
      }),
    ).toBe("/projects/p1");
  });

  it("merges attention and mail into one ranked list", () => {
    const attention: AttentionItem[] = [
      {
        id: "decision:d1",
        why_me: "provisional_decision",
        title: "Confirm 90 kW",
        project_id: "p1",
        project_name: "Cooling",
        ref_type: "decision",
        ref_id: "d1",
      },
      {
        id: "contradiction:c1",
        why_me: "open_contradiction",
        title: "Duty conflict",
        project_id: "p1",
        project_name: "Cooling",
        ref_type: "contradiction",
        ref_id: "c1",
      },
    ];
    const mail: SummaryActionItem[] = [
      {
        id: "a1",
        account_id: "acc1",
        message_id: "m1",
        text: "Reply to invoice",
        created_at: "2026-08-01T00:00:00Z",
        is_overdue: false,
      },
    ];
    const rows = mergeNeedsMeRows(attention, mail);
    expect(rows.map((r) => r.id)).toEqual([
      "contradiction:c1",
      "decision:d1",
      "mail:a1",
    ]);
    expect(rows[2]?.href).toContain("/inbox?");
    expect(rows[2]?.kind).toBe("mail");
  });
});
