import type { SessionEvent, SessionEventType } from "./types";

const syntheticTitlePrefixes = [
  "<environment_context>",
  "<permissions instructions>",
  "<collaboration_mode",
  "# AGENTS.md instructions",
];

export function isSyntheticSessionTitle(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return syntheticTitlePrefixes.some((prefix) => normalized.startsWith(prefix.toLowerCase()));
}

export function humanizeSessionTitle(title: string, project: string, cwd: string, sessionID: string): string {
  if (title.trim() && !isSyntheticSessionTitle(title)) return title.trim();
  if (project.trim()) return project.trim();
  const normalizedCwd = cwd.trim().replace(/[\\/]+$/, "");
  if (normalizedCwd) return normalizedCwd.split(/[\\/]/).pop() || normalizedCwd;
  return sessionID;
}

export function isReadableSessionEvent(event: Pick<SessionEvent, "event_type">): boolean {
  const readable: SessionEventType[] = ["user_message", "assistant_message", "tool_call", "tool_result", "error"];
  return readable.includes(event.event_type);
}
