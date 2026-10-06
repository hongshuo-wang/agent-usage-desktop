import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { Info } from "lucide-react";
import ChartCard from "../components/ChartCard";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import TimeRangeSelector from "../components/TimeRangeSelector";
import TokenSummary from "../components/dashboard/TokenSummary";
import UsageInsight from "../components/dashboard/UsageInsight";
import ActivityHeatmap from "../components/dashboard/ActivityHeatmap";
import { fetchAPI } from "../lib/api";
import { buildDashboardInsight } from "../lib/dashboardPresentation";
import type {
  CollectionIndexStatus,
  DashboardStats,
  HeatmapCell,
  ThroughputResult,
  TokensRow,
  UsageBreakdown,
  UsageFilters,
} from "../lib/types";
import {
  DEFAULT_USAGE_FILTERS,
  buildSessionsSearch,
  getInitialUsageFilters,
  getUsageRequestParams,
  persistUsageFilters,
} from "../lib/usageFilters";
import { CHART_COLORS, fmtCost, fmtTokens, getTimeRange, shortBucket } from "../lib/utils";
import { bucketThroughputSeries, buildThroughputView, type ThroughputViewMode } from "../lib/throughputScale";
import { presentProjectKey } from "../lib/queryPresentation";

type DashboardData = {
  stats: DashboardStats;
  tokens: TokensRow[];
  heatmap: HeatmapCell[];
  sources: UsageBreakdown[];
  models: UsageBreakdown[];
  projects: UsageBreakdown[];
  collectionStatus: CollectionIndexStatus;
};

const EMPTY_THROUGHPUT: ThroughputResult = {
  average_active_minute: { rpm: 0, input_tpm: 0, cache_read_tpm: 0, cache_create_tpm: 0, output_tpm: 0, total_tpm: 0 },
  peak_rolling_60s: { rpm: 0, input_tpm: 0, cache_read_tpm: 0, cache_create_tpm: 0, output_tpm: 0, total_tpm: 0 },
  p95_rolling_60s: { rpm: 0, input_tpm: 0, cache_read_tpm: 0, cache_create_tpm: 0, output_tpm: 0, total_tpm: 0 },
  series: [],
};

