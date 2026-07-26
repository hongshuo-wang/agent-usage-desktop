import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { Info } from "lucide-react";
import ChartCard from "../components/ChartCard";
import TimeRangeSelector from "../components/TimeRangeSelector";
import { fetchAPI } from "../lib/api";
import type {
  CollectionIndexStatus,
  DashboardStats,
  ThroughputResult,
  TokensRow,
  UsageBreakdown,
  UsageFilters,
} from "../lib/types";
import {
  buildSessionsSearch,
  getInitialUsageFilters,
  getUsageRequestParams,
  persistUsageFilters,
} from "../lib/usageFilters";
import { CHART_COLORS, fmtCost, fmtTokens, getTimeRange, type TimePreset } from "../lib/utils";
import { buildThroughputView, type ThroughputViewMode } from "../lib/throughputScale";
import { presentProjectKey } from "../lib/queryPresentation";

type DashboardData = {
  stats: DashboardStats;
  tokens: TokensRow[];
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
    <div className="min-w-0 space-y-4 overflow-hidden" aria-label="loading">
      {["core", "analysis", "detail"].map((band, index) => (
        <section key={band} className={index % 2 === 0 ? "bg-card/30 px-4 py-4" : "px-4 py-4"}>
          <Skeleton className="mb-4 h-4 w-32" />
          <div className={`grid gap-3 ${index === 0 ? "grid-cols-2 lg:grid-cols-5" : "grid-cols-1 lg:grid-cols-3"}`}>
            {[1, 2, 3, 4, 5].slice(0, index === 0 ? 5 : 3).map((item) => (
              <Skeleton key={item} className="h-24 min-w-0" />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function Metric({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="min-w-0 border-l-2 border-border pl-3 first:border-l-0 first:pl-0">
      <div className="truncate text-xs font-medium text-muted-foreground">{label}</div>
      <div className="mt-1 truncate font-mono text-2xl font-semibold tabular-nums">{value}</div>
      {detail && <div className="mt-1 truncate text-[11px] text-muted-foreground">{detail}</div>}
    </div>
  );
}

function BandTitle({ title, detail }: { title: string; detail?: string }) {
  return (
    <header className="mb-3 flex min-w-0 items-baseline justify-between gap-3">
      <h2 className="truncate text-sm font-semibold">{title}</h2>
      {detail && <p className="truncate text-xs text-muted-foreground">{detail}</p>}
    </header>
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
    <div className="min-w-0 space-y-1">
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
            className="group grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 rounded-md px-2 py-2 text-left transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium" title={projectPresentation.detail || row.key}>{visibleKey || t("unknown")}</span>
              {projectPresentation.detail && <span className="block truncate text-[10px] text-muted-foreground" title={projectPresentation.detail}>{projectPresentation.detail}</span>}
              <span className="mt-1 block h-1 overflow-hidden rounded bg-muted">
                <span
                  data-testid={composition ? "composition-share" : undefined}
                  className="block h-full rounded bg-accent transition-[width] duration-200"
                  style={{ width: `${composition ? barWidth : Math.max(3, barWidth)}%` }}
                />
              </span>
            </span>
            <span className="text-right">
              <span className="block font-mono text-xs font-semibold tabular-nums">{fmtTokens(row.total_tokens)}</span>
              {composition ? (
                <>
                  <span className="block font-mono text-[10px] tabular-nums text-muted-foreground">
                    {row.unknown_price ? `${fmtCost(row.total_cost)}*` : fmtCost(row.total_cost)}
                  </span>
                  <span className="block font-mono text-[10px] tabular-nums text-muted-foreground">{share.toFixed(1)}%</span>
                  <span className="block text-[10px] text-muted-foreground">{row.sessions} {t("sessions")}</span>
                </>
              ) : (
                <span className="block text-[10px] text-muted-foreground">
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
      <table className="w-full min-w-[32rem] text-[10px]">
        <thead className="text-left text-muted-foreground">
          <tr>
            {columns.map(([key, label, help], index) => (
              <th key={key} className={`pb-1.5 ${index < columns.length - 1 ? "pr-2" : ""} ${index ? "text-right" : ""}`}>
                <span className="inline-flex items-center gap-1" title={help}>
                  {label}{index > 0 && <span className="text-[9px] font-normal">({key === "rpm" ? "req/min" : "tok/min"})</span>}
                  <Info aria-hidden="true" className="h-3 w-3 shrink-0 opacity-60" />
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border font-mono tabular-nums">
          {rows.map(([testID, label, values]) => (
            <tr key={testID} data-testid={testID}>
              <th className="py-1.5 pr-2 text-left font-sans font-medium">{label}</th>
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
    <div className="min-w-0 space-y-1">
      {rows.slice(0, 8).map((row, index) => {
        const share = totalTokens > 0 ? (row.total_tokens / totalTokens) * 100 : 0;
        const displayShare = share > 0 ? share : 0;
        return (
          <button
            key={row.key || `${index}`}
            type="button"
            aria-label={`${t("viewSessionsFor")} ${row.key || t("unknown")}`}
            onClick={() => onSelect(row.key)}
            className="group grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 rounded-md px-2 py-2.5 text-left transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium">{row.key || t("unknown")}</span>
              <span className="mt-1.5 block h-2 overflow-hidden rounded bg-muted">
                <span
                  data-testid="model-usage-share"
                  role="progressbar"
                  aria-label={`${row.key || t("unknown")} ${share.toFixed(1)}%`}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(displayShare)}
                  className="block h-full rounded bg-accent transition-[width] duration-200"
                  style={{ width: `${displayShare}%` }}
                />
              </span>
            </span>
            <span className="text-right">
              <span className="block font-mono text-xs font-semibold tabular-nums">{fmtTokens(row.total_tokens)}</span>
              <span className="block font-mono text-[10px] tabular-nums text-muted-foreground">
                <span>{row.unknown_price ? `${fmtCost(row.total_cost)}*` : fmtCost(row.total_cost)}</span>
                <span> / {share.toFixed(1)}%</span>
              </span>
              <span className="block text-[10px] text-muted-foreground">
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
      const [stats, tokens, sources, models, projects, collectionStatus] = await Promise.all([
        fetchAPI<DashboardStats>("stats", request),
        fetchAPI<TokensRow[]>("tokens-over-time", trendRequest),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "source" }),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "model" }),
        fetchAPI<UsageBreakdown[]>("usage-breakdown", { ...request, dimension: "project" }),
        fetchAPI<CollectionIndexStatus>("collection-index-status", {}),
      ]);
      if (mountedRef.current && generation === overviewGenerationRef.current) {
        setData({
          stats,
          tokens: tokens || [],
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

  const updatePreset = (preset: TimePreset) => {
    setFilters((current) => ({
      ...current,
      preset,
      ...getTimeRange(preset, current.from, current.to),
    }));
  };

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
    const range = getTimeRange("last7d");
    applyQueryFilters({ ...filters, ...range, preset: "last7d", source: "", model: "", project: "" });
  };

  const openSessions = (overrides: Partial<UsageFilters>) => {
    navigate({ pathname: "/sessions", search: buildSessionsSearch(filters, overrides) });
  };

  const tokenOption = useMemo(() => ({
    tooltip: { trigger: "axis", confine: true },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: data?.tokens.map((row) => row.date) || [], axisLabel: { hideOverlap: true, fontSize: 11 } },
    yAxis: { type: "value" },
    series: [
      { name: t("input"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.input_tokens) || [], color: CHART_COLORS[0] },
      { name: t("output"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.output_tokens) || [], color: CHART_COLORS[1] },
      { name: t("cacheRead"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.cache_read) || [], color: CHART_COLORS[3] },
      { name: t("cacheCreate"), type: "bar", stack: "tokens", data: data?.tokens.map((row) => row.cache_create) || [], color: CHART_COLORS[2] },
    ],
  }), [data?.tokens, t]);

  const throughputView = useMemo(() => buildThroughputView(throughput.series, throughputMode), [throughput.series, throughputMode]);

  const throughputOption = useMemo(() => ({
    tooltip: { trigger: "axis", confine: true },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: throughput.series.map((point) => point.minute), axisLabel: { hideOverlap: true, fontSize: 10 } },
    yAxis: [
      { type: "value", name: "TPM", max: throughputView.ceiling || undefined },
      { type: "value", name: "RPM", position: "right", splitLine: { show: false } },
    ],
    series: [
      { name: t("input"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughput.series.map((point) => point.input_tpm), color: CHART_COLORS[0], markPoint: throughputMode === "trend" ? {
        symbol: "pin",
        symbolSize: 34,
        label: { formatter: t("throughputHighUsage"), fontSize: 9 },
        data: throughputView.peakIndices.map((index) => ({ coord: [index, throughputView.ceiling], value: throughput.series[index]?.total_tpm })),
      } : undefined },
      { name: t("cacheRead"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughput.series.map((point) => point.cache_read_tpm), color: CHART_COLORS[3] },
      { name: t("cacheCreate"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughput.series.map((point) => point.cache_create_tpm), color: CHART_COLORS[2] },
      { name: t("output"), type: "bar", stack: "tpm", yAxisIndex: 0, data: throughput.series.map((point) => point.output_tpm), color: CHART_COLORS[1] },
      { name: t("rpm"), type: "line", yAxisIndex: 1, data: throughput.series.map((point) => point.rpm), color: CHART_COLORS[5], smooth: true },
    ],
  }), [throughput, throughputView.ceiling, t]);

  const stats = data?.stats;
  const rangeDetail = `${filters.from} ${t("to")} ${filters.to}`;
  const collectionNeedsAttention = Boolean(data?.collectionStatus && data.collectionStatus.status !== "available");
  const noUsage = Boolean(data && !data.stats.total_calls && !data.stats.total_tokens
    && !data.sources.length && !data.models.length && !data.projects.length);

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-hidden">
      <TimeRangeSelector
        preset={filters.preset}
        onPresetChange={updatePreset}
        granularity={granularity}
        onGranularityChange={updateGranularity}
        source={filters.source}
        onSourceChange={(source) => setFilters((current) => ({ ...current, source }))}
        onRefresh={() => { void fetchData(); void fetchThroughput(); }}
        customFrom={filters.from}
        customTo={filters.to}
        onCustomFromChange={(from) => setFilters((current) => ({ ...current, preset: "custom", from }))}
        onCustomToChange={(to) => setFilters((current) => ({ ...current, preset: "custom", to }))}
        filters={filters}
        onFiltersApply={applyQueryFilters}
        onClearFilters={clearAllQueryFilters}
      />

      {data?.collectionStatus && collectionNeedsAttention && (
        <aside
          data-testid="collection-index-status"
          className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs"
        >
          <span className="font-medium">{t("collectionIndexStatus")}</span>
          <span className="font-medium text-amber-600">{t(COLLECTION_STATUS_KEYS[data.collectionStatus.status])}</span>
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
          <button type="button" onClick={() => navigate("/settings?section=index")} className="font-medium text-amber-700 underline underline-offset-2 hover:text-foreground">
            {t("openSystemDiagnostics")}
          </button>
        </aside>
      )}

      {(filters.model || filters.project) && <p className="px-1 text-[11px] text-muted-foreground">{t("overviewFilterLimitation")}</p>}

      <main aria-busy={loading} className="min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto pb-4">
        {loading && !data ? (
          <DashboardSkeleton />
        ) : error ? (
          <section className="bg-red-500/5 px-4 py-12 text-center">
            <p className="break-words text-sm text-red-500">{error}</p>
            <button
              type="button"
              onClick={() => { void fetchData(); }}
              className="mt-3 rounded bg-accent px-3 py-1.5 text-sm font-medium text-white hover:bg-accent/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >{t("retry")}</button>
          </section>
        ) : noUsage ? (
          <section className="bg-card/30 px-4 py-16 text-center">
            <h2 className="text-sm font-semibold">{t("noUsageData")}</h2>
            <p className="mt-1 text-xs text-muted-foreground">{t("noUsageDataDetail")}</p>
          </section>
        ) : data && stats ? (
          <>
            <section data-testid="dashboard-band-core" className="bg-card/40 px-4 py-4">
              <BandTitle title={t("coreMetrics")} detail={rangeDetail} />
              <div className="grid grid-cols-2 gap-x-4 gap-y-4 lg:grid-cols-5">
                <Metric label={t("totalTokens")} value={fmtTokens(stats.total_tokens)} detail={`${stats.total_calls} ${t("apiCalls")}`} />
                <Metric label={t("localCostEstimate")} value={fmtCost(stats.total_cost)} />
                <Metric label={t("sessions")} value={String(stats.total_sessions)} />
                <Metric label={t("userMessages")} value={String(stats.total_prompts)} />
                <Metric label={t("cacheHitRate")} value={`${(stats.cache_hit_rate * 100).toFixed(1)}%`} />
              </div>
            </section>

            <section data-testid="dashboard-band-analysis" className="px-4 py-4">
              <div className="grid min-w-0 grid-cols-1 gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
                <div className="min-w-0">
                  <header className="mb-3 flex min-w-0 flex-wrap items-end justify-between gap-2">
                    <div className="min-w-0">
                      <h2 className="truncate text-sm font-semibold">{t("tokenTrend")}</h2>
                      <p className="truncate text-xs text-muted-foreground">{t("clickDateForSessions")}</p>
                    </div>
                    <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
                      <span>{t("trendGranularity")}</span>
                      <select aria-label={t("trendGranularity")} value={granularity} onChange={(event) => updateGranularity(event.target.value)} className="h-8 rounded-md border border-border bg-card px-2 text-xs text-foreground">
                        {["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"].map((value) => <option key={value} value={value}>{t(`gran_${value}`)}</option>)}
                      </select>
                    </label>
                  </header>
                  <ChartCard
                    title={t("tokenUsage")}
                    option={tokenOption}
                    className="h-60"
                    onEvents={{
                      click: ({ name }) => {
                        const day = name?.match(/^\d{4}-\d{2}-\d{2}/)?.[0];
                        if (day) openSessions({ from: day, to: day });
                      },
                    }}
                  />
                  <div data-testid="token-components" className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                    {[
                      [t("input"), stats.input_tokens],
                      [t("output"), stats.output_tokens],
                      [t("cacheRead"), stats.cache_read],
                      [t("cacheCreate"), stats.cache_create],
                    ].map(([label, value]) => (
                      <div key={String(label)} className="min-w-0 rounded-md bg-muted/55 px-3 py-2">
                        <div className="truncate text-[10px] text-muted-foreground">{label}</div>
                        <div className="truncate font-mono text-sm font-semibold tabular-nums">{fmtTokens(Number(value))}</div>
                      </div>
                    ))}
                  </div>
                </div>
                <div className="min-w-0 pt-1 lg:pl-2">
                  <BandTitle title={t("modelUsage")} detail={t("unknownPriceFootnote")} />
                  <ModelUsageRows rows={data.models} onSelect={(model) => openSessions({ model })} t={t} />
                </div>
              </div>
            </section>

            <section data-testid="dashboard-band-detail" className="bg-card/25 px-4 py-4">
              <div className="grid min-w-0 grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.5fr)_minmax(14rem,0.8fr)_minmax(16rem,1fr)]">
                <div className="min-w-0">
                  <BandTitle title={t("agentComposition")} detail={t("tokens")} />
                  <BreakdownRows
                    rows={data.sources}
                    onSelect={(source) => openSessions({ source })}
                    t={t}
                    composition
                  />
                </div>
                <div className="min-w-0 pt-1 xl:pl-2">
                  <BandTitle title={t("projectRanking")} detail={t("tokens")} />
                  <BreakdownRows
                    rows={data.projects}
                    onSelect={(project) => openSessions({ project })}
                    t={t}
                    compact
                    projectLabels
                  />
                </div>
                <div
                  aria-busy={throughputLoading}
                  className="min-w-0 pt-1 xl:pl-2"
                >
                  <header className="mb-3 flex min-w-0 flex-wrap items-end justify-between gap-2">
                    <div className="min-w-0">
                      <h2 className="truncate text-sm font-semibold">{t("localObservedThroughput")}</h2>
                      <p className="truncate text-xs text-muted-foreground">{t("notProviderQuota")}</p>
                    </div>
                    <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                      <div className="inline-flex rounded-md border border-border bg-card p-0.5" aria-label={t("throughputScaleMode")}>
                        {(["trend", "absolute"] as const).map((mode) => (
                          <button
                            key={mode}
                            type="button"
                            aria-pressed={throughputMode === mode}
                            onClick={() => { setThroughputMode(mode); localStorage.setItem("au-throughput-mode", mode); }}
                            className={`px-2 py-1 text-[10px] font-medium ${throughputMode === mode ? "rounded bg-foreground text-background" : "text-muted-foreground hover:text-foreground"}`}
                          >{t(mode === "trend" ? "throughputTrendMode" : "throughputAbsoluteMode")}</button>
                        ))}
                      </div>
                      <label className="flex min-w-0 items-center gap-1.5 text-[10px] text-muted-foreground">
                        <span>{t("throughputModel")}</span>
                        <select
                          aria-label={t("throughputModel")}
                          value={throughputModel}
                          onChange={(event) => setThroughputModel(event.target.value)}
                          className="max-w-36 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground"
                        >
                          <option value="">{t("allModels")}</option>
                          {data.models.filter((row) => row.key).map((row) => (
                            <option key={row.key} value={row.key}>{row.key}</option>
                          ))}
                        </select>
                      </label>
                    </div>
                  </header>
                  <ThroughputMatrix throughput={throughput} t={t} />
                  {throughputError && (
                    <p className="mt-2 break-words text-xs text-red-500">{throughputError}</p>
                  )}
                  <ChartCard title={t("observedTPMTrend")} option={throughputOption} className="mt-3 h-48" />
                </div>
              </div>
            </section>
          </>
        ) : null}
      </main>
    </div>
  );
}
