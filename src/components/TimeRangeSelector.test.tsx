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
  onPresetChange: vi.fn(),
  granularity: "1h",
  onGranularityChange: vi.fn(),
  source: filters.source,
  onSourceChange: vi.fn(),
  onRefresh: vi.fn(),
  filters,
  onFiltersApply: vi.fn(),
  ...overrides,
});

describe("TimeRangeSelector", () => {
  it("opens a grouped editor without moving the summary", () => {
    render(<TimeRangeSelector {...props()} />);
    expect(screen.getByRole("button", { name: "refresh" })).toHaveAttribute("title", "refresh");
    expect(screen.getByRole("button", { name: "editQuery" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "editQuery" }));
    expect(screen.getByRole("button", { name: "editQuery" })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("dialog", { name: "queryEditor" })).toBeVisible();
    expect(screen.getByRole("combobox", { name: "queryAgent" })).toHaveValue("codex");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "queryEditor" })).not.toBeInTheDocument();
  });

  it("does not render the duplicate rolling query range control", () => {
    const legacyProps = { ...props(), onTimeWindowChange: vi.fn() } as any;
    render(<TimeRangeSelector {...legacyProps} />);

    expect(screen.queryByRole("combobox", { name: "timeWindow" })).not.toBeInTheDocument();
  });

  it("cancels draft edits with Escape", () => {
    render(<TimeRangeSelector {...props()} />);
    fireEvent.click(screen.getByRole("button", { name: /editQuery/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "queryProject" }), { target: { value: "changed" } });
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(props().onFiltersApply).not.toHaveBeenCalled();
  });

  it("applies draft values and removes active chips", () => {
    const onFiltersApply = vi.fn();
    render(<TimeRangeSelector {...props({ onFiltersApply })} />);
    fireEvent.click(screen.getByRole("button", { name: /editQuery/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "queryModel" }), { target: { value: "gpt-5.6" } });
    fireEvent.click(screen.getByRole("button", { name: "applyQuery" }));
    expect(onFiltersApply).toHaveBeenCalledWith(expect.objectContaining({ model: "gpt-5.6", project: "console" }));

    const remove = screen.getByRole("button", { name: /removeFilter console/ });
    fireEvent.click(remove);
    expect(onFiltersApply).toHaveBeenLastCalledWith(expect.objectContaining({ project: "" }));
  });
});
