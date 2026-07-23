import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_USAGE_FILTERS,
  buildSessionsSearch,
  getInitialUsageFilters,
  getUsageRequestParams,
  persistUsageFilters,
} from "./usageFilters";

describe("usage filters", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2025, 0, 8, 12));
  });

  it("defaults to the last seven days", () => {
    expect(DEFAULT_USAGE_FILTERS.preset).toBe("last7d");
    expect(getInitialUsageFilters("")).toEqual({
      preset: "last7d",
      from: "2025-01-02",
      to: "2025-01-08",
      source: "",
      model: "",
      project: "",
    });
  });

  it("persists local time and source preferences", () => {
    persistUsageFilters({
      preset: "custom",
      from: "2025-01-03",
      to: "2025-01-04",
      source: "codex",
      model: "model-a",
      project: "project-a",
    });

    expect(getInitialUsageFilters("")).toEqual({
      preset: "custom",
      from: "2025-01-03",
      to: "2025-01-04",
      source: "codex",
      model: "",
      project: "",
    });
  });

  it("lets URL parameters override local preferences on entry", () => {
    persistUsageFilters({
      preset: "last30d",
      from: "2024-12-10",
      to: "2025-01-08",
      source: "codex",
      model: "",
      project: "",
    });

    expect(getInitialUsageFilters(
      "?preset=custom&from=2025-01-06&to=2025-01-07&source=claude&model=sonnet&project=console",
    )).toEqual({
      preset: "custom",
      from: "2025-01-06",
      to: "2025-01-07",
      source: "claude",
      model: "sonnet",
      project: "console",
    });
  });

  it("keeps drill-down context out of overview request parameters", () => {
    const filters = {
      preset: "last7d" as const,
      from: "2025-01-02",
      to: "2025-01-08",
      source: "claude",
      model: "sonnet",
      project: "console",
    };

    expect(getUsageRequestParams(filters)).toEqual({
      from: "2025-01-02",
      to: "2025-01-08",
      source: "claude",
    });
    expect(buildSessionsSearch(filters, { model: "opus" })).toBe(
      "?from=2025-01-02&to=2025-01-08&source=claude&model=opus&project=console",
    );
  });
});
