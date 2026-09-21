import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import ActivityHeatmap from "./ActivityHeatmap";

const t = (key: string) => key;

function labeledCells(container: HTMLElement): string[] {
  return [...container.querySelectorAll("[title]")].map((el) => el.getAttribute("title") ?? "");
}

test("renders Sunday last, because SQLite numbers it 0", () => {
  const { container } = render(
    <ActivityHeatmap
      cells={[{ weekday: 0, hour: 5, calls: 1, tokens: 10, cost: 0.5 }]}
      t={t}
    />,
  );
  const labels = labeledCells(container);
  const monday = labels.findIndex((label) => label.startsWith("weekdayMon"));
  const sunday = labels.findIndex((label) => label.startsWith("weekdaySun"));

  expect(monday).toBeGreaterThanOrEqual(0);
  expect(sunday).toBeGreaterThan(monday);
  expect(labels).toHaveLength(7 * 24);
});

test("keeps empty hours distinct from low-activity hours", () => {
  const { container } = render(
    <ActivityHeatmap
      cells={[{ weekday: 1, hour: 0, calls: 1, tokens: 100, cost: 0 }]}
      t={t}
    />,
  );
  const cells = [...container.querySelectorAll("[title]")];
  const filled = cells.find((el) => el.getAttribute("title")!.startsWith("weekdayMon 00:00"))!;
  const empty = cells.find((el) => el.getAttribute("title")!.startsWith("weekdayMon 01:00"))!;

  expect(empty.className).toContain("bg-muted");
  expect(filled.className).toContain("bg-accent");
  expect(filled.className).not.toContain("bg-muted");
});

test("falls back to the empty state without cells", () => {
  render(<ActivityHeatmap cells={[]} t={t} />);
  expect(screen.getByText("noUsageData")).toBeInTheDocument();
  expect(screen.queryByTestId("activity-heatmap")).toBeNull();
});
