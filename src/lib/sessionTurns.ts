import type { SessionEvent } from "./types";
import { stripTransportArtifacts } from "./sessionPresentation";

export interface SessionTurn {
  /** Stable identity for React keys: the first event's id. */
  key: string;
  /** The user message that opened the round, or null for content before the first prompt. */
  prompt: SessionEvent | null;
  /** 1-based question number, or null for the leading group before any prompt. */
  round: number | null;
  events: SessionEvent[];
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
  const turns: Omit<SessionTurn, "round" | "toolCalls">[] = [];
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
    toolCalls: turn.events.filter((item) => item.event_type === "tool_call").length,
  }));
}

/** One-line prompt preview for a collapsed round header. */
export function promptPreview(content: string, max = 140): string {
  const line = stripTransportArtifacts(content).replace(/\s+/g, " ").trim();
  return line.length > max ? `${line.slice(0, max)}…` : line;
}
