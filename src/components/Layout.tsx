import { BarChart3, Languages, MessagesSquare, MonitorCog, Moon, Sun } from "lucide-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";

const navItems = [
  { path: "/", label: "title", icon: BarChart3 },
  { path: "/sessions", label: "sessionLog", icon: MessagesSquare },
  { path: "/settings", label: "settings", icon: MonitorCog },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation();
  const location = useLocation();

  useEffect(() => {
    applyTheme(localStorage.getItem("au-theme") || "system");
  }, []);

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
        <nav aria-label="Primary" className="flex flex-1 flex-col gap-1 px-3 py-3">
          {navItems.map((item) => {
            const isActive = item.path === "/"
              ? location.pathname === "/"
              : location.pathname.startsWith(item.path);
            const Icon = item.icon;
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors ${
                  isActive
                    ? "bg-accent-dim font-semibold text-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                }`}
              >
                <Icon className="h-4 w-4" aria-hidden="true" />
                {t(item.label)}
              </Link>
            );
          })}
        </nav>
        <div className="border-t border-border px-4 py-4 text-xs text-muted-foreground">
          <div className="flex items-center justify-between">
            <span>{t("theme")}</span>
            <button type="button" onClick={toggleTheme} aria-label={t("theme")} className="icon-button">
              {document.documentElement.classList.contains("dark") ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </div>
          <button type="button" onClick={toggleLang} className="mt-3 flex w-full items-center justify-between rounded-md px-2 py-1.5 hover:bg-muted">
            <span className="flex items-center gap-2"><Languages className="h-4 w-4" /> {t("language")}</span>
            <span>{i18n.language.toUpperCase()}</span>
          </button>
        </div>
      </aside>
      <div className="min-w-0 flex min-h-0 flex-1 flex-col">
      <header className="sticky top-0 z-50 border-b border-border bg-background/90 backdrop-blur-sm lg:hidden">
        <div className="flex items-center justify-between px-4 py-3">
          <div className="text-sm font-semibold">Agent Usage</div>
          <nav className="flex items-center gap-1" aria-label="Primary">
            {navItems.map((item) => {
              const isActive = item.path === "/"
                ? location.pathname === "/"
                : location.pathname.startsWith(item.path);

              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
                    isActive
                      ? "bg-accent-dim text-foreground"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  }`}
                >
                  {t(item.label)}
                </Link>
              );
            })}
          </nav>
        </div>
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
