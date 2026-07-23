import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RawEventResponse, SessionEvent, SessionSummary } from "../lib/types";
import { fetchAPI, fetchRaw } from "../lib/api";
import sessionLayoutCSS from "../styles/globals.css?raw";
import Sessions from "./Sessions";

type Exact<A, B> = (<T>() => T extends A ? 1 : 2) extends (<T>() => T extends B ? 1 : 2) ? true : false;
type RequiredEventType = "user_message" | "assistant_message" | "reasoning" | "tool_call" | "tool_result" | "error" | "metadata" | "unknown";
const sessionEventTypeIsExact: Exact<SessionEvent["event_type"], RequiredEventType> = true;
void sessionEventTypeIsExact;

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../components/TimeRangeSelector", () => ({
  default: ({ source, onSourceChange }: { source: string; onSourceChange: (source: string) => void }) => (
    <div data-testid="time-range-selector">
      <output>{source || "allSources"}</output>
      <button type="button" onClick={() => onSourceChange("codex")}>chooseCodex</button>
    </div>
  ),
}));

vi.mock("../lib/api", () => ({ fetchAPI: vi.fn(), fetchRaw: vi.fn() }));

const summary = (overrides: Partial<SessionSummary> = {}): SessionSummary => ({
  source: "claude",
  session_id: "newest",
  title: "Newest investigation",
  project: "console",
  cwd: "/work/console",
  git_branch: "main",
  start_time: "2026-07-23T09:00:00Z",
  last_activity: "2026-07-23T10:00:00Z",
  models: ["sonnet"],
  input_tokens: 100,
  output_tokens: 40,
  cache_read: 20,
  cache_create: 5,
  total_tokens: 165,
  total_cost: 0.12,
  prompts: 3,
  tool_calls: 2,
  errors: 1,
  unknown_price: false,
  coverage_status: "complete",
  source_status: "available",
  malformed_lines: 0,
  ...overrides,
});

const event = (overrides: Partial<SessionEvent> = {}): SessionEvent => ({
  id: 1,
  event_type: "assistant_message",
  source_event_type: "message",
  timestamp: "2026-07-23T09:01:00Z",
  role: "assistant",
  content: "A useful answer",
  tool_name: "",
  tool_call_id: "",
  tool_input: "",
  tool_output: "",
  event_status: "",
  duration_ms: null,
  has_raw: true,
  ...overrides,
});

const events: SessionEvent[] = [
  event({ id: 1, event_type: "tool_call", tool_name: "shell", tool_input: '{"command":"npm test"}', content: "" }),
  event({ id: 2, event_type: "tool_result", tool_output: "x".repeat(320), content: "", has_raw: false }),
  event({ id: 3, event_type: "error", content: "Rate limit exceeded", event_status: "error", has_raw: false }),
  event({ id: 4, event_type: "assistant_message", content: "A useful answer" }),
];

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

function fullSessionPage(prefix: string): SessionSummary[] {
  return Array.from({ length: 50 }, (_, index) => summary({
    source: prefix === "codex" ? "codex" : "claude",
    session_id: `${prefix}-${index}`,
    title: `${prefix} session ${index}`,
  }));
}

function fullEventPage(prefix: string): SessionEvent[] {
  return Array.from({ length: 100 }, (_, index) => event({
    id: index + 1,
    content: `${prefix} event ${index}`,
    has_raw: false,
  }));
}

function setMobile(mobile: boolean) {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === "(max-width: 899px)" ? mobile : false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
}

function renderSessions(entry = "/sessions", mobile = false) {
  setMobile(mobile);
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes><Route path="/sessions" element={<Sessions />} /></Routes>
    </MemoryRouter>,
  );
}

