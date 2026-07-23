import { Braces, LoaderCircle, X } from "lucide-react";
import type { RawEventResponse, SessionEvent } from "../../lib/types";

type Translate = (key: string) => string;

interface Props {
  event: SessionEvent;
  raw: RawEventResponse | undefined;
  rawLoading: boolean;
  rawError: string | null;
  onLoadRaw: () => void;
  onClose: () => void;
  t: Translate;
}

function InspectorField({ label, value, fallback, mono = false }: { label: string; value: string; fallback: string; mono?: boolean }) {
  return (
    <div className="border-b border-border py-3 last:border-b-0">
      <dt className="text-[10px] font-medium uppercase text-muted-foreground">{label}</dt>
      <dd className={`mt-1 whitespace-pre-wrap break-words text-xs leading-5 ${mono ? "font-mono" : ""}`}>{value || fallback}</dd>
    </div>
  );
}

export default function EventInspector({ event, raw, rawLoading, rawError, onLoadRaw, onClose, t }: Props) {
  const payloadFields: Array<[string, string, boolean]> = [];
  if (event.content || (event.event_type !== "tool_call" && event.event_type !== "tool_result")) {
    payloadFields.push([t("content"), event.content, false]);
  }
  if (event.tool_input || event.event_type === "tool_call") {
    payloadFields.push([t("toolInput"), event.tool_input, true]);
  }
  if (event.tool_output || event.event_type === "tool_result") {
    payloadFields.push([t("toolOutput"), event.tool_output, true]);
  }

  return (
    <aside data-testid="event-inspector" className="flex min-h-0 min-w-0 flex-col border-l border-border bg-card/30">
      <header className="flex min-w-0 items-center gap-2 border-b border-border px-3 py-3">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">{t("eventInspector")}</h2>
          <p className="truncate text-[10px] text-muted-foreground">{event.source_event_type || event.event_type} / {event.id}</p>
        </div>
        <button type="button" aria-label={t("closeInspector")} onClick={onClose} className="flex h-8 w-8 shrink-0 items-center justify-center rounded hover:bg-muted">
          <X className="h-4 w-4" />
        </button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-3">
        <dl>
          <InspectorField label={t("eventType")} value={event.event_type} fallback={t("sourceDataUnavailable")} />
          {event.role && <InspectorField label={t("role")} value={event.role} fallback={t("sourceDataUnavailable")} />}
          {(event.tool_name || event.event_type === "tool_call") && (
            <InspectorField label={t("toolName")} value={event.tool_name} fallback={t("sourceDataUnavailable")} />
          )}
          {payloadFields.map(([label, value, mono]) => (
            <InspectorField key={label} label={label} value={value} fallback={t("sourceDataUnavailable")} mono={mono} />
          ))}
        </dl>

        {event.has_raw && !raw && (
          <button type="button" aria-label={t("loadRawRecord")} onClick={onLoadRaw} disabled={rawLoading} className="my-3 flex w-full items-center justify-center gap-2 rounded border border-border px-3 py-2 text-xs font-medium hover:border-accent disabled:opacity-50">
            {rawLoading ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Braces className="h-4 w-4" />}
            {rawLoading ? t("loading") : t("rawRecord")}
          </button>
        )}
        {rawError && <p className="my-3 break-words text-xs text-red-500">{rawError}</p>}
        {raw && (
          <section className="my-3">
            <h3 className="mb-2 text-xs font-semibold">{t("rawRecord")}</h3>
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded border border-border bg-background p-3 font-mono text-xs leading-5">{raw.content}</pre>
          </section>
        )}
      </div>
    </aside>
  );
}
