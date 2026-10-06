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
      "#1677ea", "#2ec4a6", "#f0a02c", "#8b95a6",
      "#7a63d6", "#ff7968", "#3aa6e8", "#c96aa8",
    ]);
    // Every series colour must stay mid-luminance to survive both themes.
    for (const hex of CHART_COLORS) {
      const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
      const lum = 0.2126 * r + 0.7152 * g + 0.0722 * b;
      expect(lum).toBeGreaterThan(0.25);
      expect(lum).toBeLessThan(0.75);
    }
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
