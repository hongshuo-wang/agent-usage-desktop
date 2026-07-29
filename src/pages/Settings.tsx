import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { AlertTriangle, BarChart3, CircleCheck, CircleMinus, Database, MessageSquareText, RefreshCw, Save, Search, Star, StarOff, Upload, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import { applyOwnedHydration } from "./settingsHydration";
import type { SystemSection } from "../lib/systemNavigation";
import type {
  CollectorName,
  CollectorSetting,
  CollectorSettings,
  SessionIndexRebuildResponse,
  PricingCatalog,
  SettingsUpdateResponse,
} from "../lib/types";

type EditableCollector = CollectorSetting & { pathsText: string };
type ActionState = "idle" | "pending" | "success" | "error";
type SaveState = ActionState | "restartPending" | "restartError";
type RebuildState = ActionState | "restartPending" | "restartError";

const COLLECTOR_LABELS: Record<CollectorName, string> = {
  claude: "claudeCode",
  codex: "codex",
  openclaw: "openClaw",
  opencode: "openCode",
};

const FULL_RETROSPECTIVE = new Set<CollectorName>(["claude", "codex"]);
const LITELLM_PRICING_URL = "https://cdn.jsdelivr.net/gh/BerriAI/litellm@main/model_prices_and_context_window.json";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function formatPricePerMillion(pricePerToken: number): string {
  return `$${(pricePerToken * 1_000_000).toFixed(4)}`;
}

function openPricingSource(event: React.MouseEvent<HTMLAnchorElement>) {
  event.preventDefault();
  void invoke("open_external_url", { url: LITELLM_PRICING_URL }).catch(() => {
    window.open(LITELLM_PRICING_URL, "_blank", "noopener,noreferrer");
  });
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

export default function Settings({ section = "data-sources" }: { section?: SystemSection }) {
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
  const [pricingFile, setPricingFile] = useState<File | null>(null);
  const [pricingImportState, setPricingImportState] = useState<ActionState>("idle");
  const [pricingImportError, setPricingImportError] = useState<string | null>(null);
  const [pricingSyncState, setPricingSyncState] = useState<ActionState>("idle");
  const [pricingSyncError, setPricingSyncError] = useState<string | null>(null);
  const [pricingImportOpen, setPricingImportOpen] = useState(false);
  const [pricingCatalogState, setPricingCatalogState] = useState<ActionState>("idle");
  const [pricingCatalog, setPricingCatalog] = useState<PricingCatalog | null>(null);
  const [pricingCatalogError, setPricingCatalogError] = useState<string | null>(null);
  const [pricingCatalogSearch, setPricingCatalogSearch] = useState("");
  const [pinnedModels, setPinnedModels] = useState<string[]>(() => {
    try {
      const value = JSON.parse(localStorage.getItem("au-pinned-models") || "[]");
      return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
    } catch {
      return [];
    }
  });
  const pricingFileInputRef = useRef<HTMLInputElement>(null);
  const [settingsLoading, setSettingsLoading] = useState(true);
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirmRebuild, setConfirmRebuild] = useState(false);
  const [rebuildState, setRebuildState] = useState<RebuildState>("idle");
  const [rebuildError, setRebuildError] = useState<string | null>(null);
  const loadControllerRef = useRef<AbortController | null>(null);
  const loadGenerationRef = useRef(0);
  const pricingCatalogGenerationRef = useRef(0);
  const mountedRef = useRef(false);
  const desktopHydrationGenerationRef = useRef({
    costThreshold: 0,
    autostart: 0,
    notifications: 0,
  });
  const rebuildTriggerRef = useRef<HTMLButtonElement>(null);
  const rebuildDialogRef = useRef<HTMLElement>(null);
  const rebuildCancelRef = useRef<HTMLButtonElement>(null);
  const pricingImportCloseRef = useRef<HTMLButtonElement>(null);

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

  const loadPricingCatalog = useCallback(async (showLoading = true) => {
    const generation = ++pricingCatalogGenerationRef.current;
    if (showLoading) setPricingCatalogState("pending");
    setPricingCatalogError(null);
    try {
      const response = await fetchAPI<PricingCatalog>("pricing/models", {});
      if (!mountedRef.current || generation !== pricingCatalogGenerationRef.current) return;
      setPricingCatalog(response);
      setPricingCatalogState("success");
    } catch (error) {
      if (!mountedRef.current || generation !== pricingCatalogGenerationRef.current) return;
      setPricingCatalogState("error");
      setPricingCatalogError(errorMessage(error));
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void loadCollectorSettings();
    const thresholdGeneration = ++desktopHydrationGenerationRef.current.costThreshold;
    const autostartGeneration = ++desktopHydrationGenerationRef.current.autostart;
    const notificationsGeneration = ++desktopHydrationGenerationRef.current.notifications;
    invoke<number>("get_cost_threshold").then((value) => {
      applyOwnedHydration(
        mountedRef.current,
        thresholdGeneration,
        desktopHydrationGenerationRef.current.costThreshold,
        value,
        setCostThreshold,
      );
    }).catch(() => {});
    invoke<boolean>("plugin:autostart|is_enabled").then((value) => {
      applyOwnedHydration(
        mountedRef.current,
        autostartGeneration,
        desktopHydrationGenerationRef.current.autostart,
        value,
        setAutostart,
      );
    }).catch(() => {});
    invoke<boolean>("get_notifications_enabled").then((value) => {
      applyOwnedHydration(
        mountedRef.current,
        notificationsGeneration,
        desktopHydrationGenerationRef.current.notifications,
        value,
        setNotificationsEnabled,
      );
    }).catch(() => {});
    return () => {
      mountedRef.current = false;
      loadControllerRef.current?.abort();
    };
  }, [loadCollectorSettings]);

  useEffect(() => {
    if (confirmRebuild) rebuildCancelRef.current?.focus();
  }, [confirmRebuild]);

  useEffect(() => {
    if (pricingImportOpen) pricingImportCloseRef.current?.focus();
  }, [pricingImportOpen]);

  useEffect(() => {
    if (section !== "pricing") return;
    void loadPricingCatalog();
    return () => { pricingCatalogGenerationRef.current += 1; };
  }, [loadPricingCatalog, section]);

  const visiblePricingModels = useMemo(() => {
    const search = pricingCatalogSearch.trim().toLowerCase();
    return (pricingCatalog?.models || [])
      .filter((entry) => entry.model.toLowerCase().includes(search))
      .sort((left, right) => (
        Number(pinnedModels.includes(right.model)) - Number(pinnedModels.includes(left.model))
      ));
  }, [pinnedModels, pricingCatalog, pricingCatalogSearch]);

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
    desktopHydrationGenerationRef.current.autostart += 1;
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
    desktopHydrationGenerationRef.current.costThreshold += 1;
    setCostThreshold(value);
    void invoke("set_cost_threshold", { threshold: value }).catch(() => {});
  };

  const handleNotificationsToggle = async () => {
    if (notificationsPending) return;
    desktopHydrationGenerationRef.current.notifications += 1;
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

  const importPricing = async () => {
    if (!pricingFile || pricingImportState === "pending") return;
    setPricingImportState("pending");
    setPricingImportError(null);
    try {
      const body = await pricingFile.text();
      await fetchAPI("pricing/import", {}, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      await loadPricingCatalog(false);
      setPricingImportState("success");
      setPricingFile(null);
      if (pricingFileInputRef.current) pricingFileInputRef.current.value = "";
    } catch (error) {
      setPricingImportState("error");
      setPricingImportError(errorMessage(error));
    }
  };

  const refreshPricing = async () => {
    if (pricingSyncState === "pending") return;
    setPricingSyncState("pending");
    setPricingSyncError(null);
    try {
      await fetchAPI("pricing/sync", {}, { method: "POST" });
      await loadPricingCatalog(false);
      setPricingSyncState("success");
    } catch (error) {
      setPricingSyncState("error");
      const message = errorMessage(error);
      setPricingSyncError(message.includes("404") ? t("pricingRefreshEndpointUnavailable") : message);
    }
  };

  const togglePinnedModel = (model: string) => {
    setPinnedModels((current) => {
      const next = current.includes(model) ? current.filter((item) => item !== model) : [...current, model];
      localStorage.setItem("au-pinned-models", JSON.stringify(next));
      return next;
    });
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
    } catch (error) {
      setRebuildState("error");
      setRebuildError(errorMessage(error));
      return;
    }
    await restartAfterRebuild();
  };

  const restartAfterRebuild = async () => {
    setRebuildState("restartPending");
    setRebuildError(null);
    try {
      await invoke<number>("restart_sidecar");
      setRebuildState("success");
    } catch (error) {
      setRebuildState("restartError");
      setRebuildError(errorMessage(error));
    }
  };

  return (
    <div className="min-w-0 w-full max-w-5xl pb-8">
      {section === "data-sources" && <section id="data-sources">
        <div className="mb-4">
          <h2 className="text-base font-semibold tracking-tight">{t("collectorSettings")}</h2>
          <p className="mt-1 text-sm leading-5 text-muted-foreground">{t("collectorSettingsDetail")}</p>
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
          <div className="space-y-2">
            {collectors.map((collector) => {
              const label = t(COLLECTOR_LABELS[collector.name]);
              const supportsReplay = FULL_RETROSPECTIVE.has(collector.name);
              return (
                <div key={collector.name} className="grid min-w-0 gap-4 rounded-md bg-card/55 px-4 py-4 lg:grid-cols-[14rem_minmax(0,1fr)_10rem]">
                  <div className="min-w-0">
                    <div className="flex items-start justify-between gap-3">
                      <div>
                      <h3 className="text-sm font-medium">{label}</h3>
                      <p className="mt-1 text-[10px] text-muted-foreground tabular-nums">
                        {t("collectorCapabilityCount", { supported: supportsReplay ? 2 : 1, total: 2 })}
                      </p>
                      </div>
                      <Toggle
                        checked={collector.enabled}
                        label={`${t("collectorEnabled")} ${label}`}
                        onChange={() => updateCollector(collector.name, { enabled: !collector.enabled })}
                      />
                    </div>
                    <div className="mt-3 flex flex-wrap gap-1.5" aria-label={t("collectorCapabilities")}>
                      <span className="inline-flex items-center gap-1.5 rounded bg-accent-dim px-2 py-1 text-[10px] font-medium text-accent">
                        <BarChart3 className="h-3 w-3" aria-hidden="true" />
                        {t("tokenUsageCapability")}
                        <CircleCheck className="h-3 w-3" aria-hidden="true" />
                      </span>
                      <span className={`inline-flex items-center gap-1.5 rounded px-2 py-1 text-[10px] font-medium ${supportsReplay ? "bg-accent-dim text-accent" : "bg-muted text-muted-foreground"}`}>
                        <MessageSquareText className="h-3 w-3" aria-hidden="true" />
                        {t("sessionReplayCapability")}
                        {supportsReplay ? <CircleCheck className="h-3 w-3" aria-hidden="true" /> : <CircleMinus className="h-3 w-3" aria-hidden="true" />}
                      </span>
                    </div>
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
            <div className="flex min-w-0 flex-wrap items-end justify-end gap-4 pt-3">
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
      </section>}

      {section === "pricing" && <section id="pricing">
        <h2 className="text-base font-semibold tracking-tight">{t("pricingSourceTitle")}</h2>
        <p className="mt-1 max-w-2xl text-sm leading-5 text-muted-foreground">{t("pricingSourceDetail")}</p>

        <div className="mt-5 rounded-md bg-card/60 p-4">
          <div className="flex min-w-0 flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium">{t("defaultPricingCatalog")}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t("defaultPricingCatalogDetail")}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                aria-label={t("refreshPricing")}
                onClick={() => { void refreshPricing(); }}
                disabled={pricingSyncState === "pending"}
                className="inline-flex h-9 items-center gap-2 rounded bg-accent px-3 text-xs font-medium text-white transition-colors hover:bg-accent/90 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50"
              >
                <RefreshCw className={`h-4 w-4 ${pricingSyncState === "pending" ? "animate-spin" : ""}`} />
                {pricingSyncState === "pending" ? t("refreshingPricing") : t("refreshPricing")}
              </button>
            </div>
          </div>

          <dl className="mt-4 grid gap-px overflow-hidden rounded border border-border bg-border sm:grid-cols-3">
            <div className="bg-card px-3 py-3">
              <dt className="text-[10px] text-muted-foreground">{t("pricingProvider")}</dt>
              <dd className="mt-1 text-xs font-medium">{pricingCatalog?.source || "LiteLLM"}</dd>
            </div>
            <div className="bg-card px-3 py-3">
              <dt className="text-[10px] text-muted-foreground">{t("pricingLastUpdated")}</dt>
              <dd className="mt-1 font-mono text-xs tabular-nums">
                {pricingCatalog?.pricing_last_synced_at
                  ? new Date(pricingCatalog.pricing_last_synced_at).toLocaleString(i18n.language)
                  : pricingCatalogState === "pending" ? t("loadingPricing") : t("notAvailable")}
              </dd>
            </div>
            <div className="bg-card px-3 py-3">
              <dt className="text-[10px] text-muted-foreground">{t("pricingModels")}</dt>
              <dd className="mt-1 font-mono text-xs font-semibold tabular-nums">
                {pricingCatalog ? pricingCatalog.models.length : pricingCatalogState === "pending" ? "..." : "-"}
              </dd>
            </div>
          </dl>

          {pricingSyncState === "success" && <p className="mt-3 text-xs text-green">{t("pricingRefreshed")}</p>}
          {pricingSyncError && <p className="mt-3 break-words text-xs text-red-500">{t("pricingRefreshFailed")}: {pricingSyncError}</p>}
        </div>

        <section className="mt-6" aria-labelledby="pricing-catalog-title">
          <div className="flex min-w-0 flex-wrap items-end justify-between gap-3">
            <div>
              <h3 id="pricing-catalog-title" className="text-sm font-semibold">{t("pricingCatalogTitle")}</h3>
              <p className="mt-1 text-xs text-muted-foreground">{t("pricingCatalogDetail")}</p>
            </div>
            <label className="relative min-w-52 flex-1 sm:max-w-xs">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                type="search"
                aria-label={t("searchPricingModels")}
                value={pricingCatalogSearch}
                onChange={(event) => setPricingCatalogSearch(event.target.value)}
                placeholder={t("searchPricingModels")}
                className="h-9 w-full rounded border border-border bg-card pl-8 pr-2.5 text-xs text-foreground outline-none focus:border-accent"
              />
            </label>
          </div>

          <div className="mt-3 flex h-[clamp(18rem,48vh,32rem)] min-w-0 flex-col overflow-hidden rounded border border-border bg-card">
            {pricingCatalogState === "pending" && (
              <p className="m-auto text-sm text-muted-foreground">{t("loadingPricing")}</p>
            )}
            {pricingCatalogState === "error" && (
              <div className="m-auto px-5 text-center">
                <p className="break-words text-sm text-red-500">{pricingCatalogError}</p>
                <button
                  type="button"
                  onClick={() => { void loadPricingCatalog(); }}
                  className="mt-3 inline-flex items-center gap-2 rounded border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted"
                >
                  <RefreshCw className="h-3.5 w-3.5" /> {t("retry")}
                </button>
              </div>
            )}
            {pricingCatalogState === "success" && pricingCatalog && (
              <>
                <div className="border-b border-border px-3 py-2 text-[10px] text-muted-foreground">
                  {t("pricingModelCount", { count: visiblePricingModels.length })}
                </div>
                <div className="min-h-0 flex-1 overflow-auto">
                  <table className="w-full min-w-[46rem] text-left text-xs">
                    <thead className="sticky top-0 z-10 bg-muted text-muted-foreground">
                      <tr>
                        <th className="w-12 px-3 py-2 font-medium">{t("pinned")}</th>
                        <th className="px-3 py-2 font-medium">{t("model")}</th>
                        <th className="px-3 py-2 text-right font-medium">{t("pricingInputPrice")}</th>
                        <th className="px-3 py-2 text-right font-medium">{t("pricingOutputPrice")}</th>
                        <th className="px-3 py-2 text-right font-medium">{t("pricingCacheReadPrice")}</th>
                        <th className="px-3 py-2 text-right font-medium">{t("pricingCacheCreatePrice")}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {visiblePricingModels.map((entry) => (
                        <tr key={entry.model} className="hover:bg-muted/50">
                          <td className="px-3 py-2">
                            <button
                              type="button"
                              aria-label={`${t(pinnedModels.includes(entry.model) ? "unpinModel" : "pinModel")} ${entry.model}`}
                              aria-pressed={pinnedModels.includes(entry.model)}
                              onClick={() => togglePinnedModel(entry.model)}
                              className="icon-button h-7 w-7 text-muted-foreground hover:text-accent"
                            >
                              {pinnedModels.includes(entry.model) ? <Star className="h-4 w-4 fill-current" /> : <StarOff className="h-4 w-4" />}
                            </button>
                          </td>
                          <td className="max-w-[24rem] truncate px-3 py-2 font-mono" title={entry.model}>{entry.model}</td>
                          <td className="whitespace-nowrap px-3 py-2 text-right font-mono tabular-nums">{formatPricePerMillion(entry.input_cost_per_token)}</td>
                          <td className="whitespace-nowrap px-3 py-2 text-right font-mono tabular-nums">{formatPricePerMillion(entry.output_cost_per_token)}</td>
                          <td className="whitespace-nowrap px-3 py-2 text-right font-mono tabular-nums">{formatPricePerMillion(entry.cache_read_input_token_cost)}</td>
                          <td className="whitespace-nowrap px-3 py-2 text-right font-mono tabular-nums">{formatPricePerMillion(entry.cache_creation_input_token_cost)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {visiblePricingModels.length === 0 && (
                    <p className="py-10 text-center text-sm text-muted-foreground">{t("noPricingModels")}</p>
                  )}
                </div>
              </>
            )}
          </div>
        </section>

        <div className="mt-5 flex min-w-0 flex-wrap items-end justify-between gap-4">
          <label className="w-48 text-[10px] text-muted-foreground">
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
            aria-label={t("savePricingSettings")}
            onClick={() => { void saveCollectorSettings(); }}
            disabled={saveState === "pending" || saveState === "restartPending" || settingsLoading}
            className="inline-flex h-9 items-center gap-2 rounded border border-border px-3 text-xs font-medium transition-colors hover:bg-muted active:translate-y-px disabled:opacity-50"
          >
            <Save className="h-4 w-4" /> {saveState === "pending" ? t("savingSettings") : t("save")}
          </button>
        </div>
        {saveState === "success" && <p className="mt-2 text-xs text-green">{t("settingsSavedAndRestarted")}</p>}
        {saveError && <p className="mt-2 break-words text-xs text-red-500">{saveError}</p>}

        <button
          type="button"
          aria-label={t("customPricingFile")}
          onClick={() => { setPricingImportOpen(true); setPricingImportState("idle"); setPricingImportError(null); }}
          className="mt-6 inline-flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <Upload className="h-3.5 w-3.5" /> {t("customPricingFile")}
        </button>
      </section>}

      {section === "pricing" && pricingImportOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          role="presentation"
          onMouseDown={(event) => { if (event.target === event.currentTarget) setPricingImportOpen(false); }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="pricing-import-title"
            onKeyDown={(event) => { if (event.key === "Escape") setPricingImportOpen(false); }}
            className="w-full max-w-md rounded-lg border border-border bg-background p-5 shadow-xl"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h2 id="pricing-import-title" className="text-sm font-semibold">{t("customPricingFile")}</h2>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("customPricingFileDetail")}</p>
              </div>
              <button
                ref={pricingImportCloseRef}
                type="button"
                aria-label={t("closePricingImport")}
                onClick={() => setPricingImportOpen(false)}
                className="icon-button shrink-0 text-muted-foreground hover:text-foreground"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <a
              href={LITELLM_PRICING_URL}
              target="_blank"
              rel="noreferrer"
              onClick={openPricingSource}
              className="mt-4 inline-block text-xs text-accent underline underline-offset-2 hover:text-accent/80"
            >
              {t("pricingSourceLink")}
            </a>
            <label className="mt-4 block min-w-0 text-[10px] text-muted-foreground">
              <span className="mb-1 block">{t("pricingFile")}</span>
              <input
                ref={pricingFileInputRef}
                type="file"
                accept=".json,application/json"
                aria-label={t("pricingFile")}
                onChange={(event) => {
                  setPricingFile(event.target.files?.[0] ?? null);
                  setPricingImportState("idle");
                  setPricingImportError(null);
                }}
                className="block h-9 w-full min-w-0 cursor-pointer rounded border border-border bg-card px-2 py-1.5 text-xs text-foreground file:mr-2 file:rounded file:border-0 file:bg-muted file:px-2 file:py-1 file:text-xs file:font-medium"
              />
            </label>
            <div className="mt-4 flex justify-end">
              <button
                type="button"
                aria-label={t("importPricing")}
                onClick={() => { void importPricing(); }}
                disabled={!pricingFile || pricingImportState === "pending"}
                className="inline-flex h-9 items-center gap-2 rounded bg-accent px-3 text-xs font-medium text-white hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Upload className="h-4 w-4" />
                {pricingImportState === "pending" ? t("importingPricing") : t("importPricing")}
              </button>
            </div>
            {pricingImportState === "success" && <p className="mt-3 text-xs text-green">{t("pricingImported")}</p>}
            {pricingImportError && <p className="mt-3 break-words text-xs text-red-500">{t("pricingImportFailed")}: {pricingImportError}</p>}
          </section>
        </div>
      )}

      {section === "index-diagnostics" && <section id="index-diagnostics">
        <h2 className="text-base font-semibold tracking-tight">{t("sessionIndex")}</h2>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">{t("sessionIndexDetail")}</p>
        <button
          type="button"
          ref={rebuildTriggerRef}
          aria-label={t("rebuildSessionIndex")}
          aria-disabled={rebuildState === "pending" || rebuildState === "restartPending"}
          onClick={() => { if (rebuildState !== "pending" && rebuildState !== "restartPending") setConfirmRebuild(true); }}
          className="mt-4 inline-flex items-center gap-2 rounded border border-border px-3 py-2 text-xs font-medium hover:bg-muted aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
        >
          <Database className="h-4 w-4" /> {rebuildState === "pending" || rebuildState === "restartPending" ? t("rebuildingIndex") : t("rebuildSessionIndex")}
        </button>
        {rebuildState === "success" && <p className="mt-3 text-xs text-green">{t("rebuildStartedAndRestarted")}</p>}
        {rebuildState === "restartError" && (
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <p className="text-xs text-amber-600">{t("rebuildCompletedRestartFailed")}</p>
            <button type="button" onClick={() => { void restartAfterRebuild(); }} className="inline-flex items-center gap-2 rounded border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted">
              <RefreshCw className="h-4 w-4" /> {t("retryRestart")}
            </button>
          </div>
        )}
        {rebuildError && <p className="mt-3 break-words text-xs text-red-500">{rebuildError}</p>}
      </section>}

      {section === "preferences" && <section id="preferences">
        <h2 className="mb-4 text-base font-semibold tracking-tight">{t("desktopPreferences")}</h2>
        <div className="pb-5">
          <h3 className="mb-4 text-sm font-semibold">{t("appearanceAndLanguage")}</h3>
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <h4 className="mb-2 text-xs font-medium text-muted-foreground">{t("theme")}</h4>
              <SegmentedControl
                value={theme}
                options={["light", "dark", "system"].map((value) => ({ value, label: t(value) }))}
                onChange={handleThemeChange}
              />
            </div>
            <div>
              <h4 className="mb-2 text-xs font-medium text-muted-foreground">{t("language")}</h4>
              <SegmentedControl
                value={i18n.language}
                options={[{ value: "en", label: "English" }, { value: "zh", label: "中文" }]}
                onChange={handleLangChange}
              />
            </div>
          </div>
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-4 rounded-md bg-card/55 px-3 py-3">
            <span className="text-sm">{t("autostart")}</span>
            <Toggle checked={autostart} disabled={autostartPending} label={t("autostart")} onChange={() => { void handleAutostartToggle(); }} />
          </div>
          {autostartError && <p className="py-2 text-xs text-red-500">{t("autostartUpdateFailed")}</p>}
          <div className="flex items-center justify-between gap-4 rounded-md bg-card/55 px-3 py-3">
            <span className="text-sm">{t("notification")}</span>
            <Toggle checked={notificationsEnabled} disabled={notificationsPending} label={t("notification")} onChange={() => { void handleNotificationsToggle(); }} />
          </div>
          {notificationError && <p className="py-2 text-xs text-red-500">{t("notificationUpdateFailed")}</p>}
          <label className="block rounded-md bg-card/55 px-3 py-3 text-xs text-muted-foreground">
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
      </section>}

      {section === "index-diagnostics" && confirmRebuild && (
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
