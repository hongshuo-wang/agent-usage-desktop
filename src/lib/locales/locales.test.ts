import { describe, expect, it } from "vitest";
import en from "./en.json";
import zh from "./zh.json";

describe("locale contracts", () => {
  it("provides the dashboard summary and insight copy in both languages", () => {
    const expected = {
      usageOverview: ["Usage overview", "用量概览"],
      peakUsage: ["Peak usage", "最高用量"],
      topModel: ["Top model", "主要模型"],
      topProject: ["Top project", "主要项目"],
      viewRelatedSessions: ["View related sessions", "查看相关会话"],
      cacheServedRatio: ["Cache served ratio", "缓存服务比例"],
    } as const;

    for (const [key, [english, chinese]] of Object.entries(expected)) {
      expect(en).toHaveProperty(key, english);
      expect(zh).toHaveProperty(key, chinese);
    }
  });

  it("keeps English and Chinese key sets identical", () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort());
  });

  it("does not retain removed configuration-management surfaces", () => {
    const removedKeys = [
      "config", "configManagement", "providers", "providerProfiles", "mcpServers",
      "skillsManagement", "backups", "syncStatus", "accountSettings", "teamSettings",
    ];
    for (const key of removedKeys) {
      expect(en).not.toHaveProperty(key);
      expect(zh).not.toHaveProperty(key);
    }
    const text = `${Object.values(en).join(" ")} ${Object.values(zh).join(" ")}`;
    expect(text).not.toMatch(/MCP|Skills management|Provider profiles|配置备份|团队设置/);
  });
});
