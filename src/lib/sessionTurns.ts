import type { SessionEvent } from "./types";
import { stripTransportArtifacts } from "./sessionPresentation";

export type SessionEventItem =
  | { key: string; kind: "event"; event: SessionEvent }
  | { key: string; kind: "tool"; call: SessionEvent; result: SessionEvent };

/**
 * One model step: an assistant message plus the tool traffic it produced until
 * the next assistant message. Events recorded before the first assistant
 * message (prompt, session metadata) form a leading step with `index === null`.
 */
export interface SessionStep {
  key: string;
  index: number | null;
  events: SessionEvent[];
  items: SessionEventItem[];
  toolCalls: number;
}

export interface SessionTurn {
  /** Stable identity for React keys: the first event's id. */
  key: string;
  /** The user message that opened the round, or null for content before the first prompt. */
  prompt: SessionEvent | null;
  /** 1-based question number, or null for the leading group before any prompt. */
  round: number | null;
  events: SessionEvent[];
  steps: SessionStep[];
  toolCalls: number;
}

const isPrompt = (event: SessionEvent) => event.event_type === "user_message";

/**
 * Groups a flat timeline into question/answer rounds: a round opens on a user
 * message and absorbs every following event until the next user message, so
 * streamed parts and multi-part prompts stay in one round. Events seen before
 * the first prompt — session metadata, an opening assistant message — form a
 * leading round with `round === null`.
 */
export function groupSessionTurns(events: SessionEvent[]): SessionTurn[] {
  const turns: { key: string; prompt: SessionEvent | null; events: SessionEvent[] }[] = [];
  for (const event of events) {
    const current = turns[turns.length - 1];
    const alreadyAnswered = current ? current.events.some((item) => !isPrompt(item)) : true;
    if (!current || (isPrompt(event) && alreadyAnswered)) {
      turns.push({ key: String(event.id), prompt: isPrompt(event) ? event : null, events: [event] });
    } else {
      current.events.push(event);
    }
  }
  let round = 0;
  return turns.map((turn) => ({
    ...turn,
    round: turn.prompt ? ++round : null,
    steps: groupSessionSteps(turn.events),
    toolCalls: turn.events.filter((item) => item.event_type === "tool_call").length,
  }));
}

/** One-line prompt preview for a collapsed round header. */
export function promptPreview(content: string, max = 140): string {
  const line = stripTransportArtifacts(content).replace(/\s+/g, " ").trim();
  return line.length > max ? `${line.slice(0, max)}…` : line;
}

/**
 * Folds each tool call together with the result that answers it, matched on
 * `tool_call_id`. Results are only claimed by a call recorded before them, so a
 * repeated id can never attach a result to the wrong call. A call without a
 * matching result stays a plain event rather than gaining an empty result slot.
 */
export function pairToolEvents(events: SessionEvent[]): SessionEventItem[] {
  const resultsById = new Map<string, number[]>();
  events.forEach((event, index) => {
    if (event.event_type !== "tool_result" || !event.tool_call_id) return;
    const queue = resultsById.get(event.tool_call_id) ?? [];
    queue.push(index);
    resultsById.set(event.tool_call_id, queue);
  });

  const claimed = new Set<number>();
  const items: SessionEventItem[] = [];
  events.forEach((event, index) => {
    if (claimed.has(index)) return;
    const queue = event.tool_call_id ? resultsById.get(event.tool_call_id) : undefined;
    const answer = queue?.find((candidate) => candidate > index && !claimed.has(candidate));
    if (event.event_type === "tool_call" && answer !== undefined) {
      claimed.add(answer);
      items.push({ key: String(event.id), kind: "tool", call: event, result: events[answer] });
      return;
    }
    items.push({ key: String(event.id), kind: "event", event });
  });
  return items;
}

/** Splits one turn into model steps, delimited by assistant messages. */
function groupSessionSteps(events: SessionEvent[]): SessionStep[] {
  const groups: SessionEvent[][] = [];
  for (const event of events) {
    if (event.event_type === "assistant_message" || groups.length === 0) groups.push([event]);
    else groups[groups.length - 1].push(event);
  }
  let index = 0;
  return groups.map((group) => ({
    key: String(group[0].id),
    index: group.some((event) => event.event_type === "assistant_message") ? ++index : null,
    events: group,
    items: pairToolEvents(group),
    toolCalls: group.filter((event) => event.event_type === "tool_call").length,
  }));
}
