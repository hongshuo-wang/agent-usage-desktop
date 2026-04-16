import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { open } from "@tauri-apps/plugin-shell";
import { getWebUIUrl } from "../lib/api";

const navItems = [
  { path: "/", label: "title" },
  { path: "/sessions", label: "sessionLog" },
  { path: "/settings", label: "settings" },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation();
  const location = useLocation();

  const handleOpenWebUI = async () => {
    const url = await getWebUIUrl();
    open(url);
  };

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
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-50 border-b border-border bg-background/85 backdrop-blur-sm">
        <div className="mx-auto max-w-[1200px] px-6 py-3 flex items-center justify-between">
          <nav className="flex items-center gap-6">
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={`text-sm font-medium transition-colors ${
                  location.pathname === item.path
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t(item.label)}
              </Link>
            ))}
          </nav>
          <div className="flex items-center gap-3">
            <button onClick={handleOpenWebUI} className="text-sm text-muted-foreground hover:text-foreground">
              {t("openWebUI")}
            </button>
            <button onClick={toggleTheme} className="text-sm text-muted-foreground hover:text-foreground">
              {t("theme")}
            </button>
            <button onClick={toggleLang} className="text-sm text-muted-foreground hover:text-foreground">
              {i18n.language.toUpperCase()}
            </button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-[1200px] px-6 py-6">
        {children}
      </main>
    </div>
  );
}

function applyTheme(theme: string) {
  const resolved = theme === "system"
    ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
    : theme;
  document.documentElement.classList.toggle("dark", resolved === "dark");
}
