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

type ThroughputBucket = "minute" | "hour" | "day";

const BUCKET_ORDER: ThroughputBucket[] = ["minute", "hour", "day"];

/** Bars beyond this stop being distinguishable, so the series gets coarsened. */
const MAX_CHART_BARS = 96;

function bucketKey(minute: string, bucket: ThroughputBucket): string {
  if (bucket === "day") return minute.slice(0, 10);
  if (bucket === "hour") return minute.slice(0, 13);
  return minute;
}

/**
 * The API reports only the minutes that saw traffic, so a month-long range
 * arrives as thousands of points for a few hundred pixels of chart — enough
 * that every bar and the RPM line smear into one solid mass. Collapse them onto
 * the finest time bucket that still fits, keeping the busiest minute of each:
 * every bar stays a single real observation, so the stacked TPM parts still add
 * up and the trend/absolute ceiling keeps meaning what it meant.
 */
export function bucketThroughputSeries(series: ThroughputPoint[]): ThroughputPoint[] {
  const bucket = BUCKET_ORDER.find(
    (candidate) => new Set(series.map((point) => bucketKey(point.minute, candidate))).size <= MAX_CHART_BARS,
  ) ?? "day";
  if (bucket === "minute") return series;

  const peaks = new Map<string, ThroughputPoint>();
  for (const point of series) {
    const key = bucketKey(point.minute, bucket);
    const current = peaks.get(key);
    // Label the bucket, not the winning minute: an axis of peak timestamps
    // reads as a minute-level series and hides how much time each bar spans.
    if (!current || point.total_tpm > current.total_tpm) peaks.set(key, { ...point, minute: key });
  }
  return [...peaks.values()];
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
