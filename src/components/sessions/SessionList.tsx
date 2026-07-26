import { Search } from "lucide-react";
import type { SessionSummary } from "../../lib/types";
import { fmtCost, fmtTokens, relativeTime } from "../../lib/utils";
import { humanizeSessionTitle } from "../../lib/sessionPresentation";
import { presentProjectKey } from "../../lib/queryPresentation";

type Translate = (key: string) => string;

interface Props {
  sessions: SessionSummary[];
  selectedKey: string | null;
  search: string;
  onSearchChange: (value: string) => void;
  onSelect: (session: SessionSummary) => void;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
  t: Translate;
}

export const sessionIdentity = (session: Pick<SessionSummary, "source" | "session_id">) =>
  `${session.source}\u0000${session.session_id}`;

export function sessionStatusLabels(session: SessionSummary): string[] {
  const labels: string[] = [];
  if (session.source_status === "stats_only" || session.coverage_status === "stats_only") {
    labels.push("sessionStatsOnly");
  } else if (session.coverage_status === "partial") {
    labels.push("sessionPartial");
  } else if (session.coverage_status === "complete") {
    labels.push("sessionComplete");
  }
  if (session.source_status === "missing_source") labels.push("sessionMissingSource");
  if (session.source_status === "rebuild_required") labels.push("sessionRebuildRequired");
  if (session.source_status === "stale_parser") labels.push("sessionStaleParser");
  if (session.malformed_lines > 0) labels.push("sessionMalformed");
  if (session.unknown_price) labels.push("sessionUnknownPrice");
  return labels;
}

export function formatSessionDuration(startTime: string, lastActivity: string, fallback: string): string {
  const start = Date.parse(startTime);
  const end = Date.parse(lastActivity);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return fallback;
  const minutes = Math.floor((end - start) / 60_000);
  const days = Math.floor(minutes / (24 * 60));
  const hours = Math.floor((minutes % (24 * 60)) / 60);
  const remainingMinutes = minutes % 60;
  return [days ? `${days}d` : "", hours ? `${hours}h` : "", `${remainingMinutes}m`].filter(Boolean).join(" ");
}

export default function SessionList({
  sessions, selectedKey, search, onSearchChange, onSelect, loading, error,
  onRetry, hasMore, loadingMore, onLoadMore, t,
}: Props) {
  return (
    <aside data-testid="session-list" className="flex min-h-0 min-w-0 flex-col border-r border-border bg-card/20">
      <header className="p-3 pb-2">
        <h1 className="mb-2 text-sm font-semibold">{t("sessionRetrospective")}</h1>
        <label className="flex h-9 min-w-0 items-center gap-2 rounded border border-border bg-background px-2.5 focus-within:border-accent">
          <Search aria-hidden="true" className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            type="search"
            aria-label={t("searchSessions")}
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder={t("searchSessionContent")}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none"
          />
        </label>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading && sessions.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-muted-foreground">{t("loadingSessions")}</p>
        ) : error ? (
          <div className="px-4 py-10 text-center">
            <p className="break-words text-sm text-red-500">{error}</p>
            <button type="button" onClick={onRetry} className="mt-3 rounded bg-accent px-3 py-1.5 text-sm font-medium text-white">
              {t("retry")}
            </button>
          </div>
        ) : sessions.length === 0 ? (
          <div className="px-4 py-10 text-center">
            <p className="text-sm font-medium">{t("noSessionsFound")}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t("adjustSessionFilters")}</p>
          </div>
        ) : (
          <ol className="space-y-1 px-2 pb-2">
            {sessions.map((session) => {
              const key = sessionIdentity(session);
              const selected = selectedKey === key;
              const rawProject = session.project || session.cwd || session.session_id;
              const projectPresentation = presentProjectKey(rawProject);
              const sessionTitle = humanizeSessionTitle(session.title, session.project, session.cwd, session.session_id);
              const displayTitle = presentProjectKey(sessionTitle).label === "unnamedProject" ? t("unnamedProject") : sessionTitle;
              return (
                <li key={key} data-testid="session-list-item">
                  <button
                    type="button"
                    onClick={() => onSelect(session)}
                    aria-current={selected ? "true" : undefined}
                    className={`w-full min-w-0 rounded-md px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${selected ? "bg-accent-dim" : "hover:bg-muted/70"}`}
                  >
                    <div className="flex min-w-0 items-start justify-between gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm font-medium" title={projectPresentation.detail || displayTitle}>{displayTitle}</span>
                      <time className="shrink-0 text-[10px] text-muted-foreground" dateTime={session.last_activity}>
                        {relativeTime(session.last_activity, t)}
                      </time>
                    </div>
                    <div className="mt-1 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                      <span className="shrink-0 uppercase">{session.source}</span>
                      <span className="truncate" title={projectPresentation.detail || rawProject}>{projectPresentation.label === "unnamedProject" ? t("unnamedProject") : projectPresentation.label}</span>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[10px] text-muted-foreground">
                      <span>{fmtTokens(session.total_tokens)} {t("tokens")}</span>
                      <span>{fmtCost(session.total_cost)}</span>
                      <span>{session.prompts} {t("prompts")}</span>
                    </div>
                    <div className="mt-1 text-[10px] text-muted-foreground">
                      {t("duration")}: {formatSessionDuration(session.start_time, session.last_activity, t("sourceDataUnavailable"))}
                    </div>
                    {sessionStatusLabels(session).length > 0 && (
                      <div className="mt-2 flex flex-wrap gap-1">
                        {sessionStatusLabels(session).map((label) => (
                          <span key={label} className="border-l-2 border-accent px-1.5 text-[10px] text-muted-foreground">{t(label)}</span>
                        ))}
                      </div>
                    )}
                  </button>
                </li>
              );
            })}
          </ol>
        )}
      </div>

      {hasMore && !error && (
        <button type="button" onClick={onLoadMore} disabled={loadingMore} className="mx-2 mb-2 rounded-md bg-muted/50 px-3 py-2 text-xs font-medium hover:bg-muted disabled:opacity-50">
          {loadingMore ? t("loading") : t("loadMoreSessions")}
        </button>
      )}
    </aside>
  );
}
