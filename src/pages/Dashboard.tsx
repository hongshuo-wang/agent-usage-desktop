import { useState, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import { fmtCost, fmtTokens, getTimeRange, TimePreset, CHART_COLORS } from "../lib/utils";
import TimeRangeSelector from "../components/TimeRangeSelector";
import ChartCard from "../components/ChartCard";
import Sparkline from "../components/Sparkline";

interface DashboardStats {
  total_tokens: number;
  total_cost: number;
  total_sessions: number;
  total_prompts: number;
  total_calls: number;
  cache_hit_rate: number;
}

interface TokensRow {
  date: string;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
}

interface TokensOverTime {
  labels: string[];
  input: number[];
  output: number[];
  cache_read: number[];
  cache_creation: number[];
}

interface CostRow {
  date: string;
  value: number;
  model: string;
}

interface CostOverTime {
  labels: string[];
  series: { model: string; data: number[] }[];
}

interface CostByModel {
  model: string;
  cost: number;
}

function transformTokens(rows: TokensRow[]): TokensOverTime {
  return {
    labels: rows.map((r) => r.date),
    input: rows.map((r) => r.input_tokens),
    output: rows.map((r) => r.output_tokens),
    cache_read: rows.map((r) => r.cache_read),
    cache_creation: rows.map((r) => r.cache_create),
  };
}

function transformCost(rows: CostRow[]): CostOverTime {
  const labelSet = [...new Set(rows.map((r) => r.date))];
  const models = [...new Set(rows.map((r) => r.model))];
  const lookup = new Map(rows.map((r) => [`${r.date}|${r.model}`, r.value]));
  return {
    labels: labelSet,
    series: models.map((m) => ({
      model: m,
      data: labelSet.map((l) => lookup.get(`${l}|${m}`) || 0),
    })),
  };
}

/* ── Skeleton loader ── */
function Skeleton({ className }: { className?: string }) {
  return <div className={`animate-pulse rounded bg-muted ${className || ""}`} />;
}

function DashboardSkeleton() {
  return (
    <div className="grid grid-cols-[220px_1fr] gap-4 flex-1 min-h-0 min-w-0 overflow-hidden">
      <div className="space-y-4 min-h-0 overflow-hidden">
        <div className="pb-4 border-b border-border space-y-2">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-10 w-36" />
          <Skeleton className="h-3 w-24" />
        </div>
        {[1, 2, 3].map((i) => (
          <div key={i} className="py-3 space-y-2">
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-6 w-20" />
            <Skeleton className="h-1 w-full" />
          </div>
        ))}
      </div>
      <div className="flex flex-col gap-2 min-h-0 min-w-0 overflow-hidden">
        <Skeleton className="flex-[2] rounded-xl" />
        <div className="grid grid-cols-[3fr_2fr] gap-2 flex-[1] min-h-0 min-w-0">
          <Skeleton className="rounded-xl" />
          <Skeleton className="rounded-xl" />
        </div>
      </div>
    </div>
  );
}

/* ── Metric with sparkline ── */
function MetricRow({ label, value, color, valueColor, sparkData }: {
  label: string; value: string; color: string; valueColor?: string; sparkData?: number[];
}) {
  return (
    <div className="py-2 border-b border-border last:border-b-0">
      <div className="text-[11px] text-muted-foreground mb-0.5">{label}</div>
      <div className="text-lg font-bold font-mono" style={valueColor ? { color: valueColor } : undefined}>{value}</div>
      {sparkData && sparkData.length > 0 && (
        <div className="mt-1.5">
          <Sparkline data={sparkData} color={color} height={24} />
        </div>
      )}
    </div>
  );
}

/* ── Auxiliary stat cell ── */
function AuxCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-card border border-border rounded-lg p-2 text-center">
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className="text-sm font-semibold font-mono">{value}</div>
    </div>
  );
}

