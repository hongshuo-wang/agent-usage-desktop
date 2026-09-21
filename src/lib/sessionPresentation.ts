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

export function cwdBaseName(cwd: string): string {
  const normalized = cwd.trim().replace(/[\\/]+$/, "");
  if (!normalized) return "";
  return normalized.split(/[\\/]/).pop() || normalized;
}

export function humanizeSessionTitle(title: string, project: string, cwd: string, sessionID: string): string {
  const normalizedTitle = stripTransportArtifacts(title);
  if (normalizedTitle && !isSyntheticSessionTitle(normalizedTitle)) return normalizedTitle;
  // cwd wins over project: the stored project key is the mangled session directory.
  const base = cwdBaseName(cwd);
  if (base) return base;
  if (project.trim()) return project.trim();
  return sessionID;
}

// pi and claude name session directories after the whole cwd, so the project
// key reads "--Users-me-work-app--". Only the recorded cwd can name the folder
// unambiguously, so without it the key is shown untouched rather than guessed.
export function shortProjectName(project: string, cwd: string): string {
  return cwdBaseName(cwd) || project.trim();
}

export function isReadableSessionEvent(event: Pick<SessionEvent, "event_type" | "content">): boolean {
  if (isTransportArtifactContent(event.content)) return false;
  const readable: SessionEventType[] = ["user_message", "assistant_message", "tool_call", "tool_result", "error"];
  return readable.includes(event.event_type);
}
