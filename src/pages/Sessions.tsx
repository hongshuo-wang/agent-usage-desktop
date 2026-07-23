import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import EventInspector from "../components/sessions/EventInspector";
import SessionList, { sessionIdentity } from "../components/sessions/SessionList";
import SessionTimeline from "../components/sessions/SessionTimeline";
import TimeRangeSelector from "../components/TimeRangeSelector";
import { fetchAPI, fetchRaw } from "../lib/api";
import type { RawEventResponse, SessionEvent, SessionSummary, UsageFilters } from "../lib/types";
import { DEFAULT_USAGE_FILTERS, getInitialUsageFilters, persistUsageFilters } from "../lib/usageFilters";
import { getTimeRange, type TimePreset } from "../lib/utils";

const SESSION_PAGE_SIZE = 50;
const EVENT_PAGE_SIZE = 100;
const MOBILE_QUERY = "(max-width: 899px)";

type DrilldownContext = Pick<UsageFilters, "from" | "to" | "source" | "model" | "project">;

const isAbortError = (error: unknown) =>
  typeof error === "object" && error !== null && "name" in error && error.name === "AbortError";

function initialDrilldown(search: string): DrilldownContext {
  const query = new URLSearchParams(search);
  return {
    from: query.get("from") || "",
    to: query.get("to") || "",
    source: query.get("source") || "",
    model: query.get("model") || "",
    project: query.get("project") || "",
  };
}

