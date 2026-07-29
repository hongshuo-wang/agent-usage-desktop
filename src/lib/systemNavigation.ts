export const SYSTEM_NAV_ITEMS = [
  { path: "/settings/data-sources", label: "dataSources" },
  { path: "/settings/pricing", label: "pricing" },
  { path: "/settings/index-diagnostics", label: "indexDiagnostics" },
  { path: "/settings/preferences", label: "preferences" },
] as const;

export type SystemSection = typeof SYSTEM_NAV_ITEMS[number]["path"] extends `/settings/${infer Section}`
  ? Section
  : never;
