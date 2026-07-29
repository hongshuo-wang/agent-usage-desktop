import type { UsageFilters } from "./types";
import { getTimeRange, type TimePreset } from "./utils";

const VALID_PRESETS = new Set<TimePreset>([
  "today", "thisWeek", "thisMonth", "thisYear",
  "last3d", "last7d", "last30d", "custom",
]);

export const DEFAULT_USAGE_FILTERS: UsageFilters = {
  preset: "today",
  from: "",
  to: "",
  source: "",
  model: "",
  project: "",
};

function storedPreset(): TimePreset {
  const value = localStorage.getItem("au-preset") as TimePreset | null;
  return value && VALID_PRESETS.has(value) ? value : DEFAULT_USAGE_FILTERS.preset;
}

export function getInitialUsageFilters(search: string): UsageFilters {
  const query = new URLSearchParams(search);
  const queryPreset = query.get("preset") as TimePreset | null;
  const hasQueryRange = query.has("from") || query.has("to");
  const preset = queryPreset && VALID_PRESETS.has(queryPreset)
    ? queryPreset
    : hasQueryRange ? "custom" : storedPreset();
  const storedFrom = localStorage.getItem("au-custom-from") || "";
  const storedTo = localStorage.getItem("au-custom-to") || "";
  const range = getTimeRange(preset, storedFrom, storedTo);

  return {
    preset,
    from: query.get("from") || range.from,
    to: query.get("to") || range.to,
    source: query.has("source") ? query.get("source") || "" : localStorage.getItem("au-source") || "",
    model: query.get("model") || "",
    project: query.get("project") || "",
  };
}

export function persistUsageFilters(filters: UsageFilters): void {
  localStorage.setItem("au-preset", filters.preset);
  localStorage.setItem("au-source", filters.source);
  localStorage.setItem("au-custom-from", filters.from);
  localStorage.setItem("au-custom-to", filters.to);
}

export function getUsageRequestParams(filters: UsageFilters): Record<string, string | undefined> {
  return {
    from: filters.from,
    to: filters.to,
    source: filters.source || undefined,
  };
}

export function buildSessionsSearch(
  filters: UsageFilters,
  overrides: Partial<Pick<UsageFilters, "from" | "to" | "source" | "model" | "project">> = {},
): string {
  const values = { ...filters, ...overrides };
  const query = new URLSearchParams();
  for (const key of ["from", "to", "source", "model", "project"] as const) {
    if (values[key]) query.set(key, values[key]);
  }
  return `?${query.toString()}`;
}
