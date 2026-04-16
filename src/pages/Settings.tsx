import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { invoke } from "@tauri-apps/api/core";

export default function Settings() {
  const { t, i18n } = useTranslation();
  const [autostart, setAutostart] = useState(false);
  const [theme, setTheme] = useState(localStorage.getItem("au-theme") || "system");
  const [costThreshold, setCostThreshold] = useState(10);
  const [notificationsEnabled, setNotificationsEnabled] = useState(
    localStorage.getItem("au-notifications") !== "false"
  );

  // Load cost threshold from Tauri backend on mount
  useEffect(() => {
    invoke<number>("get_cost_threshold").then(setCostThreshold).catch(() => {});
  }, []);

  const handleThemeChange = (value: string) => {
    setTheme(value);
    localStorage.setItem("au-theme", value);
    const resolved = value === "system"
      ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : value;
    document.documentElement.classList.toggle("dark", resolved === "dark");
  };

  const handleLangChange = (value: string) => {
    i18n.changeLanguage(value);
    localStorage.setItem("au-lang", value);
  };

  const handleAutostartToggle = async () => {
    try {
      if (autostart) {
        await invoke("plugin:autostart|disable");
      } else {
        await invoke("plugin:autostart|enable");
      }
      setAutostart(!autostart);
    } catch (e) {
      console.error("Autostart toggle failed:", e);
    }
  };

  const handleThresholdChange = (value: number) => {
    setCostThreshold(value);
    invoke("set_cost_threshold", { threshold: value }).catch(() => {});
  };

  const handleNotificationsToggle = () => {
    const next = !notificationsEnabled;
    setNotificationsEnabled(next);
    localStorage.setItem("au-notifications", String(next));
  };

  return (
    <div className="max-w-lg space-y-8">
      {/* Theme */}
      <section>
        <h3 className="text-sm font-semibold mb-3">{t("theme")}</h3>
        <div className="flex gap-2">
          {["light", "dark", "system"].map((v) => (
            <button key={v} onClick={() => handleThemeChange(v)}
              className={`px-4 py-2 rounded-lg text-sm border transition-colors ${
                theme === v ? "bg-accent text-white border-accent" : "border-border text-muted-foreground hover:text-foreground"
              }`}>
              {t(v)}
            </button>
          ))}
        </div>
      </section>

      {/* Language */}
      <section>
        <h3 className="text-sm font-semibold mb-3">{t("language")}</h3>
        <div className="flex gap-2">
          {[{ value: "en", label: "English" }, { value: "zh", label: "中文" }].map((lang) => (
            <button key={lang.value} onClick={() => handleLangChange(lang.value)}
              className={`px-4 py-2 rounded-lg text-sm border transition-colors ${
                i18n.language === lang.value ? "bg-accent text-white border-accent" : "border-border text-muted-foreground hover:text-foreground"
              }`}>
              {lang.label}
            </button>
          ))}
        </div>
      </section>

      {/* Autostart */}
      <section>
        <h3 className="text-sm font-semibold mb-3">{t("autostart")}</h3>
        <button onClick={handleAutostartToggle}
          className={`px-4 py-2 rounded-lg text-sm border transition-colors ${
            autostart ? "bg-accent text-white border-accent" : "border-border text-muted-foreground"
          }`}>
          {autostart ? t("enabled") : t("disabled")}
        </button>
      </section>

      {/* Notifications */}
      <section>
        <h3 className="text-sm font-semibold mb-3">{t("notification")}</h3>
        <div className="space-y-3">
          <button onClick={handleNotificationsToggle}
            className={`px-4 py-2 rounded-lg text-sm border transition-colors ${
              notificationsEnabled ? "bg-accent text-white border-accent" : "border-border text-muted-foreground"
            }`}>
            {notificationsEnabled ? t("enabled") : t("disabled")}
          </button>
          {notificationsEnabled && (
            <div className="flex items-center gap-3">
              <label className="text-sm text-muted-foreground">{t("dailyCostThreshold")}</label>
              <input type="number" value={costThreshold} min={0} step={1}
                onChange={(e) => handleThresholdChange(Number(e.target.value))}
                className="w-24 bg-card border border-border rounded-lg px-3 py-2 text-sm" />
              <span className="text-sm text-muted-foreground">USD</span>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
