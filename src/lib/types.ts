import type { TimePreset } from "./utils";

export type UsageFilters = {
  preset: TimePreset;
  from: string;
  to: string;
  source: string;
  model: string;
  project: string;
};

export interface DashboardStats {
  total_tokens: number;
  total_cost: number;
  total_sessions: number;
  total_prompts: number;
  total_calls: number;
  cache_hit_rate: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
}

export interface TokensRow {
  date: string;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
}

export interface UsageBreakdown {
  key: string;
  total_tokens: number;
  total_cost: number;
  sessions: number;
  calls: number;
  cache_hit_rate: number;
  unknown_price: boolean;
}

export interface ThroughputValues {
  rpm: number;
  input_tpm: number;
  cache_read_tpm: number;
  cache_create_tpm: number;
  output_tpm: number;
  total_tpm: number;
}

export interface ThroughputPoint extends ThroughputValues {
  minute: string;
}

export interface ThroughputResult {
  average_active_minute: ThroughputValues;
  peak_rolling_60s: ThroughputValues;
  p95_rolling_60s: ThroughputValues;
  series: ThroughputPoint[];
}

export interface CollectionIndexStatus {
  status: "empty" | "stats_only" | "missing_source" | "rebuild_required" | "stale_parser" | "partial" | "available";
  last_indexed_at: string | null;
  source_count: number;
  file_count: number;
  complete_files: number;
  partial_files: number;
  missing_files: number;
  rebuild_required_files: number;
  stale_parser_files: number;
  malformed_lines: number;
}
