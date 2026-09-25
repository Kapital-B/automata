import { describe, expect, it } from "vitest";
import { activityHref, activityLabel, groupActivityByDay, isAdverse } from "@/lib/activity";
import type { ActivityItem } from "@/lib/auth";

function item(over: Partial<ActivityItem> = {}): ActivityItem {
  return {
    kind: "decision_accepted",
    occurred_at: new Date().toISOString(),
    project_id: "p1",
    project_code: "DC01",
    project_name: "Cooling",
    title: "Proceed",
    ref_type: "decision",
    ref_id: "d1",
    ...over,
  };
}

describe("activity", () => {
  it("labels every known kind as what happened", () => {
    expect(activityLabel("decision_accepted")).toBe("Decision accepted");
    expect(activityLabel("contradiction_opened")).toBe("Contradiction opened");
    expect(activityLabel("fact_superseded")).toBe("Fact superseded");
    // An unknown kind degrades to something readable rather than blank.
    expect(activityLabel("something_new")).toBe("something new");
  });

  it("marks only adverse events", () => {
    expect(isAdverse("contradiction_opened")).toBe(true);
    expect(isAdverse("decision_accepted")).toBe(false);
    expect(isAdverse("contradiction_resolved")).toBe(false);
  });

  it("deep links by referenced object", () => {
    expect(activityHref(item({ ref_type: "issue", ref_id: "i1" }))).toBe("/projects/p1/issues/i1");
    expect(activityHref(item({ ref_type: "decision" }))).toBe("/projects/p1#position");
    expect(activityHref(item({ ref_type: "fact_version" }))).toBe("/projects/p1#position");
    expect(activityHref(item({ ref_type: "contradiction" }))).toBe("/projects/p1#needs-you");
    expect(activityHref(item({ ref_type: "mystery" }))).toBe("/projects/p1");
  });

  describe("groupActivityByDay", () => {
    const now = new Date("2026-09-20T12:00:00Z");

    it("labels the two most recent days relatively", () => {
      const groups = groupActivityByDay(
        [
          item({ ref_id: "a", occurred_at: "2026-09-20T09:00:00Z" }),
          item({ ref_id: "b", occurred_at: "2026-09-19T09:00:00Z" }),
          item({ ref_id: "c", occurred_at: "2026-09-15T09:00:00Z" }),
        ],
        now,
      );
      expect(groups.map((g) => g.label).slice(0, 2)).toEqual(["Today", "Yesterday"]);
      expect(groups).toHaveLength(3);
      expect(groups[2].label).not.toBe("Today");
    });

    it("keeps input order within a day", () => {
      const groups = groupActivityByDay(
        [
          item({ ref_id: "newer", occurred_at: "2026-09-20T11:00:00Z" }),
          item({ ref_id: "older", occurred_at: "2026-09-20T08:00:00Z" }),
        ],
        now,
      );
      expect(groups).toHaveLength(1);
      expect(groups[0].items.map((i) => i.ref_id)).toEqual(["newer", "older"]);
    });

    it("drops events with an unparseable timestamp rather than crashing", () => {
      const groups = groupActivityByDay([item({ occurred_at: "not-a-date" })], now);
      expect(groups).toHaveLength(0);
    });

    it("returns no groups for no events", () => {
      expect(groupActivityByDay([], now)).toEqual([]);
    });
  });
});
