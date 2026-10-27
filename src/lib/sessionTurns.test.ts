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

describe("session steps and tool pairing", () => {
  const call = (id: number, callId: string, name = "shell") => event({
    id, event_type: "tool_call", role: "assistant", content: "", tool_name: name, tool_call_id: callId, tool_input: `run ${name}`,
  });
  const result = (id: number, callId: string, output = "done") => event({
    id, event_type: "tool_result", role: "tool", content: "", tool_call_id: callId, tool_output: output,
  });

  it("splits a round into model steps and keeps the prompt in a leading step", () => {
    const turns = groupSessionTurns([
      prompt(1, "Fix it"),
      event({ id: 2 }),
      call(3, "call-1"),
      result(4, "call-1"),
      event({ id: 5, content: "Fixed" }),
    ]);

    const [step, first, second] = turns[0].steps;
    expect(turns[0].steps).toHaveLength(3);
    expect(step.index).toBeNull();
    expect(step.events.map((item) => item.id)).toEqual([1]);
    expect(first.index).toBe(1);
    expect(first.events.map((item) => item.id)).toEqual([2, 3, 4]);
    expect(first.toolCalls).toBe(1);
    expect(second.index).toBe(2);
    expect(second.events.map((item) => item.id)).toEqual([5]);
  });

  it("folds a tool result into the call that answers it", () => {
    const turns = groupSessionTurns([prompt(1, "Run it"), event({ id: 2 }), call(3, "call-1"), result(4, "call-1")]);
    const items = turns[0].steps[1].items;

    expect(items).toHaveLength(2);
    expect(items[1].kind).toBe("tool");
    expect(items[1].kind === "tool" && items[1].result?.id).toBe(4);
  });

  it("keeps an unanswered call as a plain card instead of an empty result", () => {
    const turns = groupSessionTurns([prompt(1, "Run it"), event({ id: 2 }), call(3, "call-missing"), event({ id: 4 })]);
    const items = turns[0].steps[1].items;

    expect(items.map((item) => item.kind)).toEqual(["event", "event"]);
  });

  it("ignores a result recorded before its call and never pairs across steps", () => {
    const outOfOrder = groupSessionTurns([
      prompt(1, "Run it"),
      event({ id: 2 }),
      result(3, "call-1"),
      call(4, "call-1"),
    ]);
    expect(outOfOrder[0].steps[1].items.map((item) => item.kind)).toEqual(["event", "event", "event"]);

    const acrossSteps = groupSessionTurns([
      prompt(1, "Run it"),
      event({ id: 2 }),
      call(3, "call-1"),
      event({ id: 4 }),
      result(5, "call-1"),
    ]);
    expect(acrossSteps[0].steps[1].items.map((item) => item.kind)).toEqual(["event", "event"]);
    expect(acrossSteps[0].steps[2].items.map((item) => item.kind)).toEqual(["event", "event"]);
  });

  it("pairs a repeated call id once instead of reusing the same result", () => {
    const turns = groupSessionTurns([
      prompt(1, "Run twice"),
      event({ id: 2 }),
      call(3, "call-1"),
      result(4, "call-1", "first"),
      call(5, "call-1"),
      result(6, "call-1", "second"),
    ]);
    const items = turns[0].steps[1].items;

    expect(items.map((item) => item.key)).toEqual(["2", "3", "5"]);
    expect(items[1].kind === "tool" && items[1].result?.id).toBe(4);
    expect(items[2].kind === "tool" && items[2].result?.id).toBe(6);
  });

  it("leaves unpaired tool traffic as single cards", () => {
    const turns = groupSessionTurns([prompt(1, "Run it"), event({ id: 2 }), call(3, ""), result(4, "")]);
    expect(turns[0].steps[1].items.map((item) => item.kind)).toEqual(["event", "event", "event"]);
  });
});

describe("prompt preview", () => {
  it("collapses whitespace, drops image markers, and truncates", () => {
    expect(promptPreview("  line one\n\n  line two  ")).toBe("line one line two");
    expect(promptPreview("[image #1] describe this")).toBe("describe this");
    expect(promptPreview("x".repeat(200))).toHaveLength(141);
  });
});
