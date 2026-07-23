import { ArrowLeft, LoaderCircle } from "lucide-react";
import type { SessionEvent, SessionSummary } from "../../lib/types";
import { fmtCost, fmtTokens } from "../../lib/utils";
import EventCard from "./EventCard";

type Translate = (key: string) => string;

interface Props {
  session: SessionSummary | null;
  events: SessionEvent[];
  loading: boolean;
  loadingMore: boolean;
  error: string | null;
  hasMore: boolean;
  isMobile: boolean;
  onBack: () => void;
  onRetry: () => void;
  onLoadMore: () => void;
  onInspect: (event: SessionEvent) => void;
  t: Translate;
}

export default function SessionTimeline({
  session, events, loading, loadingMore, error, hasMore, isMobile,
  onBack, onRetry, onLoadMore, onInspect, t,
}: Props) {
  if (!session) {
    return <section data-testid="session-timeline" className="flex min-h-0 items-center justify-center text-sm text-muted-foreground">{t("selectSession")}</section>;
  }

  return (
    <section data-testid="session-timeline" className="flex min-h-0 min-w-0 flex-col bg-background">
      <header className="border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          {isMobile && (
            <button type="button" aria-label={t("backToSessions")} onClick={onBack} className="flex h-8 w-8 shrink-0 items-center justify-center rounded hover:bg-muted">
              <ArrowLeft className="h-4 w-4" />
            </button>
          )}
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">{session.title}</h2>
            <p className="truncate text-xs text-muted-foreground">{session.source} / {session.project || session.cwd || session.session_id}</p>
          </div>
        </div>
        <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-4">
          <div><dt className="text-[10px] text-muted-foreground">{t("tokens")}</dt><dd className="font-mono">{fmtTokens(session.total_tokens)}</dd></div>
          <div><dt className="text-[10px] text-muted-foreground">{t("cost")}</dt><dd className="font-mono">{fmtCost(session.total_cost)}</dd></div>
          <div><dt className="text-[10px] text-muted-foreground">{t("toolCalls")}</dt><dd className="font-mono">{session.tool_calls}</dd></div>
          <div><dt className="text-[10px] text-muted-foreground">{t("errors")}</dt><dd className="font-mono">{session.errors}</dd></div>
        </dl>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {loading && events.length === 0 ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" /> {t("loadingEvents")}
          </div>
        ) : error ? (
          <div className="py-12 text-center">
            <p className="break-words text-sm text-red-500">{error}</p>
            <button type="button" onClick={onRetry} className="mt-3 rounded bg-accent px-3 py-1.5 text-sm font-medium text-white">{t("retry")}</button>
          </div>
        ) : events.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm font-medium">{t("noEventsIndexed")}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t("noEventsIndexedDetail")}</p>
          </div>
        ) : (
          <div className="mx-auto flex max-w-3xl flex-col gap-2">
            {events.map((item) => <EventCard key={item.id} event={item} onInspect={onInspect} t={t} />)}
            {hasMore && (
              <button type="button" aria-label={t("loadMoreEvents")} onClick={onLoadMore} disabled={loadingMore} className="mt-2 rounded border border-border px-3 py-2 text-xs font-medium hover:bg-muted disabled:opacity-50">
                {loadingMore ? t("loading") : t("loadMoreEvents")}
              </button>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
