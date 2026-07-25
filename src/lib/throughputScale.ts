import type { ThroughputPoint } from "./types";

export type ThroughputViewMode = "trend" | "absolute";

export type ThroughputView = {
  mode: ThroughputViewMode;
  ceiling: number;
  peakIndices: number[];
  series: ThroughputPoint[];
};

function percentile(values: number[], rank: number): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const position = (sorted.length - 1) * rank;
  const lower = Math.floor(position);
  const upper = Math.ceil(position);
  if (lower === upper) return sorted[lower];
  const fraction = position - lower;
  return sorted[lower] + (sorted[upper] - sorted[lower]) * fraction;
}

export function buildThroughputView(series: ThroughputPoint[], mode: ThroughputViewMode): ThroughputView {
  if (!series.length) return { mode, ceiling: 0, peakIndices: [], series };
  const max = Math.max(...series.map((point) => point.total_tpm), 0);
  if (mode === "absolute") return { mode, ceiling: max, peakIndices: [], series };

  const p95 = percentile(series.map((point) => point.total_tpm), 0.95);
  const ceiling = Math.max(1, p95);
  const peakIndices = series
    .map((point, index) => (point.total_tpm > ceiling ? index : -1))
    .filter((index) => index >= 0);
  return { mode, ceiling, peakIndices, series };
}
