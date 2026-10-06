import { describe, expect, it } from "vitest";
import { releaseSummary } from "./releaseSummary";

describe("releaseSummary", () => {
  it("prefers the current language overview", () => {
    const body = `
## 概览
- 统一总览筛选。
- 新增会话回溯。

## Summary
- Shared analytics filters.
`;

    expect(releaseSummary(body, "zh")).toEqual(["统一总览筛选。", "新增会话回溯。"]);
    expect(releaseSummary(body, "en")).toEqual(["Shared analytics filters."]);
  });

  it("supports legacy overview headings and limits the result", () => {
    const body = `
### Highlights
- One
- Two
- Three
- Four
- Five
`;

    expect(releaseSummary(body, "en")).toEqual(["One", "Two", "Three", "Four"]);
  });

  it("returns no summary for generated changelog-only content", () => {
    expect(releaseSummary("## What's Changed\n- fix: update dependency", "en")).toEqual([]);
  });
});
