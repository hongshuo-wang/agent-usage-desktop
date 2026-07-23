import { describe, expect, it } from "vitest";
import en from "./en.json";
import zh from "./zh.json";

describe("locale contracts", () => {
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
