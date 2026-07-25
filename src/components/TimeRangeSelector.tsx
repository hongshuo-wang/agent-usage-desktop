import { useEffect, useRef, useState } from "react";
import { ChevronDown, RefreshCw, SlidersHorizontal, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { UsageFilters } from "../lib/types";
import { getActiveQueryChips, presentProjectKey } from "../lib/queryPresentation";
import { getTimeRange, type TimePreset } from "../lib/utils";

const PRESETS: TimePreset[] = ["today", "thisWeek", "thisMonth", "thisYear", "last3d", "last7d", "last30d", "custom"];
const SOURCES = [
  { value: "", label: "allSources" },
  { value: "claude", label: "claudeCode" },
  { value: "codex", label: "codex" },
  { value: "openclaw", label: "openClaw" },
  { value: "opencode", label: "openCode" },
];

function isPreciseRange(value?: string): boolean {
  return Boolean(value?.includes("T"));
}

function dateTimeInputValue(value?: string): string {
  if (!value || !isPreciseRange(value)) return value || "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

interface Props {
  preset: TimePreset;
  onPresetChange: (p: TimePreset) => void;
  granularity: string;
  onGranularityChange: (g: string) => void;
  source: string;
  onSourceChange: (s: string) => void;
  onRefresh: () => void;
  customFrom?: string;
  customTo?: string;
  onCustomFromChange?: (v: string) => void;
  onCustomToChange?: (v: string) => void;
  timeWindowHours?: number | null;
  onTimeWindowChange?: (hours: number | null) => void;
  filters?: UsageFilters;
  onFiltersApply?: (filters: UsageFilters) => void;
  onClearFilters?: () => void;
}

export default function TimeRangeSelector({
  preset, onPresetChange,
  source, onSourceChange, onRefresh, customFrom, customTo,
  onCustomFromChange, onCustomToChange, timeWindowHours = null, onTimeWindowChange,
  filters, onFiltersApply, onClearFilters,
}: Props) {
  const { t } = useTranslation();
  const editorRef = useRef<HTMLDivElement>(null);
  const firstFieldRef = useRef<HTMLSelectElement>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [draft, setDraft] = useState<UsageFilters>(() => filters || {
    preset,
    from: customFrom || "",
    to: customTo || "",
    source,
    model: "",
    project: "",
  });
  const activeFilters = filters ? getActiveQueryChips(filters) : [];
  const preciseRange = isPreciseRange(draft.from) || isPreciseRange(draft.to);

  useEffect(() => {
    if (!editorOpen) setDraft(filters || { preset, from: customFrom || "", to: customTo || "", source, model: "", project: "" });
  }, [customFrom, customTo, editorOpen, filters, preset, source]);

  useEffect(() => {
    if (!editorOpen) return undefined;
    firstFieldRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setEditorOpen(false);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [editorOpen]);

  const commit = () => {
    if (onFiltersApply) onFiltersApply(draft);
    else {
      onPresetChange(draft.preset);
      onSourceChange(draft.source);
      onCustomFromChange?.(draft.from);
      onCustomToChange?.(draft.to);
    }
    setEditorOpen(false);
  };

  const removeFilter = (key: "source" | "model" | "project") => {
    const next = { ...(filters || draft), [key]: "" } as UsageFilters;
    if (onFiltersApply) onFiltersApply(next);
    else if (key === "source") onSourceChange("");
  };

  const updateDraftPreset = (nextPreset: TimePreset) => {
    setDraft((current) => ({ ...current, preset: nextPreset, ...getTimeRange(nextPreset, current.from, current.to) }));
  };

  const handleDateChange = (value: string, key: "from" | "to") => {
    if (preciseRange && value) {
      const parsed = new Date(value);
      setDraft((current) => ({ ...current, preset: "custom", [key]: Number.isNaN(parsed.getTime()) ? value : parsed.toISOString() }));
      return;
    }
    setDraft((current) => ({ ...current, preset: "custom", [key]: value }));
  };

  const summaryPreset = t(draft.preset);
  const summarySource = draft.source ? t(SOURCES.find((item) => item.value === draft.source)?.label || draft.source) : t("allSources");

  return (
    <div className="relative z-20 min-w-0">
      <div className="flex min-w-0 flex-wrap items-center gap-2 rounded-xl border border-border bg-card px-3 py-2 shadow-sm">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
            <span className="shrink-0 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">{t("currentQuery")}</span>
            <span className="truncate font-semibold">{summaryPreset} · {summarySource}</span>
            <span className="truncate text-xs text-muted-foreground">{draft.from} {t("to")} {draft.to}</span>
          </div>
          {activeFilters.length > 0 && (
            <div className="mt-1 flex min-w-0 flex-wrap items-center gap-1.5" aria-label={t("activeFilters")}>
              {activeFilters.map((chip) => (
                <span key={chip.key} className="inline-flex max-w-full items-center gap-1 rounded-full border border-border bg-muted/50 px-2 py-0.5 text-[11px] text-muted-foreground" title={chip.value}>
                  <span className="truncate">{t(chip.key === "source" ? "source" : chip.key === "model" ? "model" : "project")}: {chip.key === "project" && presentProjectKey(chip.value).label === "unnamedProject" ? t("unnamedProject") : chip.value}</span>
                  <button type="button" aria-label={`${t("removeFilter")} ${chip.value}`} onClick={() => removeFilter(chip.key)} className="rounded-full p-0.5 hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"><X className="h-3 w-3" /></button>
                </span>
              ))}
            </div>
          )}
        </div>
        <button type="button" onClick={() => setEditorOpen((open) => !open)} aria-expanded={editorOpen} aria-controls="query-editor" className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-semibold text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent">
          <SlidersHorizontal className="h-3.5 w-3.5" />
          {t("editQuery")}
          <ChevronDown className={`h-3.5 w-3.5 transition-transform ${editorOpen ? "rotate-180" : ""}`} />
        </button>
        <button type="button" onClick={onRefresh} aria-label={t("refresh")} title={t("refresh")} className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"><RefreshCw className="h-3.5 w-3.5" /></button>
      </div>

      {editorOpen && (
        <>
          <button type="button" aria-label={t("cancelQuery")} onClick={() => setEditorOpen(false)} className="fixed inset-0 z-30 cursor-default bg-black/10" />
          <div ref={editorRef} id="query-editor" role="dialog" aria-modal="true" aria-label={t("queryEditor")} className="absolute right-0 top-[calc(100%+0.5rem)] z-40 w-full max-w-md rounded-xl border border-border bg-card p-4 shadow-xl sm:w-[25rem]">
            <div className="flex items-center justify-between"><div><h2 className="text-sm font-semibold">{t("queryEditor")}</h2><p className="mt-0.5 text-[11px] text-muted-foreground">{t("currentQuery")}</p></div><button type="button" onClick={() => setEditorOpen(false)} aria-label={t("cancelQuery")} className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"><X className="h-4 w-4" /></button></div>
            <div className="mt-4 space-y-4">
              <fieldset><legend className="text-[11px] font-semibold text-muted-foreground">{t("queryTimeRange")}</legend><div className="mt-2 flex flex-wrap gap-1.5">{PRESETS.map((item) => <button key={item} type="button" onClick={() => updateDraftPreset(item)} className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${draft.preset === item ? "bg-accent text-white" : "border border-border text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{t(item)}</button>)}</div>{draft.preset === "custom" && !timeWindowHours && <div className="mt-2 flex items-center gap-2"><input aria-label={`${t("queryTimeRange")} ${t("from")}`} type={preciseRange ? "datetime-local" : "date"} value={dateTimeInputValue(draft.from)} onChange={(event) => handleDateChange(event.target.value, "from")} className="h-8 min-w-0 flex-1 rounded-md border border-border bg-background px-2 text-xs" /><span className="text-xs text-muted-foreground">{t("to")}</span><input aria-label={`${t("queryTimeRange")} ${t("to")}`} type={preciseRange ? "datetime-local" : "date"} value={dateTimeInputValue(draft.to)} onChange={(event) => handleDateChange(event.target.value, "to")} className="h-8 min-w-0 flex-1 rounded-md border border-border bg-background px-2 text-xs" /></div>}</fieldset>
              <label className="block"><span className="text-[11px] font-semibold text-muted-foreground">{t("queryAgent")}</span><select ref={firstFieldRef} aria-label={t("queryAgent")} value={draft.source} onChange={(event) => setDraft((current) => ({ ...current, source: event.target.value }))} className="mt-1.5 h-9 w-full rounded-md border border-border bg-background px-2.5 text-xs text-foreground">{SOURCES.map((item) => <option key={item.value} value={item.value}>{t(item.label)}</option>)}</select></label>
              <label className="block"><span className="text-[11px] font-semibold text-muted-foreground">{t("queryProject")}</span><input aria-label={t("queryProject")} value={draft.project} onChange={(event) => setDraft((current) => ({ ...current, project: event.target.value }))} placeholder={t("allProjects")} className="mt-1.5 h-9 w-full rounded-md border border-border bg-background px-2.5 text-xs text-foreground placeholder:text-muted-foreground" /></label>
              <label className="block"><span className="text-[11px] font-semibold text-muted-foreground">{t("queryModel")}</span><input aria-label={t("queryModel")} value={draft.model} onChange={(event) => setDraft((current) => ({ ...current, model: event.target.value }))} placeholder={t("allModels")} className="mt-1.5 h-9 w-full rounded-md border border-border bg-background px-2.5 text-xs text-foreground placeholder:text-muted-foreground" /></label>
            </div>
            <div className="mt-5 flex items-center justify-end gap-2 border-t border-border pt-3"><button type="button" onClick={() => { setDraft(filters || draft); setEditorOpen(false); }} className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground">{t("cancelQuery")}</button><button type="button" onClick={commit} className="rounded-md bg-accent px-3 py-1.5 text-xs font-semibold text-white hover:bg-accent/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent">{t("applyQuery")}</button></div>
          </div>
        </>
      )}

      {onTimeWindowChange && (
        <div className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{t("timeWindow")}</span>
          <select aria-label={t("timeWindow")} value={timeWindowHours ?? ""} onChange={(event) => onTimeWindowChange(event.target.value ? Number(event.target.value) : null)} className="h-8 rounded-md border border-border bg-card px-2 text-xs text-foreground"><option value="">{t("currentRange")}</option>{[1, 6, 12, 24].map((hours) => <option key={hours} value={hours}>{t(`window_${hours}h`)}</option>)}</select>
        </div>
      )}
      {onClearFilters && activeFilters.length > 0 && <button type="button" onClick={onClearFilters} className="mt-1 text-[11px] font-medium text-muted-foreground underline underline-offset-2 hover:text-foreground">{t("clearAll")}</button>}
    </div>
  );
}
