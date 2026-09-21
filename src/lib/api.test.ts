import { beforeEach, describe, expect, it, vi } from "vitest";

const invoke = vi.fn();

vi.mock("@tauri-apps/api/core", () => ({ invoke }));

describe("API request cancellation", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
    invoke.mockResolvedValue(9800);
  });

  it("passes signals through and never refreshes the cached port after AbortError", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response("{}", { status: 200, headers: { "content-type": "application/json" } }))
      .mockRejectedValueOnce(Object.assign(new Error("aborted"), { name: "AbortError" }));
    vi.stubGlobal("fetch", fetchMock);
    const { fetchAPI } = await import("./api");

    await fetchAPI("health", {});
    const controller = new AbortController();
    controller.abort();

    await expect(fetchAPI("sessions", { limit: 50 }, { signal: controller.signal }))
      .rejects.toMatchObject({ name: "AbortError" });
    expect(invoke).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ signal: controller.signal });
  });
});
