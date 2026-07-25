import { describe, expect, it } from "vitest";
import { buildThroughputView } from "./throughputScale";

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
