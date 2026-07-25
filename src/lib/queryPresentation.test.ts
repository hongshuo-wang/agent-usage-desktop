import { describe, expect, it } from "vitest";
import { formatQuerySummary, getActiveQueryChips, presentProjectKey } from "./queryPresentation";

describe("query presentation", () => {
  it("formats the compact summary from committed filters", () => {
    expect(formatQuerySummary({ preset: "last7d", from: "2026-07-19", to: "2026-07-25", source: "" })).toEqual([
      "last7d",
      "allSources",
      "2026-07-19",
      "2026-07-25",
    ]);
  });

  it("returns only active non-default chips", () => {
    expect(getActiveQueryChips({ source: "codex", model: "gpt-5.6", project: "console" })).toEqual([
      { key: "source", value: "codex" },
      { key: "model", value: "gpt-5.6" },
      { key: "project", value: "console" },
    ]);
  });

  it("hides opaque session IDs behind the unnamed project label", () => {
    expect(presentProjectKey("019f8f9a-a104-7b02-9fa8-25c914b5dffd")).toEqual({ label: "unnamedProject", detail: "019f8f9a-a104-7b02-9fa8-25c914b5dffd" });
    expect(presentProjectKey("agent-usage-desktop")).toEqual({ label: "agent-usage-desktop" });
  });
});
