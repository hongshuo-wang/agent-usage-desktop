import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import EventInspector from "../components/sessions/EventInspector";
import PageHeader from "../components/PageHeader";
import SessionList, { sessionIdentity } from "../components/sessions/SessionList";
import SessionTimeline from "../components/sessions/SessionTimeline";
import TimeRangeSelector from "../components/TimeRangeSelector";
import { fetchAPI } from "../lib/api";
import type { SessionEvent, SessionSummary, UsageFilters } from "../lib/types";
import { buildSessionsSearch, DEFAULT_USAGE_FILTERS, getInitialUsageFilters, persistUsageFilters } from "../lib/usageFilters";
import { getTimeRange } from "../lib/utils";

const SESSION_PAGE_SIZE = 50;
const EVENT_PAGE_SIZE = 100;
const MOBILE_QUERY = "(max-width: 899px)";

const isAbortError = (error: unknown) =>
  typeof error === "object" && error !== null && "name" in error && error.name === "AbortError";


function useMobileLayout(): boolean {
  const [mobile, setMobile] = useState(() => window.matchMedia(MOBILE_QUERY).matches);
  useEffect(() => {
    const media = window.matchMedia(MOBILE_QUERY);
    const update = () => setMobile(media.matches);
    media.addEventListener("change", update);
    update();
    return () => media.removeEventListener("change", update);
  }, []);
  return mobile;
}

function sortSessions(rows: SessionSummary[]): SessionSummary[] {
  return [...rows].sort((left, right) => right.last_activity.localeCompare(left.last_activity));
}

function sortEvents(rows: SessionEvent[]): SessionEvent[] {
  return [...rows].sort((left, right) => left.timestamp.localeCompare(right.timestamp) || left.id - right.id);
}

