import { describe, expect, it } from "vitest";

import { getTimeRange } from "./utils";

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
