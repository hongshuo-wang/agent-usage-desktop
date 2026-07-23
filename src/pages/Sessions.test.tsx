import { act, render, screen, waitFor, within } from "@testing-library/react";
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
    vi.clearAllMocks();
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

  it("does not reopen a collapsed row when a pending request resolves", async () => {
    const lateDetails = [{
      model: "late-model",
      calls: 1,
      input_tokens: 1,
      output_tokens: 1,
      cache_read: 0,
      cache_create: 0,
      cost_usd: 0,
    }];
    let resolveDetails!: (details: typeof lateDetails) => void;
    const pendingDetails = new Promise<typeof lateDetails>((resolve) => {
      resolveDetails = resolve;
    });
    vi.mocked(fetchRaw).mockReturnValueOnce(pendingDetails);
    const user = userEvent.setup();
    render(<Sessions />);

    const claudeRow = (await screen.findByText("claude")).closest("tr");
    if (!claudeRow) throw new Error("claude session row not rendered");
    const expandButton = within(claudeRow).getByRole("button");

    await user.click(expandButton);
    expect(await screen.findByText("Loading...")).toBeInTheDocument();
    await user.click(expandButton);
    expect(screen.queryByText("Loading...")).not.toBeInTheDocument();

    await act(async () => {
      resolveDetails(lateDetails);
      await pendingDetails;
    });
    expect(screen.queryByText("late-model")).not.toBeInTheDocument();
  });

  it("keeps a collapsed row closed when a pending request rejects", async () => {
    let rejectDetails!: (reason?: unknown) => void;
    const pendingDetails = new Promise<never>((_, reject) => {
      rejectDetails = reject;
    });
    vi.mocked(fetchRaw).mockReturnValueOnce(pendingDetails);
    const user = userEvent.setup();
    render(<Sessions />);

    const claudeRow = (await screen.findByText("claude")).closest("tr");
    if (!claudeRow) throw new Error("claude session row not rendered");
    const expandButton = within(claudeRow).getByRole("button");

    await user.click(expandButton);
    expect(await screen.findByText("Loading...")).toBeInTheDocument();
    await user.click(expandButton);

    await act(async () => {
      rejectDetails(new Error("late failure"));
      try {
        await pendingDetails;
      } catch {
        // The component handles this rejected detail request.
      }
    });
    expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
  });
});
