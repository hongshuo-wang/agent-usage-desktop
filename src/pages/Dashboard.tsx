import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import { fmtCost, fmtTokens, getTimeRange, TimePreset, CHART_COLORS } from "../lib/utils";
import StatCard from "../components/StatCard";
import TimeRangeSelector from "../components/TimeRangeSelector";
import ChartCard from "../components/ChartCard";

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

interface CostByModel {
  model: string;
  cost: number;
}

export default function Dashboard() {
  const { t } = useTranslation();
  const [preset, setPreset] = useState<TimePreset>(
    (localStorage.getItem("au-preset") as TimePreset) || "today"
  );
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [source, setSource] = useState(localStorage.getItem("au-source") || "");
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [tokensData, setTokensData] = useState<TokensOverTime | null>(null);
  const [costData, setCostData] = useState<CostOverTime | null>(null);
  const [pieData, setPieData] = useState<CostByModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    const range = getTimeRange(preset);
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
  }, [preset, granularity, source]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const tokensOption = tokensData?.labels ? {
    tooltip: { trigger: "axis" },
    legend: { data: [t("input"), t("output"), t("cacheRead"), t("cacheCreate")] },
    xAxis: { type: "category", data: tokensData.labels },
    yAxis: { type: "value" },
    series: [
      { name: t("input"), type: "bar", stack: "tokens", data: tokensData.input, color: "#3b82f6" },
      { name: t("output"), type: "bar", stack: "tokens", data: tokensData.output, color: "#22c55e" },
      { name: t("cacheRead"), type: "bar", stack: "tokens", data: tokensData.cache_read, color: "#f59e0b" },
      { name: t("cacheCreate"), type: "bar", stack: "tokens", data: tokensData.cache_creation, color: "#8b5cf6" },
    ],
  } : {};

  const costOption = costData?.series ? {
    tooltip: { trigger: "axis" },
    legend: { data: costData.series.map((s) => s.model) },
    xAxis: { type: "category", data: costData.labels },
    yAxis: { type: "value" },
    series: costData.series.map((s, i) => ({
      name: s.model,
      type: "bar",
      stack: "cost",
      data: s.data,
      color: CHART_COLORS[i % CHART_COLORS.length],
    })),
  } : {};

  const pieOption = pieData.length ? {
    tooltip: { trigger: "item" },
    series: [{
      type: "pie",
      radius: ["40%", "70%"],
      data: pieData.map((d, i) => ({
        name: d.model,
        value: d.cost,
        itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
      })),
      label: { formatter: "{b}: {d}%" },
    }],
  } : {};

  return (
    <div className="space-y-4">
      <TimeRangeSelector
        preset={preset} onPresetChange={setPreset}
        granularity={granularity} onGranularityChange={setGranularity}
        source={source} onSourceChange={setSource}
        onRefresh={fetchData}
      />
      {loading && !stats ? (
        <div className="flex items-center justify-center py-20 text-muted-foreground text-sm">
          {t("loading")}...
        </div>
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-20 gap-3">
          <p className="text-red-500 text-sm">{error}</p>
          <button onClick={fetchData} className="px-4 py-2 bg-accent text-white rounded-lg text-sm hover:bg-accent-hover">
            {t("retry")}
          </button>
        </div>
      ) : (
      <>
      <div className="grid grid-cols-6 gap-4">
        <StatCard label={t("totalTokens")} value={fmtTokens(stats?.total_tokens || 0)} color="#3b82f6" />
        <StatCard label={t("totalCost")} value={fmtCost(stats?.total_cost || 0)} color="#22c55e" />
        <StatCard label={t("sessions")} value={String(stats?.total_sessions || 0)} color="#f59e0b" />
        <StatCard label={t("prompts")} value={String(stats?.total_prompts || 0)} color="#f472b6" />
        <StatCard label={t("apiCalls")} value={fmtTokens(stats?.total_calls || 0)} color="#2563eb" />
        <StatCard label={t("cacheHitRate")} value={((stats?.cache_hit_rate || 0) * 100).toFixed(1) + "%"} color="#8b5cf6" />
      </div>
      <div className="grid grid-cols-5 gap-4">
        <div className="col-span-5">
          <ChartCard title={t("tokenUsage")} option={tokensOption} />
        </div>
        <div className="col-span-3">
          <ChartCard title={t("costOverTime")} option={costOption} />
        </div>
        <div className="col-span-2">
          <ChartCard title={t("costByModel")} option={pieOption} />
        </div>
      </div>
      </>
      )}
    </div>
  );
}
