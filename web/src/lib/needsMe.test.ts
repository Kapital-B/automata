import { describe, expect, it } from "vitest";
import type { AttentionItem } from "@/lib/auth";
import { attentionHref, mergeNeedsMeRows, whyMeLabel } from "@/lib/needsMe";

describe("needsMe helpers", () => {
  it("labels why_me values for the operator", () => {
    expect(whyMeLabel("open_contradiction")).toBe("Contradiction");
    expect(whyMeLabel("mail_action_item")).toBe("To-do");
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
    ).toBe("/projects/p1#needs-you");

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
    ).toBe("/projects/p1#needs-you");

    expect(
      attentionHref({
        id: "fact-version:v1",
        why_me: "provisional_fact",
        title: "Confirm fact: Duty",
        project_id: "p1",
        ref_type: "fact_version",
        ref_id: "v1",
      }),
    ).toBe("/projects/p1#needs-you");

    expect(
      attentionHref({
        id: "issue-role:1",
        why_me: "member_role",
        title: "Awaiting input",
        project_id: "p1",
        ref_type: "issue",
        ref_id: "",
      }),
    ).toBe("/projects/p1#issues");

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
    expect(rows[0]?.href).toBe("/projects/p1#needs-you");
    expect(rows[1]?.href).toBe("/projects/p1#needs-you");
    expect(rows[2]?.href).toContain("/inbox?");
    expect(rows[2]?.kind).toBe("mail");
  });

  it("places a mail to-do on its project and issue", () => {
    const [row] = mergeNeedsMeRows([
      {
        id: "mail:a2",
        why_me: "mail_action_item",
        title: "Reply to Jan",
        project_id: "p1",
        project_name: "Cooling",
        ref_type: "action_item",
        ref_id: "a2",
        account_id: "acc1",
        message_id: "m2",
        issue_id: "i1",
        issue_title: "Pump P-03 duty",
        due_at: "2026-10-01T09:00:00Z",
      },
    ]);
    expect(row?.whyMeLabel).toBe("To-do");
    expect(row?.projectLabel).toBe("Cooling");
    expect(row?.issueTitle).toBe("Pump P-03 duty");
    expect(row?.issueHref).toBe("/projects/p1/issues/i1");
    expect(row?.dueAt).toBe("2026-10-01T09:00:00Z");
    // The row itself still opens the message the to-do came from.
    expect(row?.href).toBe("/inbox?message_id=m2&account_id=acc1");
  });
});
