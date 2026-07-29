import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Dashboard from "./Dashboard";
import { fetchAPI } from "../lib/api";
import en from "../lib/locales/en.json";
import zh from "../lib/locales/zh.json";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../components/TimeRangeSelector", () => ({
  default: ({
    preset,
    onSourceChange,
    onGranularityChange,
    onRefresh,
  }: {
    preset: string;
    onSourceChange: (source: string) => void;
    onGranularityChange: (granularity: string) => void;
    onRefresh: () => void;
  }) => (
    <div data-testid="time-range-selector">
      {preset}
      <button type="button" onClick={() => onSourceChange("codex")}>change-overview-source</button>
      <button type="button" onClick={() => onGranularityChange("1d")}>change-granularity</button>
      <button type="button" onClick={onRefresh}>refresh-overview</button>
    </div>
  ),
}));

vi.mock("../components/ChartCard", () => ({
  default: ({ title, option, onEvents }: {
    title: string;
    option: object;
    onEvents?: { click?: (params: { name: string }) => void };
  }) => (
    <div data-testid={`chart-${title}`} data-option={JSON.stringify(option)}>
      {title}
      {onEvents?.click && (
        <button onClick={() => onEvents.click?.({ name: "2025-01-03" })}>
          trend-date
        </button>
      )}
    </div>
  ),
}));

vi.mock("../lib/api", () => ({ fetchAPI: vi.fn() }));

const stats = {
  total_tokens: 410,
  total_cost: 1.2345,
  priced_cost_usd: 1,
  unpriced_records: 2,
  legacy_cost_usd: 0.2345,
  pricing_last_synced_at: "2025-01-02T05:06:07Z",
  total_sessions: 7,
  total_prompts: 13,
  total_calls: 19,
  cache_hit_rate: 0.25,
  input_tokens: 111,
  output_tokens: 222,
  cache_read: 33,
  cache_create: 44,
};

const tokenRows = [{
  date: "2025-01-03",
  input_tokens: 111,
  output_tokens: 222,
  cache_read: 33,
  cache_create: 44,
}];

const breakdowns = {
  source: [
    { key: "claude", total_tokens: 250, total_cost: 0.8, sessions: 4, calls: 9, cache_hit_rate: 0.2, unknown_price: false },
    { key: "codex", total_tokens: 150, total_cost: 0.4, sessions: 3, calls: 6, cache_hit_rate: 0.1, unknown_price: true },
  ],
  model: [{ key: "sonnet", total_tokens: 200, total_cost: 0.7, sessions: 3, calls: 8, cache_hit_rate: 0.3, unknown_price: false }],
  project: [{ key: "console", total_tokens: 150, total_cost: 0.5, sessions: 2, calls: 6, cache_hit_rate: 0.4, unknown_price: true }],
};

const throughput = {
  average_active_minute: { rpm: 1.5, input_tpm: 11, cache_read_tpm: 3, cache_create_tpm: 4, output_tpm: 22, total_tpm: 40 },
  peak_rolling_60s: { rpm: 4, input_tpm: 30, cache_read_tpm: 5, cache_create_tpm: 6, output_tpm: 50, total_tpm: 91 },
  p95_rolling_60s: { rpm: 3, input_tpm: 20, cache_read_tpm: 4, cache_create_tpm: 5, output_tpm: 40, total_tpm: 69 },
  series: [{ minute: "2025-01-03 12:00", rpm: 2, input_tpm: 11, cache_read_tpm: 3, cache_create_tpm: 4, output_tpm: 22, total_tpm: 40 }],
};

