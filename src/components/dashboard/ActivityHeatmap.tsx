import { Fragment } from "react";
import type { HeatmapCell } from "../../lib/types";
import { fmtCost, fmtTokens } from "../../lib/utils";

// SQLite's STRFTIME('%w') numbers Sunday as 0; readers expect Monday first.
const WEEK_ORDER = [1, 2, 3, 4, 5, 6, 0];
const WEEKDAY_KEYS = [
  "weekdaySun", "weekdayMon", "weekdayTue", "weekdayWed",
  "weekdayThu", "weekdayFri", "weekdaySat",
];
const HOURS = Array.from({ length: 24 }, (_, hour) => hour);
const LEGEND_SHADES = ["bg-muted", "bg-accent/20", "bg-accent/45", "bg-accent/70", "bg-accent"];

/** Five legible steps; an empty cell must not look like a low-activity one. */
function shade(tokens: number, peak: number): string {
  if (tokens <= 0) return "bg-muted";
  const ratio = peak > 0 ? tokens / peak : 0;
  if (ratio > 0.6) return "bg-accent";
  if (ratio > 0.3) return "bg-accent/70";
  if (ratio > 0.1) return "bg-accent/45";
  return "bg-accent/20";
}

export default function ActivityHeatmap({ cells, t }: {
  cells: HeatmapCell[];
  t: (key: string) => string;
}) {
  if (cells.length === 0) {
    return <div className="py-8 text-center text-xs text-muted-foreground">{t("noUsageData")}</div>;
  }

  const buckets = new Map(cells.map((cell) => [`${cell.weekday}-${cell.hour}`, cell]));
  const peak = cells.reduce((max, cell) => Math.max(max, cell.tokens), 0);

  return (
    <div data-testid="activity-heatmap" className="min-w-0 overflow-x-auto">
      <div className="min-w-[34rem]">
        <div className="grid grid-cols-[2.25rem_repeat(24,minmax(0,1fr))] gap-[3px]">
          <span aria-hidden="true" />
          {HOURS.map((hour) => (
            <span key={hour} className="text-center text-[9px] tabular-nums text-muted-foreground">
              {hour % 3 === 0 ? hour : ""}
            </span>
          ))}

          {WEEK_ORDER.map((weekday) => (
            <Fragment key={weekday}>
              <span className="self-center text-[10px] text-muted-foreground">
                {t(WEEKDAY_KEYS[weekday])}
              </span>
              {HOURS.map((hour) => {
                const cell = buckets.get(`${weekday}-${hour}`);
                const label = `${t(WEEKDAY_KEYS[weekday])} ${String(hour).padStart(2, "0")}:00`;
                const detail = cell
                  ? `${label} · ${cell.calls} ${t("calls")} · ${fmtTokens(cell.tokens)} ${t("tokens")} · ${fmtCost(cell.cost)}`
                  : `${label} · ${t("noUsageData")}`;
                return (
                  <span
                    key={hour}
                    title={detail}
                    aria-label={detail}
                    className={`h-8 rounded-[2px] ${shade(cell?.tokens ?? 0, peak)}`}
                  />
                );
              })}
            </Fragment>
          ))}
        </div>

        <div className="mt-3 flex items-center gap-1.5 text-[10px] text-muted-foreground">
          <span>{t("heatmapLess")}</span>
          {LEGEND_SHADES.map((step) => (
            <span key={step} className={`h-3 w-3 rounded-[2px] ${step}`} />
          ))}
          <span>{t("heatmapMore")}</span>
        </div>
      </div>
    </div>
  );
}
