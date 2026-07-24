import { useCallback, useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { AlertTriangle, Database, RefreshCw, Save } from "lucide-react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import type {
  CollectorName,
  CollectorSetting,
  CollectorSettings,
  SessionIndexRebuildResponse,
  SettingsUpdateResponse,
} from "../lib/types";

type EditableCollector = CollectorSetting & { pathsText: string };
type ActionState = "idle" | "pending" | "success" | "error";
type SaveState = ActionState | "restartPending" | "restartError";

const COLLECTOR_LABELS: Record<CollectorName, string> = {
  claude: "claudeCode",
  codex: "codex",
  openclaw: "openClaw",
  opencode: "openCode",
};

const FULL_RETROSPECTIVE = new Set<CollectorName>(["claude", "codex"]);

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function Toggle({
  checked,
  disabled = false,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={onChange}
      className={`relative h-6 w-11 shrink-0 rounded-full border transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${checked ? "border-accent bg-accent" : "border-border bg-muted"}`}
    >
      <span className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-[left] ${checked ? "left-5" : "left-0.5"}`} />
    </button>
  );
}

function SegmentedControl({
  value,
  options,
  onChange,
}: {
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
}) {
  return (
    <div className="inline-flex max-w-full overflow-x-auto rounded border border-border p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className={`px-3 py-1.5 text-xs font-medium transition-colors ${value === option.value ? "bg-accent text-white" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

export default function Settings() {
  const { t, i18n } = useTranslation();
  const [autostart, setAutostart] = useState(false);
  const [theme, setTheme] = useState(localStorage.getItem("au-theme") || "system");
  const [costThreshold, setCostThreshold] = useState(10);
  const [notificationsEnabled, setNotificationsEnabled] = useState(true);
  const [notificationsPending, setNotificationsPending] = useState(false);
  const [notificationError, setNotificationError] = useState<string | null>(null);
  const [autostartPending, setAutostartPending] = useState(false);
  const [autostartError, setAutostartError] = useState<string | null>(null);
  const [collectors, setCollectors] = useState<EditableCollector[] | null>(null);
  const [pricingInterval, setPricingInterval] = useState("");
  const [settingsLoading, setSettingsLoading] = useState(true);
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirmRebuild, setConfirmRebuild] = useState(false);
  const [rebuildState, setRebuildState] = useState<ActionState>("idle");
  const [rebuildError, setRebuildError] = useState<string | null>(null);
  const loadControllerRef = useRef<AbortController | null>(null);
  const loadGenerationRef = useRef(0);
  const mountedRef = useRef(false);
  const rebuildTriggerRef = useRef<HTMLButtonElement>(null);
  const rebuildDialogRef = useRef<HTMLElement>(null);
  const rebuildCancelRef = useRef<HTMLButtonElement>(null);

  const loadCollectorSettings = useCallback(async () => {
    loadControllerRef.current?.abort();
    const controller = new AbortController();
    const generation = ++loadGenerationRef.current;
    loadControllerRef.current = controller;
    const isCurrent = () => (
      mountedRef.current
      && loadControllerRef.current === controller
      && loadGenerationRef.current === generation
      && !controller.signal.aborted
    );
    setSettingsLoading(true);
    setSettingsError(null);
    try {
      const response = await fetchAPI<CollectorSettings>("settings/collectors", {}, { signal: controller.signal });
      if (!isCurrent()) return;
      setCollectors(response.collectors.map((collector) => ({
        ...collector,
        pathsText: collector.paths.join("\n"),
      })));
      setPricingInterval(response.pricing_sync_interval);
    } catch (error) {
      if (!isCurrent()) return;
      setCollectors(null);
      setSettingsError(errorMessage(error));
    } finally {
      if (isCurrent()) setSettingsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void loadCollectorSettings();
    invoke<number>("get_cost_threshold").then(setCostThreshold).catch(() => {});
    invoke<boolean>("plugin:autostart|is_enabled").then(setAutostart).catch(() => {});
    invoke<boolean>("get_notifications_enabled").then(setNotificationsEnabled).catch(() => {});
    return () => {
      mountedRef.current = false;
      loadControllerRef.current?.abort();
    };
  }, [loadCollectorSettings]);

  useEffect(() => {
    if (confirmRebuild) rebuildCancelRef.current?.focus();
  }, [confirmRebuild]);

  const handleThemeChange = (value: string) => {
    setTheme(value);
    localStorage.setItem("au-theme", value);
    const resolved = value === "system"
      ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : value;
    document.documentElement.classList.toggle("dark", resolved === "dark");
  };

  const handleLangChange = (value: string) => {
    void i18n.changeLanguage(value);
    localStorage.setItem("au-lang", value);
  };

  const handleAutostartToggle = async () => {
    if (autostartPending) return;
    const confirmed = autostart;
    const next = !confirmed;
    setAutostart(next);
    setAutostartPending(true);
    setAutostartError(null);
    try {
      await invoke(next ? "plugin:autostart|enable" : "plugin:autostart|disable");
    } catch (error) {
      setAutostart(confirmed);
      setAutostartError(errorMessage(error));
    } finally {
      setAutostartPending(false);
    }
  };

  const handleThresholdChange = (value: number) => {
    setCostThreshold(value);
    void invoke("set_cost_threshold", { threshold: value }).catch(() => {});
  };

  const handleNotificationsToggle = async () => {
    if (notificationsPending) return;
    const confirmed = notificationsEnabled;
    const next = !confirmed;
    setNotificationsEnabled(next);
    setNotificationsPending(true);
    setNotificationError(null);
    try {
      await invoke("set_notifications_enabled", { enabled: next });
    } catch (error) {
      setNotificationsEnabled(confirmed);
      setNotificationError(errorMessage(error));
    } finally {
      setNotificationsPending(false);
    }
  };

  const updateCollector = (name: CollectorName, update: Partial<EditableCollector>) => {
    setCollectors((current) => current?.map((collector) => (
      collector.name === name ? { ...collector, ...update } : collector
    )) || null);
    setSaveState("idle");
    setSaveError(null);
  };

  const saveCollectorSettings = async () => {
    if (!collectors) return;
    setSaveState("pending");
    setSaveError(null);
    const payload: CollectorSettings = {
      collectors: collectors.map(({ pathsText, ...collector }) => ({
        ...collector,
        paths: pathsText.split(/\r?\n/).map((path) => path.trim()).filter(Boolean),
      })),
      pricing_sync_interval: pricingInterval,
    };
    try {
      await fetchAPI<SettingsUpdateResponse>("settings/collectors", {}, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
    } catch (error) {
      setSaveState("error");
      setSaveError(errorMessage(error));
      return;
    }
    await restartAfterSave();
  };

  const restartAfterSave = async () => {
    setSaveState("restartPending");
    setSaveError(null);
    try {
      await invoke<number>("restart_sidecar");
      setSaveState("success");
    } catch (error) {
      setSaveState("restartError");
      setSaveError(errorMessage(error));
    }
  };

  const closeRebuildDialog = () => {
    setConfirmRebuild(false);
    queueMicrotask(() => rebuildTriggerRef.current?.focus());
  };

  const handleRebuildDialogKeyDown = (event: React.KeyboardEvent<HTMLElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      closeRebuildDialog();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = Array.from(
      rebuildDialogRef.current?.querySelectorAll<HTMLElement>("button:not([disabled])") || [],
    );
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const rebuildSessionIndex = async () => {
    closeRebuildDialog();
    setRebuildState("pending");
    setRebuildError(null);
    try {
      await fetchAPI<SessionIndexRebuildResponse>("session-index/rebuild", {}, { method: "POST" });
      await invoke<number>("restart_sidecar");
      setRebuildState("success");
    } catch (error) {
      setRebuildState("error");
      setRebuildError(errorMessage(error));
    }
  };

  return (
    <div className="min-w-0 max-w-5xl space-y-8 pb-8">
      <section className="border-y border-border py-5">
        <h2 className="mb-4 text-sm font-semibold">{t("appearanceAndLanguage")}</h2>
        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t("theme")}</h3>
            <SegmentedControl
              value={theme}
              options={["light", "dark", "system"].map((value) => ({ value, label: t(value) }))}
              onChange={handleThemeChange}
            />
          </div>
          <div>
            <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t("language")}</h3>
            <SegmentedControl
              value={i18n.language}
              options={[{ value: "en", label: "English" }, { value: "zh", label: "中文" }]}
              onChange={handleLangChange}
            />
          </div>
        </div>
      </section>

      <section className="border-y border-border py-5">
        <div className="mb-4">
          <h2 className="text-sm font-semibold">{t("collectorSettings")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("collectorSettingsDetail")}</p>
        </div>
        {settingsLoading ? (
          <p className="py-8 text-sm text-muted-foreground">{t("loadingSettings")}</p>
        ) : settingsError ? (
          <div className="py-6">
            <p className="break-words text-sm text-red-500">{settingsError}</p>
            <button type="button" onClick={() => { void loadCollectorSettings(); }} className="mt-3 inline-flex items-center gap-2 rounded border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted">
              <RefreshCw className="h-4 w-4" /> {t("retry")}
            </button>
          </div>
        ) : collectors ? (
          <div className="divide-y divide-border border-t border-border">
            {collectors.map((collector) => {
              const label = t(COLLECTOR_LABELS[collector.name]);
              return (
                <div key={collector.name} className="grid min-w-0 gap-4 py-4 lg:grid-cols-[12rem_minmax(0,1fr)_12rem]">
                  <div className="flex items-start justify-between gap-3 lg:block">
                    <div>
                      <h3 className="text-sm font-medium">{label}</h3>
                      <p className="mt-1 text-[10px] text-muted-foreground">
                        {t(FULL_RETROSPECTIVE.has(collector.name) ? "fullRetrospective" : "statisticsOnly")}
                      </p>
                    </div>
                    <Toggle
                      checked={collector.enabled}
                      label={`${t("collectorEnabled")} ${label}`}
                      onChange={() => updateCollector(collector.name, { enabled: !collector.enabled })}
                    />
                  </div>
                  <label className="min-w-0 text-[10px] text-muted-foreground">
                    <span className="mb-1 block">{t("collectorPaths")}</span>
                    <textarea
                      aria-label={`${t("collectorPaths")} ${label}`}
                      value={collector.pathsText}
                      onChange={(event) => updateCollector(collector.name, { pathsText: event.target.value })}
                      rows={Math.max(2, collector.pathsText.split(/\r?\n/).length)}
                      className="w-full resize-y rounded border border-border bg-card px-2.5 py-2 font-mono text-xs text-foreground outline-none focus:border-accent"
                    />
                  </label>
                  <label className="min-w-0 text-[10px] text-muted-foreground">
                    <span className="mb-1 block">{t("scanInterval")}</span>
                    <input
                      type="text"
                      aria-label={`${t("scanInterval")} ${label}`}
                      value={collector.scan_interval}
                      onChange={(event) => updateCollector(collector.name, { scan_interval: event.target.value })}
                      className="h-9 w-full rounded border border-border bg-card px-2.5 font-mono text-xs text-foreground outline-none focus:border-accent"
                    />
                  </label>
                </div>
              );
            })}
            <div className="flex min-w-0 flex-wrap items-end justify-between gap-4 py-4">
              <label className="min-w-48 text-[10px] text-muted-foreground">
                <span className="mb-1 block">{t("pricingSyncInterval")}</span>
                <input
                  type="text"
                  aria-label={t("pricingSyncInterval")}
                  value={pricingInterval}
                  onChange={(event) => { setPricingInterval(event.target.value); setSaveState("idle"); setSaveError(null); }}
                  className="h-9 w-full rounded border border-border bg-card px-2.5 font-mono text-xs text-foreground outline-none focus:border-accent"
                />
              </label>
              <button
                type="button"
                aria-label={t("saveCollectorSettings")}
                onClick={() => { void saveCollectorSettings(); }}
                disabled={saveState === "pending" || saveState === "restartPending"}
                className="inline-flex h-9 items-center gap-2 rounded bg-accent px-3 text-xs font-medium text-white hover:bg-accent/90 disabled:opacity-50"
              >
                <Save className="h-4 w-4" /> {saveState === "pending" ? t("savingSettings") : t("save")}
              </button>
            </div>
            {saveState === "success" && <p className="pb-3 text-xs text-green">{t("settingsSavedAndRestarted")}</p>}
            {saveState === "restartError" && (
              <div className="flex flex-wrap items-center gap-3 pb-3">
                <p className="text-xs text-amber-600">{t("settingsSavedRestartFailed")}</p>
                <button type="button" onClick={() => { void restartAfterSave(); }} className="inline-flex items-center gap-2 rounded border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted">
                  <RefreshCw className="h-4 w-4" /> {t("retryRestart")}
                </button>
              </div>
            )}
            {saveError && <p className="pb-3 break-words text-xs text-red-500">{saveError}</p>}
          </div>
        ) : null}
      </section>

      <section className="border-y border-border py-5">
        <h2 className="mb-4 text-sm font-semibold">{t("desktopPreferences")}</h2>
        <div className="divide-y divide-border border-t border-border">
          <div className="flex items-center justify-between gap-4 py-4">
            <span className="text-sm">{t("autostart")}</span>
            <Toggle checked={autostart} disabled={autostartPending} label={t("autostart")} onChange={() => { void handleAutostartToggle(); }} />
          </div>
          {autostartError && <p className="py-2 text-xs text-red-500">{t("autostartUpdateFailed")}</p>}
          <div className="flex items-center justify-between gap-4 py-4">
            <span className="text-sm">{t("notification")}</span>
            <Toggle checked={notificationsEnabled} disabled={notificationsPending} label={t("notification")} onChange={() => { void handleNotificationsToggle(); }} />
          </div>
          {notificationError && <p className="py-2 text-xs text-red-500">{t("notificationUpdateFailed")}</p>}
          <label className="block py-4 text-xs text-muted-foreground">
            <span className="mb-1 block">{t("dailyCostThreshold")}</span>
            <span className="flex items-center gap-2">
              <input
                type="number"
                aria-label={t("dailyCostThreshold")}
                value={costThreshold}
                min={0}
                step={1}
                onChange={(event) => handleThresholdChange(Number(event.target.value))}
                className="h-9 w-28 rounded border border-border bg-card px-2.5 font-mono text-sm text-foreground"
              />
              <span>USD</span>
            </span>
          </label>
        </div>
      </section>

      <section className="border-y border-border py-5">
        <h2 className="text-sm font-semibold">{t("sessionIndex")}</h2>
        <p className="mt-1 text-xs text-muted-foreground">{t("sessionIndexDetail")}</p>
        <button
          type="button"
          ref={rebuildTriggerRef}
          aria-label={t("rebuildSessionIndex")}
          aria-disabled={rebuildState === "pending"}
          onClick={() => { if (rebuildState !== "pending") setConfirmRebuild(true); }}
          className="mt-4 inline-flex items-center gap-2 rounded border border-border px-3 py-2 text-xs font-medium hover:bg-muted aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
        >
          <Database className="h-4 w-4" /> {rebuildState === "pending" ? t("rebuildingIndex") : t("rebuildSessionIndex")}
        </button>
        {rebuildState === "success" && <p className="mt-3 text-xs text-green">{t("rebuildStartedAndRestarted")}</p>}
        {rebuildError && <p className="mt-3 break-words text-xs text-red-500">{rebuildError}</p>}
      </section>

      {confirmRebuild && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="presentation">
          <section ref={rebuildDialogRef} role="dialog" aria-modal="true" aria-labelledby="confirm-rebuild-title" onKeyDown={handleRebuildDialogKeyDown} className="w-full max-w-md rounded border border-border bg-background p-5 shadow-xl">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-accent" />
              <div>
                <h2 id="confirm-rebuild-title" className="text-sm font-semibold">{t("confirmRebuildTitle")}</h2>
                <p className="mt-2 text-xs leading-5 text-muted-foreground">{t("confirmRebuildDetail")}</p>
              </div>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button ref={rebuildCancelRef} type="button" onClick={closeRebuildDialog} className="rounded border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted">{t("cancel")}</button>
              <button type="button" aria-label={t("confirmRebuild")} onClick={() => { void rebuildSessionIndex(); }} className="rounded bg-accent px-3 py-1.5 text-xs font-medium text-white hover:bg-accent/90">{t("confirmRebuild")}</button>
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
