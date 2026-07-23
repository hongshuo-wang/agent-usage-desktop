import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Dashboard from "./Dashboard";
import { fetchAPI } from "../lib/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../components/TimeRangeSelector", () => ({
  default: ({ preset }: { preset: string }) => (
    <div data-testid="time-range-selector">{preset}</div>
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
    { key: "codex", total_tokens: 150, total_cost: 0.4, sessions: 3, calls: 6, cache_hit_rate: 0.1, unknown_price: false },
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
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "stats") return stats;
      if (path === "tokens-over-time") return tokenRows;
      if (path === "throughput") return throughput;
      if (path === "collection-index-status") return collectionStatus;
      if (path === "usage-breakdown") {
        return breakdowns[String(params.dimension) as keyof typeof breakdowns];
      }
      throw new Error(`Unexpected API path: ${path}`);
    });
  });

  it("renders the three full-width bands in order with exact token components", async () => {
    renderDashboard();

    expect(await screen.findByTestId("dashboard-band-core")).toBeInTheDocument();
    expect(screen.getAllByTestId(/^dashboard-band-/).map((band) => band.dataset.testid)).toEqual([
      "dashboard-band-core",
      "dashboard-band-analysis",
      "dashboard-band-detail",
    ]);
    expect(within(screen.getByTestId("token-components")).getByText("111")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("222")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("33")).toBeInTheDocument();
    expect(within(screen.getByTestId("token-components")).getByText("44")).toBeInTheDocument();
    expect(screen.getByText("localObservedThroughput")).toBeInTheDocument();
    expect(screen.getByText("notProviderQuota")).toBeInTheDocument();
  });

  it("uses URL time and source for overview data while showing model and project as clearable context", async () => {
    localStorage.setItem("au-preset", "last30d");
    localStorage.setItem("au-source", "codex");
    renderDashboard("/?preset=custom&from=2025-01-01&to=2025-01-03&source=claude&model=sonnet&project=console");

    const context = await screen.findByTestId("drilldown-context");
    expect(within(context).getByText("drillDownContext")).toBeInTheDocument();
    expect(screen.getByTestId("time-range-selector")).toHaveTextContent("custom");
    expect(context).toHaveTextContent("sonnet");
    expect(context).toHaveTextContent("console");
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

    await userEvent.setup().click(screen.getByRole("button", { name: "clearModelFilter sonnet" }));
    expect(screen.queryByRole("button", { name: "clearModelFilter sonnet" })).not.toBeInTheDocument();
    expect(screen.getByText("console")).toBeInTheDocument();
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

  it("shows durable local index status and last indexed time", async () => {
    renderDashboard();

    const status = await screen.findByTestId("collection-index-status");
    expect(within(status).getByText("collectionIndexStatus")).toBeInTheDocument();
    expect(within(status).getByText("collectionStatusAvailable")).toBeInTheDocument();
    expect(within(status).getByText("lastIndexUpdate")).toBeInTheDocument();
    expect(within(status).getByText("2025-01-02 03:04")).toBeInTheDocument();
    expect(within(status).queryByText(/collector ready/i)).not.toBeInTheDocument();
  });
});
