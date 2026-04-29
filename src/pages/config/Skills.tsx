import { openPath as open, revealItemInDir } from "@tauri-apps/plugin-opener";
import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import ConfirmPanel, { type AffectedFile } from "../../components/ConfirmPanel";
import SyncStatus from "../../components/SyncStatus";
import { TOOL_LABELS, TOOLS, type ToolTarget } from "../../components/ToolTargets";
import { ApiError, fetchRaw, mutateAPI } from "../../lib/api";

type SyncMethod = "symlink" | "copy";
type SkillStatus =
  | "using_selected"
  | "missing_variant"
  | "missing_install"
  | "out_of_sync"
  | "unmanaged"
  | "not_installed";

type SkillOverview = {
  library_path: string;
  cli: {
    available: boolean;
    command: string;
    message: string;
  };
  tool_availability?: Partial<Record<ToolTarget, boolean>>;
  summary: {
    managed_skills: number;
    visible_skills: number;
    connected_tools: number;
    issue_count: number;
    unmanaged_skills: number;
  };
  skills: SkillOverviewItem[];
};

type SkillOverviewItem = {
  id: number;
  name: string;
  description: string;
  managed: boolean;
  enabled: boolean;
  primary_path: string;
  library: {
    present: boolean;
    path: string;
    hash: string;
    variant_id: number;
  };
  variants: SkillVariant[];
  tools: Partial<Record<ToolTarget, ToolState>>;
  issues: SkillIssue[];
  discovered: DiscoveredInstall[];
};

type SkillVariant = {
  id: number;
  source_path: string;
  origin_tool: string;
  hash: string;
  managed: boolean;
};

type ToolState = {
  enabled: boolean;
  method: SyncMethod;
  selected_variant_id: number;
  selected_path: string;
  selected_hash: string;
  status: SkillStatus;
  actual: ActualInstall[];
};

type ActualInstall = {
  path: string;
  hash: string;
  method: SyncMethod;
};

type DiscoveredInstall = {
  path: string;
  tool: string;
  hash: string;
  method: SyncMethod;
};

type SkillIssue = {
  tool: string;
  code: string;
};

type MutationResponse = {
  affected_files: AffectedFile[];
};

type SkillsCLIToolStatus = "connected" | "available" | "missing_cli";

type SkillsCLIToolCard = {
  tool: ToolTarget;
  label: string;
  cliAvailable: boolean;
  skillConnected: boolean;
  status: SkillsCLIToolStatus;
};

type ImportManagedSkillResponse = MutationResponse & {
  skill_id: number;
  variant_id: number;
  created_new: boolean;
};

type FilterMode = "all" | "issues" | "managed" | "unmanaged";
type SkillsAPIFlavor = "overview" | "legacy";
type ScopeMode = ToolTarget;

type SkillFormState = {
  name: string;
  description: string;
  sourcePath: string;
};

type LegacySkillTarget = {
  method: SyncMethod;
  enabled: boolean;
  variant_id?: number;
};

type LegacySkill = {
  id: number;
  name: string;
  source_path: string;
  description: string;
  enabled: boolean;
  targets: Partial<Record<ToolTarget, LegacySkillTarget>>;
  created_at: string;
};

type LegacyInstallRef = {
  path: string;
  installed: boolean;
  method: string;
  hash?: string;
};

type LegacyInventoryEntry = {
  name: string;
  description: string;
  path: string;
  tool: string;
  hash: string;
  is_library: boolean;
  is_symlink: boolean;
  importable: boolean;
  represented: boolean;
  conflict: boolean;
  install_status?: Partial<Record<ToolTarget, LegacyInstallRef>>;
};

type LegacyInventory = {
  library_path: string;
  cli: {
    available: boolean;
    command: string;
    message: string;
  };
  tool_availability?: Partial<Record<ToolTarget, boolean>>;
  library: LegacyInventoryEntry[];
  discovered: LegacyInventoryEntry[];
  conflicts: Array<{
    name: string;
    library: LegacyInventoryEntry;
    external: LegacyInventoryEntry;
  }>;
  summary: {
    library_count: number;
    discovered_count: number;
    importable_count: number;
    conflict_count: number;
  };
};

type Translate = (key: string, options?: Record<string, unknown>) => string;

const PRIMARY_BUTTON =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-xl bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const SECONDARY_BUTTON =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-border bg-background px-4 py-2 text-sm text-foreground transition-colors hover:bg-muted motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const GHOST_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border border-border/70 px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const INPUT_CLASS =
  "w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors focus:border-accent focus:ring-4 focus:ring-accent/10";

function selectionKey(skill: SkillOverviewItem) {
  return skill.id > 0 ? `managed:${skill.id}` : `unmanaged:${skill.name.toLowerCase()}`;
}

function createFormState(skill: SkillOverviewItem | null): SkillFormState {
  if (!skill) {
    return { name: "", description: "", sourcePath: "" };
  }
  return {
    name: skill.name,
    description: skill.description ?? "",
    sourcePath: skill.primary_path ?? skill.library.path ?? "",
  };
}

function hashPreview(hash: string) {
  return hash ? hash.slice(0, 10) : "unknown";
}

function pathTail(path: string) {
  const normalized = path.replace(/[\\/]+$/, "");
  const index = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  return index >= 0 ? normalized.slice(index + 1) : normalized;
}

function getFolderPath(path: string) {
  const normalized = path.trim().replace(/[\\/]+$/, "");
  if (!normalized) {
    return "";
  }

  if (normalized === "/") {
    return "/";
  }

  if (/^[A-Za-z]:\\?$/.test(normalized)) {
    return normalized.endsWith("\\") ? normalized : `${normalized}\\`;
  }

  const lastSeparator = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  if (lastSeparator === -1) {
    return "";
  }
  if (lastSeparator === 0) {
    return normalized[0] === "/" ? "/" : "";
  }

  const driveRootMatch = normalized.slice(0, lastSeparator + 1).match(/^[A-Za-z]:\\$/);
  if (driveRootMatch) {
    return driveRootMatch[0];
  }

  return normalized.slice(0, lastSeparator);
}

function getOpenCandidates(path: string) {
  const candidates = [path.trim(), getFolderPath(path)];
  return candidates.filter((candidate, index, all) => candidate && all.indexOf(candidate) === index);
}

function normalizeSkillName(name: string) {
  return name.trim().toLowerCase();
}

function getAgentUsageInstalledAgents(overview: SkillOverview | null | undefined) {
  const skill = (overview?.skills ?? []).find((item) => normalizeSkillName(item.name) === "agent-usage-desktop");
  if (!skill) {
    return [] as ToolTarget[];
  }

  return TOOLS.filter((tool) => {
    const state = skill.tools[tool];
    if (state?.enabled || (state?.actual.length ?? 0) > 0) {
      return true;
    }
    return skill.discovered.some((entry) => entry.tool === tool);
  });
}

function skillsCliManualAgent(tool: ToolTarget) {
  return tool === "claude" ? "claude-code" : tool;
}

