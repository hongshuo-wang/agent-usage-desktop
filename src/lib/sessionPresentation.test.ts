import { describe, expect, it } from "vitest";
import type { SessionEvent } from "./types";
import { humanizeSessionTitle, isReadableSessionEvent } from "./sessionPresentation";

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
  has_raw: false,
});

describe("session presentation", () => {
  it("falls back from synthetic context titles", () => {
    expect(humanizeSessionTitle("<environment_context> <cwd>/work</cwd>", "agent-usage", "/work/agent-usage", "id")).toBe("agent-usage");
    expect(humanizeSessionTitle("Real request", "agent-usage", "/work/agent-usage", "id")).toBe("Real request");
  });

  it("keeps conversation, tools and errors readable while hiding technical events", () => {
    expect(isReadableSessionEvent(event("user_message", "hello"))).toBe(true);
    expect(isReadableSessionEvent(event("assistant_message", "answer"))).toBe(true);
    expect(isReadableSessionEvent(event("tool_call"))).toBe(true);
    expect(isReadableSessionEvent(event("error", "failed"))).toBe(true);
    expect(isReadableSessionEvent(event("metadata", "{}"))).toBe(false);
    expect(isReadableSessionEvent(event("unknown", "{}"))).toBe(false);
    expect(isReadableSessionEvent(event("reasoning", "internal"))).toBe(false);
  });
});
