import { AlertCircle, Bot, ChevronDown, ChevronRight, CircleUserRound, Terminal } from "lucide-react";
import { useState } from "react";
import type { SessionEvent } from "../../lib/types";

type Translate = (key: string) => string;

const LONG_RESULT_LENGTH = 240;

function visibleValue(event: SessionEvent): string {
  if (event.event_type === "tool_call") return event.tool_input;
  if (event.event_type === "tool_result") return event.tool_output;
  return event.content;
}

function initiallyExpanded(event: SessionEvent): boolean {
  if (event.event_type === "error") return true;
  if (event.event_type === "tool_call") return false;
  if (event.event_type === "tool_result" && visibleValue(event).length > LONG_RESULT_LENGTH) return false;
  return true;
}

function EventIcon({ event }: { event: SessionEvent }) {
  if (event.event_type === "error") return <AlertCircle aria-hidden="true" className="h-4 w-4 text-red-500" />;
  if (event.event_type === "tool_call" || event.event_type === "tool_result") return <Terminal aria-hidden="true" className="h-4 w-4 text-accent" />;
  if (event.role === "user") return <CircleUserRound aria-hidden="true" className="h-4 w-4" />;
  return <Bot aria-hidden="true" className="h-4 w-4" />;
}

export default function EventCard({ event, onInspect, t }: { event: SessionEvent; onInspect: (event: SessionEvent) => void; t: Translate }) {
  const [expanded, setExpanded] = useState(() => initiallyExpanded(event));
  const value = visibleValue(event);
  const canCollapse = event.event_type === "tool_call" || event.event_type === "error" ||
    (event.event_type === "tool_result" && value.length > LONG_RESULT_LENGTH);

  return (
    <article
      data-testid={`event-card-${event.id}`}
      tabIndex={0}
      onClick={() => onInspect(event)}
      onKeyDown={(keyEvent) => {
        if (keyEvent.key === "Enter" || keyEvent.key === " ") onInspect(event);
      }}
      className={`rounded border px-3 py-2.5 outline-none transition-colors hover:border-accent focus-visible:ring-2 focus-visible:ring-accent ${event.event_type === "error" ? "border-red-500/40 bg-red-500/5" : "border-border bg-card"}`}
    >
      <header className="flex min-w-0 items-center gap-2">
        <EventIcon event={event} />
        <span className="min-w-0 flex-1 truncate text-xs font-semibold">
          {event.tool_name || t(`eventType_${event.event_type}`)}
        </span>
        {event.duration_ms !== null && <span className="font-mono text-[10px] text-muted-foreground">{event.duration_ms}ms</span>}
        <time dateTime={event.timestamp} className="shrink-0 font-mono text-[10px] text-muted-foreground">
          {new Date(event.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}
        </time>
        {canCollapse && (
          <button
            type="button"
            aria-label={t(expanded ? "collapseEvent" : "expandEvent")}
            onClick={(clickEvent) => { clickEvent.stopPropagation(); setExpanded((current) => !current); }}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted"
          >
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </button>
        )}
      </header>
      {expanded && (
        <pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-5 text-foreground">
          {value || t("sourceDataUnavailable")}
        </pre>
      )}
      {!expanded && <p className="mt-1 text-[10px] text-muted-foreground">{t("eventCollapsed")}</p>}
    </article>
  );
}
