import { useState, useEffect, useCallback, Fragment } from "react";
import { useTranslation } from "react-i18next";
import { fetchAPI, getWebUIUrl } from "../lib/api";
import { fmtCost, fmtTokens, getTimeRange, relativeTime, TimePreset } from "../lib/utils";
import TimeRangeSelector from "../components/TimeRangeSelector";

interface Session {
  session_id: string;
  source: string;
  project: string;
  cwd: string;
  git_branch: string;
  start_time: string;
  prompts: number;
  tokens: number;
  total_cost: number;
}

interface SessionDetail {
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
  cost_usd: number;
}

const PAGE_SIZE = 20;
const BADGE_COLORS: Record<string, string> = {
  claude: "bg-blue-500/10 text-blue-500 border-blue-500/20",
  codex: "bg-green-500/10 text-green-500 border-green-500/20",
  openclaw: "bg-orange-500/10 text-orange-500 border-orange-500/20",
  opencode: "bg-purple-500/10 text-purple-500 border-purple-500/20",
};

function DetailTable({ details, t }: { details: SessionDetail[]; t: (key: string) => string }) {
  return (
    <table className="w-full text-xs">
      <thead>
        <tr>
          <th className="text-left py-2 text-muted-foreground">{t("model")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("calls")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("input")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("output")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("cacheRead")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("cacheCreate")}</th>
          <th className="text-left py-2 text-muted-foreground">{t("cost")}</th>
        </tr>
      </thead>
      <tbody>
        {details.map((d, i) => (
          <tr key={i}>
            <td className="py-1.5">{d.model}</td>
            <td className="py-1.5">{d.calls}</td>
            <td className="py-1.5 font-mono">{fmtTokens(d.input_tokens)}</td>
            <td className="py-1.5 font-mono">{fmtTokens(d.output_tokens)}</td>
            <td className="py-1.5 font-mono">{fmtTokens(d.cache_read)}</td>
            <td className="py-1.5 font-mono">{fmtTokens(d.cache_create)}</td>
            <td className="py-1.5 font-mono text-green-500">{fmtCost(d.cost_usd)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export default function Sessions() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [preset, setPreset] = useState<TimePreset>((localStorage.getItem("au-preset") as TimePreset) || "today");
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [source, setSource] = useState(localStorage.getItem("au-source") || "");
  const [sortKey, setSortKey] = useState("start_time");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState("");
  const [expanded, setExpanded] = useState<Record<string, SessionDetail[] | null>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    const range = getTimeRange(preset);
    setLoading(true);
    setError(null);
    try {
      const data = await fetchAPI<Session[]>("sessions", { ...range, granularity, source: source || undefined });
      setSessions(data || []);
    } catch (e) {
      console.error("Sessions fetch error:", e);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [preset, granularity, source]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const filtered = sessions.filter((s) =>
    !filter || (s.project || s.cwd || "").toLowerCase().includes(filter.toLowerCase())
  );
  const sorted = [...filtered].sort((a, b) => {
    const va = (a as any)[sortKey] ?? "";
    const vb = (b as any)[sortKey] ?? "";
    const cmp = typeof va === "number" ? va - vb : String(va).localeCompare(String(vb));
    return sortDir === "asc" ? cmp : -cmp;
  });
  const totalPages = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
  const paged = sorted.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  const toggleSort = (key: string) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("desc"); }
  };

  const toggleExpand = async (sid: string) => {
    if (expanded[sid] !== undefined) {
      setExpanded((prev) => { const next = { ...prev }; delete next[sid]; return next; });
    } else {
      setExpanded((prev) => ({ ...prev, [sid]: null }));
      try {
        const url = await getWebUIUrl();
        const res = await fetch(`${url}/api/session-detail?session_id=${encodeURIComponent(sid)}`);
        const data = await res.json();
        setExpanded((prev) => ({ ...prev, [sid]: data }));
      } catch {
        setExpanded((prev) => { const next = { ...prev }; delete next[sid]; return next; });
      }
    }
  };

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-3">
      <TimeRangeSelector preset={preset} onPresetChange={setPreset}
        granularity={granularity} onGranularityChange={setGranularity}
        source={source} onSourceChange={setSource} onRefresh={fetchData} />

      <div className="bg-card border border-border rounded-xl shadow-sm flex-1 min-h-0 flex flex-col overflow-hidden">
        <div className="p-5 border-b border-border flex items-center justify-between">
          <h3 className="text-base font-semibold">{t("sessionLog")}</h3>
          <input type="text" value={filter} onChange={(e) => { setFilter(e.target.value); setPage(1); }}
            placeholder={t("filterProject")}
            className="bg-background border border-border rounded-lg px-3 py-2 text-sm w-56" />
        </div>
        <div className="overflow-auto flex-1 min-h-0">
          {loading && sessions.length === 0 ? (
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
          <table className="w-full text-sm">
            <thead>
              <tr>
                {[
                  { key: "source", label: "source" },
                  { key: "project", label: "project" },
                  { key: "git_branch", label: "branch" },
                  { key: "start_time", label: "time" },
                  { key: "prompts", label: "prompts" },
                  { key: "tokens", label: "tokens" },
                  { key: "total_cost", label: "cost" },
                ].map((col) => (
                  <th key={col.key} onClick={() => toggleSort(col.key)}
                    className="text-left px-6 py-3 text-muted-foreground font-medium cursor-pointer hover:text-foreground">
                    {t(col.label)} {sortKey === col.key ? (sortDir === "asc" ? "▲" : "▼") : ""}
                  </th>
                ))}
                <th className="w-10" />
              </tr>
            </thead>
            <tbody>
              {paged.map((s) => (
                <Fragment key={s.session_id}>
                  <tr className="hover:bg-muted/30 transition-colors">
                    <td className="px-6 py-3">
                      <span className={`inline-flex px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase border ${BADGE_COLORS[s.source] || ""}`}>
                        {s.source}
                      </span>
                    </td>
                    <td className="px-6 py-3 max-w-[280px] truncate">{s.project || s.cwd || "-"}</td>
                    <td className="px-6 py-3">{s.git_branch || "-"}</td>
                    <td className="px-6 py-3">{relativeTime(s.start_time, t)}</td>
                    <td className="px-6 py-3">{s.prompts}</td>
                    <td className="px-6 py-3">{fmtTokens(s.tokens || 0)}</td>
                    <td className="px-6 py-3 font-medium text-green-500">{fmtCost(s.total_cost || 0)}</td>
                    <td className="px-6 py-3">
                      <button onClick={() => toggleExpand(s.session_id)}
                        className="w-7 h-7 rounded-md border border-border flex items-center justify-center hover:border-accent">
                        <span className={`transition-transform ${expanded[s.session_id] !== undefined ? "rotate-90" : ""}`}>▶</span>
                      </button>
                    </td>
                  </tr>
                  {expanded[s.session_id] !== undefined && (
                    <tr>
                      <td colSpan={8} className="px-6 py-3 bg-muted/20">
                        {expanded[s.session_id] === null ? (
                          <span className="text-muted-foreground text-xs">Loading...</span>
                        ) : (
                          <DetailTable details={expanded[s.session_id] || []} t={t} />
                        )}
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
          )}
        </div>
        <div className="px-6 py-4 flex items-center justify-end gap-2">
          <span className="text-muted-foreground text-sm mr-auto">
            {Math.min((page - 1) * PAGE_SIZE + 1, sorted.length)}-{Math.min(page * PAGE_SIZE, sorted.length)} of {sorted.length}
          </span>
          <button disabled={page <= 1} onClick={() => setPage(page - 1)}
            className="px-3 py-1 border border-border rounded-lg text-sm disabled:opacity-50">←</button>
          <button disabled={page >= totalPages} onClick={() => setPage(page + 1)}
            className="px-3 py-1 border border-border rounded-lg text-sm disabled:opacity-50">→</button>
        </div>
      </div>
    </div>
  );
}
