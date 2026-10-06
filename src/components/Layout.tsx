import { ChevronDown, Info, Languages, MessagesSquare, MonitorCog, Moon, PieChart, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { SYSTEM_NAV_ITEMS } from "../lib/systemNavigation";

const navItems = [
  { path: "/", label: "title", icon: PieChart },
  { path: "/sessions", label: "sessionLog", icon: MessagesSquare },
  { path: "/settings/data-sources", label: "settings", icon: MonitorCog, expandable: true },
  { path: "/settings/about", label: "about", icon: Info },
];

function isActivePath(itemPath: string, pathname: string) {
  if (itemPath === "/") return pathname === "/";
  if (itemPath === "/settings/about") return pathname === itemPath;
  if (itemPath === "/settings/data-sources") {
    return pathname.startsWith("/settings/") && pathname !== "/settings/about";
  }
  return pathname.startsWith(itemPath);
}

/** The logo already carries the whole palette; the shell just frames it. */
function BrandMark({ className = "h-9 w-9" }: { className?: string }) {
  return (
    <span className={`mark-tile ${className}`}>
      <img src="/logo.svg" alt="" className="h-[76%] w-[76%]" />
    </span>
  );
}

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const [systemOpen, setSystemOpen] = useState(() => location.pathname.startsWith("/settings/") && location.pathname !== "/settings/about");
  const [isDark, setIsDark] = useState(false);
  const language = i18n.language.startsWith("en") ? "en" : "zh";

  useEffect(() => {
    const theme = localStorage.getItem("au-theme") || "system";
    applyTheme(theme);
    setIsDark(document.documentElement.classList.contains("dark"));
  }, []);

  // Screen readers pick a pronunciation from the document language.
  useEffect(() => {
    document.documentElement.lang = i18n.language;
  }, [i18n.language]);

  useEffect(() => {
    setSystemOpen(location.pathname.startsWith("/settings/") && location.pathname !== "/settings/about");
  }, [location.pathname]);

  const toggleTheme = () => {
    const current = localStorage.getItem("au-theme") || "system";
    const next = current === "light" ? "dark" : current === "dark" ? "system" : "light";
    localStorage.setItem("au-theme", next);
    applyTheme(next);
    setIsDark(document.documentElement.classList.contains("dark"));
  };

  const selectLanguage = (next: string) => {
    i18n.changeLanguage(next);
    localStorage.setItem("au-lang", next);
  };

  return (
    <div className="relative flex h-screen overflow-hidden bg-background text-foreground">
      <aside className="rail relative z-10 hidden w-60 shrink-0 flex-col overflow-hidden lg:flex">
        <div className="flex items-center gap-3 px-4 pb-5 pt-5">
          <BrandMark />
          <span className="min-w-0">
            <span className="block truncate text-sm font-semibold tracking-tight">Agent Usage</span>
            <span className="block truncate text-2xs text-muted-foreground">{t("localObservability")}</span>
          </span>
        </div>

        <nav
          aria-label="Primary"
          data-testid="desktop-navigation"
          className="flex flex-1 flex-col gap-1 px-3 py-2"
        >
          {navItems.map((item) => {
            const isActive = isActivePath(item.path, location.pathname);
            const Icon = item.icon;
            const isSystem = item.expandable === true;
            const itemClass = `rail-link focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
              isActive ? "rail-link-active font-semibold" : ""
            }`;
            return (
              <div key={item.path}>
                {isSystem ? (
                  <button
                    type="button"
                    aria-expanded={systemOpen}
                    aria-controls="desktop-system-navigation"
                    onClick={() => setSystemOpen((open) => !open)}
                    className={`${itemClass} w-full text-left`}
                  >
                    <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                    <span className="flex-1">{t(item.label)}</span>
                    <ChevronDown
                      className={`h-3.5 w-3.5 shrink-0 transition-transform duration-300 ${systemOpen ? "rotate-180" : ""}`}
                      aria-hidden="true"
                    />
                  </button>
                ) : (
                  <Link to={item.path} aria-current={isActive ? "page" : undefined} className={itemClass}>
                    <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                    {t(item.label)}
                  </Link>
                )}
                {isSystem && systemOpen && (
                  <div
                    id="desktop-system-navigation"
                    className="my-1 ml-4 grid gap-px border-l border-border pl-2.5"
                    aria-label={t("systemWorkspace")}
                  >
                    {SYSTEM_NAV_ITEMS.map((child) => {
                      const childActive = location.pathname === child.path;
                      return (
                        <Link
                          key={child.path}
                          to={child.path}
                          aria-current={childActive ? "page" : undefined}
                          className={`flex min-h-7 items-center rounded-md px-2 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                            childActive
                              ? "-ml-[0.6875rem] border-l border-accent pl-[0.625rem] font-semibold text-accent"
                              : "text-muted-foreground hover:bg-muted hover:text-foreground"
                          }`}
                        >
                          {t(child.label)}
                        </Link>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </nav>

        <div className="flex items-center gap-2 border-t border-border/70 px-3 py-3">
          <button
            type="button"
            onClick={toggleTheme}
            aria-label={t("theme")}
            title={t("theme")}
            className="icon-button text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            {isDark ? <Sun className="h-4 w-4" aria-hidden="true" /> : <Moon className="h-4 w-4" aria-hidden="true" />}
          </button>
          <div className="flex flex-1 items-center justify-end gap-1.5">
            <Languages className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div className="flex rounded-lg bg-muted p-0.5">
              {[
                { value: "zh", label: "中文" },
                { value: "en", label: "EN" },
              ].map((option) => (
                <button
                  key={option.value}
                  type="button"
                  aria-pressed={language === option.value}
                  onClick={() => selectLanguage(option.value)}
                  className={`rounded-md px-2 py-1 text-2xs font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                    language === option.value
                      ? "bg-card text-accent shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
        </div>
      </aside>

      <div className="relative z-10 flex min-h-0 min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-50 border-b border-border bg-background lg:hidden">
          <div className="flex items-center justify-between px-4 py-3">
            <div className="flex items-center gap-2">
              <BrandMark className="h-7 w-7" />
              <span className="text-sm font-semibold tracking-tight">Agent Usage</span>
            </div>
            <nav className="flex items-center gap-1" aria-label="Primary" data-testid="mobile-navigation">
              {navItems.map((item) => {
                const isActive = isActivePath(item.path, location.pathname);
                if (item.expandable) {
                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      aria-current={isActive ? "page" : undefined}
                      className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                        isActive ? "bg-accent-dim text-accent" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                      }`}
                    >
                      {t(item.label)}
                    </Link>
                  );
                }
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    aria-current={isActive ? "page" : undefined}
                    className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                      isActive ? "bg-accent-dim text-accent" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                  >
                    {t(item.label)}
                  </Link>
                );
              })}
            </nav>
          </div>
          {location.pathname.startsWith("/settings/") && location.pathname !== "/settings/about" && (
            <nav aria-label={t("systemWorkspace")} className="flex gap-1 overflow-x-auto border-t border-border px-4 py-2">
              {SYSTEM_NAV_ITEMS.map((item) => (
                <Link
                  key={item.path}
                  to={item.path}
                  aria-current={location.pathname === item.path ? "page" : undefined}
                  className={`shrink-0 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                    location.pathname === item.path
                      ? "bg-accent-dim text-accent"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  }`}
                >
                  {t(item.label)}
                </Link>
              ))}
            </nav>
          )}
        </header>

        <main className="app-main flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto px-4 py-5 sm:px-6 lg:px-9 lg:py-7">
          {children}
        </main>
      </div>
    </div>
  );
}

function applyTheme(theme: string) {
  const resolved = theme === "system"
    ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
    : theme;
  document.documentElement.classList.toggle("dark", resolved === "dark");
}
