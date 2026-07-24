import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { fetchAPI } from "../lib/api";
import * as settingsHydration from "./settingsHydration";
import Settings from "./Settings";

const changeLanguage = vi.fn();

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en", changeLanguage },
  }),
}));

vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn() }));
vi.mock("../lib/api", () => ({ fetchAPI: vi.fn() }));

const settings = {
  collectors: [
    { name: "claude", enabled: true, paths: ["/claude/a", "/claude/b"], scan_interval: "1m0s" },
    { name: "codex", enabled: true, paths: ["/codex"], scan_interval: "1m0s" },
    { name: "openclaw", enabled: true, paths: ["/openclaw"], scan_interval: "2m0s" },
    { name: "opencode", enabled: false, paths: ["/opencode.db"], scan_interval: "5m0s" },
  ],
  pricing_sync_interval: "1h0m0s",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function mockDesktopSettings() {
  vi.mocked(invoke).mockImplementation(async (command) => {
    if (command === "get_cost_threshold") return 10;
    if (command === "plugin:autostart|is_enabled") return false;
    if (command === "get_notifications_enabled") return true;
    if (command === "restart_sidecar") return 9900;
    return undefined;
  });
}

describe("application settings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    document.documentElement.classList.remove("dark");
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false }),
    });
    mockDesktopSettings();
    vi.mocked(fetchAPI).mockImplementation(async (path, _params, init) => {
      if (path === "settings/collectors" && init?.method === "PUT") return { restart_required: true } as never;
      if (path === "settings/collectors") return settings as never;
      if (path === "session-index/rebuild") return { status: "rebuild_required", sources: 4 } as never;
      throw new Error(`unexpected path ${path}`);
    });
  });

  it("renders only app settings with collector capabilities and multi-path values", async () => {
    render(<Settings />);
    expect(screen.getByText("loadingSettings")).toBeVisible();

    expect(await screen.findByRole("switch", { name: "collectorEnabled claudeCode" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "collectorEnabled codex" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "collectorEnabled openClaw" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "collectorEnabled openCode" })).not.toBeChecked();
    expect(screen.getAllByText("fullRetrospective")).toHaveLength(2);
    expect(screen.getAllByText("statisticsOnly")).toHaveLength(2);
    expect(screen.getByRole("textbox", { name: "collectorPaths claudeCode" })).toHaveValue("/claude/a\n/claude/b");
    expect(screen.getByRole("textbox", { name: "pricingSyncInterval" })).toHaveValue("1h0m0s");
    expect(screen.getByRole("switch", { name: "notification" })).toBeChecked();
    expect(screen.getByRole("spinbutton", { name: "dailyCostThreshold" })).toHaveValue(10);

    expect(screen.queryByText(/hermes|provider|mcp|skills|backup|account|team/i)).not.toBeInTheDocument();
  });

  it("saves edited collector settings and restarts only after the PUT succeeds", async () => {
    const user = userEvent.setup();
    render(<Settings />);
    const claudeToggle = await screen.findByRole("switch", { name: "collectorEnabled claudeCode" });
    await user.click(claudeToggle);
    fireEvent.change(screen.getByRole("textbox", { name: "collectorPaths claudeCode" }), {
      target: { value: "/new/one\n/new/two" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "scanInterval claudeCode" }), {
      target: { value: "30s" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "pricingSyncInterval" }), {
      target: { value: "2h" },
    });
    await user.click(screen.getByRole("button", { name: "saveCollectorSettings" }));

    await waitFor(() => expect(screen.getByText("settingsSavedAndRestarted")).toBeVisible());
    const put = vi.mocked(fetchAPI).mock.calls.find(([path, , init]) => path === "settings/collectors" && init?.method === "PUT");
    expect(put).toBeDefined();
    const body = JSON.parse(String(put?.[2]?.body));
    expect(body.collectors[0]).toEqual({
      name: "claude", enabled: false, paths: ["/new/one", "/new/two"], scan_interval: "30s",
    });
    expect(body.pricing_sync_interval).toBe("2h");
    expect(invoke).toHaveBeenCalledWith("restart_sidecar");
  });

  it("shows save errors and does not restart after a failed PUT", async () => {
    vi.mocked(fetchAPI).mockImplementation(async (path, _params, init) => {
      if (path === "settings/collectors" && init?.method === "PUT") throw new Error("save rejected");
      if (path === "settings/collectors") return settings as never;
      throw new Error(`unexpected path ${path}`);
    });
    render(<Settings />);
    await screen.findByRole("button", { name: "saveCollectorSettings" });
    fireEvent.click(screen.getByRole("button", { name: "saveCollectorSettings" }));

    expect(await screen.findByText("save rejected")).toBeVisible();
    expect(invoke).not.toHaveBeenCalledWith("restart_sidecar");
  });

  it("retries only the restart when settings were saved but not applied", async () => {
    let restartCalls = 0;
    vi.mocked(invoke).mockImplementation(async (command) => {
      if (command === "get_cost_threshold") return 10;
      if (command === "plugin:autostart|is_enabled") return false;
      if (command === "get_notifications_enabled") return true;
      if (command === "restart_sidecar") {
        restartCalls += 1;
        if (restartCalls === 1) throw new Error("restart rejected");
        return 9900;
      }
      return undefined;
    });
    const user = userEvent.setup();
    render(<Settings />);
    await user.click(await screen.findByRole("button", { name: "saveCollectorSettings" }));

    expect(await screen.findByText("settingsSavedRestartFailed")).toBeVisible();
    expect(screen.getByText("restart rejected")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "retryRestart" }));

    expect(await screen.findByText("settingsSavedAndRestarted")).toBeVisible();
    const putCalls = vi.mocked(fetchAPI).mock.calls.filter(([path, , init]) => (
      path === "settings/collectors" && init?.method === "PUT"
    ));
    expect(putCalls).toHaveLength(1);
    expect(restartCalls).toBe(2);
  });

  it("retries only the restart when an index rebuild completed but was not applied", async () => {
    let restartCalls = 0;
    vi.mocked(invoke).mockImplementation(async (command) => {
      if (command === "get_cost_threshold") return 10;
      if (command === "plugin:autostart|is_enabled") return false;
      if (command === "get_notifications_enabled") return true;
      if (command === "restart_sidecar") {
        restartCalls += 1;
        if (restartCalls === 1) throw new Error("restart rejected");
        return 9900;
      }
      return undefined;
    });
    const user = userEvent.setup();
    render(<Settings />);
    await user.click(await screen.findByRole("button", { name: "rebuildSessionIndex" }));
    await user.click(screen.getByRole("button", { name: "confirmRebuild" }));

    expect(await screen.findByText("rebuildCompletedRestartFailed")).toBeVisible();
    expect(screen.getByText("restart rejected")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "retryRestart" }));

    expect(await screen.findByText("rebuildStartedAndRestarted")).toBeVisible();
    const rebuildCalls = vi.mocked(fetchAPI).mock.calls.filter(([path]) => path === "session-index/rebuild");
    expect(rebuildCalls).toHaveLength(1);
    expect(restartCalls).toBe(2);
  });

  it.each([
    ["success", false],
    ["failure", true],
  ])("requires rebuild confirmation and handles %s without premature restart", async (_name, fail) => {
    if (fail) {
      vi.mocked(fetchAPI).mockImplementation(async (path) => {
        if (path === "settings/collectors") return settings as never;
        if (path === "session-index/rebuild") throw new Error("rebuild rejected");
        throw new Error(`unexpected path ${path}`);
      });
    }
    const user = userEvent.setup();
    render(<Settings />);
    await screen.findByRole("button", { name: "rebuildSessionIndex" });
    await user.click(screen.getByRole("button", { name: "rebuildSessionIndex" }));
    const dialog = screen.getByRole("dialog", { name: "confirmRebuildTitle" });
    expect(fetchAPI).not.toHaveBeenCalledWith("session-index/rebuild", expect.anything(), expect.anything());

    await user.click(within(dialog).getByRole("button", { name: "confirmRebuild" }));
    if (fail) {
      expect(await screen.findByText("rebuild rejected")).toBeVisible();
      expect(invoke).not.toHaveBeenCalledWith("restart_sidecar");
    } else {
      await waitFor(() => expect(invoke).toHaveBeenCalledWith("restart_sidecar"));
      expect(screen.getByText("rebuildStartedAndRestarted")).toBeVisible();
    }
  });

  it("supports segmented language/theme controls and app notification settings", async () => {
    const user = userEvent.setup();
    render(<Settings />);
    await screen.findByRole("button", { name: "dark" });
    await user.click(screen.getByRole("button", { name: "dark" }));
    expect(screen.getByRole("button", { name: "dark" })).toHaveAttribute("aria-pressed", "true");
    expect(document.documentElement).toHaveClass("dark");
    await user.click(screen.getByRole("button", { name: "中文" }));
    expect(changeLanguage).toHaveBeenCalledWith("zh");

    await user.click(screen.getByRole("switch", { name: "notification" }));
    expect(invoke).toHaveBeenCalledWith("set_notifications_enabled", { enabled: false });
    fireEvent.change(screen.getByRole("spinbutton", { name: "dailyCostThreshold" }), { target: { value: "25" } });
    expect(invoke).toHaveBeenCalledWith("set_cost_threshold", { threshold: 25 });
  });

  it("serializes preference toggles and rolls back failed updates", async () => {
    const notificationUpdate = deferred<unknown>();
    const autostartUpdate = deferred<unknown>();
    vi.mocked(invoke).mockImplementation((command) => {
      if (command === "get_cost_threshold") return Promise.resolve(10);
      if (command === "plugin:autostart|is_enabled") return Promise.resolve(false);
      if (command === "get_notifications_enabled") return Promise.resolve(true);
      if (command === "set_notifications_enabled") return notificationUpdate.promise;
      if (command === "plugin:autostart|enable") return autostartUpdate.promise;
      return Promise.resolve(undefined);
    });
    const user = userEvent.setup();
    render(<Settings />);

    const notification = await screen.findByRole("switch", { name: "notification" });
    await user.click(notification);
    expect(notification).toBeDisabled();
    fireEvent.click(notification);
    expect(vi.mocked(invoke).mock.calls.filter(([command]) => command === "set_notifications_enabled")).toHaveLength(1);
    notificationUpdate.reject(new Error("notification rejected"));
    await waitFor(() => expect(notification).toBeChecked());
    expect(screen.getByText("notificationUpdateFailed")).toBeVisible();

    const autostart = screen.getByRole("switch", { name: "autostart" });
    await user.click(autostart);
    expect(autostart).toBeDisabled();
    fireEvent.click(autostart);
    expect(vi.mocked(invoke).mock.calls.filter(([command]) => command === "plugin:autostart|enable")).toHaveLength(1);
    autostartUpdate.reject(new Error("autostart rejected"));
    await waitFor(() => expect(autostart).not.toBeChecked());
    expect(screen.getByText("autostartUpdateFailed")).toBeVisible();
  });

  it("does not let slow desktop hydration overwrite successful user changes", async () => {
    const initialThreshold = deferred<number>();
    const initialAutostart = deferred<boolean>();
    const initialNotifications = deferred<boolean>();
    vi.mocked(invoke).mockImplementation((command) => {
      if (command === "get_cost_threshold") return initialThreshold.promise;
      if (command === "plugin:autostart|is_enabled") return initialAutostart.promise;
      if (command === "get_notifications_enabled") return initialNotifications.promise;
      return Promise.resolve(undefined);
    });
    const user = userEvent.setup();
    render(<Settings />);

    const autostart = screen.getByRole("switch", { name: "autostart" });
    const notification = screen.getByRole("switch", { name: "notification" });
    const threshold = screen.getByRole("spinbutton", { name: "dailyCostThreshold" });
    await user.click(autostart);
    await user.click(notification);
    fireEvent.change(threshold, { target: { value: "25" } });
    expect(autostart).toBeChecked();
    expect(notification).not.toBeChecked();
    expect(threshold).toHaveValue(25);

    await act(async () => {
      initialAutostart.resolve(false);
      initialNotifications.resolve(true);
      initialThreshold.resolve(10);
      await Promise.all([
        initialAutostart.promise,
        initialNotifications.promise,
        initialThreshold.promise,
      ]);
    });
    expect(autostart).toBeChecked();
    expect(notification).not.toBeChecked();
    expect(threshold).toHaveValue(25);
  });

  it("settles desktop hydration as unmounted without writing state", async () => {
    const initialThreshold = deferred<number>();
    const initialAutostart = deferred<boolean>();
    const initialNotifications = deferred<boolean>();
    vi.mocked(invoke).mockImplementation((command) => {
      if (command === "get_cost_threshold") return initialThreshold.promise;
      if (command === "plugin:autostart|is_enabled") return initialAutostart.promise;
      if (command === "get_notifications_enabled") return initialNotifications.promise;
      return Promise.resolve(undefined);
    });
    const applySpy = vi.spyOn(settingsHydration, "applyOwnedHydration");
    const view = render(<Settings />);
    view.unmount();

    await act(async () => {
      initialThreshold.resolve(10);
      initialAutostart.resolve(false);
      initialNotifications.resolve(true);
      await Promise.all([
        initialThreshold.promise,
        initialAutostart.promise,
        initialNotifications.promise,
      ]);
    });

    expect(applySpy).toHaveBeenCalledTimes(3);
    expect(applySpy.mock.calls.every(([mounted]) => mounted === false)).toBe(true);
    applySpy.mockRestore();
  });

  it("renders a retryable collector settings error", async () => {
    let calls = 0;
    vi.mocked(fetchAPI).mockImplementation(async (path) => {
      if (path !== "settings/collectors") throw new Error(`unexpected path ${path}`);
      calls += 1;
      if (calls === 1) throw new Error("settings offline");
      return settings as never;
    });
    const user = userEvent.setup();
    render(<Settings />);
    expect(await screen.findByText("settings offline")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "retry" }));
    expect(await screen.findByRole("switch", { name: "collectorEnabled claudeCode" })).toBeVisible();
  });

  it("aborts superseded collector loads and ignores stale responses", async () => {
    const first = deferred<typeof settings>();
    const second = deferred<typeof settings>();
    const signals: AbortSignal[] = [];
    let calls = 0;
    vi.mocked(fetchAPI).mockImplementation((path, _params, init) => {
      if (path !== "settings/collectors") return Promise.reject(new Error(`unexpected path ${path}`));
      signals.push(init?.signal as AbortSignal);
      calls += 1;
      return (calls === 1 ? first.promise : second.promise) as never;
    });
    render(<StrictMode><Settings /></StrictMode>);
    await waitFor(() => expect(signals).toHaveLength(2));
    expect(signals[0]).toBeInstanceOf(AbortSignal);
    expect(signals[0].aborted).toBe(true);
    expect(signals[1].aborted).toBe(false);

    second.resolve({
      ...settings,
      collectors: settings.collectors.map((collector) => (
        collector.name === "claude" ? { ...collector, enabled: false } : collector
      )),
    });
    expect(await screen.findByRole("switch", { name: "collectorEnabled claudeCode" })).not.toBeChecked();
    first.resolve(settings);
    await waitFor(() => expect(screen.getByRole("switch", { name: "collectorEnabled claudeCode" })).not.toBeChecked());
  });

  it("aborts collector loading on unmount", async () => {
    const pending = deferred<typeof settings>();
    let signal: AbortSignal | undefined;
    vi.mocked(fetchAPI).mockImplementation((_path, _params, init) => {
      signal = init?.signal as AbortSignal;
      return pending.promise as never;
    });
    const view = render(<Settings />);
    await waitFor(() => expect(signal).toBeInstanceOf(AbortSignal));
    view.unmount();
    expect(signal?.aborted).toBe(true);
    pending.resolve(settings);
  });

  it("traps focus in the rebuild dialog and restores it after Escape", async () => {
    const user = userEvent.setup();
    render(<Settings />);
    const trigger = await screen.findByRole("button", { name: "rebuildSessionIndex" });
    await user.click(trigger);
    const dialog = screen.getByRole("dialog", { name: "confirmRebuildTitle" });
    const cancel = within(dialog).getByRole("button", { name: "cancel" });
    const confirm = within(dialog).getByRole("button", { name: "confirmRebuild" });

    expect(cancel).toHaveFocus();
    await user.tab();
    expect(confirm).toHaveFocus();
    await user.tab();
    expect(cancel).toHaveFocus();
    await user.tab({ shift: true });
    expect(confirm).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("restores focus to the rebuild trigger after confirmation", async () => {
    const rebuild = deferred<never>();
    vi.mocked(fetchAPI).mockImplementation((path) => {
      if (path === "settings/collectors") return Promise.resolve(settings) as never;
      if (path === "session-index/rebuild") return rebuild.promise;
      return Promise.reject(new Error(`unexpected path ${path}`));
    });
    const user = userEvent.setup();
    render(<Settings />);
    const trigger = await screen.findByRole("button", { name: "rebuildSessionIndex" });
    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: "confirmRebuild" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });
});
