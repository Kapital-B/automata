import { describe, expect, it } from "vitest";
import { dueLabel, inboxHref } from "@/lib/todos";

describe("dueLabel", () => {
  const now = new Date(2026, 8, 26, 15, 0);

  it("says nothing without a date", () => {
    expect(dueLabel(undefined, now)).toBeUndefined();
    expect(dueLabel("not a date", now)).toBeUndefined();
  });

  it("reads today and tomorrow by calendar day, not by hours", () => {
    expect(dueLabel(new Date(2026, 8, 26, 9, 0).toISOString(), now)).toEqual({ text: "Due today", overdue: false });
    expect(dueLabel(new Date(2026, 8, 27, 8, 0).toISOString(), now)).toEqual({ text: "Due tomorrow", overdue: false });
  });

  it("flags anything before today as overdue", () => {
    expect(dueLabel(new Date(2026, 8, 25, 23, 0).toISOString(), now)).toEqual({
      text: "Overdue since yesterday",
      overdue: true,
    });
    expect(dueLabel(new Date(2026, 8, 20).toISOString(), now)?.text).toBe("Overdue by 6 days");
  });

  it("names later dates", () => {
    const label = dueLabel(new Date(2026, 9, 2).toISOString(), now);
    expect(label?.overdue).toBe(false);
    expect(label?.text).toMatch(/^Due /);
  });
});

it("links a to-do to its message", () => {
  expect(inboxHref("m 1", "a1")).toBe("/inbox?message_id=m%201&account_id=a1");
});
