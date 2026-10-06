import { CalendarDays, ChevronDown, RefreshCw, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { DayPicker, type DateRange } from "react-day-picker";
import { enUS, zhCN } from "date-fns/locale";
import { format } from "date-fns";
import { useTranslation } from "react-i18next";
import type { UsageFilters } from "../lib/types";
import { getActiveQueryChips, presentProjectKey } from "../lib/queryPresentation";
import { getTimeRange, type TimePreset } from "../lib/utils";

const PRESETS: TimePreset[] = ["today", "thisWeek", "thisMonth", "thisYear", "last7d", "last30d", "custom"];
const SOURCES = [
  { value: "", label: "allSources" },
  { value: "claude", label: "claudeCode" },
  { value: "codex", label: "codex" },
  { value: "openclaw", label: "openClaw" },
  { value: "opencode", label: "openCode" },
  { value: "pi", label: "piAgent" },
];

function parseCalendarDate(value?: string): Date | undefined {
  if (!value) return undefined;
  const [date] = value.split("T");
  const [year, month, day] = date.split("-").map(Number);
  if (!year || !month || !day) return undefined;
  const parsed = new Date(year, month - 1, day);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed;
}

function formatFilterDate(value: Date | undefined, language: string): string {
  if (!value) return "";
  return format(value, language.startsWith("zh") ? "yyyy年M月d日" : "MMM d, yyyy", {
    locale: language.startsWith("zh") ? zhCN : enUS,
  });
}

interface Props {
  preset: TimePreset;
  source: string;
  onRefresh: () => void;
  filters: UsageFilters;
  onFiltersApply: (filters: UsageFilters) => void;
  onClearFilters: () => void;
}

/** A compact filter bar with a keyboard-friendly range calendar instead of native date inputs. */
export default function TimeRangeSelector({ preset, onRefresh, filters, onFiltersApply, onClearFilters }: Props) {
  const { t, i18n } = useTranslation();
  const language = i18n?.language || "zh";
  const [calendarOpen, setCalendarOpen] = useState(false);
  const calendarRef = useRef<HTMLDivElement>(null);
  const precise = Boolean(filters.from?.includes("T") || filters.to?.includes("T"));
  const presets = PRESETS.includes(preset) ? PRESETS : [preset, ...PRESETS];
  const chips = getActiveQueryChips(filters).filter((chip) => chip.key !== "source");
  const selectedRange: DateRange = {
    from: parseCalendarDate(filters.from),
    to: parseCalendarDate(filters.to),
  };

  useEffect(() => {
    if (!calendarOpen) return;
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!calendarRef.current?.contains(event.target as Node)) setCalendarOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setCalendarOpen(false);
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [calendarOpen]);

  const apply = (patch: Partial<UsageFilters>) => onFiltersApply({ ...filters, ...patch });
  const choosePreset = (next: TimePreset) => {
    apply({ preset: next, ...getTimeRange(next, filters.from, filters.to) });
    if (next !== "custom") setCalendarOpen(false);
  };
  const chooseRange = (range: DateRange | undefined) => {
    if (!range?.from) return;
    const toISODate = (date: Date) => {
      const year = date.getFullYear();
      const month = String(date.getMonth() + 1).padStart(2, "0");
      const day = String(date.getDate()).padStart(2, "0");
      return `${year}-${month}-${day}`;
    };
    const nextFrom = toISODate(range.from);
    const nextTo = range.to ? toISODate(range.to) : nextFrom;
    apply({ preset: "custom", from: precise ? `${nextFrom}T00:00:00.000Z` : nextFrom, to: precise ? `${nextTo}T23:59:59.999Z` : nextTo });
    if (range.to) setCalendarOpen(false);
  };
  const chipText = (chip: { key: string; value: string }) =>
    chip.key === "project" && presentProjectKey(chip.value).label === "unnamedProject" ? t("unnamedProject") : chip.value;
  const dateFromLabel = formatFilterDate(selectedRange.from, language) || t("from");
  const dateToLabel = formatFilterDate(selectedRange.to, language) || t("to");

  return (
    <div className="filter-bar flex min-w-0 flex-col gap-2">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5">
        <div role="group" aria-label={t("queryTimeRange")} className="range-presets flex min-w-0 flex-wrap items-center gap-1">
          {presets.map((item) => (
            <button
              key={item}
              type="button"
              aria-pressed={preset === item}
              onClick={() => choosePreset(item)}
              className={`range-preset inline-flex h-8 items-center rounded-lg px-2.5 text-xs font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                preset === item ? "is-active bg-accent text-on-accent" : "border border-border text-muted-foreground hover:bg-muted hover:text-foreground"
              }`}
            >
              {t(item)}
            </button>
          ))}
        </div>

        {preset === "custom" && (
          <div ref={calendarRef} className="date-picker-wrap">
            <button
              type="button"
              aria-label={t("queryTimeRange")}
              aria-expanded={calendarOpen}
              onClick={() => setCalendarOpen((open) => !open)}
              className="date-range-fields date-range-trigger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <CalendarDays className="date-range-icon" aria-hidden="true" />
              <span className="date-field-value">{dateFromLabel}</span>
              <span className="date-range-separator" aria-hidden="true">–</span>
              <span className="date-field-value">{dateToLabel}</span>
              <ChevronDown className={`date-range-chevron ${calendarOpen ? "is-open" : ""}`} aria-hidden="true" />
            </button>
            {calendarOpen && (
              <div className="date-picker-popover" role="dialog" aria-label={t("queryTimeRange")}>
                <DayPicker
                  mode="range"
                  selected={selectedRange}
                  onSelect={chooseRange}
                  defaultMonth={selectedRange.from || new Date()}
                  locale={language.startsWith("zh") ? zhCN : enUS}
                  weekStartsOn={language.startsWith("zh") ? 1 : 0}
                  showOutsideDays
                  fixedWeeks
                  classNames={{
                    months: "rdp-months",
                    month: "rdp-month",
                    month_caption: "rdp-caption",
                    caption_label: "rdp-caption-label",
                    nav: "rdp-nav",
                    button_previous: "rdp-nav-button",
                    button_next: "rdp-nav-button",
                    month_grid: "rdp-table",
                    weekdays: "rdp-weekdays",
                    weekday: "rdp-weekday",
                    week: "rdp-week",
                    day: "rdp-day",
                    day_button: "rdp-day-button",
                    range_start: "rdp-range-start",
                    range_middle: "rdp-range-middle",
                    range_end: "rdp-range-end",
                    today: "rdp-today",
                    outside: "rdp-outside",
                  }}
                />
              </div>
            )}
          </div>
        )}

        <select
          aria-label={t("queryAgent")}
          value={filters.source}
          onChange={(event) => apply({ source: event.target.value })}
          className="field h-8 w-auto"
        >
          {SOURCES.map((item) => <option key={item.value} value={item.value}>{t(item.label)}</option>)}
        </select>

        <button
          type="button"
          onClick={onRefresh}
          aria-label={t("refresh")}
          title={t("refresh")}
          className="icon-button ml-auto border border-border text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

      {chips.length > 0 && (
        <div className="filter-chips flex min-w-0 flex-wrap items-center gap-1.5" aria-label={t("activeFilters")}>
          {chips.map((chip) => (
            <span key={chip.key} className="chip chip-accent max-w-full" title={chip.value}>
              <span className="truncate">{t(chip.key)}: {chipText(chip)}</span>
              <button
                type="button"
                aria-label={`${t("removeFilter")} ${chip.value}`}
                onClick={() => apply({ [chip.key]: "" })}
                className="rounded-full p-0.5 hover:bg-accent/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </button>
            </span>
          ))}
          <button type="button" onClick={onClearFilters} className="text-2xs font-medium text-muted-foreground underline underline-offset-2 hover:text-foreground">
            {t("clearAll")}
          </button>
        </div>
      )}
    </div>
  );
}