function getStatusLabel(t: Translate, status: SkillStatus) {
  switch (status) {
    case "using_selected":
      return t("skillsStatusUsingSelected");
    case "missing_variant":
      return t("skillsStatusMissingVariant");
    case "missing_install":
      return t("skillsStatusMissingInstall");
    case "out_of_sync":
      return t("skillsStatusOutOfSync");
    case "unmanaged":
      return t("skillsStatusUnmanaged");
    default:
      return t("skillsStatusNotInstalled");
  }
}

function getStatusTone(status: SkillStatus) {
  switch (status) {
    case "using_selected":
      return "border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    case "missing_variant":
    case "missing_install":
    case "out_of_sync":
    case "unmanaged":
      return "border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300";
    default:
      return "border-border bg-background text-muted-foreground";
  }
}

function buildTargetsPayload(skill: SkillOverviewItem, updates?: Partial<Record<ToolTarget, Partial<ToolState>>>) {
  return Object.fromEntries(
    TOOLS.map((tool) => {
      const current = skill.tools[tool];
      const next = updates?.[tool];
      return [
        tool,
        {
          enabled: next?.enabled ?? current?.enabled ?? false,
          method: next?.method ?? current?.method ?? "symlink",
          variant_id: next?.selected_variant_id ?? current?.selected_variant_id ?? skill.library.variant_id,
        },
      ];
    })
  );
}

function matchesQuery(skill: SkillOverviewItem, query: string) {
  if (!query.trim()) {
    return true;
  }
  const search = query.trim().toLowerCase();
  const haystack = [
    skill.name,
    skill.description,
    skill.primary_path,
    skill.library.path,
    ...skill.variants.map((variant) => variant.source_path),
  ]
    .join("\n")
    .toLowerCase();
  return haystack.includes(search);
}

function issuesForTool(skill: SkillOverviewItem, tool: ToolTarget) {
  return skill.issues.filter((issue) => issue.tool === tool);
}

function matchesFilter(skill: SkillOverviewItem, filter: FilterMode, scope: ToolTarget) {
  if (filter === "issues") {
    return issuesForTool(skill, scope).length > 0;
  }
  if (filter === "managed") {
    return skill.managed;
  }
  if (filter === "unmanaged") {
    return !skill.managed;
  }
  return true;
}

function matchesScope(skill: SkillOverviewItem, scope: ScopeMode) {
  const state = skill.tools[scope];
  if (state?.enabled || (state?.actual.length ?? 0) > 0) {
    return true;
  }
  if (skill.discovered.some((entry) => entry.tool === scope)) {
    return true;
  }
  return skill.issues.some((issue) => issue.tool === scope);
}

function toolIssueSummary(issues: SkillIssue[]) {
  return issues.reduce<Record<string, string[]>>((groups, issue) => {
    if (!groups[issue.code]) {
      groups[issue.code] = [];
    }
    groups[issue.code].push(issue.tool);
    return groups;
  }, {});
}

function buildToolStateFromLegacy(
  tool: ToolTarget,
  skill: LegacySkill,
  libraryEntry: LegacyInventoryEntry | undefined
): ToolState {
  const target = skill.targets?.[tool];
  const install = libraryEntry?.install_status?.[tool];
  const actual = install?.installed
    ? [
        {
          path: install.path,
          hash: install.hash ?? "",
          method: (install.method === "copy" ? "copy" : "symlink") as SyncMethod,
        },
      ]
    : [];
  let status: SkillStatus = "not_installed";
  if (target?.enabled) {
    if (actual.length === 0) {
      status = "missing_install";
    } else if (libraryEntry?.hash && actual[0].hash && actual[0].hash !== libraryEntry.hash) {
      status = "out_of_sync";
    } else {
      status = "using_selected";
    }
  } else if (actual.length > 0) {
    status = "out_of_sync";
  }

  return {
    enabled: Boolean(target?.enabled),
    method: target?.method === "copy" ? "copy" : "symlink",
    selected_variant_id: target?.variant_id ?? skill.id,
    selected_path: skill.source_path,
    selected_hash: libraryEntry?.hash ?? "",
    status,
    actual,
  };
}

function buildLegacyOverview(skills: LegacySkill[], inventory: LegacyInventory): SkillOverview {
  const libraryByName = new Map<string, LegacyInventoryEntry>();
  for (const entry of inventory.library) {
    libraryByName.set(normalizeSkillName(entry.name), entry);
  }

  const discoveredByName = new Map<string, LegacyInventoryEntry[]>();
  for (const entry of inventory.discovered) {
    const key = normalizeSkillName(entry.name);
    const current = discoveredByName.get(key) ?? [];
    current.push(entry);
    discoveredByName.set(key, current);
  }

  const managedByName = new Map<string, LegacySkill>();
  const items: SkillOverviewItem[] = skills.map((skill) => {
    const key = normalizeSkillName(skill.name);
    managedByName.set(key, skill);
    const libraryEntry = libraryByName.get(key);
    const variants: SkillVariant[] = [
      {
        id: skill.id,
        source_path: skill.source_path,
        origin_tool: "global",
        hash: libraryEntry?.hash ?? "",
        managed: true,
      },
    ];
    const tools = Object.fromEntries(
      TOOLS.map((tool) => [tool, buildToolStateFromLegacy(tool, skill, libraryEntry)])
    ) as Partial<Record<ToolTarget, ToolState>>;
    const issues: SkillIssue[] = TOOLS.flatMap((tool) => {
      const state = tools[tool];
      if (!state?.enabled) {
        return [];
      }
      if (state.status === "missing_install" || state.status === "out_of_sync") {
        return [{ tool, code: state.status }];
      }
      return [];
    });

    return {
      id: skill.id,
      name: skill.name,
      description: skill.description,
      managed: true,
      enabled: skill.enabled,
      primary_path: skill.source_path,
      library: {
        present: Boolean(libraryEntry?.path ?? skill.source_path),
        path: libraryEntry?.path ?? skill.source_path,
        hash: libraryEntry?.hash ?? "",
        variant_id: skill.id,
      },
      variants,
      tools,
      issues,
      discovered: (discoveredByName.get(key) ?? []).map((entry) => ({
        path: entry.path,
        tool: entry.tool,
        hash: entry.hash,
        method: entry.is_symlink ? "symlink" : "copy",
      })),
    };
  });

  for (const [key, entries] of discoveredByName.entries()) {
    if (managedByName.has(key)) {
      continue;
    }
    const primary = entries[0];
    const tools: Partial<Record<ToolTarget, ToolState>> = {};
    for (const entry of entries) {
      if (entry.tool === "global") {
        continue;
      }
      const tool = entry.tool as ToolTarget;
      if (!TOOLS.includes(tool)) {
        continue;
      }
      tools[tool] = {
        enabled: false,
        method: entry.is_symlink ? "symlink" : "copy",
        selected_variant_id: 0,
        selected_path: entry.path,
        selected_hash: entry.hash,
        status: "unmanaged",
        actual: [{ path: entry.path, hash: entry.hash, method: entry.is_symlink ? "symlink" : "copy" }],
      };
    }

    items.push({
      id: 0,
      name: primary.name,
      description: primary.description,
      managed: false,
      enabled: false,
      primary_path: primary.path,
      library: {
        present: false,
        path: inventory.library_path ? `${inventory.library_path}/${pathTail(primary.path)}` : "",
        hash: "",
        variant_id: 0,
      },
      variants: entries.map((entry, index) => ({
        id: -(index + 1),
        source_path: entry.path,
        origin_tool: entry.tool,
        hash: entry.hash,
        managed: false,
      })),
      tools,
      issues: [],
      discovered: entries.map((entry) => ({
        path: entry.path,
        tool: entry.tool,
        hash: entry.hash,
        method: entry.is_symlink ? "symlink" : "copy",
      })),
    });
  }

  const summary = {
    managed_skills: items.filter((item) => item.managed).length,
    visible_skills: items.length,
    connected_tools: items.reduce(
      (count, item) => count + Object.values(item.tools).filter((state) => state?.enabled).length,
      0
    ),
    issue_count: items.reduce((count, item) => count + item.issues.length, 0),
    unmanaged_skills: items.filter((item) => !item.managed).length,
  };

  items.sort((left, right) => {
    if (left.issues.length !== right.issues.length) {
      return right.issues.length - left.issues.length;
    }
    if (left.managed !== right.managed) {
      return left.managed ? -1 : 1;
    }
    return left.name.localeCompare(right.name);
  });

  return {
    library_path: inventory.library_path,
    cli: inventory.cli,
    tool_availability: inventory.tool_availability,
    summary,
    skills: items,
  };
}