export default function Sessions() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const isMobile = useMobileLayout();
  const [filters, setFilters] = useState<UsageFilters>(() => getInitialUsageFilters(location.search));
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [selected, setSelected] = useState<SessionSummary | null>(null);
  const [listLoading, setListLoading] = useState(true);
  const [listLoadingMore, setListLoadingMore] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [listHasMore, setListHasMore] = useState(false);
  const [listRetry, setListRetry] = useState(0);
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [eventsLoadingMore, setEventsLoadingMore] = useState(false);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [eventsHasMore, setEventsHasMore] = useState(false);
  const [eventsRetry, setEventsRetry] = useState(0);
  const [inspectedEvent, setInspectedEvent] = useState<SessionEvent | null>(null);
  const [mobileDetailVisible, setMobileDetailVisible] = useState(false);
  const listController = useRef<AbortController | null>(null);
  const eventController = useRef<AbortController | null>(null);
  const selectedLifecycleKey = useRef<string | null>(null);

  useEffect(() => persistUsageFilters(filters), [filters]);

  useLayoutEffect(() => {
    const nextKey = selected ? sessionIdentity(selected) : null;
    if (nextKey === selectedLifecycleKey.current) return;
    selectedLifecycleKey.current = nextKey;
    setInspectedEvent(null);
  }, [selected?.source, selected?.session_id]);

  const sessionParams = useCallback((offset: number) => ({
    from: filters.from,
    to: filters.to,
    source: filters.source || undefined,
    model: filters.model || undefined,
    project: filters.project || undefined,
    limit: SESSION_PAGE_SIZE,
    offset,
  }), [filters]);

  useEffect(() => {
    const controller = new AbortController();
    listController.current?.abort();
    listController.current = controller;
    setListLoading(true);
    setListLoadingMore(false);
    setListError(null);
    setListHasMore(false);

    const run = async () => {
      try {
        const rows = await fetchAPI<SessionSummary[]>("sessions", sessionParams(0), { signal: controller.signal });
        if (controller.signal.aborted || listController.current !== controller) return;
        const ordered = sortSessions(rows || []);
        setSessions(ordered);
        setListHasMore(ordered.length === SESSION_PAGE_SIZE);
        setSelected((current) => {
          if (current) {
            const retained = ordered.find((row) => sessionIdentity(row) === sessionIdentity(current));
            if (retained) return retained;
          }
          return ordered[0] || null;
        });
      } catch (error) {
        if (!controller.signal.aborted && listController.current === controller && !isAbortError(error)) {
          setListError(error instanceof Error ? error.message : String(error));
          setSessions([]);
          setSelected(null);
        }
      } finally {
        if (!controller.signal.aborted && listController.current === controller) setListLoading(false);
      }
    };

    void run();
    return () => controller.abort();
  }, [sessionParams, listRetry]);

  const loadMoreSessions = useCallback(async () => {
    const controller = new AbortController();
    listController.current?.abort();
    listController.current = controller;
    setListLoadingMore(true);
    setListError(null);
    try {
      const rows = await fetchAPI<SessionSummary[]>("sessions", sessionParams(sessions.length), { signal: controller.signal });
      if (controller.signal.aborted || listController.current !== controller) return;
      setSessions((current) => sortSessions([...current, ...(rows || [])]));
      setListHasMore((rows || []).length === SESSION_PAGE_SIZE);
    } catch (error) {
      if (!controller.signal.aborted && listController.current === controller && !isAbortError(error)) {
        setListError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (!controller.signal.aborted && listController.current === controller) setListLoadingMore(false);
    }
  }, [sessionParams, sessions.length]);

  useEffect(() => {
    eventController.current?.abort();
    setEvents([]);
    setEventsError(null);
    setEventsHasMore(false);
    setEventsLoadingMore(false);
    if (!selected) {
      setEventsLoading(false);
      return;
    }

    const controller = new AbortController();
    eventController.current = controller;
    setEventsLoading(true);
    const run = async () => {
      try {
        const path = `sessions/${encodeURIComponent(selected.source)}/${encodeURIComponent(selected.session_id)}/events`;
        const rows = await fetchAPI<SessionEvent[]>(path, { limit: EVENT_PAGE_SIZE, offset: 0 }, { signal: controller.signal });
        if (controller.signal.aborted || eventController.current !== controller) return;
        setEvents(sortEvents(rows || []));
        setEventsHasMore((rows || []).length === EVENT_PAGE_SIZE);
      } catch (error) {
        if (!controller.signal.aborted && eventController.current === controller && !isAbortError(error)) {
          setEventsError(error instanceof Error ? error.message : String(error));
        }
      } finally {
        if (!controller.signal.aborted && eventController.current === controller) setEventsLoading(false);
      }
    };
    void run();
    return () => controller.abort();
  }, [selected?.source, selected?.session_id, eventsRetry]);

  const selectSession = useCallback((session: SessionSummary) => {
    if (!selected || sessionIdentity(selected) !== sessionIdentity(session)) {
      setInspectedEvent(null);
      setSelected(session);
    }
    if (isMobile) setMobileDetailVisible(true);
  }, [isMobile, selected]);

  const loadMoreEvents = useCallback(async () => {
    if (!selected) return;
    const controller = new AbortController();
    eventController.current?.abort();
    eventController.current = controller;
    setEventsLoadingMore(true);
    setEventsError(null);
    try {
      const path = `sessions/${encodeURIComponent(selected.source)}/${encodeURIComponent(selected.session_id)}/events`;
      const rows = await fetchAPI<SessionEvent[]>(path, { limit: EVENT_PAGE_SIZE, offset: events.length }, { signal: controller.signal });
      if (controller.signal.aborted || eventController.current !== controller) return;
      setEvents((current) => sortEvents([...current, ...(rows || [])]));
      setEventsHasMore((rows || []).length === EVENT_PAGE_SIZE);
    } catch (error) {
      if (!controller.signal.aborted && eventController.current === controller && !isAbortError(error)) {
        setEventsError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (!controller.signal.aborted && eventController.current === controller) setEventsLoadingMore(false);
    }
  }, [events.length, selected]);

  const inspectEvent = useCallback((event: SessionEvent) => {
    setInspectedEvent(event);
  }, []);

  const closeInspector = useCallback(() => {
    setInspectedEvent(null);
  }, []);

  const clearAllFilters = () => {
    const range = getTimeRange(DEFAULT_USAGE_FILTERS.preset);
    setFilters({ ...DEFAULT_USAGE_FILTERS, ...range });
    navigate({ pathname: "/sessions", search: "" }, { replace: true });
  };

  const applyQueryFilters = (next: UsageFilters) => {
    setFilters(next);
    navigate({ pathname: "/sessions", search: buildSessionsSearch(next) }, { replace: true });
  };

  const selectedKey = selected ? sessionIdentity(selected) : null;

  const list = (
    <SessionList
      sessions={sessions}
      selectedKey={selectedKey}
      onSelect={selectSession}
      loading={listLoading}
      error={listError}
      onRetry={() => setListRetry((value) => value + 1)}
      hasMore={listHasMore}
      loadingMore={listLoadingMore}
      onLoadMore={() => { void loadMoreSessions(); }}
      t={t}
    />
  );

  const timeline = (
    <SessionTimeline
      session={selected}
      events={events}
      loading={eventsLoading}
      loadingMore={eventsLoadingMore}
      error={eventsError}
      hasMore={eventsHasMore}
      isMobile={isMobile}
      onBack={() => { setMobileDetailVisible(false); closeInspector(); }}
      onRetry={() => setEventsRetry((value) => value + 1)}
      onLoadMore={() => { void loadMoreEvents(); }}
      onInspect={inspectEvent}
      t={t}
    />
  );

  const inspector = inspectedEvent ? (
    <EventInspector
      event={inspectedEvent}
      onClose={closeInspector}
      t={t}
    />
  ) : null;

  return (
    <div className="mx-auto flex min-h-0 w-full min-w-0 max-w-[1400px] flex-1 flex-col gap-3">
      <PageHeader title={t("sessionLog")} hint={t("sessionRetrospective")} />
      <TimeRangeSelector
        preset={filters.preset}
        source={filters.source}
        onRefresh={() => setListRetry((value) => value + 1)}
        filters={filters}
        onFiltersApply={applyQueryFilters}
        onClearFilters={clearAllFilters}
      />

      <main
        data-testid="session-center-grid"
        data-inspector-open={String(Boolean(inspectedEvent))}
        className="session-center-grid min-h-0 min-w-0 flex-1 overflow-hidden"
      >
        {isMobile ? (
          mobileDetailVisible ? (inspector || timeline) : list
        ) : (
          <>{list}{timeline}{inspector}</>
        )}
      </main>
    </div>
  );
}
