import { describe, expect, it } from "vitest";
import { buildDashboardInsight } from "./dashboardPresentation";
import type { TokensRow, UsageBreakdown } from "./types";

function breakdown(key: string, totalTokens: number): UsageBreakdown {
  return {
    key,
    total_tokens: totalTokens,
    total_cost: 0,
    sessions: 1,
    calls: 1,
    cache_hit_rate: 0,
    unknown_price: false,
  };
}

describe("buildDashboardInsight", () => {
  it("presents the peak, top model, and top project without modifying source rows", () => {
    const tokens: TokensRow[] = [
      { date: "2025-01-01", input_tokens: 10, output_tokens: 11, cache_read: 7, cache_create: 8 },
      { date: "2025-01-02 15:00", input_tokens: 25, output_tokens: 25, cache_read: 25, cache_create: 25 },
    ];
    const models = [breakdown("opus", 20), breakdown("sonnet", 80)];
    const projects = [breakdown("console", 60)];
    const originalTokens = structuredClone(tokens);
    const originalModels = structuredClone(models);
    const originalProjects = structuredClone(projects);

    expect(buildDashboardInsight(tokens, models, projects)).toEqual({
      peak: { timestamp: "2025-01-02 15:00", day: "2025-01-02", totalTokens: 100 },
      topModel: { key: "sonnet", totalTokens: 80 },
      topProject: { key: "console", totalTokens: 60 },
    });
    expect(tokens).toEqual(originalTokens);
    expect(models).toEqual(originalModels);
    expect(projects).toEqual(originalProjects);
  });

  it("keeps the earliest equal peak and handles missing breakdowns", () => {
    const tokens: TokensRow[] = [
      { date: "first", input_tokens: 4, output_tokens: 3, cache_read: 2, cache_create: 1 },
      { date: "second", input_tokens: 1, output_tokens: 2, cache_read: 3, cache_create: 4 },
    ];

    expect(buildDashboardInsight(tokens, [], [])).toEqual({
      peak: { timestamp: "first", day: "first", totalTokens: 10 },
      topModel: null,
      topProject: null,
    });
  });

  it("returns null presentation values when every input is empty", () => {
    expect(buildDashboardInsight([], [], [])).toEqual({
      peak: null,
      topModel: null,
      topProject: null,
    });
  });
});