function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded bg-muted ${className}`} />;
}

function DashboardSkeleton() {
  return (
    <div className="min-w-0 space-y-4" aria-label="loading">
      <Skeleton className="h-36 w-full" />
      <Skeleton className="h-24 w-full" />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.55fr)_minmax(17rem,1fr)]">
        <Skeleton className="h-80" />
        <Skeleton className="h-80" />
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Skeleton className="h-56" />
        <Skeleton className="h-56" />
      </div>
      <Skeleton className="h-52 w-full" />
    </div>
  );
}

function BreakdownRows({
  rows,
  onSelect,
  t,
  compact = false,
  composition = false,
  projectLabels = false,
}: {
  rows: UsageBreakdown[];
  onSelect: (key: string) => void;
  t: (key: string) => string;
  compact?: boolean;
  composition?: boolean;
  projectLabels?: boolean;
}) {
  if (!rows.length) {
    return <div className="py-8 text-center text-xs text-muted-foreground">{t("noUsageData")}</div>;
  }
  const maxTokens = Math.max(...rows.map((row) => row.total_tokens), 1);
  const totalTokens = rows.reduce((sum, row) => sum + row.total_tokens, 0);
  return (
    <div className="min-w-0 space-y-0.5">
      {rows.slice(0, compact ? 6 : 8).map((row, index) => {
        const projectPresentation = projectLabels ? presentProjectKey(row.key) : { label: row.key };
        const visibleKey = projectPresentation.label === "unnamedProject" ? t("unnamedProject") : projectPresentation.label;
        const share = totalTokens > 0 ? (row.total_tokens / totalTokens) * 100 : 0;
        const barWidth = composition ? share : (row.total_tokens / maxTokens) * 100;
        return (
          <button
            key={row.key || `${index}`}
            type="button"
            aria-label={`${t("viewSessionsFor")} ${visibleKey || t("unknown")}`}
            onClick={() => onSelect(row.key)}
            className="group grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium" title={projectPresentation.detail || row.key}>{visibleKey || t("unknown")}</span>
              {projectPresentation.detail && <span className="block truncate text-2xs text-muted-foreground" title={projectPresentation.detail}>{projectPresentation.detail}</span>}
              <span className="mt-1.5 block h-1 overflow-hidden rounded-full bg-muted">
                <span
                  data-testid={composition ? "composition-share" : undefined}
                  className="block h-full rounded-full bg-accent transition-[width] duration-200"
                  style={{ width: `${composition ? barWidth : Math.max(3, barWidth)}%` }}
                />
              </span>
            </span>
            <span className="text-right">
              <span className="block text-xs font-semibold tabular-nums">{fmtTokens(row.total_tokens)}</span>
              {composition ? (
                <>
                  <span className="block text-2xs tabular-nums text-muted-foreground">
                    {fmtCost(row.total_cost)}
                  </span>
                  <span className="block text-2xs tabular-nums text-muted-foreground">{share.toFixed(1)}%</span>
                  <span className="block text-2xs text-muted-foreground">{row.sessions} {t("sessions")}</span>
                </>
              ) : (
                <span className="block text-2xs text-muted-foreground">
                  {row.sessions} {t("sessions")} / {row.calls} {t("calls")}
                </span>
              )}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function formatThroughput(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function HelpTooltip({ label, align = "right" }: { label: string; align?: "left" | "right" }) {
  const tooltipID = useId();
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ left: 0, top: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const tooltipRef = useRef<HTMLSpanElement>(null);

  const updatePosition = useCallback(() => {
    const trigger = triggerRef.current;
    const tooltip = tooltipRef.current;
    if (!trigger || !tooltip) return;

    const triggerRect = trigger.getBoundingClientRect();
    const tooltipRect = tooltip.getBoundingClientRect();
    const viewportMargin = 8;
    const gap = 6;
    const preferredLeft = align === "left" ? triggerRect.left : triggerRect.right - tooltipRect.width;
    const maxLeft = Math.max(viewportMargin, window.innerWidth - tooltipRect.width - viewportMargin);
    const left = Math.min(Math.max(preferredLeft, viewportMargin), maxLeft);
    const below = triggerRect.bottom + gap;
    const top = below + tooltipRect.height <= window.innerHeight - viewportMargin
      ? below
      : Math.max(viewportMargin, triggerRect.top - tooltipRect.height - gap);

    setPosition({ left, top });
  }, [align]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [open, updatePosition]);

  return (
    <span
      className="relative inline-flex shrink-0"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        ref={triggerRef}
        type="button"
        aria-label={label}
        aria-describedby={open ? tooltipID : undefined}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setOpen(false);
        }}
        className="inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        <Info aria-hidden="true" className="h-3 w-3" />
      </button>
      {open && createPortal(
        <span
          ref={tooltipRef}
          id={tooltipID}
          role="tooltip"
          style={{ left: position.left, top: position.top }}
          className="pointer-events-none fixed z-[100] w-max max-w-64 rounded-lg border border-border bg-card px-2.5 py-2 text-left font-sans text-2xs font-normal leading-4 text-foreground shadow-lg"
        >
          {label}
        </span>,
        document.body,
      )}
    </span>
  );
}

function ThroughputMatrix({ throughput, t }: {
  throughput: ThroughputResult;
  t: (key: string) => string;
}) {
  const rows = [
    ["throughput-average", t("averageActiveMinute"), throughput.average_active_minute],
    ["throughput-peak", t("peakRolling60s"), throughput.peak_rolling_60s],
    ["throughput-p95", t("p95Rolling60s"), throughput.p95_rolling_60s],
  ] as const;
  const columns = [
    ["window", t("window"), t("throughputWindowHelp")],
    ["rpm", t("rpm"), t("rpmHelp")],
    ["total_tpm", t("totalTPM"), t("totalTPMHelp")],
    ["input", t("input"), t("inputTPMHelp")],
    ["cache_read", t("cacheRead"), t("cacheReadTPMHelp")],
    ["cache_create", t("cacheCreate"), t("cacheCreateTPMHelp")],
    ["output", t("output"), t("outputTPMHelp")],
  ] as const;
  return (
    <div data-testid="throughput-matrix" className="min-w-0 overflow-x-auto">
      <table className="w-full min-w-[32rem] text-2xs">
        <thead className="text-left text-muted-foreground">
          <tr>
            {columns.map(([key, label, help], index) => (
              <th
                key={key}
                className={`whitespace-nowrap pb-1.5 font-medium ${index < columns.length - 1 ? "pr-2" : ""} ${index ? "text-right" : ""}`}
              >
                <span className="inline-flex items-center gap-0.5">
                  {label}
                  <HelpTooltip label={help} align={index === 0 ? "left" : "right"} />
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border tabular-nums">
          {rows.map(([testID, label, values]) => (
            <tr key={testID} data-testid={testID}>
              <th className="py-1.5 pr-2 text-left font-medium">{label}</th>
              <td className="py-1.5 pr-2 text-right">{formatThroughput(values.rpm)}</td>
              <td className="py-1.5 pr-2 text-right font-semibold">{formatThroughput(values.total_tpm)}</td>
              <td className="py-1.5 pr-2 text-right">{formatThroughput(values.input_tpm)}</td>
              <td className="py-1.5 pr-2 text-right">{formatThroughput(values.cache_read_tpm)}</td>
              <td className="py-1.5 pr-2 text-right">{formatThroughput(values.cache_create_tpm)}</td>
              <td className="py-1.5 text-right">{formatThroughput(values.output_tpm)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

const COLLECTION_STATUS_KEYS: Record<CollectionIndexStatus["status"], string> = {
  empty: "collectionStatusEmpty",
  stats_only: "collectionStatusStatsOnly",
  missing_source: "collectionStatusMissing",
  rebuild_required: "collectionStatusRebuild",
  stale_parser: "collectionStatusStale",
  partial: "collectionStatusPartial",
  available: "collectionStatusAvailable",
  stale: "collectionStatusScanStale",
};

function formatLastIndexed(value: string | null): string {
  return value ? value.slice(0, 16).replace("T", " ") : "";
}

function ModelUsageRows({ rows, onSelect, t }: {
  rows: UsageBreakdown[];
  onSelect: (key: string) => void;
  t: (key: string) => string;
}) {
  if (!rows.length) {
    return <div className="py-8 text-center text-xs text-muted-foreground">{t("noUsageData")}</div>;
  }

  const totalTokens = rows.reduce((sum, row) => sum + row.total_tokens, 0);
  return (
    <div className="min-w-0 space-y-0.5">
      {rows.slice(0, 8).map((row, index) => {
        const share = totalTokens > 0 ? (row.total_tokens / totalTokens) * 100 : 0;
        const displayShare = share > 0 ? share : 0;
        return (
          <button
            key={row.key || `${index}`}
            type="button"
            aria-label={`${t("viewSessionsFor")} ${row.key || t("unknown")}`}
            onClick={() => onSelect(row.key)}
            className="group grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium">{row.key || t("unknown")}</span>
              <span className="mt-1.5 block h-1.5 overflow-hidden rounded-full bg-muted">
                <span
                  data-testid="model-usage-share"
                  role="progressbar"
                  aria-label={`${row.key || t("unknown")} ${share.toFixed(1)}%`}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(displayShare)}
                  className="block h-full rounded-full bg-accent transition-[width] duration-200"
                  style={{ width: `${displayShare}%` }}
                />
              </span>
            </span>
            <span className="text-right">
              <span className="block text-xs font-semibold tabular-nums">{fmtTokens(row.total_tokens)}</span>
              <span className="block text-2xs tabular-nums text-muted-foreground">
                <span>{fmtCost(row.total_cost)}</span>
                <span> / {share.toFixed(1)}%</span>
              </span>
              <span className="block text-2xs text-muted-foreground">
                {`${row.sessions} ${t("sessions")} / ${row.calls} ${t("calls")}`}
              </span>
            </span>
          </button>
        );
      })}
    </div>
  );
}

export default function Dashboard() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const [filters, setFilters] = useState<UsageFilters>(() => getInitialUsageFilters(location.search));
  const [granularity, setGranularity] = useState(() => localStorage.getItem("au-granularity") || "1h");
  const [data, setData] = useState<DashboardData | null>(null);
  const [throughput, setThroughput] = useState<ThroughputResult>(EMPTY_THROUGHPUT);
  const [throughputModel, setThroughputModel] = useState("");
  const [throughputMode, setThroughputMode] = useState<ThroughputViewMode>(() => (
    localStorage.getItem("au-throughput-mode") === "absolute" ? "absolute" : "trend"
  ));
  const [loading, setLoading] = useState(true);
  const [throughputLoading, setThroughputLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [throughputError, setThroughputError] = useState<string | null>(null);
  const overviewGenerationRef = useRef(0);
  const throughputGenerationRef = useRef(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => persistUsageFilters(filters), [filters]);

  const fetchData = useCallback(async () => {
    const generation = ++overviewGenerationRef.current;
    const request = getUsageRequestParams(filters);
    const trendRequest = { ...request, granularity };
    setLoading(true);
    setError(null);
    try {
      const [stats, tokens, heatmap, sources, models, projects, collectionStatus] = await Promise.all([
        fetchAPI<DashboardStats>("stats", request),
        fetchAPI<TokensRow[]>("tokens-over-time", trendRequest),
        fetchAPI<HeatmapCell[]>("activity-heatmap", request),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "source" }),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "model" }),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "project" }),
        fetchAPI<CollectionIndexStatus>("collection-index-status", {}),
      ]);
      if (mountedRef.current && generation === overviewGenerationRef.current) {
        setData({
          stats,
          tokens: tokens || [],
          heatmap: heatmap || [],
          sources: sources || [],
          models: models || [],
          projects: projects || [],
          collectionStatus,
        });
      }
    } catch (cause) {
      if (mountedRef.current && generation === overviewGenerationRef.current) {
        console.error("Dashboard fetch error:", cause);
        setError(cause instanceof Error ? cause.message : String(cause));
      }
    } finally {
      if (mountedRef.current && generation === overviewGenerationRef.current) {
        setLoading(false);
      }
    }
  }, [filters.from, filters.to, filters.source, granularity]);

  useEffect(() => { void fetchData(); }, [fetchData]);

  const fetchThroughput = useCallback(async () => {
    const generation = ++throughputGenerationRef.current;
    setThroughputLoading(true);
    setThroughputError(null);
    try {
      const request = {
        ...getUsageRequestParams(filters),
        ...(throughputModel ? { model: throughputModel } : {}),
      };
      const result = await fetchAPI<ThroughputResult>("throughput", request);
      if (mountedRef.current && generation === throughputGenerationRef.current) {
        setThroughput(result || EMPTY_THROUGHPUT);
      }
    } catch (cause) {
      if (mountedRef.current && generation === throughputGenerationRef.current) {
        console.error("Throughput fetch error:", cause);
        setThroughputError(cause instanceof Error ? cause.message : String(cause));
      }
    } finally {
      if (mountedRef.current && generation === throughputGenerationRef.current) {
        setThroughputLoading(false);
      }
    }
  }, [filters.from, filters.to, filters.source, throughputModel]);

  useEffect(() => { void fetchThroughput(); }, [fetchThroughput]);

  const updateGranularity = (value: string) => {
    setGranularity(value);
    localStorage.setItem("au-granularity", value);
  };

  const applyQueryFilters = (next: UsageFilters) => {
    setFilters(next);
    const query = new URLSearchParams(location.search);
    for (const key of ["from", "to", "source", "model", "project"] as const) {
      if (next[key]) query.set(key, next[key]);
      else query.delete(key);
    }
    navigate({ pathname: location.pathname, search: query.toString() }, { replace: true });
  };

  const clearAllQueryFilters = () => {
    const range = getTimeRange(DEFAULT_USAGE_FILTERS.preset);
    applyQueryFilters({ ...filters, ...range, preset: DEFAULT_USAGE_FILTERS.preset, source: "", model: "", project: "" });
  };

  const openSessions = (overrides: Partial<UsageFilters>) => {
    navigate({ pathname: "/sessions", search: buildSessionsSearch(filters, overrides) });
  };

  const tokenOption = useMemo(() => ({
    tooltip: { trigger: "axis", confine: true },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: {
      type: "category",
      data: data?.tokens.map((row) => row.date) || [],
      // Display-only shortening: click-to-sessions reads the raw bucket value.
      axisLabel: { hideOverlap: true, fontSize: 11, formatter: shortBucket },
    },
    yAxis: { type: "value" },
    series: [
      { name: t("input"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.input_tokens) || [], color: CHART_COLORS[0] },
      { name: t("output"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.output_tokens) || [], color: CHART_COLORS[1] },
      { name: t("cacheRead"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.cache_read) || [], color: CHART_COLORS[3] },
      { name: t("cacheCreate"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.cache_create) || [], color: CHART_COLORS[2] },
    ],
  }), [data?.tokens, t]);

  const throughputSeries = useMemo(() => bucketThroughputSeries(throughput.series), [throughput.series]);
  const throughputView = useMemo(() => buildThroughputView(throughputSeries, throughputMode), [throughputSeries, throughputMode]);

  const usageInsight = useMemo(() => buildDashboardInsight(
    data?.tokens || [],
    data?.models || [],
    data?.projects || [],
  ), [data?.tokens, data?.models, data?.projects]);

  const throughputOption = useMemo(() => ({
    tooltip: { trigger: "axis", confine: true },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: {
      type: "category",
      data: throughputSeries.map((point) => point.minute),
      axisLabel: { hideOverlap: true, fontSize: 10, formatter: shortBucket },
    },
    yAxis: [
      { type: "value", name: "TPM", max: throughputView.ceiling || undefined },
      { type: "value", name: "RPM", position: "right", splitLine: { show: false } },
    ],
    series: [
      { name: t("input"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughputSeries.map((point) => point.input_tpm), color: CHART_COLORS[0], markPoint: throughputMode === "trend" ? {
        symbol: "pin",
        symbolSize: 34,
        label: { formatter: t("throughputHighUsage"), fontSize: 9 },
        data: throughputView.peakIndices.map((index) => ({ coord: [index, throughputView.ceiling], value: throughputSeries[index]?.total_tpm })),
      } : undefined },
      { name: t("cacheRead"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughputSeries.map((point) => point.cache_read_tpm), color: CHART_COLORS[3] },
      { name: t("cacheCreate"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughputSeries.map((point) => point.cache_create_tpm), color: CHART_COLORS[2] },
      { name: t("output"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughputSeries.map((point) => point.output_tpm), color: CHART_COLORS[1] },
      { name: t("rpm"), type: "line", yAxisIndex: 1, data: throughputSeries.map((point) => point.rpm), color: CHART_COLORS[5], smooth: true },
    ],
  }), [throughputSeries, throughputView.ceiling, t, throughputMode]);

  const stats = data?.stats;
  const rangeDetail = `${filters.from} ${t("to")} ${filters.to}`;
  const collectionNeedsAttention = Boolean(data?.collectionStatus && data.collectionStatus.status !== "available");
  const noUsage = Boolean(data && !data.stats.total_calls && !data.stats.total_tokens
    && !data.sources.length && !data.models.length && !data.projects.length);

  return (
    <div className="mx-auto flex min-h-0 w-full min-w-0 max-w-[1180px] flex-1 flex-col gap-3 overflow-hidden">
      <section className="panel relative min-w-0 overflow-visible border-b border-border pb-3">
        <div className="pb-2">
          <PageHeader title={t("title")} hint={t("localObservability")} />
        </div>
        <div className="border-t border-border/70 pt-2.5">
          <TimeRangeSelector
            preset={filters.preset}
            source={filters.source}
            onRefresh={() => { void fetchData(); void fetchThroughput(); }}
            filters={filters}
            onFiltersApply={applyQueryFilters}
            onClearFilters={clearAllQueryFilters}
          />
        </div>
      </section>

      {data?.collectionStatus && collectionNeedsAttention && (
        <aside
          data-testid="collection-index-status"
          className="panel flex min-w-0 flex-row flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-2.5 text-xs"
        >
          <span className="chip chip-warn">{t(COLLECTION_STATUS_KEYS[data.collectionStatus.status])}</span>
          <span className="font-medium">{t("collectionIndexStatus")}</span>
          <span className="text-muted-foreground">{t("lastIndexUpdate")}</span>
          {data.collectionStatus.last_indexed_at ? (
            <time dateTime={data.collectionStatus.last_indexed_at} className="font-mono tabular-nums">
              {formatLastIndexed(data.collectionStatus.last_indexed_at)}
            </time>
          ) : (
            <span className="text-muted-foreground">{t("notAvailable")}</span>
          )}
          <span className="ml-auto text-muted-foreground">
            {data.collectionStatus.source_count} {t("indexedSources")} / {data.collectionStatus.file_count} {t("indexedFiles")} / {data.collectionStatus.malformed_lines} {t("malformedLines")}
          </span>
          <button type="button" onClick={() => navigate("/settings/index-diagnostics")} className="font-medium text-warning underline underline-offset-2 hover:text-foreground">
            {t("openSystemDiagnostics")}
          </button>
        </aside>
      )}

      {(filters.model || filters.project) && <p className="min-w-0 text-2xs text-muted-foreground">{t("overviewFilterLimitation")}</p>}

      <main aria-busy={loading} className="min-h-0 min-w-0 flex-1 overflow-y-auto pb-4">
        {loading && !data ? (
          <DashboardSkeleton />
        ) : error ? (
          <section className="panel items-center px-4 py-12 text-center">
            <p className="break-words text-sm text-danger">{error}</p>
            <button
              type="button"
              onClick={() => { void fetchData(); }}
              className="btn btn-primary mx-auto mt-4"
            >{t("retry")}</button>
          </section>
        ) : noUsage ? (
          <section className="panel items-center px-4 py-16 text-center">
            <h2 className="text-sm font-semibold">{t("noUsageData")}</h2>
            <p className="mt-1 text-xs text-muted-foreground">{t("noUsageDataDetail")}</p>
          </section>
        ) : data && stats ? (
          <div className="stagger min-w-0 space-y-6">
            <TokenSummary stats={stats} rangeDetail={rangeDetail} />
            <UsageInsight
              insight={usageInsight}
              onOpenDay={(day) => openSessions({ from: day, to: day })}
              onOpenModel={(model) => openSessions({ model })}
              onOpenProject={(project) => openSessions({ project })}
            />

            <section
              data-testid="dashboard-band-analysis"
              className="grid min-w-0 grid-cols-1 gap-x-8 gap-y-6 lg:grid-cols-[minmax(0,1.55fr)_minmax(17rem,1fr)]"
            >
              <Panel
                title={t("tokenTrend")}
                hint={t("clickDateForSessions")}
                actions={
                  <label className="flex items-center gap-1.5 text-2xs text-muted-foreground">
                    <span>{t("trendGranularity")}</span>
                    <select
                      aria-label={t("trendGranularity")}
                      value={granularity}
                      onChange={(event) => updateGranularity(event.target.value)}
                      className="field h-7 w-auto"
                    >
                      {["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"].map((value) => <option key={value} value={value}>{t(`gran_${value}`)}</option>)}
                    </select>
                  </label>
                }
              >
                <ChartCard
                  option={tokenOption}
                  className="h-60"
                  onEvents={{
                    click: ({ name }) => {
                      const day = name?.match(/^\d{4}-\d{2}-\d{2}/)?.[0];
                      if (day) openSessions({ from: day, to: day });
                    },
                  }}
                />
                <div data-testid="token-components" className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {[
                    [t("input"), stats.input_tokens],
                    [t("output"), stats.output_tokens],
                    [t("cacheRead"), stats.cache_read],
                    [t("cacheCreate"), stats.cache_create],
                  ].map(([label, value]) => (
                    <div key={String(label)} className="inset min-w-0 px-3 py-2">
                      <div className="truncate text-2xs text-muted-foreground">{label}</div>
                      <div className="mt-0.5 truncate text-[13px] font-semibold tabular-nums">{fmtTokens(Number(value))}</div>
                    </div>
                  ))}
                </div>
              </Panel>
              <Panel title={t("modelUsage")} hint={t("tokens")}>
                <ModelUsageRows rows={data.models} onSelect={(model) => openSessions({ model })} t={t} />
              </Panel>
            </section>

            <section
              data-testid="dashboard-band-detail"
              className="grid min-w-0 grid-cols-1 gap-x-8 gap-y-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(15rem,0.85fr)]"
            >
              <Panel title={t("agentComposition")} hint={t("tokens")}>
                <BreakdownRows
                  rows={data.sources}
                  onSelect={(source) => openSessions({ source })}
                  t={t}
                  composition
                />
              </Panel>
              <Panel title={t("projectRanking")} hint={t("tokens")}>
                <BreakdownRows
                  rows={data.projects}
                  onSelect={(project) => openSessions({ project })}
                  t={t}
                  compact
                  projectLabels
                />
              </Panel>
            </section>

            <section data-testid="dashboard-band-rhythm">
              <Panel title={t("activityHeatmap")} hint={t("tokens")}>
                <ActivityHeatmap cells={data.heatmap} t={t} rangeDetail={rangeDetail} />
              </Panel>
            </section>

            <section data-testid="dashboard-band-throughput">
              <div aria-busy={throughputLoading} className="min-w-0">
                <Panel
                  title={
                    <span className="truncate">{t("localObservedThroughput")}</span>
                  }
                  actions={
                    <>
                      <div className="inline-flex rounded-lg border border-border p-0.5" aria-label={t("throughputScaleMode")}>
                        {(["trend", "absolute"] as const).map((mode) => (
                          <button
                            key={mode}
                            type="button"
                            aria-pressed={throughputMode === mode}
                            onClick={() => { setThroughputMode(mode); localStorage.setItem("au-throughput-mode", mode); }}
                            className={`rounded-md px-2 py-1 text-2xs font-medium transition-colors ${throughputMode === mode ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground"}`}
                          >{t(mode === "trend" ? "throughputTrendMode" : "throughputAbsoluteMode")}</button>
                        ))}
                      </div>
                      <label className="flex min-w-0 items-center gap-1.5 text-2xs text-muted-foreground">
                        <span>{t("throughputModel")}</span>
                        <select
                          aria-label={t("throughputModel")}
                          value={throughputModel}
                          onChange={(event) => setThroughputModel(event.target.value)}
                          className="field h-7 w-auto max-w-36"
                        >
                          <option value="">{t("allModels")}</option>
                          {data.models.filter((row) => row.key).map((row) => (
                            <option key={row.key} value={row.key}>{row.key}</option>
                          ))}
                        </select>
                      </label>
                      <HelpTooltip label={t("localObservedThroughputHelp")} align="left" />
                    </>
                  }
                >
                  <div className="grid min-w-0 grid-cols-1 gap-5 xl:grid-cols-[minmax(0,32rem)_minmax(0,1fr)]">
                    <div className="min-w-0">
                      <ThroughputMatrix throughput={throughput} t={t} />
                      {throughputError && (
                        <p className="mt-2 break-words text-xs text-danger">{throughputError}</p>
                      )}
                    </div>
                    <ChartCard title={t("observedTPMTrend")} option={throughputOption} className="h-48" />
                  </div>
                </Panel>
              </div>
            </section>
          </div>
        ) : null}
      </main>
    </div>
  );
}
