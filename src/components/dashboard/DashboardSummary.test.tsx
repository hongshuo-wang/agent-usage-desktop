import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { DashboardInsight } from "../../lib/dashboardPresentation";
import type { DashboardStats } from "../../lib/types";
import TokenSummary from "./TokenSummary";
import UsageInsight from "./UsageInsight";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const stats: DashboardStats = {
  total_tokens: 1_250_000,
  total_cost: 12.345,
  priced_cost_usd: 12.345,
  unpriced_records: 2,
  legacy_cost_usd: 0,
  pricing_last_synced_at: null,
  total_sessions: 42,
  total_prompts: 186,
  total_calls: 240,
  cache_hit_rate: 0.734,
  input_tokens: 500_000,
  output_tokens: 250_000,
  cache_read: 450_000,
  cache_create: 50_000,
};

const insight: DashboardInsight = {
  peak: { timestamp: "2026-07-24T10:00:00Z", day: "2026-07-24", totalTokens: 450_000 },
  topModel: { key: "gpt-5.6", totalTokens: 800_000 },
  topProject: { key: "agent-usage-desktop", totalTokens: 640_000 },
};

describe("TokenSummary", () => {
  it("places the formatted token total before its secondary metrics", () => {
    render(<TokenSummary stats={stats} rangeDetail="2026-07-01 - 2026-07-28" />);

    const section = screen.getByTestId("dashboard-band-core");
    const tokenTotal = within(section).getByTestId("primary-token-total");
    const secondaryMetrics = within(section).getByTestId("secondary-metrics");

    expect(tokenTotal).toHaveTextContent("1.3M");
    expect(tokenTotal.compareDocumentPosition(secondaryMetrics) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(secondaryMetrics).getByText("42")).toBeInTheDocument();
    expect(within(secondaryMetrics).getByText("186")).toBeInTheDocument();
    expect(within(secondaryMetrics).getByText("73.4%")).toBeInTheDocument();
  });

  it("keeps estimated cost muted without comparison or pricing-warning copy", () => {
    render(<TokenSummary stats={stats} rangeDetail="Last 30 days" />);

    expect(screen.getByTestId("estimated-cost")).toHaveTextContent("$12.35");
    expect(screen.getByTestId("estimated-cost")).toHaveClass("text-muted-foreground");
    expect(screen.queryByText(/previous|comparison|unpriced|pricingCoverage|unknownPrice/i)).not.toBeInTheDocument();
  });
});

describe("UsageInsight", () => {
  it("renders independent facts and calls only the matching drill-down callback", () => {
    const onOpenDay = vi.fn();
    const onOpenModel = vi.fn();
    const onOpenProject = vi.fn();
    render(
      <UsageInsight
        insight={insight}
        onOpenDay={onOpenDay}
        onOpenModel={onOpenModel}
        onOpenProject={onOpenProject}
      />,
    );

    const section = screen.getByTestId("dashboard-band-insight");
    expect(within(section).getByRole("heading", { name: "usageOverview" })).toBeInTheDocument();
    expect(section).toHaveTextContent("450.0K");
    expect(section).toHaveTextContent("800.0K");
    expect(section).toHaveTextContent("640.0K");

    fireEvent.click(within(section).getByRole("button", { name: "peakUsage" }));
    expect(onOpenDay).toHaveBeenCalledWith("2026-07-24");
    expect(onOpenModel).not.toHaveBeenCalled();
    expect(onOpenProject).not.toHaveBeenCalled();

    onOpenDay.mockClear();
    fireEvent.click(within(section).getByRole("button", { name: "topModel" }));
    expect(onOpenModel).toHaveBeenCalledWith("gpt-5.6");
    expect(onOpenDay).not.toHaveBeenCalled();
    expect(onOpenProject).not.toHaveBeenCalled();

    onOpenModel.mockClear();
    fireEvent.click(within(section).getByRole("button", { name: "topProject" }));
    expect(onOpenProject).toHaveBeenCalledWith("agent-usage-desktop");
    expect(onOpenDay).not.toHaveBeenCalled();
    expect(onOpenModel).not.toHaveBeenCalled();
  });

  it("renders nothing when no insight facts are available", () => {
    const { container } = render(
      <UsageInsight
        insight={{ peak: null, topModel: null, topProject: null }}
        onOpenDay={vi.fn()}
        onOpenModel={vi.fn()}
        onOpenProject={vi.fn()}
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
