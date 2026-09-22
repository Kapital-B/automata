import { describe, expect, it } from "vitest";
import { explainReason } from "@/lib/triage";

const projects = [
  { id: "p1", code: "DC01", name: "Cooling Upgrade" },
  { id: "p2", code: "OT02", name: "Other" },
];

describe("explainReason", () => {
  it("reads a model match", () => {
    expect(explainReason("llm:DC01", projects)).toBe("the model matched DC01 · Cooling Upgrade");
  });

  it("passes the model's own wording through", () => {
    expect(explainReason("mentions the chiller replacement", projects)).toBe(
      "mentions the chiller replacement",
    );
  });

  it("reads a project code match", () => {
    expect(explainReason("code:DC01", projects)).toBe("mentions DC01 · Cooling Upgrade");
  });

  // Assignments made before the scorer was removed still carry its tokens, so
  // they have to keep rendering rather than showing raw.
  it("still decodes retired scorer tokens", () => {
    expect(explainReason("code:DC01+sender_domain:acme.com", projects)).toBe(
      "mentions DC01 · Cooling Upgrade, sender at acme.com files here",
    );
    expect(explainReason("participant_overlap", projects)).toBe("shares people with this project");
  });

  it("is empty for no reason", () => {
    expect(explainReason(undefined, projects)).toBe("");
  });
});
