import { describe, expect, it } from "vitest";
import { bucketThroughputSeries, buildThroughputView } from "./throughputScale";

const series = [
  { minute: "10:00", total_tpm: 10 },
  { minute: "10:01", total_tpm: 12 },
  { minute: "10:02", total_tpm: 1000 },
  { minute: "10:03", total_tpm: 14 },
  { minute: "10:04", total_tpm: 16 },
].map((point) => ({
  rpm: 1,
  input_tpm: point.total_tpm,
  cache_read_tpm: 0,
  cache_create_tpm: 0,
  output_tpm: 0,
  ...point,
}));

describe("buildThroughputView", () => {
  it("uses a robust trend ceiling and marks legitimate high-usage minutes", () => {
    const view = buildThroughputView(series, "trend");

    expect(view.mode).toBe("trend");
    expect(view.ceiling).toBeLessThan(1000);
    expect(view.peakIndices).toEqual([2]);
    expect(view.series).toEqual(series);
  });

  it("keeps the absolute ceiling for real-value inspection", () => {
    const view = buildThroughputView(series, "absolute");

    expect(view.ceiling).toBe(1000);
    expect(view.peakIndices).toEqual([]);
    expect(view.series).toEqual(series);
  });

  it("falls back to zero for empty data without throwing", () => {
    expect(buildThroughputView([], "trend")).toMatchObject({ ceiling: 0, peakIndices: [], series: [] });
  });
});

describe("bucketThroughputSeries", () => {
  const at = (minute: string, total: number) => ({
    minute,
    rpm: 1,
    input_tpm: total,
    cache_read_tpm: 0,
    cache_create_tpm: 0,
    output_tpm: 0,
    total_tpm: total,
  });

  // 5 days x 20 hours x 2 minutes: too many distinct minutes AND hours to plot,
  // but only 5 days, so the day bucket is the one that should win.
  const spreading = () => Array.from({ length: 200 }, (_, index) => {
    const day = String(19 + Math.floor(index / 40)).padStart(2, "0");
    const hour = String(Math.floor((index % 40) / 2)).padStart(2, "0");
    const minute = String(index % 2).padStart(2, "0");
    return at(`2026-08-${day} ${hour}:${minute}`, index);
  });

  it("leaves a series that already fits the chart untouched", () => {
    const short = [at("2026-08-19 10:00", 10), at("2026-08-19 10:01", 20)];

    expect(bucketThroughputSeries(short)).toEqual(short);
  });

  it("collapses a long range onto days, keeping each day's busiest minute", () => {
    const bucketed = bucketThroughputSeries(spreading());

    expect(bucketed.map((point) => point.minute)).toEqual([
      "2026-08-19",
      "2026-08-20",
      "2026-08-21",
      "2026-08-22",
      "2026-08-23",
    ]);
    expect(bucketed.map((point) => point.total_tpm)).toEqual([39, 79, 119, 159, 199]);
  });

  it("prefers hours over days when hours already fit", () => {
    const oneDay = Array.from({ length: 200 }, (_, index) =>
      at(`2026-08-19 ${String(Math.floor(index / 10)).padStart(2, "0")}:${String(index % 60).padStart(2, "0")}`, index));

    const bucketed = bucketThroughputSeries(oneDay);

    expect(bucketed).toHaveLength(20);
    // the axis carries the hour, and the busiest minute inside it keeps its numbers
    expect(bucketed[0].minute).toBe("2026-08-19 00");
    expect(bucketed[0].total_tpm).toBe(9);
  });

  it("keeps every stacked part of the chosen minute so the bar still totals up", () => {
    const rows = spreading().map((point) => ({ ...point, output_tpm: point.total_tpm, total_tpm: point.total_tpm * 2 }));

    const bucketed = bucketThroughputSeries(rows);

    expect(bucketed).toHaveLength(5);
    expect(bucketed.every((point) => point.input_tpm + point.output_tpm === point.total_tpm)).toBe(true);
  });
});