function FilterChip({
  active,
  count,
  label,
  onClick,
}: {
  active: boolean;
  count: number;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-full border px-4 py-2 text-sm transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
        active
          ? "border-accent bg-accent text-white"
          : "border-border bg-background text-muted-foreground hover:text-foreground"
      }`}
    >
      <span>{label}</span>
      <span className={`rounded-full px-2 py-0.5 text-xs ${active ? "bg-white/20" : "bg-muted"}`}>{count}</span>
    </button>
  );
}

function ScopeTab({
  scope,
  active,
  count,
  issues,
  onClick,
}: {
  scope: ScopeMode;
  active: boolean;
  count: number;
  issues: number;
  onClick: () => void;
}) {
  const label = TOOL_LABELS[scope];
  const tabId = `skills-tab-${scope}`;
  const panelId = `skills-panel-${scope}`;

  return (
    <button
      id={tabId}
      role="tab"
      aria-selected={active}
      aria-controls={panelId}
      tabIndex={active ? 0 : -1}
      type="button"
      onClick={onClick}
      className={`inline-flex min-h-11 cursor-pointer items-center gap-3 rounded-t-2xl border border-b-0 px-4 py-3 text-left transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
        active
          ? "border-border bg-card text-foreground shadow-[0_-1px_0_0_var(--border)]"
          : "border-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      }`}
    >
      <span className="text-sm font-semibold">{label}</span>
      <span
        className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
          active ? "bg-accent text-white" : "bg-muted text-muted-foreground"
        }`}
      >
        {count}
      </span>
      {issues > 0 ? (
        <span className="rounded-full border border-amber-500/25 bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
          {issues}
        </span>
      ) : null}
    </button>
  );
}

function SkillsCLIStatusChip({
  available,
  toolCards,
  loading,
  t,
}: {
  available?: boolean;
  toolCards: SkillsCLIToolCard[];
  loading: boolean;
  t: Translate;
}) {
  const panelId = useId();
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const cliAvailable = Boolean(available);
  const hasConnectedTools = toolCards.some((card) => card.status === "connected");
  const defaultTool =
    toolCards.find((card) => card.skillConnected)?.tool ??
    toolCards.find((card) => card.cliAvailable)?.tool ??
    toolCards[0]?.tool ??
    "codex";
  const [selectedTool, setSelectedTool] = useState<ToolTarget>(defaultTool);
  const statusLabel = loading
    ? t("loading")
    : cliAvailable
      ? t("skillsCliAvailableState")
      : t("skillsCliUnavailableState");
  const selectedCard = toolCards.find((card) => card.tool === selectedTool) ?? toolCards[0];
  const selectedAgent = skillsCliManualAgent(selectedCard?.tool ?? "codex");
  const installCommand = `npx --yes skills add hongshuo-wang/agent-usage-desktop --global --skill agent-usage-desktop --agent ${selectedAgent} --yes`;
  const uninstallCommand = `npx --yes skills remove agent-usage-desktop --global --agent ${selectedAgent} --yes`;

  useEffect(() => {
    if (toolCards.some((card) => card.tool === selectedTool)) {
      return;
    }
    setSelectedTool(defaultTool);
  }, [defaultTool, selectedTool, toolCards]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const handlePointerDown = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleEscape);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [open]);

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={t("skillsCliPanelToggle")}
        className={`inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
          hasConnectedTools
            ? "border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300"
            : cliAvailable
              ? "border-accent/20 bg-accent/8 text-accent"
              : "border-border bg-background/70 text-muted-foreground"
        }`}
      >
        <span className="font-medium">{t("skillsCliSupportTitle")}</span>
        <span
          className={`whitespace-nowrap rounded-full px-2 py-0.5 text-[11px] font-medium ${
            cliAvailable
              ? "bg-white/15 text-current"
              : "border border-border bg-muted text-muted-foreground"
          }`}
        >
          {statusLabel}
        </span>
      </button>
      {open ? (
        <div
          id={panelId}
          className="absolute left-0 top-full z-20 mt-2 w-[min(32rem,calc(100vw-2rem))] rounded-3xl border border-border bg-card p-4 text-left shadow-lg"
        >
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="text-sm font-semibold text-foreground">{t("skillsCliSupportTitle")}</div>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                {cliAvailable ? t("skillsCliPanelBody") : t("skillsCliPanelUnavailableBody")}
              </p>
            </div>
            <span
              className={`shrink-0 whitespace-nowrap rounded-full px-3 py-1 text-[11px] font-medium ${
                cliAvailable
                  ? "border border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                  : "border border-border bg-muted text-muted-foreground"
              }`}
            >
              {statusLabel}
            </span>
          </div>

          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            {toolCards.map((card) => {
              const statusTone =
                card.status === "connected"
                  ? "border-emerald-500/20 bg-emerald-500/8"
                  : card.status === "available"
                    ? "border-amber-500/20 bg-amber-500/8"
                    : "border-border/70 bg-background";
              const statusText =
                card.status === "connected"
                  ? t("skillsCliPerToolConnected")
                  : card.status === "available"
                    ? t("skillsCliPerToolAvailable")
                    : t("skillsCliPerToolMissing");
              const selected = selectedCard?.tool === card.tool;

              return (
                <button
                  key={card.tool}
                  type="button"
                  onClick={() => setSelectedTool(card.tool)}
                  className={`rounded-2xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
                    selected ? "border-accent bg-accent/6 shadow-sm" : statusTone
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-foreground">{card.label}</div>
                      <div className="mt-1 text-xs text-muted-foreground">{statusText}</div>
                    </div>
                    <span
                      className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${
                        card.status === "connected"
                          ? "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300"
                          : card.status === "available"
                            ? "bg-amber-500/12 text-amber-700 dark:text-amber-300"
                            : "bg-muted text-muted-foreground"
                      }`}
                    >
                      {selected ? t("skillsCliSelected") : statusText}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>

          <div className="mt-4 rounded-2xl border border-border/70 bg-background px-3 py-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div className="text-sm font-medium text-foreground">{t("skillsCliManualCommandLabel")}</div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t("skillsCliManualCommandDescription", { tool: selectedCard?.label ?? TOOL_LABELS.codex })}
                </p>
              </div>
              <span className="rounded-full border border-border bg-card px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                {selectedCard?.label ?? TOOL_LABELS.codex}
              </span>
            </div>
            <div className="mt-3 space-y-2">
              <div>
                <div className="mb-1 text-xs font-medium text-muted-foreground">{t("skillsCliInstallCommandLabel")}</div>
                <div className="rounded-xl border border-border/70 bg-card px-3 py-2 font-mono text-xs text-foreground">
                  {installCommand}
                </div>
              </div>
              <div>
                <div className="mb-1 text-xs font-medium text-muted-foreground">{t("skillsCliUninstallCommandLabel")}</div>
                <div className="rounded-xl border border-border/70 bg-card px-3 py-2 font-mono text-xs text-foreground">
                  {uninstallCommand}
                </div>
              </div>
              {!selectedCard?.cliAvailable ? (
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("skillsCliManualCommandMissingHint", { tool: selectedCard?.label ?? TOOL_LABELS.codex })}
                </p>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function ModalShell({
  title,
  subtitle,
  onClose,
  children,
  widthClass = "max-w-5xl",
}: {
  title: string;
  subtitle?: string;
  onClose: () => void;
  children: ReactNode;
  widthClass?: string;
}) {
  const { t } = useTranslation();
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    panelRef.current?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      previousFocusRef.current?.focus();
    };
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className={`max-h-[90vh] w-full ${widthClass} overflow-hidden rounded-[28px] border border-border bg-card shadow-2xl focus:outline-none`}
      >
        <div className="flex items-start justify-between gap-4 border-b border-border px-6 py-5">
          <div className="min-w-0">
            <h2 id={titleId} className="text-2xl font-semibold tracking-tight text-foreground">
              {title}
            </h2>
            {subtitle ? <p className="mt-2 text-sm leading-6 text-muted-foreground">{subtitle}</p> : null}
          </div>
          <button type="button" onClick={onClose} className={GHOST_BUTTON}>
            {t("cancel")}
          </button>
        </div>
        <div className="max-h-[calc(90vh-92px)] overflow-y-auto px-6 py-6">{children}</div>
      </div>
    </div>
  );
}

function ToolConfigCard({
  skill,
  tool,
  state,
  emphasized = false,
  mutatingTool,
  apiFlavor,
  t,
  onOpenFolder,
  onMutateToolTarget,
  onImportManaged,
}: {
  skill: SkillOverviewItem;
  tool: ToolTarget;
  state?: ToolState;
  emphasized?: boolean;
  mutatingTool: string | null;
  apiFlavor: SkillsAPIFlavor;
  t: Translate;
  onOpenFolder: (path: string) => Promise<void>;
  onMutateToolTarget: (tool: ToolTarget, updates: Partial<ToolState>, successMessage: string) => Promise<void>;
  onImportManaged: (skill: SkillOverviewItem, sourcePath: string, tool: ToolTarget | "global") => Promise<void>;
}) {
  const canSelectVariants = skill.managed && skill.variants.length > 0;
  const status = state?.status ?? "not_installed";

  return (
    <div
      className={`rounded-[24px] border p-4 ${
        emphasized ? "border-accent/35 bg-accent/6" : "border-border bg-background/70"
      }`}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="text-base font-semibold text-foreground">{TOOL_LABELS[tool]}</div>
          <div className="flex flex-wrap items-center gap-2">
            <span className={`inline-flex rounded-full border px-2.5 py-1 text-[11px] font-medium ${getStatusTone(status)}`}>
              {getStatusLabel(t, status)}
            </span>
            {state?.enabled ? (
              <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                {state.method}
              </span>
            ) : null}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() =>
              void onMutateToolTarget(
                tool,
                {
                  enabled: !(state?.enabled ?? false),
                  selected_variant_id:
                    state?.selected_variant_id || skill.library.variant_id || skill.variants[0]?.id || 0,
                },
                state?.enabled ? t("skillsDisconnected") : t("skillsConnected")
              )
            }
            disabled={!skill.managed || mutatingTool === tool || !canSelectVariants}
            className={SECONDARY_BUTTON}
          >
            {state?.enabled ? t("disconnect") : t("connect")}
          </button>
          {state?.actual[0]?.path ? (
            <button
              type="button"
              onClick={() => void onOpenFolder(state.actual[0].path)}
              className={GHOST_BUTTON}
            >
              {t("skillsOpenInstalled")}
            </button>
          ) : null}
        </div>
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <div className="rounded-2xl border border-border/70 bg-card px-3 py-3">
          <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            {t("skillsExpectedPath")}
          </div>
          <div className="mt-2 break-all text-sm text-foreground">
            {state?.selected_path || skill.library.path || skill.primary_path || "-"}
          </div>
        </div>
        <div className="rounded-2xl border border-border/70 bg-card px-3 py-3">
          <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            {t("skillsInstalledPath")}
          </div>
          <div className="mt-2 break-all text-sm text-foreground">{state?.actual[0]?.path || "-"}</div>
        </div>
      </div>

      {skill.managed ? (
        <div className="mt-4 space-y-3">
          <div className="flex flex-wrap gap-2">
            {(["symlink", "copy"] as SyncMethod[]).map((method) => (
              <button
                key={method}
                type="button"
                onClick={() => void onMutateToolTarget(tool, { method }, t("skillsMethodUpdated"))}
                disabled={mutatingTool === tool}
                className={`rounded-full px-3 py-1.5 text-xs font-medium transition-colors ${
                  (state?.method ?? "symlink") === method
                    ? "bg-accent text-white"
                    : "border border-border text-muted-foreground hover:text-foreground"
                }`}
              >
                {method}
              </button>
            ))}
          </div>
          <div className="grid gap-2">
            {skill.variants.map((variant) => {
              const active = state?.selected_variant_id === variant.id && state?.enabled;
              return (
                <button
                  key={`${tool}-${variant.id}`}
                  type="button"
                  onClick={() =>
                    void onMutateToolTarget(
                      tool,
                      { enabled: true, selected_variant_id: variant.id },
                      t("skillsVariantApplied")
                    )
                  }
                  disabled={mutatingTool === tool}
                  className={`flex min-h-11 cursor-pointer items-center justify-between rounded-xl border px-3 py-2 text-left text-sm transition-colors ${
                    active
                      ? "border-accent bg-accent/8 text-foreground"
                      : "border-border bg-background hover:bg-muted"
                  }`}
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{pathTail(variant.source_path)}</span>
                    <span className="block text-xs text-muted-foreground">hash {hashPreview(variant.hash)}</span>
                  </span>
                  {active ? (
                    <span className="rounded-full border border-accent/20 bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent">
                      {t("skillsCurrentSelection")}
                    </span>
                  ) : null}
                </button>
              );
            })}
          </div>
        </div>
      ) : null}

      {!skill.managed && state?.actual[0]?.path && apiFlavor === "overview" ? (
        <div className="mt-4">
          <button
            type="button"
            onClick={() => void onImportManaged(skill, state.actual[0].path, tool)}
            disabled={mutatingTool === tool}
            className={PRIMARY_BUTTON}
          >
            {t("skillsAdoptFromTool", { tool: TOOL_LABELS[tool] })}
          </button>
        </div>
      ) : null}
    </div>
  );
}

export default function SkillsPage() {
  const { t } = useTranslation();
  const scopeOrder: ScopeMode[] = [...TOOLS];
  const [overview, setOverview] = useState<SkillOverview | null>(null);
  const [apiFlavor, setApiFlavor] = useState<SkillsAPIFlavor>("overview");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [mutatingTool, setMutatingTool] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [scope, setScope] = useState<ScopeMode>("claude");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<FilterMode>("all");
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [form, setForm] = useState<SkillFormState>(createFormState(null));
  const [pendingDelete, setPendingDelete] = useState<SkillOverviewItem | null>(null);
  const [creating, setCreating] = useState(false);
  const loadOverview = async (preferredKey?: string) => {
    setLoading(true);
    setError(null);
    try {
      let next: SkillOverview;
      try {
        next = await fetchRaw<SkillOverview>("config/skills/overview");
        setApiFlavor("overview");
      } catch (err) {
        if (!(err instanceof ApiError) || (err.status !== 404 && err.status !== 405)) {
          throw err;
        }
        const [skills, inventory] = await Promise.all([
          fetchRaw<LegacySkill[]>("config/skills"),
          fetchRaw<LegacyInventory>("config/skills/inventory"),
        ]);
        next = buildLegacyOverview(skills, inventory);
        setApiFlavor("legacy");
      }

      setOverview(next);
      setSelectedKey((current) => {
        const desired = preferredKey ?? current;
        if (desired && next.skills.some((skill) => selectionKey(skill) === desired)) {
          return desired;
        }
        return current ?? null;
      });
      return next;
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsLoadFailed"));
      return null;
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadOverview();
  }, []);

  const visibleSkills = useMemo(() => {
    return (overview?.skills ?? []).filter(
      (skill) => matchesQuery(skill, query) && matchesFilter(skill, filter, scope) && matchesScope(skill, scope)
    );
  }, [filter, overview, query, scope]);

  const selectedSkill = useMemo(() => {
    return overview?.skills.find((skill) => selectionKey(skill) === selectedKey) ?? null;
  }, [overview, selectedKey]);

  useEffect(() => {
    setForm(createFormState(selectedSkill));
    if (selectedSkill) {
      setCreating(false);
    }
  }, [selectedSkill]);

  const hasUnsavedChanges =
    selectedSkill?.managed &&
    (form.name !== selectedSkill.name ||
      form.description !== (selectedSkill.description ?? "") ||
      form.sourcePath !== (selectedSkill.primary_path ?? ""));

  const scopedSkills = useMemo(() => {
    return (overview?.skills ?? []).filter((skill) => matchesScope(skill, scope));
  }, [overview, scope]);

  const filteredCounts = useMemo(() => {
    const skills = scopedSkills.filter((skill) => matchesQuery(skill, query));
    return {
      all: skills.length,
      issues: skills.filter((skill) => issuesForTool(skill, scope).length > 0).length,
      managed: skills.filter((skill) => skill.managed).length,
      unmanaged: skills.filter((skill) => !skill.managed).length,
    };
  }, [query, scopedSkills, scope]);

  const scopeCounts = useMemo(() => {
    const skills = overview?.skills ?? [];
    return {
      claude: skills.filter((skill) => matchesScope(skill, "claude")).length,
      codex: skills.filter((skill) => matchesScope(skill, "codex")).length,
      opencode: skills.filter((skill) => matchesScope(skill, "opencode")).length,
      openclaw: skills.filter((skill) => matchesScope(skill, "openclaw")).length,
    } satisfies Record<ToolTarget, number>;
  }, [overview]);

  const scopeIssueCounts = useMemo(() => {
    const skills = overview?.skills ?? [];
    return {
      claude: skills.filter((skill) => skill.issues.some((issue) => issue.tool === "claude")).length,
      codex: skills.filter((skill) => skill.issues.some((issue) => issue.tool === "codex")).length,
      opencode: skills.filter((skill) => skill.issues.some((issue) => issue.tool === "opencode")).length,
      openclaw: skills.filter((skill) => skill.issues.some((issue) => issue.tool === "openclaw")).length,
    } satisfies Record<ToolTarget, number>;
  }, [overview]);

  const agentUsageInstalledAgents = useMemo(() => getAgentUsageInstalledAgents(overview), [overview]);

  const cliToolCards = useMemo(() => {
    return TOOLS.map((tool) => {
      const cliAvailable = overview?.tool_availability?.[tool] ?? false;
      const skillConnected = agentUsageInstalledAgents.includes(tool);
      const status: SkillsCLIToolStatus = !cliAvailable ? "missing_cli" : skillConnected ? "connected" : "available";
      return {
        tool,
        label: TOOL_LABELS[tool],
        cliAvailable,
        skillConnected,
        status,
      };
    });
  }, [agentUsageInstalledAgents, overview]);

  const saveSkill = async () => {
    if (!selectedSkill?.managed || selectedSkill.id <= 0) {
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await mutateAPI<MutationResponse>("PUT", `config/skills/${selectedSkill.id}`, {
        name: form.name.trim(),
        source_path: form.sourcePath.trim(),
        description: form.description.trim(),
        enabled: selectedSkill.enabled,
      });
      setMessage(t("skillsSaved"));
      await loadOverview(`managed:${selectedSkill.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsSaveFailed"));
    } finally {
      setSaving(false);
    }
  };

  const mutateToolTarget = async (
    tool: ToolTarget,
    updates: Partial<ToolState>,
    successMessage: string
  ) => {
    if (!selectedSkill?.managed || selectedSkill.id <= 0) {
      return;
    }
    setMutatingTool(tool);
    setError(null);
    setMessage(null);
    try {
      await mutateAPI<MutationResponse>("PUT", `config/skills/${selectedSkill.id}/targets`, {
        targets: buildTargetsPayload(selectedSkill, { [tool]: updates }),
      });
      setMessage(successMessage);
      await loadOverview(`managed:${selectedSkill.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutatingTool(null);
    }
  };

  const importManaged = async (
    skill: SkillOverviewItem,
    sourcePath: string,
    tool: ToolTarget | "global"
  ) => {
    if (apiFlavor !== "overview") {
      setError("Current sidecar does not support importing discovered skills yet. Restart the desktop app to load the newer backend.");
      return;
    }
    setMutatingTool(tool);
    setError(null);
    setMessage(null);
    try {
      const response = await mutateAPI<ImportManagedSkillResponse>("POST", "config/skills/import-managed", {
        skill_id: skill.id > 0 ? skill.id : 0,
        name: skill.name,
        tool,
        source_path: sourcePath,
      });
      setMessage(response.created_new ? t("skillsImportedNew") : t("skillsImported"));
      await loadOverview(`managed:${response.skill_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutatingTool(null);
    }
  };

  const deleteSkill = async () => {
    if (!pendingDelete?.managed || pendingDelete.id <= 0) {
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await mutateAPI<MutationResponse>("DELETE", `config/skills/${pendingDelete.id}`);
      setPendingDelete(null);
      setSelectedKey(null);
      setMessage(t("skillsDeleted"));
      await loadOverview();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsDeleteFailed"));
      setSaving(false);
    } finally {
      setSaving(false);
    }
  };

  const createSkill = async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await mutateAPI<{ id: number; affected_files: AffectedFile[] }>("POST", "config/skills", {
        name: form.name.trim(),
        source_path: form.sourcePath.trim(),
        description: form.description.trim(),
        targets: {},
      });
      setCreating(false);
      setMessage(t("skillsCreated"));
      await loadOverview(`managed:${response.id}`);
      setSelectedKey(`managed:${response.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsCreateFailed"));
    } finally {
      setSaving(false);
    }
  };

  const openFolder = async (path: string) => {
    const candidates = getOpenCandidates(path);
    if (candidates.length === 0) {
      return;
    }

    setError(null);

    for (const candidate of candidates) {
      try {
        await open(candidate);
        return;
      } catch {
        // Fall through to reveal fallback below.
      }
    }

    try {
      await revealItemInDir(candidates[0]);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsOpenFailed"));
    }
  };

  const activeScopeTitle = TOOL_LABELS[scope];
  const activeScopeDescription = t("skillsScopeToolDescription", { tool: TOOL_LABELS[scope] });
  const activePanelId = `skills-panel-${scope}`;
  const activeTabId = `skills-tab-${scope}`;
  const selectedScopeIssues = selectedSkill ? issuesForTool(selectedSkill, scope) : [];

  return (
    <div className="h-full overflow-y-auto pr-1">
      <div className="flex flex-col gap-4 pb-6">
        <section className="rounded-[24px] border border-border bg-card px-5 py-4">
          <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
            <div className="min-w-0 space-y-2">
              <div className="flex flex-wrap items-center gap-3">
                <span className="rounded-full border border-border bg-background px-3 py-1 text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                  {t("skillsPageBadge")}
                </span>
                <h1 className="text-2xl font-semibold tracking-tight text-foreground">{t("skills")}</h1>
                <SyncStatus />
                <SkillsCLIStatusChip
                  available={overview?.cli.available}
                  toolCards={cliToolCards}
                  loading={loading && !overview}
                  t={t}
                />
              </div>
              <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{t("skillsOverviewDescription")}</p>
            </div>

            <div className="flex flex-wrap gap-3">
              <button
                type="button"
                onClick={() => {
                  setCreating(true);
                  setSelectedKey(null);
                  setForm(createFormState(null));
                }}
                className={SECONDARY_BUTTON}
              >
                {t("skillsCreateNew")}
              </button>
              <button type="button" onClick={() => void loadOverview(selectedKey ?? undefined)} className={SECONDARY_BUTTON}>
                {t("refresh")}
              </button>
              {overview?.library_path ? (
                <button type="button" onClick={() => void openFolder(overview.library_path)} className={GHOST_BUTTON}>
                  {t("skillsOpenLibrary")}
                </button>
              ) : null}
            </div>
          </div>
        </section>

        {error ? (
          <div className="rounded-2xl border border-red-500/25 bg-red-500/8 px-4 py-3 text-sm text-red-600 dark:text-red-300">
            {error}
          </div>
        ) : null}
        {message ? (
          <div className="rounded-2xl border border-emerald-500/25 bg-emerald-500/8 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-300">
            {message}
          </div>
        ) : null}

        <section className="rounded-[24px] border border-border bg-card p-5">
          <div
            role="tablist"
            aria-label={t("skillsCurrentCli")}
            className="overflow-x-auto"
            onKeyDown={(event) => {
              const currentIndex = scopeOrder.indexOf(scope);
              if (currentIndex < 0) {
                return;
              }
              const focusTab = (nextScope: ScopeMode) => {
                setScope(nextScope);
                requestAnimationFrame(() => {
                  document.getElementById(`skills-tab-${nextScope}`)?.focus();
                });
              };

              if (event.key === "ArrowRight") {
                event.preventDefault();
                focusTab(scopeOrder[(currentIndex + 1) % scopeOrder.length]);
              } else if (event.key === "ArrowLeft") {
                event.preventDefault();
                focusTab(scopeOrder[(currentIndex - 1 + scopeOrder.length) % scopeOrder.length]);
              } else if (event.key === "Home") {
                event.preventDefault();
                focusTab(scopeOrder[0]);
              } else if (event.key === "End") {
                event.preventDefault();
                focusTab(scopeOrder[scopeOrder.length - 1]);
              }
            }}
          >
            <div className="flex min-w-max items-end gap-2 border-b border-border">
              {scopeOrder.map((item) => (
                <ScopeTab
                  key={item}
                  scope={item}
                  active={scope === item}
                  count={scopeCounts[item]}
                  issues={scopeIssueCounts[item]}
                  onClick={() => setScope(item)}
                />
              ))}
            </div>
          </div>

          <div id={activePanelId} role="tabpanel" aria-labelledby={activeTabId} className="pt-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div className="space-y-1">
                <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                  {t("skillsCurrentCli")}
                </div>
                <h2 className="text-xl font-semibold text-foreground">{activeScopeTitle}</h2>
                <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{activeScopeDescription}</p>
              </div>

              <div className="flex w-full flex-col gap-3 lg:max-w-2xl">
                <input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("skillsSearchPlaceholderNew")}
                  className={INPUT_CLASS}
                />
                <div className="flex flex-wrap items-center gap-2">
                  <FilterChip
                    active={filter === "all"}
                    count={filteredCounts.all}
                    label={t("all")}
                    onClick={() => setFilter("all")}
                  />
                  <FilterChip
                    active={filter === "issues"}
                    count={filteredCounts.issues}
                    label={t("skillsFilterIssues")}
                    onClick={() => setFilter("issues")}
                  />
                  <FilterChip
                    active={filter === "managed"}
                    count={filteredCounts.managed}
                    label={t("skillsFilterManaged")}
                    onClick={() => setFilter("managed")}
                  />
                  <FilterChip
                    active={filter === "unmanaged"}
                    count={filteredCounts.unmanaged}
                    label={t("skillsFilterUnmanaged")}
                    onClick={() => setFilter("unmanaged")}
                  />
                  <span className="ml-auto text-sm text-muted-foreground">
                    {t("skillsVisibleCount", { count: visibleSkills.length })}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5">
              {loading ? (
                <div className="rounded-[24px] border border-border bg-background px-6 py-10 text-sm text-muted-foreground">
                  {t("loading")}
                </div>
              ) : visibleSkills.length === 0 ? (
                <div className="rounded-[24px] border border-dashed border-border bg-background px-6 py-10 text-center">
                  <h3 className="text-base font-semibold text-foreground">{t("skillsEmptyTitle")}</h3>
                  <p className="mt-2 text-sm leading-7 text-muted-foreground">{t("skillsEmptyDescription")}</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {visibleSkills.map((skill) => {
                    const activeState = skill.tools[scope];
                    const scopeIssues = issuesForTool(skill, scope);
                    const activeVariant =
                      skill.variants.find((variant) => variant.id === activeState?.selected_variant_id) ?? null;
                    const currentPath =
                      activeState?.actual[0]?.path ||
                      activeState?.selected_path ||
                      skill.library.path ||
                      skill.primary_path;

                    return (
                      <article
                        key={selectionKey(skill)}
                        className="rounded-[22px] border border-border bg-background p-4 transition-colors hover:border-accent/20 hover:bg-muted/20"
                      >
                        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                          <div className="min-w-0 flex-1 space-y-3">
                            <div className="flex flex-wrap items-center gap-2">
                              <h3 className="text-base font-semibold text-foreground">{skill.name}</h3>
                              <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                {skill.managed ? t("skillsManagedLabel") : t("skillsUnmanagedLabel")}
                              </span>
                              <span
                                className={`inline-flex rounded-full border px-2.5 py-1 text-[11px] font-medium ${getStatusTone(
                                  activeState?.status ?? "not_installed"
                                )}`}
                              >
                                {getStatusLabel(t, activeState?.status ?? "not_installed")}
                              </span>
                              {scopeIssues.length > 0 ? (
                                <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                                  {t("skillsIssuesBadge", { count: scopeIssues.length })}
                                </span>
                              ) : null}
                            </div>
                            <p className="text-sm leading-7 text-muted-foreground">
                              {skill.description || t("skillsNoDescription")}
                            </p>

                            <div className="flex flex-wrap gap-2">
                              {activeState?.enabled ? (
                                <span className="inline-flex rounded-xl bg-muted px-3 py-1 text-[11px] font-medium text-muted-foreground">
                                  {activeState.method}
                                </span>
                              ) : null}
                              {activeVariant ? (
                                <span className="inline-flex rounded-xl bg-muted px-3 py-1 text-[11px] font-medium text-muted-foreground">
                                  {pathTail(activeVariant.source_path)}
                                </span>
                              ) : null}
                              <span className="inline-flex rounded-xl bg-muted px-3 py-1 text-[11px] font-medium text-muted-foreground">
                                {skill.library.present ? t("skillsInLibrary") : t("skillsNotInLibrary")}
                              </span>
                            </div>

                            <div className="rounded-2xl border border-border/70 bg-card px-3 py-3">
                              <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                {activeState?.actual[0]?.path ? t("skillsInstalledPath") : t("skillsExpectedPath")}
                              </div>
                              <div className="mt-1 break-all text-sm text-foreground">{currentPath || "-"}</div>
                            </div>
                          </div>

                          <div className="flex shrink-0 flex-wrap items-center gap-2 xl:justify-end">
                            {currentPath ? (
                              <button type="button" onClick={() => void openFolder(currentPath)} className={GHOST_BUTTON}>
                                {t("skillsOpenFolder")}
                              </button>
                            ) : null}
                            <button
                              type="button"
                              onClick={() => setSelectedKey(selectionKey(skill))}
                              className={PRIMARY_BUTTON}
                            >
                              {t("skillsViewConfig")}
                            </button>
                          </div>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </section>

        {creating ? (
          <ModalShell
            title={t("skillsCreateTitle")}
            subtitle={t("skillsCreateDescription")}
            onClose={() => {
              setCreating(false);
              setForm(createFormState(null));
            }}
            widthClass="max-w-2xl"
          >
            <div className="space-y-5">
              <div className="space-y-4 rounded-[24px] border border-border bg-background/70 p-4">
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">{t("skillName")}</label>
                  <input
                    value={form.name}
                    onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">{t("description")}</label>
                  <textarea
                    value={form.description}
                    onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                    rows={4}
                    className={`${INPUT_CLASS} resize-y`}
                  />
                </div>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">{t("sourcePath")}</label>
                  <input
                    value={form.sourcePath}
                    onChange={(event) => setForm((current) => ({ ...current, sourcePath: event.target.value }))}
                    className={INPUT_CLASS}
                  />
                </div>
              </div>
              <div className="flex flex-wrap justify-end gap-3">
                <button
                  type="button"
                  onClick={() => {
                    setCreating(false);
                    setForm(createFormState(null));
                  }}
                  className={SECONDARY_BUTTON}
                >
                  {t("cancel")}
                </button>
                <button
                  type="button"
                  onClick={() => void createSkill()}
                  disabled={!form.name.trim() || !form.sourcePath.trim() || saving}
                  className={PRIMARY_BUTTON}
                >
                  {saving ? t("loading") : t("skillsCreateNew")}
                </button>
              </div>
            </div>
          </ModalShell>
        ) : null}

        {selectedSkill ? (
          <ModalShell
            title={selectedSkill.name}
            subtitle={selectedSkill.description || t("skillsNoDescription")}
            onClose={() => setSelectedKey(null)}
          >
            <div className="space-y-6">
              <div className="flex flex-wrap items-center gap-2">
                <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                  {selectedSkill.managed ? t("skillsManagedLabel") : t("skillsUnmanagedLabel")}
                </span>
                <span className="rounded-full border border-accent/20 bg-accent/8 px-2.5 py-1 text-[11px] font-medium text-accent">
                  {t("skillsCurrentCli")}: {TOOL_LABELS[scope]}
                </span>
                {selectedScopeIssues.length > 0 ? (
                  <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                    {t("skillsIssuesBadge", { count: selectedScopeIssues.length })}
                  </span>
                ) : null}
              </div>

              <div className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
                <div className="space-y-4">
                  <div className="space-y-1">
                    <h3 className="text-base font-semibold text-foreground">
                      {t("skillsCurrentToolSection", { tool: TOOL_LABELS[scope] })}
                    </h3>
                    <p className="text-sm leading-6 text-muted-foreground">
                      {t("skillsCurrentToolDescription", { tool: TOOL_LABELS[scope] })}
                    </p>
                  </div>
                  <ToolConfigCard
                    skill={selectedSkill}
                    tool={scope}
                    state={selectedSkill.tools[scope]}
                    emphasized
                    mutatingTool={mutatingTool}
                    apiFlavor={apiFlavor}
                    t={t}
                    onOpenFolder={openFolder}
                    onMutateToolTarget={mutateToolTarget}
                    onImportManaged={importManaged}
                  />

                  {selectedScopeIssues.length > 0 ? (
                    <div className="space-y-3 rounded-[24px] border border-amber-500/30 bg-amber-500/8 p-4">
                      <h3 className="text-base font-semibold text-foreground">{t("skillsIssuesTitle")}</h3>
                      <div className="space-y-2">
                        {Object.entries(toolIssueSummary(selectedScopeIssues)).map(([code, tools]) => (
                          <div key={code} className="rounded-xl border border-amber-500/20 bg-background/80 px-3 py-2">
                            <div className="text-sm font-medium text-foreground">{getStatusLabel(t, code as SkillStatus)}</div>
                            <div className="mt-1 text-xs text-muted-foreground">
                              {tools.map((tool) => TOOL_LABELS[tool as ToolTarget] ?? tool).join(" / ")}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>

                <div className="space-y-4">
                  <div className="rounded-[24px] border border-border bg-background/70 p-4">
                    <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                      {t("skillsLibraryColumn")}
                    </div>
                    <div className="mt-3 break-all text-sm font-medium text-foreground">
                      {selectedSkill.library.path || t("skillsNotInLibrary")}
                    </div>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {t("skillsModalFocusedDescription", { tool: TOOL_LABELS[scope] })}
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {selectedSkill.library.path ? (
                        <button
                          type="button"
                          onClick={() => void openFolder(selectedSkill.library.path)}
                          className={GHOST_BUTTON}
                        >
                          {t("skillsOpenFolder")}
                        </button>
                      ) : null}
                      {!selectedSkill.managed && selectedSkill.library.path && apiFlavor === "overview" ? (
                        <button
                          type="button"
                          onClick={() => void importManaged(selectedSkill, selectedSkill.library.path, "global")}
                          disabled={mutatingTool === "global"}
                          className={PRIMARY_BUTTON}
                        >
                          {t("skillsAdoptLibrary")}
                        </button>
                      ) : null}
                    </div>
                  </div>

                  {selectedSkill.managed ? (
                    <div className="space-y-4 rounded-[24px] border border-border bg-background/70 p-4">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="space-y-1">
                          <h3 className="text-base font-semibold text-foreground">{t("skillsEditSection")}</h3>
                          <p className="text-sm leading-6 text-muted-foreground">{t("skillsEditDescription")}</p>
                        </div>
                        <button
                          type="button"
                          onClick={() => setPendingDelete(selectedSkill)}
                          className="inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border border-red-500/30 px-3 py-2 text-sm text-red-500 transition-colors hover:bg-red-500/10"
                        >
                          {t("delete")}
                        </button>
                      </div>
                      <div className="grid gap-4">
                        <div>
                          <label className="mb-1.5 block text-sm font-medium text-foreground">{t("skillName")}</label>
                          <input
                            value={form.name}
                            onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                            className={INPUT_CLASS}
                          />
                        </div>
                        <div>
                          <label className="mb-1.5 block text-sm font-medium text-foreground">{t("sourcePath")}</label>
                          <input
                            value={form.sourcePath}
                            onChange={(event) => setForm((current) => ({ ...current, sourcePath: event.target.value }))}
                            className={INPUT_CLASS}
                          />
                        </div>
                        <div>
                          <label className="mb-1.5 block text-sm font-medium text-foreground">{t("description")}</label>
                          <textarea
                            value={form.description}
                            onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                            rows={4}
                            className={`${INPUT_CLASS} resize-y`}
                          />
                        </div>
                      </div>
                      <div className="flex flex-wrap justify-end gap-3">
                        <button
                          type="button"
                          onClick={() => setForm(createFormState(selectedSkill))}
                          disabled={!hasUnsavedChanges || saving}
                          className={SECONDARY_BUTTON}
                        >
                          {t("resetChanges")}
                        </button>
                        <button
                          type="button"
                          onClick={() => void saveSkill()}
                          disabled={!hasUnsavedChanges || saving}
                          className={PRIMARY_BUTTON}
                        >
                          {saving ? t("loading") : t("save")}
                        </button>
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>

              {selectedSkill.variants.length > 0 ? (
                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <h3 className="text-base font-semibold text-foreground">{t("skillsVariantSection")}</h3>
                    <span className="text-sm text-muted-foreground">{selectedSkill.variants.length}</span>
                  </div>
                  <div className="grid gap-3">
                    {selectedSkill.variants.map((variant) => (
                      <div key={`${variant.id}-${variant.source_path}`} className="rounded-[22px] border border-border bg-background/70 p-4">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="rounded-full border border-border px-2 py-0.5 text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
                                {variant.managed ? t("skillsManagedVariant") : t("skillsDiscoveredVariant")}
                              </span>
                              <span className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
                                {variant.origin_tool || "global"}
                              </span>
                            </div>
                            <div className="mt-2 break-all text-sm font-medium text-foreground">{variant.source_path}</div>
                            <div className="mt-1 text-xs text-muted-foreground">hash {hashPreview(variant.hash)}</div>
                          </div>
                          <button
                            type="button"
                            onClick={() => void openFolder(variant.source_path)}
                            className={GHOST_BUTTON}
                          >
                            {t("skillsOpenFolder")}
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}

              {!selectedSkill.managed && selectedSkill.discovered.length > 0 ? (
                <div className="space-y-3 rounded-[24px] border border-border bg-background/70 p-4">
                  <h3 className="text-base font-semibold text-foreground">{t("skillsDiscoveredSection")}</h3>
                  <div className="space-y-2">
                    {selectedSkill.discovered.map((entry) => (
                      <div
                        key={`${entry.tool}-${entry.path}`}
                        className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/70 bg-background px-3 py-2"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium text-foreground">
                            {entry.tool === "global" ? t("skillsLibraryColumn") : TOOL_LABELS[entry.tool as ToolTarget] ?? entry.tool}
                          </div>
                          <div className="mt-1 break-all text-xs text-muted-foreground">{entry.path}</div>
                        </div>
                        <button
                          type="button"
                          onClick={() => void openFolder(entry.path)}
                          className={GHOST_BUTTON}
                        >
                          {t("skillsOpenFolder")}
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          </ModalShell>
        ) : null}

        {pendingDelete ? (
          <ConfirmPanel
            title={t("skillsDeleteConfirmTitle")}
            affectedFiles={[
              {
                path: pendingDelete.primary_path || pendingDelete.library.path || pendingDelete.name,
                tool: "global",
                operation: "delete",
              },
            ]}
            confirmLabel={t("delete")}
            loading={saving}
            onCancel={() => setPendingDelete(null)}
            onConfirm={() => void deleteSkill()}
          />
        ) : null}
      </div>
    </div>
  );
}
