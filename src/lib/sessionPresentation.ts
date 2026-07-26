import type { SessionEvent, SessionEventType } from "./types";

const syntheticTitlePrefixes = [
  "<environment_context>",
  "<permissions instructions>",
  "<collaboration_mode",
  "<user_shell_command>",
  "<image name=",
  "</image>",
  "<turn_aborted>",
  "# AGENTS.md instructions",
];

export function stripTransportArtifacts(value: string): string {
  return value.replace(/\[image\s+#\d+\]/gi, "").trim();
}

export function isTransportArtifactContent(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  const imageMarkerOnly = /\[image\s+#\d+\]/i.test(normalized) && stripTransportArtifacts(normalized) === "";
  return imageMarkerOnly || syntheticTitlePrefixes.some((prefix) => normalized.startsWith(prefix.toLowerCase()));
}

export function isSyntheticSessionTitle(value: string): boolean {
  return isTransportArtifactContent(value);
}

export function humanizeSessionTitle(title: string, project: string, cwd: string, sessionID: string): string {
  const normalizedTitle = stripTransportArtifacts(title);
  if (normalizedTitle && !isSyntheticSessionTitle(normalizedTitle)) return normalizedTitle;
  if (project.trim()) return project.trim();
  const normalizedCwd = cwd.trim().replace(/[\\/]+$/, "");
  if (normalizedCwd) return normalizedCwd.split(/[\\/]/).pop() || normalizedCwd;
  return sessionID;
}

export function isReadableSessionEvent(event: Pick<SessionEvent, "event_type" | "content">): boolean {
  if (isTransportArtifactContent(event.content)) return false;
  const readable: SessionEventType[] = ["user_message", "assistant_message", "tool_call", "tool_result", "error"];
  return readable.includes(event.event_type);
}
