import { Navigate, NavLink, Outlet, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";

const tabs = [
  { path: "/config/providers", label: "providers" },
  { path: "/config/mcp", label: "mcpServers" },
  { path: "/config/skills", label: "skills" },
  { path: "/config/files", label: "filesBackups" },
];

export default function Config() {
  const { t } = useTranslation();
  const location = useLocation();
  const normalizedPath = location.pathname.replace(/\/+$/, "") || "/";

  if (normalizedPath === "/config") {
    return <Navigate to="/config/providers" replace />;
  }

  return (
    <div className="flex flex-1 min-h-0 flex-col gap-4">
      <nav className="flex items-center gap-2 border-b border-border">
        {tabs.map((tab) => (
          <NavLink
            key={tab.path}
            to={tab.path}
            className={({ isActive }) =>
              `border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`
            }
          >
            {t(tab.label)}
          </NavLink>
        ))}
      </nav>
      <div className="flex-1 min-h-0">
        <Outlet />
      </div>
    </div>
  );
}
