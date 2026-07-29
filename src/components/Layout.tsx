import { BarChart3, ChevronDown, Languages, MessagesSquare, MonitorCog, Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { SYSTEM_NAV_ITEMS } from "../lib/systemNavigation";

const navItems = [
  { path: "/", label: "title", icon: BarChart3 },
  { path: "/sessions", label: "sessionLog", icon: MessagesSquare },
  { path: "/settings/data-sources", label: "settings", icon: MonitorCog },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const [systemOpen, setSystemOpen] = useState(() => location.pathname.startsWith("/settings"));

  useEffect(() => {
    applyTheme(localStorage.getItem("au-theme") || "system");
  }, []);

  useEffect(() => {
    setSystemOpen(location.pathname.startsWith("/settings"));
  }, [location.pathname]);

  const toggleTheme = () => {
    const current = localStorage.getItem("au-theme") || "system";
    const next = current === "light" ? "dark" : current === "dark" ? "system" : "light";
    localStorage.setItem("au-theme", next);
    applyTheme(next);
  };

  const toggleLang = () => {
    const next = i18n.language === "en" ? "zh" : "en";
    i18n.changeLanguage(next);
    localStorage.setItem("au-lang", next);
  };

  return (
    <div className="app-shell h-screen flex bg-background overflow-hidden">
      <aside className="app-rail hidden w-56 shrink-0 border-r border-border bg-card lg:flex lg:flex-col">
        <div className="px-5 py-5">
          <div className="text-sm font-semibold tracking-tight">Agent Usage</div>
          <div className="mt-1 text-[10px] text-muted-foreground">Local observability</div>
        </div>
        <nav
          aria-label="Primary"
          data-testid="desktop-navigation"
          className="flex flex-1 flex-col gap-1 px-3 py-3"
        >
          {navItems.map((item) => {
            const isActive = item.path === "/"
              ? location.pathname === "/"
              : item.path.startsWith("/settings")
                ? location.pathname.startsWith("/settings")
                : location.pathname.startsWith(item.path);
            const Icon = item.icon;
            const isSystem = item.path.startsWith("/settings");
            return (
              <div key={item.path}>
                {isSystem ? (
                  <button
                    type="button"
                    aria-expanded={systemOpen}
                    aria-controls="desktop-system-navigation"
                    onClick={() => setSystemOpen((open) => !open)}
                    className={`flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                      isActive
                        ? "font-semibold text-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                  >
                    <Icon className="h-4 w-4" aria-hidden="true" />
                    <span className="flex-1 text-left">{t(item.label)}</span>
                    <ChevronDown
                      className={`h-3.5 w-3.5 transition-transform ${systemOpen ? "rotate-180" : ""}`}
                      aria-hidden="true"
                    />
                  </button>
                ) : (
                  <Link
                    to={item.path}
                    aria-current={isActive ? "page" : undefined}
                    className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                      isActive
                        ? "bg-muted font-semibold text-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                  >
                    <Icon className="h-4 w-4" aria-hidden="true" />
                    {t(item.label)}
                  </Link>
                )}
                {isSystem && systemOpen && (
                  <div
                    id="desktop-system-navigation"
                    className="mt-1 grid gap-0.5 pb-1 pl-9 pr-1"
                    aria-label={t("systemWorkspace")}
                  >
                    {SYSTEM_NAV_ITEMS.map((child) => {
                      const childActive = location.pathname === child.path;
                      return (
                        <Link
                          key={child.path}
                          to={child.path}
                          aria-current={childActive ? "page" : undefined}
                          className={`relative flex min-h-8 items-center rounded-md pl-3.5 pr-2 text-xs transition-colors before:absolute before:left-0.5 before:h-1.5 before:w-1.5 before:rounded-full before:bg-transparent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${childActive ? "font-semibold text-accent before:bg-accent" : "text-muted-foreground hover:bg-muted/70 hover:text-foreground"}`}
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
        <div className="border-t border-border px-4 py-4 text-xs text-muted-foreground">
          <div className="flex items-center justify-between">
            <span>{t("theme")}</span>
            <button
              type="button"
              onClick={toggleTheme}
              aria-label={t("theme")}
              className="icon-button focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {document.documentElement.classList.contains("dark") ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </div>
          <button
            type="button"
            onClick={toggleLang}
            className="mt-3 flex w-full items-center justify-between rounded-md px-2 py-1.5 hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="flex items-center gap-2"><Languages className="h-4 w-4" /> {t("language")}</span>
            <span>{i18n.language.toUpperCase()}</span>
          </button>
        </div>
      </aside>
      <div className="min-w-0 flex min-h-0 flex-1 flex-col">
      <header className="sticky top-0 z-50 border-b border-border bg-background/90 backdrop-blur-sm lg:hidden">
        <div className="flex items-center justify-between px-4 py-3">
          <div className="text-sm font-semibold">Agent Usage</div>
          <nav
            className="flex items-center gap-1"
            aria-label="Primary"
            data-testid="mobile-navigation"
          >
            {navItems.map((item) => {
              const isActive = item.path === "/"
                ? location.pathname === "/"
                : item.path.startsWith("/settings")
                  ? location.pathname.startsWith("/settings")
                  : location.pathname.startsWith(item.path);

              return (
                <Link
                  key={item.path}
                  to={item.path}
                  aria-current={isActive ? "page" : undefined}
                  className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                    isActive
                      ? "bg-muted text-foreground"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  }`}
                >
                  {t(item.label)}
                </Link>
              );
            })}
          </nav>
        </div>
        {location.pathname.startsWith("/settings") && (
          <nav aria-label={t("systemWorkspace")} className="flex gap-1 overflow-x-auto border-t border-border px-4 py-2">
            {SYSTEM_NAV_ITEMS.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                aria-current={location.pathname === item.path ? "page" : undefined}
                className={`shrink-0 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${location.pathname === item.path ? "bg-accent-dim text-accent" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
              >
                {t(item.label)}
              </Link>
            ))}
          </nav>
        )}
      </header>
      <main className="app-main flex-1 min-h-0 min-w-0 flex flex-col overflow-y-auto px-4 py-5 sm:px-6 lg:px-8">
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