function hasDrilldown(context: DrilldownContext): boolean {
  return Object.values(context).some(Boolean);
}

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
  const [drilldown, setDrilldown] = useState<DrilldownContext>(() => initialDrilldown(location.search));
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [search, setSearch] = useState("");
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
  const [rawCache, setRawCache] = useState<Record<number, RawEventResponse>>({});
  const [rawLoadingID, setRawLoadingID] = useState<number | null>(null);
  const [rawError, setRawError] = useState<string | null>(null);
  const [mobileDetailVisible, setMobileDetailVisible] = useState(false);
  const listController = useRef<AbortController | null>(null);
  const eventController = useRef<AbortController | null>(null);
  const rawController = useRef<AbortController | null>(null);
  const selectedLifecycleKey = useRef<string | null>(null);

  useEffect(() => persistUsageFilters(filters), [filters]);

  useLayoutEffect(() => {
    const nextKey = selected ? sessionIdentity(selected) : null;
    if (nextKey === selectedLifecycleKey.current) return;
    selectedLifecycleKey.current = nextKey;
    rawController.current?.abort();
    setRawCache({});
    setRawError(null);
    setRawLoadingID(null);
    setInspectedEvent(null);
  }, [selected?.source, selected?.session_id]);

  const sessionParams = useCallback((offset: number) => ({
    from: filters.from,
    to: filters.to,
    source: filters.source || undefined,
    model: filters.model || undefined,
    project: filters.project || undefined,
    q: search.trim() || undefined,
    limit: SESSION_PAGE_SIZE,
    offset,
  }), [filters, search]);

  useEffect(() => {
    const controller = new AbortController();
    listController.current?.abort();
    listController.current = controller;
    setListLoading(true);
    setListError(null);

    const run = async () => {
      try {
        const rows = await fetchAPI<SessionSummary[]>("sessions", sessionParams(0), { signal: controller.signal });
        if (controller.signal.aborted) return;
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
        if (!isAbortError(error)) {
          setListError(error instanceof Error ? error.message : String(error));
          setSessions([]);
          setSelected(null);
        }
      } finally {
        if (!controller.signal.aborted) setListLoading(false);
      }
    };

    const timer = search.trim() ? window.setTimeout(() => { void run(); }, 250) : null;
    if (timer === null) void run();
    return () => {
      if (timer !== null) window.clearTimeout(timer);
      controller.abort();
    };
  }, [sessionParams, listRetry, search]);

  const loadMoreSessions = useCallback(async () => {
    const controller = new AbortController();
    listController.current?.abort();
    listController.current = controller;
    setListLoadingMore(true);
    setListError(null);
    try {
      const rows = await fetchAPI<SessionSummary[]>("sessions", sessionParams(sessions.length), { signal: controller.signal });
      if (controller.signal.aborted) return;
      setSessions((current) => sortSessions([...current, ...(rows || [])]));
      setListHasMore((rows || []).length === SESSION_PAGE_SIZE);
    } catch (error) {
      if (!isAbortError(error)) setListError(error instanceof Error ? error.message : String(error));
    } finally {
      if (!controller.signal.aborted) setListLoadingMore(false);
    }
  }, [sessionParams, sessions.length]);

  useEffect(() => {
    eventController.current?.abort();
    setEvents([]);
    setEventsError(null);
    setEventsHasMore(false);
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
        if (controller.signal.aborted) return;
        setEvents(sortEvents(rows || []));
        setEventsHasMore((rows || []).length === EVENT_PAGE_SIZE);
      } catch (error) {
        if (!isAbortError(error)) setEventsError(error instanceof Error ? error.message : String(error));
      } finally {
        if (!controller.signal.aborted) setEventsLoading(false);
      }
    };
    void run();
    return () => controller.abort();
  }, [selected?.source, selected?.session_id, eventsRetry]);

  const selectSession = useCallback((session: SessionSummary) => {
    if (!selected || sessionIdentity(selected) !== sessionIdentity(session)) {
      rawController.current?.abort();
      setRawCache({});
      setRawError(null);
      setRawLoadingID(null);
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
      if (controller.signal.aborted) return;
      setEvents((current) => sortEvents([...current, ...(rows || [])]));
      setEventsHasMore((rows || []).length === EVENT_PAGE_SIZE);
    } catch (error) {
      if (!isAbortError(error)) setEventsError(error instanceof Error ? error.message : String(error));
    } finally {
      if (!controller.signal.aborted) setEventsLoadingMore(false);
    }
  }, [events.length, selected]);

  const loadRaw = useCallback(async () => {
    if (!selected || !inspectedEvent || rawCache[inspectedEvent.id]) return;
    rawController.current?.abort();
    const controller = new AbortController();
    rawController.current = controller;
    setRawLoadingID(inspectedEvent.id);
    setRawError(null);
    try {
      const path = `sessions/${encodeURIComponent(selected.source)}/${encodeURIComponent(selected.session_id)}/events/${inspectedEvent.id}/raw`;
      const raw = await fetchRaw<RawEventResponse>(path, { signal: controller.signal });
      if (!controller.signal.aborted) setRawCache((current) => ({ ...current, [inspectedEvent.id]: raw }));
    } catch (error) {
      if (!isAbortError(error)) setRawError(error instanceof Error ? error.message : String(error));
    } finally {
      if (!controller.signal.aborted) setRawLoadingID(null);
    }
  }, [inspectedEvent, rawCache, selected]);

  const updatePreset = (preset: TimePreset) => setFilters((current) => ({
    ...current,
    preset,
    ...getTimeRange(preset, current.from, current.to),
  }));

  const clearAllFilters = () => {
    const range = getTimeRange(DEFAULT_USAGE_FILTERS.preset);
    setFilters({ ...DEFAULT_USAGE_FILTERS, ...range });
    setDrilldown({ from: "", to: "", source: "", model: "", project: "" });
    navigate({ pathname: "/sessions", search: "" }, { replace: true });
  };

  const selectedKey = selected ? sessionIdentity(selected) : null;
  const contextItems = useMemo(() => Object.entries(drilldown).filter(([, value]) => value), [drilldown]);

  const list = (
    <SessionList
      sessions={sessions}
      selectedKey={selectedKey}
      search={search}
      onSearchChange={setSearch}
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
      onBack={() => { setMobileDetailVisible(false); setInspectedEvent(null); }}
      onRetry={() => setEventsRetry((value) => value + 1)}
      onLoadMore={() => { void loadMoreEvents(); }}
      onInspect={setInspectedEvent}
      t={t}
    />
  );

  const inspector = inspectedEvent ? (
    <EventInspector
      event={inspectedEvent}
      raw={rawCache[inspectedEvent.id]}
      rawLoading={rawLoadingID === inspectedEvent.id}
      rawError={rawError}
      onLoadRaw={() => { void loadRaw(); }}
      onClose={() => setInspectedEvent(null)}
      t={t}
    />
  ) : null;

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
      <TimeRangeSelector
        preset={filters.preset}
        onPresetChange={updatePreset}
        granularity={granularity}
        onGranularityChange={(value) => { setGranularity(value); localStorage.setItem("au-granularity", value); }}
        source={filters.source}
        onSourceChange={(source) => setFilters((current) => ({ ...current, source }))}
        onRefresh={() => setListRetry((value) => value + 1)}
        customFrom={filters.from}
        customTo={filters.to}
        onCustomFromChange={(from) => setFilters((current) => ({ ...current, preset: "custom", from }))}
        onCustomToChange={(to) => setFilters((current) => ({ ...current, preset: "custom", to }))}
      />

      <div className="flex min-w-0 flex-wrap items-end gap-3 px-1" data-testid="session-model-project-filters">
        <label className="flex min-w-48 flex-1 flex-col gap-1 text-[10px] text-muted-foreground sm:max-w-64">
          <span>{t("modelFilter")}</span>
          <input
            type="text"
            aria-label={t("modelFilter")}
            value={filters.model}
            onChange={(event) => setFilters((current) => ({ ...current, model: event.target.value }))}
            autoComplete="off"
            className="h-8 min-w-0 rounded border border-border bg-card px-2.5 text-xs text-foreground outline-none focus:border-accent"
          />
        </label>
        <label className="flex min-w-48 flex-1 flex-col gap-1 text-[10px] text-muted-foreground sm:max-w-64">
          <span>{t("projectFilter")}</span>
          <input
            type="text"
            aria-label={t("projectFilter")}
            value={filters.project}
            onChange={(event) => setFilters((current) => ({ ...current, project: event.target.value }))}
            autoComplete="off"
            className="h-8 min-w-0 rounded border border-border bg-card px-2.5 text-xs text-foreground outline-none focus:border-accent"
          />
        </label>
      </div>

      {hasDrilldown(drilldown) && (
        <aside data-testid="session-filter-context" className="flex min-w-0 flex-wrap items-center gap-2 border-y border-border px-3 py-2 text-xs">
          <span className="font-medium text-muted-foreground">{t("inheritedFilters")}</span>
          {contextItems.map(([key, value]) => (
            <span key={key} className="max-w-full truncate border-l-2 border-accent px-2">{t(key)}: {value}</span>
          ))}
          <button type="button" aria-label={t("clearAllFilters")} onClick={clearAllFilters} className="ml-auto rounded border border-border px-2 py-1 font-medium hover:bg-muted">
            {t("clearAll")}
          </button>
        </aside>
      )}

      <main
        data-testid="session-center-grid"
        data-inspector-open={String(Boolean(inspectedEvent))}
        className="session-center-grid min-h-0 min-w-0 flex-1 overflow-hidden border-y border-border"
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