function mockContracts(sessionRows: SessionSummary[] = [summary()], eventRows: SessionEvent[] = events) {
  vi.mocked(fetchAPI).mockImplementation(async (path) => {
    if (path === "sessions") return sessionRows as never;
    if (path.includes("/events")) return eventRows as never;
    throw new Error(`unexpected path ${path}`);
  });
  vi.mocked(fetchRaw).mockResolvedValue({
    path: "/tmp/session.jsonl",
    offset: 0,
    length: 16,
    content_type: "json",
    content: '{"raw":true}',
  } satisfies RawEventResponse);
}

describe("session retrospective center", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockContracts();
  });

  afterEach(() => { vi.useRealTimers(); });

  it("lists newest sessions first, selects the newest, and sends debounced search to q", async () => {
    vi.useFakeTimers();
    const older = summary({ session_id: "older", title: "Older session", last_activity: "2026-07-22T10:00:00Z" });
    mockContracts([older, summary()]);
    renderSessions();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });

    const items = screen.getAllByTestId("session-list-item");
    expect(items[0]).toHaveTextContent("Newest investigation");
    expect(screen.getByTestId("session-timeline")).toHaveTextContent("Newest investigation");

    fireEvent.change(screen.getByRole("searchbox", { name: "searchSessions" }), { target: { value: "needle" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(249); });
    expect(fetchAPI).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(fetchAPI).toHaveBeenCalledWith("sessions", expect.objectContaining({
      q: "needle", limit: 50, offset: 0,
    }), expect.objectContaining({ signal: expect.any(AbortSignal) }));
  });

  it("aborts list load-more when a filter starts a new full first page and restores the button", async () => {
    const pendingMore = deferred<SessionSummary[]>();
    let firstPageCalls = 0;
    let loadMoreSignal: AbortSignal | undefined;
    vi.mocked(fetchAPI).mockImplementation((path, params, init) => {
      if (path.includes("/events")) return Promise.resolve([]) as never;
      if (params.offset === 50) {
        loadMoreSignal = init?.signal ?? undefined;
        return pendingMore.promise as never;
      }
      firstPageCalls += 1;
      return Promise.resolve(fullSessionPage(firstPageCalls === 1 ? "claude" : "codex")) as never;
    });
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByRole("button", { name: "loadMoreSessions" }));
    expect(loadMoreSignal?.aborted).toBe(false);

    await user.click(screen.getByRole("button", { name: "chooseCodex" }));
    await waitFor(() => expect(loadMoreSignal?.aborted).toBe(true));
    await screen.findAllByText("codex session 0");
    const restored = screen.getByRole("button", { name: "loadMoreSessions" });
    expect(restored).toBeEnabled();
    expect(restored).toHaveTextContent("loadMoreSessions");
  });

  it("shows inherited Dashboard filters and clears all drill-down context at once", async () => {
    const user = userEvent.setup();
    renderSessions("/sessions?from=2026-07-01&to=2026-07-03&source=claude&model=sonnet&project=console");

    const context = await screen.findByTestId("session-filter-context");
    for (const value of ["2026-07-01", "2026-07-03", "claude", "sonnet", "console"]) {
      expect(context).toHaveTextContent(value);
    }
    expect(fetchAPI).toHaveBeenCalledWith("sessions", expect.objectContaining({
      from: "2026-07-01", to: "2026-07-03", source: "claude", model: "sonnet", project: "console",
    }), expect.anything());

    await user.click(within(context).getByRole("button", { name: "clearAllFilters" }));
    await waitFor(() => expect(screen.queryByTestId("session-filter-context")).not.toBeInTheDocument());
  });

  it("sends user-editable model and project filters to the backend and allows clearing them", async () => {
    renderSessions("/sessions?from=2026-07-01&to=2026-07-03&model=sonnet&project=console");
    const initialListSignal = vi.mocked(fetchAPI).mock.calls.find(([path]) => path === "sessions")?.[2]?.signal;
    const modelInput = await screen.findByRole("textbox", { name: "modelFilter" });
    const projectInput = screen.getByRole("textbox", { name: "projectFilter" });
    expect(modelInput).toHaveValue("sonnet");
    expect(projectInput).toHaveValue("console");

    fireEvent.change(modelInput, { target: { value: "opus" } });
    fireEvent.change(projectInput, { target: { value: "dashboard" } });
    await waitFor(() => expect(fetchAPI).toHaveBeenCalledWith("sessions", expect.objectContaining({
      model: "opus", project: "dashboard",
    }), expect.objectContaining({ signal: expect.any(AbortSignal) })));
    expect(initialListSignal?.aborted).toBe(true);

    fireEvent.change(modelInput, { target: { value: "" } });
    fireEvent.change(projectInput, { target: { value: "" } });
    await waitFor(() => expect(fetchAPI).toHaveBeenCalledWith("sessions", expect.objectContaining({
      model: undefined, project: undefined,
    }), expect.objectContaining({ signal: expect.any(AbortSignal) })));
  });

  it("shows list duration and the complete source-backed session header", async () => {
    mockContracts([summary({
      coverage_status: "partial",
      start_time: "2026-07-23T08:30:00Z",
      last_activity: "2026-07-23T10:00:00Z",
      models: ["sonnet", "haiku"],
    })]);
    renderSessions();

    const listItem = (await screen.findAllByTestId("session-list-item"))[0];
    expect(listItem).toHaveTextContent("1h 30m");
    const timeline = screen.getByTestId("session-timeline");
    expect(within(timeline).getByText("Newest investigation")).toBeVisible();
    for (const [label, value] of [
      ["agent", "claude"],
      ["project", "console"],
      ["branch", "main"],
      ["startTime", "2026-07-23T08:30:00Z"],
      ["models", "sonnet, haiku"],
      ["totalTokens", "165"],
      ["inputTokens", "100"],
      ["outputTokens", "40"],
      ["cacheRead", "20"],
      ["cacheCreate", "5"],
      ["toolCalls", "2"],
      ["estimatedCost", "$0.1200"],
    ]) {
      const labelElement = within(timeline).getByText(label);
      expect(labelElement.parentElement).toHaveTextContent(value);
    }
    expect(within(timeline).getByText("sessionPartial")).toBeVisible();
  });

  it("collapses tool calls and long results by default while expanding errors", async () => {
    renderSessions();
    const timeline = await screen.findByTestId("session-timeline");
    await within(timeline).findByText("Rate limit exceeded");

    expect(within(timeline).queryByText('{"command":"npm test"}')).not.toBeInTheDocument();
    expect(within(timeline).queryByText("x".repeat(320))).not.toBeInTheDocument();
    expect(within(timeline).getByText("Rate limit exceeded")).toBeVisible();
  });

  it("opens the inspector on event click and restores center width when closed", async () => {
    const user = userEvent.setup();
    renderSessions();
    const answer = await screen.findByText("A useful answer");

    await user.click(answer);
    expect(screen.getByTestId("event-inspector")).toHaveTextContent("A useful answer");
    expect(screen.getByTestId("event-inspector")).toHaveClass("session-event-inspector");
    expect(screen.getByTestId("session-center-grid")).toHaveAttribute("data-inspector-open", "true");

    await user.click(screen.getByRole("button", { name: "closeInspector" }));
    expect(screen.queryByTestId("event-inspector")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-center-grid")).toHaveAttribute("data-inspector-open", "false");
  });

  it("uses a two-column inspector overlay from 900px to 1099px and three columns from 1100px", () => {
    expect(sessionLayoutCSS).toContain("position: relative");
    expect(sessionLayoutCSS).toContain("@media (min-width: 900px) and (max-width: 1099px)");
    expect(sessionLayoutCSS).toContain("@media (min-width: 1100px)");
    expect(sessionLayoutCSS).toContain(".session-event-inspector");
  });

  it("does not inspect an event when Enter or Space originates on its collapse button", async () => {
    renderSessions();
    const expand = (await screen.findAllByRole("button", { name: "expandEvent" }))[0];
    fireEvent.keyDown(expand, { key: "Enter" });
    fireEvent.keyDown(expand, { key: " " });
    expect(screen.queryByTestId("event-inspector")).not.toBeInTheDocument();
  });

  it("shows the localized fallback for an invalid event timestamp", async () => {
    mockContracts([summary()], [event({ id: 22, timestamp: "not-a-timestamp", has_raw: false })]);
    renderSessions();
    const card = await screen.findByTestId("event-card-22");
    expect(within(card).getByText("sourceDataUnavailable")).toBeVisible();
    expect(within(card).queryByText("Invalid Date")).not.toBeInTheDocument();
  });

  it("shows every normalized event field with labels and exact source values", async () => {
    const detailed = event({
      id: 12,
      event_type: "tool_call",
      source_event_type: "response_item:function_call",
      timestamp: "2026-07-23T09:02:03Z",
      role: "assistant",
      content: "provided content",
      tool_name: "shell",
      tool_call_id: "call-7",
      tool_input: '{"command":"go test ./..."}',
      tool_output: "provided output",
      event_status: "success",
      duration_ms: 125,
      has_raw: false,
    });
    mockContracts([summary()], [detailed]);
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByText("eventCollapsed"));
    const inspector = screen.getByTestId("event-inspector");
    for (const [label, value] of [
      ["eventType", "tool_call"],
      ["sourceEventType", "response_item:function_call"],
      ["timestamp", "2026-07-23T09:02:03Z"],
      ["eventStatus", "success"],
      ["duration", "125 ms"],
      ["toolCallId", "call-7"],
      ["content", "provided content"],
      ["toolInput", '{"command":"go test ./..."}'],
      ["toolOutput", "provided output"],
    ]) {
      const labelElement = within(inspector).getByText(label);
      expect(labelElement.parentElement).toHaveTextContent(value);
    }
    expect(fetchRaw).not.toHaveBeenCalled();
  });

  it("does not request raw data until Raw record is explicitly clicked and caches it for the session", async () => {
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByText("A useful answer"));
    expect(fetchRaw).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "loadRawRecord" }));
    expect(await screen.findByText('{"raw":true}')).toBeVisible();
    expect(fetchRaw).toHaveBeenCalledWith(
      "sessions/claude/newest/events/4/raw",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    await user.click(screen.getByRole("button", { name: "closeInspector" }));
    await user.click(screen.getByText("A useful answer"));
    expect(screen.getByText('{"raw":true}')).toBeVisible();
    expect(fetchRaw).toHaveBeenCalledTimes(1);
  });

  it("clears the inspector and raw cache when search switches the selected session", async () => {
    const older = summary({ session_id: "older", title: "Older matching session", last_activity: "2026-07-22T10:00:00Z" });
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "sessions") return (params.q ? [older] : [summary()]) as never;
      return [event({ id: 4, event_type: "assistant_message", content: "A useful answer" })] as never;
    });
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByText("A useful answer"));
    await user.click(screen.getByRole("button", { name: "loadRawRecord" }));
    expect(await screen.findByText('{"raw":true}')).toBeVisible();

    await user.type(screen.getByRole("searchbox", { name: "searchSessions" }), "older");
    const timeline = screen.getByTestId("session-timeline");
    await within(timeline).findByText("Older matching session", {}, { timeout: 1000 });
    expect(screen.queryByTestId("event-inspector")).not.toBeInTheDocument();

    await user.click(await within(timeline).findByText("A useful answer"));
    expect(screen.queryByText('{"raw":true}')).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "loadRawRecord" }));
    await waitFor(() => expect(fetchRaw).toHaveBeenCalledTimes(2));
    expect(fetchRaw).toHaveBeenLastCalledWith("sessions/claude/older/events/4/raw", expect.anything());
  });

  it.each(["resolve", "reject"] as const)("isolates raw event B when aborted event A later %s", async (outcome) => {
    const rawA = deferred<RawEventResponse>();
    const rawB = deferred<RawEventResponse>();
    const rawSignals: AbortSignal[] = [];
    vi.mocked(fetchRaw).mockReset();
    vi.mocked(fetchRaw)
      .mockImplementationOnce((_path, init) => { rawSignals.push(init?.signal as AbortSignal); return rawA.promise; })
      .mockImplementationOnce((_path, init) => { rawSignals.push(init?.signal as AbortSignal); return rawB.promise; });
    mockContracts([summary()], [
      event({ id: 31, content: "Raw event A" }),
      event({ id: 32, content: "Raw event B" }),
    ]);
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByText("Raw event A"));
    await user.click(screen.getByRole("button", { name: "loadRawRecord" }));
    await user.click(screen.getByText("Raw event B"));
    expect(rawSignals[0].aborted).toBe(true);
    expect(screen.getByRole("button", { name: "loadRawRecord" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "loadRawRecord" }));
    expect(screen.getByRole("button", { name: "loadRawRecord" })).toBeDisabled();
    await act(async () => {
      if (outcome === "resolve") {
        rawA.resolve({ path: "/a", offset: 0, length: 1, content_type: "text", content: "late A" });
      } else {
        rawA.reject(new Error("late A failure"));
      }
      await Promise.resolve();
    });
    expect(screen.queryByText(/late A/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "loadRawRecord" })).toBeDisabled();

    await act(async () => rawB.resolve({ path: "/b", offset: 0, length: 1, content_type: "text", content: "raw B payload" }));
    expect(await screen.findByText("raw B payload")).toBeVisible();
  });

  it("aborts obsolete list and event requests and qualifies colliding session IDs by source", async () => {
    const controllers: AbortSignal[] = [];
    let listCall = 0;
    vi.mocked(fetchAPI).mockImplementation((path, _params, init) => {
      if (path === "sessions") {
        listCall += 1;
        if (listCall === 1) {
          controllers.push(init?.signal as AbortSignal);
          return new Promise((_, reject) => init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")))) as never;
        }
        return Promise.resolve([
          summary({ source: "claude", session_id: "shared", title: "Claude shared" }),
          summary({ source: "codex", session_id: "shared", title: "Codex shared" }),
        ]) as never;
      }
      controllers.push(init?.signal as AbortSignal);
      return new Promise((_, reject) => init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")))) as never;
    });
    const user = userEvent.setup();
    renderSessions();
    await user.click(screen.getByRole("button", { name: "chooseCodex" }));
    await waitFor(() => expect(controllers[0].aborted).toBe(true));
    await user.click(await screen.findByText("Codex shared"));
    expect(fetchAPI).toHaveBeenCalledWith(
      "sessions/codex/shared/events",
      { limit: 100, offset: 0 },
      expect.anything(),
    );
  });

  it("replaces the list with detail below 900px and restores it with Back", async () => {
    const user = userEvent.setup();
    mockContracts([summary(), summary({ session_id: "older", title: "Older session" })]);
    renderSessions("/sessions", true);
    expect(await screen.findByTestId("session-list")).toBeVisible();

    await user.click(screen.getByText("Older session"));
    expect(screen.queryByTestId("session-list")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-timeline")).toHaveTextContent("Older session");
    await user.click(screen.getByRole("button", { name: "backToSessions" }));
    expect(screen.getByTestId("session-list")).toBeVisible();
  });

  it("distinguishes stats-only, partial, missing, malformed, and unknown-price states", async () => {
    mockContracts([
      summary({ session_id: "stats", title: "Stats", coverage_status: "stats_only", source_status: "stats_only" }),
      summary({ session_id: "partial", title: "Partial", coverage_status: "partial" }),
      summary({ session_id: "missing", title: "Missing", source_status: "missing_source" }),
      summary({ session_id: "malformed", title: "Malformed", malformed_lines: 4 }),
      summary({ session_id: "price", title: "Price", unknown_price: true }),
    ]);
    renderSessions();

    await screen.findAllByText("Stats");
    for (const key of ["sessionStatsOnly", "sessionPartial", "sessionMissingSource", "sessionMalformed", "sessionUnknownPrice"]) {
      expect(screen.getAllByText(key)[0]).toBeVisible();
    }
  });

  it("renders source-provided fields only and localizes unavailable data", async () => {
    const user = userEvent.setup();
    mockContracts([summary()], [event({ id: 8, content: "", role: "assistant", has_raw: false })]);
    renderSessions();
    const unavailable = await screen.findByText("sourceDataUnavailable");
    expect(unavailable).toBeVisible();
    await user.click(unavailable);
    const inspector = screen.getByTestId("event-inspector");
    for (const label of ["eventStatus", "duration", "toolCallId", "content", "toolInput", "toolOutput"]) {
      const labelElement = within(inspector).getByText(label);
      expect(labelElement.parentElement).toHaveTextContent("sourceDataUnavailable");
    }
    expect(screen.queryByText(/system prompt|request body|tool schema/i)).not.toBeInTheDocument();
  });

  it("renders loading, empty, and error states", async () => {
    let resolve!: (rows: SessionSummary[]) => void;
    vi.mocked(fetchAPI).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    const view = renderSessions();
    expect(screen.getByText("loadingSessions")).toBeVisible();
    await act(async () => resolve([]));
    expect(await screen.findByText("noSessionsFound")).toBeVisible();

    view.unmount();
    vi.mocked(fetchAPI).mockRejectedValueOnce(new Error("offline"));
    renderSessions();
    expect(await screen.findByText("offline")).toBeVisible();
    expect(screen.getByRole("button", { name: "retry" })).toBeVisible();
  });

  it("loads chronological events in 100-item pages and can load more", async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => event({
      id: index + 1,
      timestamp: `2026-07-23T09:${String(99 - index).padStart(2, "0")}:00Z`,
      content: `event-${index + 1}`,
      has_raw: false,
    }));
    vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
      if (path === "sessions") return [summary()] as never;
      if (params.offset === 100) return [event({ id: 101, content: "event-101", has_raw: false })] as never;
      return firstPage as never;
    });
    renderSessions();
    expect(await screen.findByText("event-100")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "loadMoreEvents" }));
    expect(await screen.findByText("event-101")).toBeVisible();
    expect(fetchAPI).toHaveBeenCalledWith(expect.stringContaining("/events"), { limit: 100, offset: 100 }, expect.anything());
  });

  it("aborts event load-more when selection starts a new full first page and restores the button", async () => {
    const pendingMore = deferred<SessionEvent[]>();
    let loadMoreSignal: AbortSignal | undefined;
    vi.mocked(fetchAPI).mockImplementation((path, params, init) => {
      if (path === "sessions") return Promise.resolve([
        summary(),
        summary({ session_id: "older", title: "Older session" }),
      ]) as never;
      if (params.offset === 100) {
        loadMoreSignal = init?.signal ?? undefined;
        return pendingMore.promise as never;
      }
      return Promise.resolve(fullEventPage(path.includes("/older/") ? "older" : "newest")) as never;
    });
    const user = userEvent.setup();
    renderSessions();
    await user.click(await screen.findByRole("button", { name: "loadMoreEvents" }));
    expect(loadMoreSignal?.aborted).toBe(false);

    await user.click(screen.getByText("Older session"));
    await waitFor(() => expect(loadMoreSignal?.aborted).toBe(true));
    const timeline = screen.getByTestId("session-timeline");
    await within(timeline).findByText("older event 0");
    expect(within(timeline).getByRole("button", { name: "loadMoreEvents" })).toBeEnabled();
  });
});
