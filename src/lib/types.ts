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
  priced_cost_usd: number;
  unpriced_records: number;
  legacy_cost_usd: number;
  pricing_last_synced_at: string | null;
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

export interface HeatmapCell {
  weekday: number;
  hour: number;
  calls: number;
  tokens: number;
  cost: number;
}

export interface ThroughputResult {
  average_active_minute: ThroughputValues;
  peak_rolling_60s: ThroughputValues;
  p95_rolling_60s: ThroughputValues;
  series: ThroughputPoint[];
}

export interface CollectionIndexStatus {
  status: "empty" | "stats_only" | "missing_source" | "rebuild_required" | "stale_parser" | "partial" | "available" | "stale";
  last_indexed_at: string | null;
  last_scan_at: string | null;
  source_count: number;
  file_count: number;
  complete_files: number;
  partial_files: number;
  missing_files: number;
  rebuild_required_files: number;
  stale_parser_files: number;
  malformed_lines: number;
}

export interface SessionSummary {
  source: string;
  session_id: string;
  title: string;
  project: string;
  cwd: string;
  git_branch: string;
  start_time: string;
  last_activity: string;
  models: string[];
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
  total_tokens: number;
  total_cost: number;
  prompts: number;
  tool_calls: number;
  errors: number;
  unknown_price: boolean;
  coverage_status: string;
  source_status: string;
  malformed_lines: number;
}

export type SessionEventType =
  | "user_message"
  | "assistant_message"
  | "reasoning"
  | "tool_call"
  | "tool_result"
  | "error"
  | "metadata"
  | "unknown";

export interface SessionEvent {
  id: number;
  event_type: SessionEventType;
  source_event_type: string;
  timestamp: string;
  role: string;
  content: string;
  tool_name: string;
  tool_call_id: string;
  tool_input: string;
  tool_output: string;
  event_status: string;
  duration_ms: number | null;
}

export type CollectorName = "claude" | "codex" | "openclaw" | "opencode" | "pi";

export interface CollectorSetting {
  name: CollectorName;
  enabled: boolean;
  paths: string[];
  scan_interval: string;
}

export interface CollectorSettings {
  collectors: CollectorSetting[];
  pricing_sync_interval: string;
  /** Days of indexed session content to keep; 0 keeps everything. */
  session_event_retention_days: number;
}

export interface PurgeSessionEventsResponse {
  deleted: number;
  vacuumed: boolean;
  bytes_freed: number;
}

export interface SettingsUpdateResponse {
  restart_required: boolean;
}

export interface PricingModel {
  model: string;
  input_cost_per_token: number;
  output_cost_per_token: number;
  cache_read_input_token_cost: number;
  cache_creation_input_token_cost: number;
}

export interface PricingCatalog {
  pricing_last_synced_at: string | null;
  source: string;
  revision: string;
  models: PricingModel[];
}

export interface SessionIndexRebuildResponse {
  status: string;
  sources: number;
}

export interface AppInfo {
  version: string;
  repository: string;
}

export interface UpdateCheck {
  current_version: string;
  release_found: boolean;
  latest_version?: string;
  update_available?: boolean;
  url?: string;
  name?: string;
  body?: string;
}

export interface Release {
  tag_name: string;
  html_url: string;
  name: string;
  body: string;
  published_at: string;
}
