import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { UsageFilters } from "../lib/types";
import TimeRangeSelector from "./TimeRangeSelector";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const filters: UsageFilters = {
  preset: "last7d",
  from: "2026-07-19",
  to: "2026-07-25",
  source: "codex",
  model: "",
  project: "console",
};

const props = (overrides: Partial<React.ComponentProps<typeof TimeRangeSelector>> = {}) => ({
  preset: filters.preset,
  source: filters.source,
  onRefresh: vi.fn(),
  filters,
  onFiltersApply: vi.fn(),
  onClearFilters: vi.fn(),
  ...overrides,
});

/** Round-trips through a real state owner so the row behaves as it does in a page. */
function Controlled({ initial }: { initial: UsageFilters }) {
  const [current, setCurrent] = useState(initial);
  return <TimeRangeSelector {...props({ filters: current, preset: current.preset, source: current.source, onFiltersApply: setCurrent })} />;
}

describe("TimeRangeSelector", () => {
  it("applies a preset in one click and never opens an editor", () => {
    const onFiltersApply = vi.fn();
    render(<TimeRangeSelector {...props({ onFiltersApply })} />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "last7d" })).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByRole("button", { name: "today" }));

    const next = onFiltersApply.mock.calls[0][0] as UsageFilters;
    expect(next.preset).toBe("today");
    expect(next.from).toBe(next.to);
    expect(next.from).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("keeps a preset that is no longer a visible button selectable", () => {
    render(<TimeRangeSelector {...props({ preset: "last3d", filters: { ...filters, preset: "last3d" } })} />);
    expect(screen.getByRole("button", { name: "last3d" })).toHaveAttribute("aria-pressed", "true");
  });

  it("switches source inline and removes model/project chips", () => {
    const onFiltersApply = vi.fn();
    render(<TimeRangeSelector {...props({ onFiltersApply })} />);

    fireEvent.change(screen.getByRole("combobox", { name: "queryAgent" }), { target: { value: "claude" } });
    expect(onFiltersApply).toHaveBeenCalledWith(expect.objectContaining({ source: "claude" }));

    fireEvent.click(screen.getByRole("button", { name: /removeFilter console/ }));
    expect(onFiltersApply).toHaveBeenLastCalledWith(expect.objectContaining({ project: "" }));
  });

  it("asks the page to clear everything from the chip row", () => {
    const onClearFilters = vi.fn();
    render(<TimeRangeSelector {...props({ onClearFilters })} />);
    fireEvent.click(screen.getByRole("button", { name: "clearAll" }));
    expect(onClearFilters).toHaveBeenCalled();
  });

  it("reveals the range calendar only for the custom preset", () => {
    render(<Controlled initial={filters} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "custom" }));
    expect(screen.getByRole("button", { name: "queryTimeRange" })).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(screen.getByRole("button", { name: "queryTimeRange" }));
    expect(screen.getByRole("dialog", { name: "queryTimeRange" })).toBeInTheDocument();
    expect(screen.getByText("2026年7月")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "custom" })).toHaveAttribute("aria-pressed", "true");
  });
});
