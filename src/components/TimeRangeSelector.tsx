import { useTranslation } from "react-i18next";
import { TimePreset } from "../lib/utils";

const PRESETS: TimePreset[] = ["today", "thisWeek", "thisMonth", "thisYear", "last3d", "last7d", "last30d", "custom"];
const GRANULARITIES = ["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"];
const SOURCES = [
  { value: "", label: "allSources" },
  { value: "claude", label: "claudeCode" },
  { value: "codex", label: "codex" },
  { value: "openclaw", label: "openClaw" },
  { value: "opencode", label: "openCode" },
];

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
}

export default function TimeRangeSelector({
  preset, onPresetChange, granularity, onGranularityChange,
  source, onSourceChange, onRefresh, customFrom, customTo,
  onCustomFromChange, onCustomToChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="flex bg-card border border-border rounded-lg p-0.5">
        {PRESETS.map((p) => (
          <button
            key={p}
            onClick={() => { onPresetChange(p); localStorage.setItem("au-preset", p); }}
            className={`px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${
              preset === p ? "bg-accent text-white" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t(p)}
          </button>
        ))}
      </div>

      {preset === "custom" && (
        <div className="flex items-center gap-2">
          <input type="date" value={customFrom} onChange={(e) => onCustomFromChange?.(e.target.value)}
            className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm" />
          <span className="text-muted-foreground text-sm">{t("to")}</span>
          <input type="date" value={customTo} onChange={(e) => onCustomToChange?.(e.target.value)}
            className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm" />
        </div>
      )}

      <select value={granularity}
        onChange={(e) => { onGranularityChange(e.target.value); localStorage.setItem("au-granularity", e.target.value); }}
        className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm">
        {GRANULARITIES.map((g) => (
          <option key={g} value={g}>{t(`gran_${g}`)}</option>
        ))}
      </select>

      <select value={source}
        onChange={(e) => { onSourceChange(e.target.value); localStorage.setItem("au-source", e.target.value); }}
        className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm">
        {SOURCES.map((s) => (
          <option key={s.value} value={s.value}>{t(s.label)}</option>
        ))}
      </select>

      <button onClick={onRefresh}
        className="ml-auto bg-accent text-white px-3 py-1.5 rounded-lg text-sm font-medium hover:bg-accent/90">
        {t("refresh")}
      </button>
    </div>
  );
}
