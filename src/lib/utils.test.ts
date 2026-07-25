import { describe, expect, it } from "vitest";

import { getTimeRange, getTimeWindowRange } from "./utils";

describe("getTimeRange", () => {
  it("returns the supplied dates for a custom range", () => {
    expect(getTimeRange("custom", "2026-07-01", "2026-07-23")).toEqual({
      from: "2026-07-01",
      to: "2026-07-23",
    });

    const element = document.createElement("div");
    document.body.appendChild(element);
    expect(element).toBeInTheDocument();
  });
});

describe("getTimeWindowRange", () => {
  it("returns an ISO range ending now for a rolling hour window", () => {
    const range = getTimeWindowRange(6);
    const from = Date.parse(range.from);
    const to = Date.parse(range.to);
    expect(Number.isFinite(from)).toBe(true);
    expect(Number.isFinite(to)).toBe(true);
    expect(to - from).toBe(6 * 60 * 60 * 1000);
  });
});
