import { describe, expect, it } from "vitest";

import { CHART_COLORS, getTimeRange, shortBucket } from "./utils";

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

describe("chart colors", () => {
  it("uses the restrained dashboard palette", () => {
    expect(CHART_COLORS).toEqual([
      "#0071e3", "#5ac8fa", "#64d2ff", "#8e8e93",
      "#34c759", "#ff9f0a", "#5856d6", "#af52de",
    ]);
  });
});

it("shortBucket trims the year and fills in the clock for axis labels", () => {
  expect(shortBucket("2026-08-27")).toBe("08-27");
  expect(shortBucket("2026-08-27 20")).toBe("08-27 20:00");
  expect(shortBucket("2026-08-27 20:30")).toBe("08-27 20:30");
  expect(shortBucket("2026-08-27T09")).toBe("08-27 09:00");
  // Monthly buckets and anything unexpected pass through untouched.
  expect(shortBucket("2026-08")).toBe("2026-08");
  expect(shortBucket("other")).toBe("other");
});
