import { ArrowLeft, LoaderCircle } from "lucide-react";
import { useState } from "react";
import type { SessionEvent, SessionSummary } from "../../lib/types";
import { fmtCost, fmtTokens } from "../../lib/utils";
import EventCard from "./EventCard";
import { sessionStatusLabels } from "./SessionList";
import { humanizeSessionTitle, isReadableSessionEvent } from "../../lib/sessionPresentation";

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

function HeaderField({ label, value, fallback, mono = false }: { label: string; value: string; fallback: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-[10px] text-muted-foreground">{label}</dt>
      <dd className={`truncate text-xs ${mono ? "font-mono" : ""}`}>{value || fallback}</dd>
    </div>
  );
}

export default function SessionTimeline({
  session, events, loading, loadingMore, error, hasMore, isMobile,
  onBack, onRetry, onLoadMore, onInspect, t,
}: Props) {
  const [showTechnical, setShowTechnical] = useState(false);
  if (!session) {
    return <section data-testid="session-timeline" className="flex min-h-0 items-center justify-center text-sm text-muted-foreground">{t("selectSession")}</section>;
  }
  const visibleEvents = events.filter((item) => showTechnical || isReadableSessionEvent(item));

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
            <h2 className="truncate text-sm font-semibold">{humanizeSessionTitle(session.title, session.project, session.cwd, session.session_id) || t("sourceDataUnavailable")}</h2>
          </div>
        </div>
        <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-5">
          <HeaderField label={t("agent")} value={session.source} fallback={t("sourceDataUnavailable")} />
          <HeaderField label={t("project")} value={session.project} fallback={t("sourceDataUnavailable")} />
          <HeaderField label={t("branch")} value={session.git_branch} fallback={t("sourceDataUnavailable")} />
          <HeaderField label={t("startTime")} value={session.start_time} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("models")} value={session.models.join(", ")} fallback={t("sourceDataUnavailable")} />
        </dl>
        <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1 text-[10px] text-muted-foreground">
          <span>{t("coverageStatus")}:</span>
          {sessionStatusLabels(session).map((label) => (
            <span key={label} className="border-l-2 border-accent px-1.5">{t(label)}</span>
          ))}
        </div>
        <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-4">
          <HeaderField label={t("totalTokens")} value={fmtTokens(session.total_tokens)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("inputTokens")} value={fmtTokens(session.input_tokens)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("outputTokens")} value={fmtTokens(session.output_tokens)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("cacheRead")} value={fmtTokens(session.cache_read)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("cacheCreate")} value={fmtTokens(session.cache_create)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("toolCalls")} value={String(session.tool_calls)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("estimatedCost")} value={fmtCost(session.total_cost)} fallback={t("sourceDataUnavailable")} mono />
          <HeaderField label={t("errors")} value={String(session.errors)} fallback={t("sourceDataUnavailable")} mono />
        </dl>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="mx-auto mb-3 flex max-w-3xl items-center justify-between gap-3 text-[10px] text-muted-foreground">
          <span>{showTechnical ? t("allEventsVisible") : t("readableConversation")}</span>
          <button
            type="button"
            aria-pressed={showTechnical}
            onClick={() => setShowTechnical((current) => !current)}
            className="rounded-md border border-border bg-card px-2.5 py-1.5 font-medium hover:bg-muted"
          >{showTechnical ? t("readableMode") : t("allEventsMode")}</button>
        </div>
        {loading && events.length === 0 ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" /> {t("loadingEvents")}
          </div>
        ) : error ? (
          <div className="py-12 text-center">
            <p className="break-words text-sm text-red-500">{error}</p>
            <button type="button" onClick={onRetry} className="mt-3 rounded bg-accent px-3 py-1.5 text-sm font-medium text-white">{t("retry")}</button>
          </div>
        ) : visibleEvents.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm font-medium">{t("noEventsIndexed")}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t("noEventsIndexedDetail")}</p>
          </div>
        ) : (
          <div className="mx-auto flex max-w-3xl flex-col gap-2">
            {visibleEvents.map((item) => <EventCard key={item.id} event={item} onInspect={onInspect} t={t} />)}
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
