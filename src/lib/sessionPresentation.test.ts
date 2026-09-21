import { describe, expect, it } from "vitest";
import type { SessionEvent } from "./types";
import { cwdBaseName, humanizeSessionTitle, isReadableSessionEvent, shortProjectName } from "./sessionPresentation";

const event = (event_type: SessionEvent["event_type"], content = ""): SessionEvent => ({
  id: 1,
  event_type,
  source_event_type: event_type,
  timestamp: "2025-01-01T00:00:00Z",
  role: event_type === "user_message" ? "user" : "assistant",
  content,
  tool_name: "",
  tool_call_id: "",
  tool_input: "",
  tool_output: "",
  event_status: "",
  duration_ms: null,
});

describe("session presentation", () => {
  it("falls back from synthetic context titles", () => {
    expect(humanizeSessionTitle("<environment_context> <cwd>/work</cwd>", "agent-usage", "/work/agent-usage", "id")).toBe("agent-usage");
    expect(humanizeSessionTitle("<user_shell_command>npm test</user_shell_command>", "agent-usage", "/work/agent-usage", "id")).toBe("agent-usage");
    expect(humanizeSessionTitle('<image name=[Image #1] path="/tmp/synthetic.png">', "agent-usage", "/work/agent-usage", "id")).toBe("agent-usage");
    expect(humanizeSessionTitle("[Image #1]Explain the screenshot", "agent-usage", "/work/agent-usage", "id")).toBe("Explain the screenshot");
    expect(humanizeSessionTitle("Real request", "agent-usage", "/work/agent-usage", "id")).toBe("Real request");
  });

  it("shows the folder name from cwd instead of the mangled session directory", () => {
    // pi / claude record the session directory name, which encodes the whole path.
    expect(shortProjectName("--Users-me-Documents-work-agent-usage-desktop--", "/Users/me/Documents/work/agent-usage-desktop")).toBe("agent-usage-desktop");
    // A hyphenated folder name cannot be recovered from the mangled key, so cwd wins.
    expect(shortProjectName("--Users-me-work-capinfo-bj-promotion-hotline--", "/Users/me/work/capinfo/bj-promotion-hotline")).toBe("bj-promotion-hotline");
    expect(shortProjectName("", "/Users/me/Documents/work/TomlJump/")).toBe("TomlJump");
    // Without a cwd the mangled key stays untouched instead of guessing a folder.
    expect(shortProjectName("--Users-me-work-local-captcha-solver--", "")).toBe("--Users-me-work-local-captcha-solver--");
    expect(shortProjectName("agent-usage-desktop", "")).toBe("agent-usage-desktop");
    expect(shortProjectName("", "")).toBe("");
  });

  it("reads the folder name from either separator style", () => {
    expect(cwdBaseName("/Users/me/work/app/")).toBe("app");
    expect(cwdBaseName("C:\\Users\\me\\work\\app")).toBe("app");
    expect(cwdBaseName("   ")).toBe("");
  });

  it("keeps conversation, tools and errors readable while hiding technical events", () => {
    expect(isReadableSessionEvent(event("user_message", "hello"))).toBe(true);
    expect(isReadableSessionEvent(event("user_message", "<user_shell_command>npm test</user_shell_command>"))).toBe(false);
    expect(isReadableSessionEvent(event("user_message", '<image name=[Image #1] path="/tmp/synthetic.png">'))).toBe(false);
    expect(isReadableSessionEvent(event("assistant_message", "answer"))).toBe(true);
    expect(isReadableSessionEvent(event("tool_call"))).toBe(true);
    expect(isReadableSessionEvent(event("error", "failed"))).toBe(true);
    expect(isReadableSessionEvent(event("metadata", "{}"))).toBe(false);
    expect(isReadableSessionEvent(event("unknown", "{}"))).toBe(false);
    expect(isReadableSessionEvent(event("reasoning", "internal"))).toBe(false);
  });
});
