import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Sessions from "./Sessions";
import { fetchAPI, fetchRaw } from "../lib/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../components/TimeRangeSelector", () => ({
  default: () => <div data-testid="time-range-selector" />,
}));

vi.mock("../lib/api", () => ({
  fetchAPI: vi.fn(),
  fetchRaw: vi.fn(),
}));

const sessions = ["claude", "codex"].map((source) => ({
  session_id: "shared-session",
  source,
  project: `${source}-project`,
  cwd: "",
  git_branch: "main",
  start_time: "2025-01-01T12:00:00Z",
  prompts: 1,
  tokens: 10,
  total_cost: 0.01,
}));

describe("Sessions detail requests", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.mocked(fetchAPI).mockResolvedValue(sessions);
    vi.mocked(fetchRaw).mockResolvedValue([]);
  });

  it("qualifies colliding session ids by source", async () => {
    const user = userEvent.setup();
    render(<Sessions />);

    const claudeRow = (await screen.findByText("claude")).closest("tr");
    const codexRow = screen.getByText("codex").closest("tr");
    if (!claudeRow || !codexRow) throw new Error("session rows not rendered");

    await user.click(within(claudeRow).getByRole("button"));
    await waitFor(() => expect(fetchRaw).toHaveBeenCalledWith(
      "session-detail?source=claude&session_id=shared-session",
    ));

    await user.click(within(codexRow).getByRole("button"));
    await waitFor(() => expect(fetchRaw).toHaveBeenCalledWith(
      "session-detail?source=codex&session_id=shared-session",
    ));
    expect(fetchRaw).toHaveBeenCalledTimes(2);
  });
});
