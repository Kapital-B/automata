import { describe, expect, it } from "vitest";
import type { AttentionItem } from "@/lib/auth";
import { attentionHref, mergeNeedsMeRows, whyMeLabel } from "@/lib/needsMe";

describe("needsMe helpers", () => {
  it("labels why_me values for the operator", () => {
    expect(whyMeLabel("open_contradiction")).toBe("Contradiction");
    expect(whyMeLabel("mail_action_item")).toBe("Mail action");
  });

  it("deep-links Needs me into Position or Open (U5)", () => {
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
    ).toBe("/projects/p1?mode=position");

    expect(
      attentionHref({
        id: "contradiction:c1",
        why_me: "open_contradiction",
        title: "Duty conflict",
        project_id: "p1",
        project_name: "Cooling",
        ref_type: "contradiction",
        ref_id: "c1",
      }),
    ).toBe("/projects/p1?mode=position");

    expect(
      attentionHref({
        id: "fact-version:v1",
        why_me: "provisional_fact",
        title: "Confirm fact: Duty",
        project_id: "p1",
        ref_type: "fact_version",
        ref_id: "v1",
      }),
    ).toBe("/projects/p1?mode=position");

    expect(
      attentionHref({
        id: "issue-role:1",
        why_me: "member_role",
        title: "Awaiting input",
        project_id: "p1",
        ref_type: "issue",
        ref_id: "",
      }),
    ).toBe("/projects/p1?mode=open");

    expect(
      attentionHref({
        id: "mail:a1",
        why_me: "mail_action_item",
        title: "Reply to invoice",
        ref_type: "action_item",
        ref_id: "a1",
        account_id: "acc1",
        message_id: "m1",
      }),
    ).toBe("/inbox?message_id=m1&account_id=acc1");
  });

  it("maps a server-merged attention list including mail", () => {
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
      {
        id: "mail:a1",
        why_me: "mail_action_item",
        title: "Reply to invoice",
        ref_type: "action_item",
        ref_id: "a1",
        account_id: "acc1",
        message_id: "m1",
      },
    ];
    const rows = mergeNeedsMeRows(attention);
    expect(rows.map((r) => r.id)).toEqual([
      "contradiction:c1",
      "decision:d1",
      "mail:a1",
    ]);
    expect(rows[0]?.href).toBe("/projects/p1?mode=position");
    expect(rows[1]?.href).toBe("/projects/p1?mode=position");
    expect(rows[2]?.href).toContain("/inbox?");
    expect(rows[2]?.kind).toBe("mail");
  });
});
