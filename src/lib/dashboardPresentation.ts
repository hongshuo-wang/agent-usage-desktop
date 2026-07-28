import type { TokensRow, UsageBreakdown } from "./types";

export type DashboardInsight = {
  peak: {
    timestamp: string;
    day: string;
    totalTokens: number;
  } | null;
  topModel: {
    key: string;
    totalTokens: number;
  } | null;
  topProject: {
    key: string;
    totalTokens: number;
  } | null;
};

function topBreakdown(rows: UsageBreakdown[]): { key: string; totalTokens: number } | null {
  const top = rows.reduce<UsageBreakdown | null>(
    (current, row) => current === null || row.total_tokens > current.total_tokens ? row : current,
    null,
  );

  return top === null ? null : { key: top.key, totalTokens: top.total_tokens };
}

export function buildDashboardInsight(
  tokens: TokensRow[],
  models: UsageBreakdown[],
  projects: UsageBreakdown[],
): DashboardInsight {
  const peak = tokens.reduce<DashboardInsight["peak"]>((current, row) => {
    const totalTokens = row.input_tokens + row.output_tokens + row.cache_read + row.cache_create;
    if (current !== null && current.totalTokens >= totalTokens) return current;

    const leadingDay = row.date.match(/^(\d{4}-\d{2}-\d{2})/);
    return {
      timestamp: row.date,
      day: leadingDay?.[1] ?? row.date,
      totalTokens,
    };
  }, null);

  return {
    peak,
    topModel: topBreakdown(models),
    topProject: topBreakdown(projects),
  };
}