export default function Dashboard() {
  const { t } = useTranslation();
  const [preset, setPreset] = useState<TimePreset>(
    (localStorage.getItem("au-preset") as TimePreset) || "today"
  );
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [source, setSource] = useState(localStorage.getItem("au-source") || "");
  const [customFrom, setCustomFrom] = useState(localStorage.getItem("au-custom-from") || "");
  const [customTo, setCustomTo] = useState(localStorage.getItem("au-custom-to") || "");
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [tokensData, setTokensData] = useState<TokensOverTime | null>(null);
  const [costData, setCostData] = useState<CostOverTime | null>(null);
  const [pieData, setPieData] = useState<CostByModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    const range = getTimeRange(preset, customFrom, customTo);
    const params = { ...range, granularity, source: source || undefined };
    setLoading(true);
    setError(null);
    try {
      const [s, tokRaw, costRaw, pie] = await Promise.all([
        fetchAPI<DashboardStats>("stats", params),
        fetchAPI<TokensRow[]>("tokens-over-time", params),
        fetchAPI<CostRow[]>("cost-over-time", params),
        fetchAPI<CostByModel[]>("cost-by-model", params),
      ]);
      setStats(s);
      setTokensData(tokRaw?.length ? transformTokens(tokRaw) : null);
      setCostData(costRaw?.length ? transformCost(costRaw) : null);
      setPieData(pie || []);
    } catch (e) {
      console.error("Dashboard fetch error:", e);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [preset, granularity, source, customFrom, customTo]);

  useEffect(() => { fetchData(); }, [fetchData]);

  /* ── Shorten axis labels: "2026-04-15 10" → "04-15 10h" ── */
  const fmtAxisLabel = (v: string) => {
    const m = String(v).match(/\d{4}-(\d{2}-\d{2})(?:\s+(\d+))?/);
    if (m) return m[2] !== undefined ? `${m[1]} ${m[2]}h` : m[1];
    return v;
  };
  /* Auto-calculate label interval to avoid overlap */
  const calcInterval = (count: number, chartWidth = 800) => {
    const maxLabels = Math.floor(chartWidth / 70); // ~70px per rotated label
    return count <= maxLabels ? 0 : Math.ceil(count / maxLabels) - 1;
  };

  /* ── ECharts options ── */
  const tokensOption = tokensData?.labels ? {
    tooltip: { trigger: "axis" },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: tokensData.labels, axisLabel: { hideOverlap: true, rotate: 40, fontSize: 11, formatter: fmtAxisLabel, interval: calcInterval(tokensData.labels.length) } },
    yAxis: { type: "value" },
    series: [
      { name: t("input"), type: "bar", stack: "tokens", data: tokensData.input, color: CHART_COLORS[0] },
      { name: t("output"), type: "bar", stack: "tokens", data: tokensData.output, color: CHART_COLORS[1] },
      { name: t("cacheRead"), type: "bar", stack: "tokens", data: tokensData.cache_read, color: CHART_COLORS[3] },
      { name: t("cacheCreate"), type: "bar", stack: "tokens", data: tokensData.cache_creation, color: CHART_COLORS[2] },
    ],
  } : {};

  const costOption = costData?.series ? {
    tooltip: { trigger: "axis" },
    legend: { type: "scroll", top: 0, left: "center" },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: costData.labels, axisLabel: { hideOverlap: true, rotate: 40, fontSize: 11, formatter: fmtAxisLabel, interval: calcInterval(costData.labels.length, 500) } },
    yAxis: { type: "value" },
    series: costData.series.map((s, i) => ({
      name: s.model, type: "bar", stack: "cost", data: s.data,
      color: CHART_COLORS[i % CHART_COLORS.length],
    })),
  } : {};

  const pieOption = pieData.length ? {
    tooltip: { trigger: "item", formatter: "{b}: ${c} ({d}%)" },
    legend: { type: "scroll", bottom: 0, left: "center" },
    series: [{
      type: "pie", radius: ["35%", "65%"], center: ["50%", "45%"],
      data: pieData.map((d, i) => ({
        name: d.model, value: d.cost,
        itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
      })),
      label: { show: false },
      emphasis: { label: { show: true, formatter: "{b}\n{d}%", fontSize: 12 } },
    }],
  } : {};

  /* ── Derived values for left panel ── */
  const totalInput = stats ? (stats.total_tokens - (stats.total_tokens * 0.25)) : 0; // approximate
  const cacheRate = stats ? (stats.cache_hit_rate * 100) : 0;

  /* ── Sparkline data derived from existing time-series ── */
  const tokenSpark = useMemo(() => {
    if (!tokensData?.labels) return [];
    return tokensData.input.map((v, i) => v + tokensData.output[i] + tokensData.cache_read[i] + tokensData.cache_creation[i]);
  }, [tokensData]);

  const cacheSpark = useMemo(() => {
    if (!tokensData?.labels) return [];
    return tokensData.input.map((v, i) => {
      const total = v + tokensData.cache_read[i] + tokensData.cache_creation[i];
      return total > 0 ? (tokensData.cache_read[i] / total) * 100 : 0;
    });
  }, [tokensData]);

  return (
    <div className="flex flex-col flex-1 min-h-0 min-w-0 gap-3">
      <TimeRangeSelector
        preset={preset} onPresetChange={setPreset}
        granularity={granularity} onGranularityChange={setGranularity}
        source={source} onSourceChange={setSource}
        onRefresh={fetchData}
        customFrom={customFrom} customTo={customTo}
        onCustomFromChange={(v) => { setCustomFrom(v); localStorage.setItem("au-custom-from", v); }}
        onCustomToChange={(v) => { setCustomTo(v); localStorage.setItem("au-custom-to", v); }}
      />
      {loading && !stats ? (
        <DashboardSkeleton />
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-20 gap-3">
          <p className="text-red-500 text-sm">{error}</p>
          <button onClick={fetchData} className="px-4 py-2 bg-accent text-white rounded-lg text-sm hover:bg-accent-hover cursor-pointer transition-colors duration-200">
            {t("retry")}
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-[220px_1fr] gap-4 flex-1 min-h-0 min-w-0 overflow-hidden">
          {/* ── Left Panel ── */}
          <div className="flex flex-col min-h-0 overflow-hidden">
            {/* Cost Hero */}
            <div className="mb-2 pb-2 border-b border-border">
              <div className="text-[11px] text-muted-foreground uppercase tracking-wider">{t("todayCost")}</div>
              <div className="text-[36px] font-extrabold font-mono leading-none tracking-tight mt-0.5">
                {fmtCost(stats?.total_cost || 0)}
              </div>
            </div>

            {/* Secondary metrics */}
            <div>
              <MetricRow
                label={t("tokenConsumption")}
                value={fmtTokens(stats?.total_tokens || 0)}
                color="#f97316"
                valueColor="#f97316"
                sparkData={tokenSpark}
              />
              <MetricRow
                label={t("cacheHitRate")}
                value={cacheRate.toFixed(1) + "%"}
                color="#22c55e"
                valueColor="#22c55e"
                sparkData={cacheSpark}
              />
              <MetricRow
                label={t("apiCalls")}
                value={fmtTokens(stats?.total_calls || 0)}
                color="#6366f1"
                valueColor="#818cf8"
                sparkData={tokenSpark}
              />
            </div>

            {/* Auxiliary 2x2 grid */}
            <div className="grid grid-cols-2 gap-1.5 mt-auto pt-2">
              <AuxCell label={t("sessions")} value={String(stats?.total_sessions || 0)} />
              <AuxCell label={t("prompts")} value={String(stats?.total_prompts || 0)} />
              <AuxCell label={t("inputTokens")} value={fmtTokens(totalInput)} />
              <AuxCell label={t("outputTokens")} value={fmtTokens(stats?.total_tokens ? stats.total_tokens * 0.25 : 0)} />
            </div>
          </div>

          {/* ── Right Panel ── */}
          <div className="flex flex-col gap-2 min-w-0 min-h-0 overflow-hidden">
            <ChartCard title={t("tokenUsage")} option={tokensOption} className="flex-[2] min-h-0" />
            <div className="grid grid-cols-[3fr_2fr] gap-2 flex-[1] min-h-0 min-w-0">
              <ChartCard title={t("costTrend")} option={costOption} />
              <ChartCard title={t("costByModel")} option={pieOption} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
