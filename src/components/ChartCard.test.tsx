import { expect, test } from "vitest";
import { compactNumber } from "./chartUtils";

test("shortens axis numbers without losing the magnitude", () => {
  expect(compactNumber(0)).toBe("0");
  expect(compactNumber(950)).toBe("950");
  expect(compactNumber(50_000_000)).toBe("50M");
  expect(compactNumber(700_000)).toBe("700K");
  expect(compactNumber(1_200_000)).toBe("1.2M");
  expect(compactNumber(2_500_000_000)).toBe("2.5B");
  expect(compactNumber(-50_000_000)).toBe("-50M");
});