const collectionStatus = {
  status: "available",
  last_indexed_at: "2025-01-02T03:04:05Z",
  source_count: 2,
  file_count: 3,
  complete_files: 3,
  partial_files: 0,
  missing_files: 0,
  rebuild_required_files: 0,
  stale_parser_files: 0,
  malformed_lines: 0,
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function defaultAPIResponse(path: string, params: Record<string, string | number | undefined> = {}) {
  if (path === "stats") return stats;
  if (path === "tokens-over-time") return tokenRows;
  if (path === "throughput") return throughput;
  if (path === "collection-index-status") return collectionStatus;
  if (path === "usage-breakdown") {
    return breakdowns[params.dimension as keyof typeof breakdowns];
  }
  throw new Error(`Unexpected API path: ${path}`);
}

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

function renderDashboard(entry = "/") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/sessions" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("Dashboard overview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => defaultAPIResponse(path, params));
  });

  afterEach(() => vi.restoreAllMocks());

  it("renders the token-first bands in order with exact token components", async () => {
    renderDashboard();

    expect(await screen.findByTestId("primary-token-total")).toHaveTextContent("410");
    expect(screen.getAllByTestId(/^dashboard-band-/).map((band) => band.dataset.testid)).toEqual([
      "dashboard-band-core",
      "dashboard-band-insight",
      "dashboard-band-analysis",
      "dashboard-band-detail",
    ]);
    const core = screen.getByTestId("dashboard-band-core");
    expect(within(core).getByTestId("estimated-cost")).toHaveTextContent("$1.23");
    expect(within(core).getByTestId("estimated-cost")).toHaveClass("text-muted-foreground");
    expect(within(core).queryByText(/last period|较上周/i)).not.toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("111")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("222")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("33")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("44")).toBeInTheDocument();
    expect(screen.getByText("localObservedThroughput")).toBeInTheDocument();
    expect(screen.queryByText("notProviderQuota")).not.toBeInTheDocument();
  });

  it("separates overview bands with space and surfaces instead of horizontal rules", async () => {
    renderDashboard();

    await screen.findByTestId("dashboard-band-core");
    for (const band of screen.getAllByTestId(/^dashboard-band-/)) {
      expect(band).not.toHaveClass("border-y");
    }
  });

  it("exposes throughput help as hoverable and keyboard-focusable tooltips", async () => {
    renderDashboard();
    const user = userEvent.setup();

    await screen.findByTestId("throughput-matrix");
    const sectionHelp = screen.getByRole("button", { name: "localObservedThroughputHelp" });
    expect(sectionHelp).not.toHaveAttribute("aria-describedby");
    expect(sectionHelp).toHaveClass("cursor-pointer");
    expect(sectionHelp).not.toHaveClass("cursor-help");
    expect(screen.queryByRole("tooltip", { name: "localObservedThroughputHelp" })).not.toBeInTheDocument();
    await user.hover(sectionHelp);
    expect(sectionHelp).toHaveAttribute("aria-describedby");
    expect(screen.getByRole("tooltip", { name: "localObservedThroughputHelp" })).toBeInTheDocument();
    await user.unhover(sectionHelp);

    const cacheHelp = screen.getByRole("button", { name: "cacheReadTPMHelp" });
    await user.click(cacheHelp);
    expect(cacheHelp).toHaveFocus();
    expect(cacheHelp).toHaveAttribute("aria-describedby");
    const tooltip = screen.getByRole("tooltip", { name: "cacheReadTPMHelp" });
    expect(tooltip).toBeInTheDocument();
    expect(tooltip.parentElement).toBe(document.body);
    expect(tooltip).toHaveClass("fixed", "z-[100]");
  });

  it("places model usage in analysis and agent composition in detail", async () => {
    renderDashboard();

    const analysis = await screen.findByTestId("dashboard-band-analysis");
    const detail = screen.getByTestId("dashboard-band-detail");
    expect(within(analysis).getByText("modelUsage")).toBeInTheDocument();
    expect(within(analysis).queryByText("agentComposition")).not.toBeInTheDocument();
    expect(within(detail).getByText("agentComposition")).toBeInTheDocument();
    expect(within(detail).queryByText("modelUsage")).not.toBeInTheDocument();
  });

  it("renders model usage as proportional, accessible bars with secondary cost details", async () => {
    renderDashboard();

    const row = await screen.findByRole("button", { name: "viewSessionsFor sonnet" });
    const bar = within(row).getByTestId("model-usage-share");
    expect(bar).toHaveAttribute("role", "progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "100");
    expect(bar).toHaveStyle({ width: "100%" });
    expect(within(row).getByText("$0.7000")).toBeInTheDocument();
    expect(within(row).getByText("3 sessions / 8 calls")).toBeInTheDocument();
  });

  it("uses URL time and source for overview data while keeping model and project as query context", async () => {
    localStorage.setItem("au-preset", "last30d");
    localStorage.setItem("au-source", "codex");
    renderDashboard("/?preset=custom&from=2025-01-01&to=2025-01-03&source=claude&model=sonnet&project=console");

    expect(screen.getByTestId("time-range-selector")).toHaveTextContent("custom");
    expect(screen.getByText("overviewFilterLimitation")).toBeInTheDocument();
    await waitFor(() => expect(fetchAPI).toHaveBeenCalledWith("stats", expect.objectContaining({
      from: "2025-01-01",
      to: "2025-01-03",
      source: "claude",
    })));
    const overviewCalls = vi.mocked(fetchAPI).mock.calls.filter(([path]) => path !== "usage-breakdown");
    for (const [, params] of overviewCalls) {
      expect(params).not.toHaveProperty("model");
      expect(params).not.toHaveProperty("project");
    }

    expect(screen.queryByTestId("drilldown-context")).not.toBeInTheDocument();
  });

  it.each([
    ["viewSessionsFor claude", "source=claude"],
    ["viewSessionsFor sonnet", "model=sonnet"],
    ["viewSessionsFor console", "project=console"],
  ])("navigates from %s to matching sessions", async (name, query) => {
    renderDashboard();
    await userEvent.setup().click(await screen.findByRole("button", { name }));

    expect(screen.getByTestId("location")).toHaveTextContent("/sessions?");
    expect(screen.getByTestId("location")).toHaveTextContent(query);
  });

  it("navigates from a trend date to that day", async () => {
    renderDashboard();
    await userEvent.setup().click(await screen.findByRole("button", { name: "trend-date" }));

    expect(screen.getByTestId("location")).toHaveTextContent("/sessions?");
    expect(screen.getByTestId("location")).toHaveTextContent("from=2025-01-03");
    expect(screen.getByTestId("location")).toHaveTextContent("to=2025-01-03");
  });

  it.each([
    ["peakUsage", "from=2025-01-03"],
    ["topModel", "model=sonnet"],
    ["topProject", "project=console"],
  ])("opens sessions from insight %s", async (name, query) => {
    renderDashboard();
    await userEvent.setup().click(await screen.findByRole("button", { name }));

    expect(screen.getByTestId("location")).toHaveTextContent("/sessions?");
    expect(screen.getByTestId("location")).toHaveTextContent(query);
  });

  it("renders every throughput summary and a dual-axis component trend", async () => {
    renderDashboard();

    const matrix = await screen.findByTestId("throughput-matrix");
    expect(within(matrix).getByText("rpm")).toBeInTheDocument();
    expect(within(matrix).getByText("totalTPM")).toBeInTheDocument();
    for (const [rowID, values] of [
      ["throughput-average", ["1.5", "40", "11", "3", "4", "22"]],
      ["throughput-peak", ["4", "91", "30", "5", "6", "50"]],
      ["throughput-p95", ["3", "69", "20", "4", "5", "40"]],
    ] as const) {
      const row = within(matrix).getByTestId(rowID);
      for (const value of values) expect(within(row).getByText(value)).toBeInTheDocument();
    }

    const chart = screen.getByTestId("chart-observedTPMTrend");
    const option = JSON.parse(chart.getAttribute("data-option") || "{}");
    expect(option.yAxis).toHaveLength(2);
    expect(option.series.map((series: { name: string }) => series.name)).toEqual([
      "input", "cacheRead", "cacheCreate", "output", "rpm",
    ]);
    expect(option.series.slice(0, 4).every((series: { stack?: string }) => series.stack === "tpm")).toBe(true);
    expect(option.series[4].yAxisIndex).toBe(1);
  });

  it("filters only throughput by its explicit model selector", async () => {
    const user = userEvent.setup();
    renderDashboard("/?from=2025-01-01&to=2025-01-03&source=claude&model=sonnet");

    const selector = await screen.findByRole("combobox", { name: "throughputModel" });
    expect(selector).toHaveValue("");
    expect(within(selector).getByRole("option", { name: "sonnet" })).toBeInTheDocument();
    await waitFor(() => expect(fetchAPI).toHaveBeenCalledWith("throughput", expect.objectContaining({ source: "claude" })));
    const initialThroughput = vi.mocked(fetchAPI).mock.calls.find(([path]) => path === "throughput");
    expect(initialThroughput?.[1]).not.toHaveProperty("model");

    vi.clearAllMocks();
    await user.selectOptions(selector, "sonnet");
    await waitFor(() => expect(fetchAPI).toHaveBeenCalledWith("throughput", expect.objectContaining({
      source: "claude",
      model: "sonnet",
    })));
    expect(vi.mocked(fetchAPI).mock.calls.every(([path]) => path === "throughput")).toBe(true);
  });

  it("shows Agent cost, sessions, and actual total share", async () => {
    renderDashboard();

    const row = await screen.findByRole("button", { name: "viewSessionsFor claude" });
    expect(within(row).getByText("$0.8000")).toBeInTheDocument();
    expect(within(row).getByText("4 sessions")).toBeInTheDocument();
    expect(within(row).getByText("62.5%")).toBeInTheDocument();
    expect(within(row).getByTestId("composition-share")).toHaveStyle({ width: "62.5%" });
  });

  it("does not annotate Agent costs with pricing warnings", async () => {
    renderDashboard();
    const row = await screen.findByRole("button", { name: "viewSessionsFor codex" });
    expect(within(row).getByText("$0.4000")).toBeInTheDocument();
    expect(within(row).queryByText("$0.4000*")).not.toBeInTheDocument();
    expect(screen.queryByText("unknownPriceFootnote")).not.toBeInTheDocument();
  });

  it("keeps pricing coverage details out of the daily overview", async () => {
    renderDashboard();

    const core = await screen.findByTestId("dashboard-band-core");
    expect(within(core).getByText("localCostEstimate")).toBeInTheDocument();
    expect(within(core).queryByTestId("pricing-coverage")).not.toBeInTheDocument();
    expect(en.localCostEstimate).toBe("Local cost estimate");
    expect(en.unpricedRecordsCostExcluded).toContain("cost is not included");
    expect(en.legacyCostUntraceable).toContain("source is not traceable");
    expect(zh.localCostEstimate).toBe("本地费用估算");
    expect(zh.unpricedRecordsCostExcluded).toContain("不计入");
    expect(zh.legacyCostUntraceable).toContain("无法追溯");
  });

  it("hides healthy index status from the daily overview", async () => {
    renderDashboard();

    await screen.findByTestId("dashboard-band-core");
    expect(screen.queryByTestId("collection-index-status")).not.toBeInTheDocument();
  });

  it("shows the composed empty state when every usage result is empty", async () => {
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "stats") return { ...stats, total_tokens: 0, total_calls: 0 };
      if (path === "usage-breakdown" || path === "tokens-over-time") return [];
      return defaultAPIResponse(path, params);
    });

    renderDashboard();

    expect(await screen.findByText("noUsageData")).toBeInTheDocument();
    expect(screen.getByText("noUsageDataDetail")).toBeInTheDocument();
  });

  it("keeps successful overview content when throughput fails", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "throughput") throw new Error("throughput unavailable");
      return defaultAPIResponse(path, params);
    });

    renderDashboard();

    expect(await screen.findByTestId("primary-token-total")).toHaveTextContent("410");
    expect(await screen.findByText("throughput unavailable")).toBeInTheDocument();
    expect(consoleError).toHaveBeenCalledWith("Throughput fetch error:", expect.any(Error));
  });

  it("keeps newer overview data when an older request succeeds later", async () => {
    const oldStats = deferred<typeof stats>();
    const latestStats = { ...stats, total_tokens: 902 };
    let statsCalls = 0;
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "stats") {
        statsCalls += 1;
        return statsCalls === 1 ? oldStats.promise : latestStats;
      }
      return defaultAPIResponse(path, params);
    });

    renderDashboard();
    await waitFor(() => expect(statsCalls).toBe(1));
    await userEvent.setup().click(screen.getByRole("button", { name: "change-overview-source" }));
    expect(await screen.findByText("902")).toBeInTheDocument();

    await act(async () => oldStats.resolve({ ...stats, total_tokens: 101 }));

    expect(screen.getByText("902")).toBeInTheDocument();
    expect(screen.queryByText("101")).not.toBeInTheDocument();
  });

  it("ignores an older overview failure and finally while the current request is loading", async () => {
    const oldStats = deferred<typeof stats>();
    const currentStats = deferred<typeof stats>();
    const latestStats = { ...stats, total_tokens: 902 };
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    let statsCalls = 0;
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "stats") {
        statsCalls += 1;
        if (statsCalls === 1) return oldStats.promise;
        if (statsCalls === 2) return latestStats;
        return currentStats.promise;
      }
      return defaultAPIResponse(path, params);
    });

    renderDashboard();
    await waitFor(() => expect(statsCalls).toBe(1));
    await userEvent.setup().click(screen.getByRole("button", { name: "change-overview-source" }));
    expect(await screen.findByText("902")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "change-granularity" }));
    await waitFor(() => expect(statsCalls).toBe(3));

    await act(async () => oldStats.reject(new Error("stale overview error")));

    expect(screen.getByRole("main")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByText("stale overview error")).not.toBeInTheDocument();
    expect(consoleError).not.toHaveBeenCalled();

    await act(async () => currentStats.resolve({ ...stats, total_tokens: 903 }));
    await waitFor(() => expect(screen.getByRole("main")).toHaveAttribute("aria-busy", "false"));
  });

  it("keeps newer throughput data when an older request succeeds later", async () => {
    const oldThroughput = deferred<typeof throughput>();
    const latestThroughput = {
      ...throughput,
      average_active_minute: { ...throughput.average_active_minute, total_tpm: 902 },
    };
    let throughputCalls = 0;
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "throughput") {
        throughputCalls += 1;
        return throughputCalls === 1 ? oldThroughput.promise : latestThroughput;
      }
      return defaultAPIResponse(path, params);
    });

    renderDashboard();
    const selector = await screen.findByRole("combobox", { name: "throughputModel" });
    await waitFor(() => expect(throughputCalls).toBe(1));
    await userEvent.setup().selectOptions(selector, "sonnet");
    expect(await within(screen.getByTestId("throughput-average")).findByText("902")).toBeInTheDocument();

    await act(async () => oldThroughput.resolve({
      ...throughput,
      average_active_minute: { ...throughput.average_active_minute, total_tpm: 101 },
    }));

    expect(within(screen.getByTestId("throughput-average")).getByText("902")).toBeInTheDocument();
    expect(within(screen.getByTestId("throughput-average")).queryByText("101")).not.toBeInTheDocument();
  });

  it("ignores an older throughput failure and finally while the current request is loading", async () => {
    const oldThroughput = deferred<typeof throughput>();
    const currentThroughput = deferred<typeof throughput>();
    const latestThroughput = {
      ...throughput,
      average_active_minute: { ...throughput.average_active_minute, total_tpm: 902 },
    };
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    let throughputCalls = 0;
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "throughput") {
        throughputCalls += 1;
        if (throughputCalls === 1) return oldThroughput.promise;
        if (throughputCalls === 2) return latestThroughput;
        return currentThroughput.promise;
      }
      return defaultAPIResponse(path, params);
    });

    renderDashboard();
    const selector = await screen.findByRole("combobox", { name: "throughputModel" });
    await waitFor(() => expect(throughputCalls).toBe(1));
    await userEvent.setup().selectOptions(selector, "sonnet");
    expect(await within(screen.getByTestId("throughput-average")).findByText("902")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "refresh-overview" }));
    await waitFor(() => expect(throughputCalls).toBe(3));
    await waitFor(() => expect(screen.getByText("localObservedThroughput").closest("[aria-busy]"))
      .toHaveAttribute("aria-busy", "true"));

    await act(async () => oldThroughput.reject(new Error("stale throughput error")));

    expect(screen.getByText("localObservedThroughput").closest("[aria-busy]"))
      .toHaveAttribute("aria-busy", "true");
    expect(screen.queryByText("stale throughput error")).not.toBeInTheDocument();
    expect(consoleError).not.toHaveBeenCalled();

    await act(async () => currentThroughput.resolve(throughput));
    await waitFor(() => expect(screen.getByText("localObservedThroughput").closest("[aria-busy]"))
      .toHaveAttribute("aria-busy", "false"));
  });

  it("ignores overview and throughput settlements after unmount", async () => {
    const pendingStats = deferred<typeof stats>();
    const pendingThroughput = deferred<typeof throughput>();
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "stats") return pendingStats.promise;
      if (path === "throughput") return pendingThroughput.promise;
      return defaultAPIResponse(path, params);
    });

    const view = renderDashboard();
    await waitFor(() => {
      expect(fetchAPI).toHaveBeenCalledWith("stats", expect.any(Object));
      expect(fetchAPI).toHaveBeenCalledWith("throughput", expect.any(Object));
    });
    view.unmount();

    await act(async () => {
      pendingStats.reject(new Error("unmounted overview error"));
      pendingThroughput.reject(new Error("unmounted throughput error"));
    });

    expect(consoleError).not.toHaveBeenCalled();
  });
});
