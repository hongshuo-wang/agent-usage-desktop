import { describe, expect, it } from "vitest";
import { groupSessionTurns, promptPreview } from "./sessionTurns";
import type { SessionEvent } from "./types";

const event = (overrides: Partial<SessionEvent> = {}): SessionEvent => ({
  id: 1,
  event_type: "assistant_message",
  source_event_type: "message",
  timestamp: "2026-07-23T09:01:00Z",
  role: "assistant",
  content: "A useful answer",
  tool_name: "",
  tool_call_id: "",
  tool_input: "",
  tool_output: "",
  event_status: "",
  duration_ms: null,
  ...overrides,
});

const prompt = (id: number, content: string) => event({ id, event_type: "user_message", role: "user", content });

describe("session turn grouping", () => {
  it("keeps a prompt, its streamed parts, and its tool traffic in one round", () => {
    const turns = groupSessionTurns([
      prompt(1, "Fix the failing test"),
      event({ id: 2, event_type: "reasoning" }),
      event({ id: 3, event_type: "tool_call", tool_name: "shell" }),
      event({ id: 4, event_type: "tool_result" }),
      event({ id: 5, event_type: "assistant_message", content: "Fixed" }),
      prompt(6, "Now deploy it"),
      event({ id: 7, event_type: "tool_call", tool_name: "shell" }),
    ]);

    expect(turns).toHaveLength(2);
    expect(turns[0].events.map((item) => item.id)).toEqual([1, 2, 3, 4, 5]);
    expect(turns[0].round).toBe(1);
    expect(turns[0].toolCalls).toBe(1);
    expect(turns[0].prompt?.content).toBe("Fix the failing test");
    expect(turns[1].events.map((item) => item.id)).toEqual([6, 7]);
    expect(turns[1].round).toBe(2);
    expect(turns[1].toolCalls).toBe(1);
  });

  it("keeps split parts of one prompt together and numbers only real questions", () => {
    const turns = groupSessionTurns([
      event({ id: 1, event_type: "metadata" }),
      prompt(2, "part one"),
      prompt(3, "part two"),
      event({ id: 4, event_type: "assistant_message" }),
    ]);

    expect(turns).toHaveLength(2);
    expect(turns[0].round).toBeNull();
    expect(turns[0].prompt).toBeNull();
    expect(turns[0].events.map((item) => item.id)).toEqual([1]);
    expect(turns[1].events.map((item) => item.id)).toEqual([2, 3, 4]);
    expect(turns[1].round).toBe(1);
    expect(turns[1].prompt?.id).toBe(2);
  });

  it("treats a tool result carried by the user role as an answer, not a new question", () => {
    const turns = groupSessionTurns([
      prompt(1, "Run it"),
      event({ id: 2, event_type: "tool_result", role: "user" }),
      event({ id: 3, event_type: "assistant_message" }),
    ]);

    expect(turns).toHaveLength(1);
    expect(turns[0].events).toHaveLength(3);
  });

  it("returns no rounds for an empty timeline", () => {
    expect(groupSessionTurns([])).toEqual([]);
  });
});

describe("prompt preview", () => {
  it("collapses whitespace, drops image markers, and truncates", () => {
    expect(promptPreview("  line one\n\n  line two  ")).toBe("line one line two");
    expect(promptPreview("[image #1] describe this")).toBe("describe this");
    expect(promptPreview("x".repeat(200))).toHaveLength(141);
  });
});
