import { describe, expect, it, vi } from "vitest";
import { applyOwnedHydration } from "./settingsHydration";

describe("settings hydration ownership", () => {
  it("does not write state after unmount", () => {
    const apply = vi.fn();

    applyOwnedHydration(false, 1, 1, "late value", apply);

    expect(apply).not.toHaveBeenCalled();
  });

  it("does not write state after a newer owner takes over", () => {
    const apply = vi.fn();

    applyOwnedHydration(true, 1, 2, "stale value", apply);

    expect(apply).not.toHaveBeenCalled();
  });
});
