import type { UsageFilters } from "./types";

export type QueryChip = { key: "source" | "model" | "project"; value: string };
export type ProjectPresentation = { label: string; detail?: string };

const opaqueProjectPattern = /^[0-9a-f]{8}-[0-9a-f-]{20,}$/i;

export function formatQuerySummary(filters: Pick<UsageFilters, "preset" | "from" | "to" | "source">): string[] {
  return [filters.preset, filters.source || "allSources", filters.from, filters.to];
}

export function getActiveQueryChips(filters: Pick<UsageFilters, "source" | "model" | "project">): QueryChip[] {
  return (["source", "model", "project"] as const)
    .filter((key) => Boolean(filters[key]))
    .map((key) => ({ key, value: filters[key] }));
}

export function presentProjectKey(value: string): ProjectPresentation {
  const normalized = value.trim();
  if (opaqueProjectPattern.test(normalized)) return { label: "unnamedProject", detail: normalized };
  return { label: normalized };
}
