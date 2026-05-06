import { openPath as open, revealItemInDir } from "@tauri-apps/plugin-opener";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { TOOL_LABELS, TOOLS, type ToolTarget } from "../../components/ToolTargets";
import { fetchRaw, mutateAPI } from "../../lib/api";

type SkillSourceType = "repo" | "imported_tool" | "imported_local" | "manual";
type SkillStatus = "healthy" | "needs_sync" | "update_available" | "broken";
type SkillPrimaryAction = "none" | "import_into_library" | "sync_distribution" | "update_from_source" | "repair";
type SkillComparisonState = "synced" | "different" | "missing" | "invalid" | "disabled";

type AffectedFile = {
  path: string;
  tool: string;
  operation: string;
  diff?: string;
};

type SkillOverviewActualInstall = {
  path: string;
  hash: string;
  method: string;
  valid: boolean;
  problem_reason: string;
};

type SkillOverviewVariant = {
  id: number;
  source_path: string;
  origin_tool: string;
  hash: string;
};

type SkillSourceView = {
  type: SkillSourceType;
  label: string;
  repo_owner?: string;
  repo_name?: string;
  repo_branch?: string;
  repo_subpath?: string;
  origin_tool?: string;
  url?: string;
  readme_url?: string;
  updatable: boolean;
  last_checked_at?: string;
  last_synced_at?: string;
};

type SkillDistributionView = {
  tool: ToolTarget;
  enabled: boolean;
  method: string;
  healthy: boolean;
  status: string;
  sync_state: string;
  source_kind: string;
  variant_id?: number;
  local_source_path?: string;
  local_source_hash?: string;
  local_origin_tool?: string;
  library_path?: string;
  library_hash?: string;
  library_short_hash?: string;
  installed_path?: string;
  installed_hash?: string;
  installed_short_hash?: string;
  comparison_state: SkillComparisonState;
  actual: SkillOverviewActualInstall[];
};

type SkillToolDetail = {
  id: number;
  name: string;
  description: string;
  managed: boolean;
  source_kind: string;
  status: string;
  problem_reason: string;
  definition_status?: string;
  sync_state: string;
  primary_action: string;
  issues: Array<{ code: string; path: string; scope: string; message_key: string }>;
  binding: {
    enabled: boolean;
    method: string;
    source_kind: string;
    variant_id?: number;
    local_source_path?: string;
    local_source_hash?: string;
    actual: SkillOverviewActualInstall[];
  };
  global: {
    present: boolean;
    valid: boolean;
    current_path: string;
    current_hash: string;
  };
  local: {
    present: boolean;
    valid: boolean;
    path: string;
    hash: string;
    origin_tool: string;
  };
};

type ManagedSkillView = {
  id: number;
  name: string;
  description: string;
  managed: boolean;
  source: SkillSourceView;
  library: {
    present: boolean;
    path: string;
    hash: string;
    variant_id: number;
  };
  distribution: SkillDistributionView[];
  status: SkillStatus;
  primary_action: SkillPrimaryAction;
  issue_summary: string;
  details: {
    definition_status?: string;
    problem_reason?: string;
    current_path?: string;
    current_hash?: string;
    archived_variants: SkillOverviewVariant[];
    per_tool: SkillToolDetail[];
    discovered: Array<{ path: string; tool: string; hash: string; method: string }>;
  };
};

type SkillsDashboard = {
  library_path: string;
  tool_availability: Record<ToolTarget, boolean>;
  summary: {
    managed_count: number;
    healthy_count: number;
    issue_count: number;
    needs_action_count: number;
    source_count: number;
  };
  managed: ManagedSkillView[];
};

type LocalDiscoveredSkill = {
  name: string;
  description: string;
  path: string;
  origin_tool: ToolTarget;
  hash: string;
  importable: boolean;
  status: SkillStatus;
  primary_action: SkillPrimaryAction;
  issue_summary: string;
  source: SkillSourceView;
};

type RepoDiscoveredSkill = {
  name: string;
  description: string;
  path: string;
  readme_url: string;
  hash?: string;
};

type RepoDiscoveryGroup = {
  source_id: number;
  source_label: string;
  owner: string;
  repo: string;
  branch: string;
  subpath: string;
  enabled: boolean;
  skill_count: number;
  error?: string;
  skills: RepoDiscoveredSkill[];
};

type SkillsDiscover = {
  summary: {
    local_count: number;
    repo_count: number;
    importable_count: number;
  };
  local: LocalDiscoveredSkill[];
  repos: RepoDiscoveryGroup[];
};

type SkillRepoSourceView = {
  id: number;
  owner: string;
  repo: string;
  branch: string;
  subpath: string;
  enabled: boolean;
  label: string;
  skill_count: number;
  last_error?: string;
  last_checked_at?: string;
};

type MutationResponse = {
  affected_files: AffectedFile[];
};

type ImportManagedSkillResponse = MutationResponse & {
  skill_id: number;
  variant_id: number;
  created_new: boolean;
};

type ImportExistingSkillsResponse = MutationResponse & {
  imported_count: number;
  skipped_count: number;
};

const SECONDARY_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-xl border border-border bg-background px-4 py-2 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const PRIMARY_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-xl bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const DANGER_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-xl border border-red-500/30 px-4 py-2 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-60";
const GHOST_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-xl border border-border/70 px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const INPUT_CLASS =
  "w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors focus:border-accent focus:ring-4 focus:ring-accent/10";

function getFolderPath(path: string) {
  const normalized = path.trim().replace(/[\\/]+$/, "");
  if (!normalized) {
    return "";
  }
  const lastSeparator = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  if (lastSeparator <= 0) {
    return normalized.startsWith("/") ? "/" : "";
  }
  return normalized.slice(0, lastSeparator);
}

async function openLocation(path: string) {
  if (/^https?:\/\//i.test(path)) {
    await open(path);
    return;
  }
  const candidates = [path.trim(), getFolderPath(path)].filter(Boolean);
  for (const candidate of candidates) {
    try {
      await revealItemInDir(candidate);
      return;
    } catch {
      try {
        await open(candidate);
        return;
      } catch {
        continue;
      }
    }
  }
  throw new Error("open_failed");
}

async function openFolderPath(path: string) {
  const normalized = path.trim();
  const candidates = [normalized, getFolderPath(normalized)].filter(Boolean);
  for (const candidate of candidates) {
    try {
      await open(candidate);
      return;
    } catch {
      try {
        await revealItemInDir(candidate);
        return;
      } catch {
        continue;
      }
    }
  }
  throw new Error("open_failed");
}

async function openDirectoryLocation(path: string) {
  const folder = getFolderPath(path);
  if (!folder) {
    await openLocation(path);
    return;
  }
  await openFolderPath(folder);
}

function skillStatusTone(status: SkillStatus) {
  switch (status) {
    case "broken":
      return "border-red-500/20 bg-red-500/10 text-red-700 dark:text-red-300";
    case "update_available":
      return "border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300";
    case "needs_sync":
      return "border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300";
    default:
      return "border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
  }
}

function sourceTone(sourceType: SkillSourceType) {
  switch (sourceType) {
    case "repo":
      return "bg-sky-500/12 text-sky-700 dark:text-sky-300";
    case "imported_tool":
      return "bg-amber-500/12 text-amber-700 dark:text-amber-300";
    case "imported_local":
      return "bg-violet-500/12 text-violet-700 dark:text-violet-300";
    default:
      return "bg-muted text-foreground";
  }
}

function sourceDisplayLabel(t: ReturnType<typeof useTranslation>["t"], source: SkillSourceView) {
  if (source.type === "repo") {
    if (source.repo_owner && source.repo_name) {
      return source.repo_subpath ? `${source.repo_owner}/${source.repo_name}:${source.repo_subpath}` : `${source.repo_owner}/${source.repo_name}`;
    }
    if (source.label) {
      return source.label.replace(/^https?:\/\/github\.com\//i, "");
    }
    return t("skillsSourceRepository", { defaultValue: "Repository" });
  }
  if (source.type === "imported_tool") {
    const tool = source.origin_tool && TOOLS.includes(source.origin_tool as ToolTarget) ? TOOL_LABELS[source.origin_tool as ToolTarget] : "";
    return tool ? t("skillsSourceImportedTool", { defaultValue: "Imported from {{tool}}", tool }) : source.label || t("skillsSourceImportedToolFallback", { defaultValue: "Imported from CLI" });
  }
  if (source.type === "imported_local") {
    return t("skillsSourceImportedLocal", { defaultValue: "Imported from local folder" });
  }
  return source.label || t("skillsSourceManual", { defaultValue: "Manual" });
}

function sourceSecondaryHint(t: ReturnType<typeof useTranslation>["t"], source: SkillSourceView) {
  return source.type === "repo" ? "" : t("skillsSourceUnlinkedRepoHint", { defaultValue: "Repository not linked" });
}

function toolTone(tool: ToolTarget) {
  switch (tool) {
    case "claude":
      return "bg-amber-500/12 text-amber-700 dark:text-amber-300";
    case "codex":
      return "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300";
    case "openclaw":
      return "bg-violet-500/12 text-violet-700 dark:text-violet-300";
    default:
      return "bg-sky-500/12 text-sky-700 dark:text-sky-300";
  }
}

function statusLabel(t: ReturnType<typeof useTranslation>["t"], status: SkillStatus) {
  switch (status) {
    case "broken":
      return t("skillsHubStatusBroken", { defaultValue: "Definition unavailable" });
    case "update_available":
      return t("skillsHubStatusUpdateAvailable", { defaultValue: "Source has updates" });
    case "needs_sync":
      return t("skillsHubStatusNeedsSync", { defaultValue: "Local content differs" });
    default:
      return t("skillsHubStatusHealthy", { defaultValue: "Synced" });
  }
}

function comparisonLabel(t: ReturnType<typeof useTranslation>["t"], state: SkillComparisonState | undefined) {
  switch (state) {
    case "different":
      return t("skillsComparisonDifferent", { defaultValue: "Local content differs" });
    case "missing":
      return t("skillsComparisonMissing", { defaultValue: "Not installed" });
    case "invalid":
      return t("skillsComparisonInvalid", { defaultValue: "Definition unavailable" });
    case "disabled":
      return t("skillsComparisonDisabled", { defaultValue: "Disabled" });
    default:
      return t("skillsComparisonSynced", { defaultValue: "Synced" });
  }
}

function comparisonStatus(state: SkillComparisonState | undefined): SkillStatus {
  switch (state) {
    case "different":
      return "needs_sync";
    case "invalid":
      return "broken";
    default:
      return "healthy";
  }
}

function comparisonTone(state: SkillComparisonState | undefined) {
  if (state === "missing" || state === "disabled") {
    return "border-border bg-muted text-muted-foreground";
  }
  return skillStatusTone(comparisonStatus(state));
}

function defaultToolAvailability(): Record<ToolTarget, boolean> {
  return {
    claude: false,
    codex: false,
    opencode: false,
    openclaw: false,
  };
}

function normalizeToolAvailability(value: Partial<Record<ToolTarget, boolean>> | undefined): Record<ToolTarget, boolean> {
  if (!value) {
    return {
      claude: true,
      codex: true,
      opencode: true,
      openclaw: true,
    };
  }
  const availability = defaultToolAvailability();
  for (const tool of TOOLS) {
    availability[tool] = Boolean(value[tool]);
  }
  return availability;
}

function distributionByTool(distribution: SkillDistributionView[] | undefined) {
  const byTool = new Map<ToolTarget, SkillDistributionView>();
  for (const item of distribution ?? []) {
    if (TOOLS.includes(item.tool)) {
      byTool.set(item.tool, item);
    }
  }
  return byTool;
}

function toolIndicatorState(t: ReturnType<typeof useTranslation>["t"], tool: ToolTarget, available: boolean, distribution?: SkillDistributionView) {
  const label = TOOL_LABELS[tool];
  if (!available) {
    const text = t("skillsToolIndicatorCliUnavailable", { defaultValue: "{{tool}} is not detected on this machine.", tool: label });
    return {
      label,
      title: text,
      ariaLabel: text,
      pillClass: "bg-muted text-muted-foreground",
      dotClass: "bg-muted-foreground/50",
      text,
    };
  }
  if (!distribution || distribution.comparison_state === "missing" || distribution.comparison_state === "disabled") {
    const text = t("skillsToolIndicatorMissing", { defaultValue: "{{tool}} is available, but this skill is not installed for it.", tool: label });
    return {
      label,
      title: text,
      ariaLabel: text,
      pillClass: toolTone(tool),
      dotClass: "bg-muted-foreground/60",
      text,
    };
  }
  if (distribution.comparison_state === "invalid" || distribution.comparison_state === "different" || distribution.status === "problem") {
    const text = t("skillsToolIndicatorAbnormal", { defaultValue: "{{tool}} needs attention: {{state}}.", tool: label, state: comparisonLabel(t, distribution.comparison_state) });
    return {
      label,
      title: text,
      ariaLabel: text,
      pillClass: toolTone(tool),
      dotClass: "bg-red-500",
      text,
    };
  }
  const text = t("skillsToolIndicatorSynced", { defaultValue: "{{tool}} is installed and synced.", tool: label });
  return {
    label,
    title: text,
    ariaLabel: text,
    pillClass: toolTone(tool),
    dotClass: "bg-emerald-500",
    text,
  };
}

function installMethodLabel(t: ReturnType<typeof useTranslation>["t"], method: string | undefined) {
  return method === "copy"
    ? t("skillsInstallMethodCopy", { defaultValue: "Copy" })
    : t("skillsInstallMethodSymlink", { defaultValue: "Link" });
}

function primaryActionLabel(t: ReturnType<typeof useTranslation>["t"], action: SkillPrimaryAction) {
  switch (action) {
    case "import_into_library":
      return t("skillsHubActionImport", { defaultValue: "Import into management" });
    case "sync_distribution":
      return t("skillsHubActionSync", { defaultValue: "Apply to tools again" });
    case "update_from_source":
      return t("skillsHubActionUpdate", { defaultValue: "Update source" });
    case "repair":
      return t("skillsHubActionRepair", { defaultValue: "Repair" });
    default:
      return t("skillsNoAction", { defaultValue: "No action needed" });
  }
}

function countEnabledTools(distribution: SkillDistributionView[]) {
  return distribution.filter(isConnectedDistribution).length;
}

function isConnectedDistribution(item: SkillDistributionView) {
  return Boolean(item.installed_path) && item.comparison_state !== "missing" && item.comparison_state !== "disabled";
}

function normalizeManagedSkill(skill: ManagedSkillView): ManagedSkillView {
  const distribution = Array.isArray(skill.distribution)
    ? skill.distribution.map((item) => {
        const comparisonState =
          item.enabled && !item.installed_path && item.comparison_state !== "disabled" && item.comparison_state !== "invalid"
            ? "missing"
            : item.comparison_state ?? (item.healthy ? "synced" : "different");
        return {
          ...item,
          comparison_state: comparisonState,
          actual: Array.isArray(item.actual) ? item.actual : [],
        };
      })
    : [];
  const hasContentDrift = distribution.some((item) => item.comparison_state === "different");
  const status = skill.status === "needs_sync" && !hasContentDrift ? "healthy" : skill.status;
  return {
    ...skill,
    status,
    distribution,
    details: {
      ...skill.details,
      archived_variants: Array.isArray(skill.details?.archived_variants) ? skill.details.archived_variants : [],
      per_tool: Array.isArray(skill.details?.per_tool) ? skill.details.per_tool : [],
      discovered: Array.isArray(skill.details?.discovered) ? skill.details.discovered : [],
    },
  };
}

function normalizeDashboard(data: SkillsDashboard): SkillsDashboard {
  const managed = Array.isArray(data.managed) ? data.managed.map(normalizeManagedSkill) : [];
  return {
    ...data,
    tool_availability: normalizeToolAvailability(data.tool_availability),
    managed,
  };
}

function normalizeDiscover(data: SkillsDiscover): SkillsDiscover {
  return {
    ...data,
    local: Array.isArray(data.local) ? data.local : [],
    repos: Array.isArray(data.repos)
      ? data.repos.map((group) => ({
          ...group,
          skills: Array.isArray(group.skills) ? group.skills : [],
        }))
      : [],
  };
}

function IconButton({
  label,
  onClick,
  disabled,
  kind = "secondary",
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  kind?: "secondary" | "primary" | "danger";
}) {
  const className = kind === "primary" ? PRIMARY_BUTTON : kind === "danger" ? DANGER_BUTTON : SECONDARY_BUTTON;
  return (
    <button type="button" onClick={onClick} disabled={disabled} className={className}>
      {label}
    </button>
  );
}

function DetailsButton({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex min-h-10 cursor-pointer items-center rounded-xl border border-border bg-background px-3.5 py-2 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30"
    >
      {label}
    </button>
  );
}

function ModalShell({
  title,
  subtitle,
  closeLabel,
  onClose,
  children,
}: {
  title: string;
  subtitle?: string;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/55 px-4 py-6">
      <div className="max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-[28px] border border-border bg-card p-5 shadow-2xl sm:p-6">
        <div className="flex items-start justify-between gap-4 border-b border-border pb-4">
          <div className="min-w-0">
            <h2 className="truncate text-xl font-semibold text-foreground">{title}</h2>
            {subtitle ? <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p> : null}
          </div>
          <button type="button" onClick={onClose} className={GHOST_BUTTON}>
            {closeLabel}
          </button>
        </div>
        <div className="mt-5">{children}</div>
      </div>
    </div>
  );
}

function DetailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-[24px] border border-border bg-background/70 p-4">
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      <div className="mt-3 space-y-3 text-sm">{children}</div>
    </section>
  );
}

export default function SkillsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [dashboard, setDashboard] = useState<SkillsDashboard | null>(null);
  const [discover, setDiscover] = useState<SkillsDiscover | null>(null);
  const [sources, setSources] = useState<SkillRepoSourceView[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [mutating, setMutating] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [selectedSkill, setSelectedSkill] = useState<ManagedSkillView | null>(null);
  const [discoverOpen, setDiscoverOpen] = useState(false);
  const [sourcesOpen, setSourcesOpen] = useState(false);
  const [actionsOpen, setActionsOpen] = useState(false);
  const [sourceForm, setSourceForm] = useState({
    owner: "",
    repo: "",
    branch: "main",
    subpath: "",
    enabled: true,
  });

  useEffect(() => {
    void loadAll();
  }, []);

  useEffect(() => {
    if (!error && !message) {
      return;
    }
    const timer = window.setTimeout(() => {
      setError("");
      setMessage("");
    }, 4200);
    return () => window.clearTimeout(timer);
  }, [error, message]);

  async function loadAll() {
    setLoading(true);
    setError("");
    try {
      const [dashboardResp, discoverResp, sourcesResp] = await Promise.all([
        fetchRaw<SkillsDashboard>("config/skills/dashboard"),
        fetchRaw<SkillsDiscover>("config/skills/discover"),
        fetchRaw<SkillRepoSourceView[]>("config/skills/sources/repos"),
      ]);
      const nextDashboard = normalizeDashboard(dashboardResp);
      setDashboard(nextDashboard);
      setDiscover(normalizeDiscover(discoverResp));
      setSources(Array.isArray(sourcesResp) ? sourcesResp : []);
      return nextDashboard;
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsLoadFailed"));
      return null;
    } finally {
      setLoading(false);
    }
  }

  async function handleOpen(path: string) {
    try {
      await openLocation(path);
    } catch {
      setError(t("skillsOpenFailed"));
    }
  }

  async function runMutation(action: () => Promise<void>, success: string) {
    setMutating(true);
    setError("");
    setMessage("");
    try {
      await action();
      if (success) {
        setMessage(success);
      }
      const nextDashboard = await loadAll();
      if (nextDashboard && selectedSkill) {
        setSelectedSkill(nextDashboard.managed.find((skill) => skill.id === selectedSkill.id) ?? null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutating(false);
    }
  }

  async function importExisting() {
    await runMutation(async () => {
      const result = await mutateAPI<ImportExistingSkillsResponse>("POST", "config/skills/import-existing");
      setMessage(
        t("skillsHubImportedExisting", {
          defaultValue: "Imported {{count}} existing skills.",
          count: result.imported_count,
        })
      );
    }, "");
  }

  async function refreshSources() {
    await runMutation(async () => {
      await mutateAPI<RepoDiscoveryGroup[]>("POST", "config/skills/sources/refresh");
    }, t("skillsHubRefreshDone", { defaultValue: "Sources refreshed." }));
  }

  async function importDiscovered(skill: LocalDiscoveredSkill) {
    await runMutation(async () => {
      await mutateAPI<ImportManagedSkillResponse>("POST", "config/skills/import-managed", {
        name: skill.name,
        tool: skill.origin_tool,
        source_path: skill.path,
      });
    }, t("skillsActionImportToGlobalDone", { defaultValue: "Imported into the global library." }));
  }

  async function setInstallMethod(skill: ManagedSkillView, distribution: SkillDistributionView, method: "symlink" | "copy") {
    const sourceKind = distribution.source_kind === "local" && distribution.local_source_path ? "local" : "global";
    await runMutation(async () => {
      await mutateAPI<MutationResponse>("PUT", `config/skills/${skill.id}/bindings/${distribution.tool}`, {
        method,
        enabled: true,
        variant_id: sourceKind === "global" ? skill.library.variant_id : 0,
        source_kind: sourceKind,
        local_source_path: sourceKind === "local" ? distribution.local_source_path : "",
        local_source_hash: sourceKind === "local" ? distribution.local_source_hash ?? "" : "",
        local_origin_tool: sourceKind === "local" ? distribution.local_origin_tool ?? distribution.tool : "",
      });
    }, t("skillsInstallMethodUpdated", { defaultValue: "Install method updated." }));
  }

  async function createRepoSource() {
    await runMutation(async () => {
      await mutateAPI<{ id: number }>("POST", "config/skills/sources/repos", sourceForm);
      setSourceForm({ owner: "", repo: "", branch: "main", subpath: "", enabled: true });
    }, t("skillsHubSourceAdded", { defaultValue: "Source added." }));
  }

  async function deleteRepoSource(id: number) {
    await runMutation(async () => {
      await mutateAPI<MutationResponse>("DELETE", `config/skills/sources/repos/${id}`);
    }, t("skillsHubSourceDeleted", { defaultValue: "Source removed." }));
  }

  function openSourcesPanel() {
    setActionsOpen(false);
    setSourcesOpen(true);
  }

  function openDiscoverPanel() {
    setActionsOpen(false);
    setDiscoverOpen(true);
  }

  function sourceBadge(source: SkillSourceView, size: "xs" | "sm" = "xs") {
    const label = sourceDisplayLabel(t, source);
    const hint = sourceSecondaryHint(t, source);
    const className = `inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-full px-2.5 py-1 font-medium ${size === "sm" ? "text-xs" : "text-[11px]"} ${sourceTone(source.type)}`;
    const content = (
      <>
        <span className="truncate">{label}</span>
        {hint ? <span className="shrink-0 text-[10px] font-normal opacity-70">{hint}</span> : null}
      </>
    );
    if (source.type === "repo" && source.url) {
      return (
        <button type="button" onClick={() => void handleOpen(source.url ?? "")} className={`${className} cursor-pointer hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30`}>
          {content}
        </button>
      );
    }
    return <span className={className}>{content}</span>;
  }

  const managedSkills = useMemo(() => {
    const items = dashboard?.managed ?? [];
    if (!query.trim()) {
      return items;
    }
    const search = query.trim().toLowerCase();
    return items.filter((skill) =>
      [skill.name, skill.description, sourceDisplayLabel(t, skill.source), skill.issue_summary, skill.library.path].join("\n").toLowerCase().includes(search)
    );
  }, [dashboard, query, t]);

  const localDiscoveries = useMemo(() => {
    const items = discover?.local ?? [];
    return items;
  }, [discover]);

  const repoDiscoveries = useMemo(() => {
    const groups = discover?.repos ?? [];
    return groups;
  }, [discover]);

  const toolCounts = useMemo(() => {
    const counts: Record<ToolTarget, number> = {
      claude: 0,
      codex: 0,
      opencode: 0,
      openclaw: 0,
    };
    for (const skill of dashboard?.managed ?? []) {
      for (const item of skill.distribution) {
        if (item.enabled) {
          counts[item.tool] += 1;
        }
      }
    }
    return counts;
  }, [dashboard]);

  return (
    <div className="h-full overflow-y-auto pr-1">
      {(error || message) ? (
        <div className="pointer-events-none fixed right-8 top-32 z-50 flex max-w-md justify-end">
          <div
            role="status"
            className={`pointer-events-auto rounded-2xl border px-4 py-3 text-sm shadow-2xl backdrop-blur ${
              error
                ? "border-red-500/25 bg-card/95 text-red-600 dark:text-red-300"
                : "border-emerald-500/25 bg-card/95 text-emerald-700 dark:text-emerald-300"
            }`}
          >
            {error || message}
          </div>
        </div>
      ) : null}

      <div className="flex flex-col gap-4 pb-6">
        <section className="rounded-[28px] border border-border bg-card p-4 sm:p-5">
          <div className="flex flex-col gap-3 border-b border-border pb-4 2xl:flex-row 2xl:items-center 2xl:justify-between">
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
              <IconButton label={t("skillsHubToolbarRestoreBackup", { defaultValue: "Restore Backup" })} onClick={() => navigate("/config/files")} disabled={mutating} />
              <IconButton label={t("skillsHubToolbarInstallZip", { defaultValue: "Install from ZIP" })} onClick={openDiscoverPanel} disabled={mutating} />
              <IconButton label={t("skillsHubToolbarImportExisting", { defaultValue: "Import Existing" })} onClick={() => void importExisting()} disabled={mutating} />
              <IconButton label={t("skillsHubDiscoverTab", { defaultValue: "Discover Skills" })} onClick={openDiscoverPanel} disabled={mutating} kind="primary" />
              <div className="relative">
                <button
                  type="button"
                  onClick={() => setActionsOpen((current) => !current)}
                  className="inline-flex min-h-10 cursor-pointer items-center justify-center rounded-xl border border-border bg-background px-3.5 py-2 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30"
                >
                  {t("skillsHubMoreActions", { defaultValue: "More" })}
                </button>
                {actionsOpen ? (
                  <div className="absolute right-0 top-full z-20 mt-2 w-72 overflow-hidden rounded-2xl border border-border bg-card shadow-2xl">
                    <button
                      type="button"
                      onClick={openSourcesPanel}
                      className="flex w-full items-start justify-between gap-3 px-4 py-3 text-left transition-colors hover:bg-muted"
                    >
                      <span>
                        <span className="block text-sm text-foreground">{t("skillsHubSourcesTab", { defaultValue: "Sources" })}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {t("skillsHubSourcesActionHelp", {
                            defaultValue: "Manage repository sources and review what they can discover.",
                          })}
                        </span>
                      </span>
                      <span className="pt-0.5 text-xs text-muted-foreground">{sources.length}</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setActionsOpen(false);
                        void refreshSources();
                      }}
                      className="flex w-full items-start px-4 py-3 text-left transition-colors hover:bg-muted"
                    >
                      <span>
                        <span className="block text-sm text-foreground">{t("skillsHubToolbarRefresh", { defaultValue: "Refresh Sources" })}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {t("skillsHubRefreshActionHelp", {
                            defaultValue: "Re-scan repository sources and check whether managed sources have updates.",
                          })}
                        </span>
                      </span>
                    </button>
                    {dashboard?.library_path ? (
                      <button
                        type="button"
                        onClick={() => {
                          setActionsOpen(false);
                          void handleOpen(dashboard.library_path);
                        }}
                        className="flex w-full items-start px-4 py-3 text-left transition-colors hover:bg-muted"
                      >
                        <span>
                          <span className="block text-sm text-foreground">{t("skillsOpenGlobalLibrary")}</span>
                          <span className="mt-1 block text-xs text-muted-foreground">
                            {t("skillsHubOpenLibraryHelp", {
                              defaultValue: "Open the central managed library folder on disk.",
                            })}
                          </span>
                        </span>
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>

            <div className="flex shrink-0 flex-wrap items-center gap-2">
              {TOOLS.map((tool) => (
                <span key={tool} className={`inline-flex rounded-full px-3.5 py-2 text-sm font-medium ${toolTone(tool)}`}>
                  {TOOL_LABELS[tool]}: {toolCounts[tool]}
                </span>
              ))}
            </div>

            <div className="w-full 2xl:max-w-md">
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("skillsSearchPlaceholderNew")}
                className={INPUT_CLASS}
              />
            </div>
          </div>

          <div className="pt-5">
            {loading ? <div className="rounded-[24px] border border-border bg-background px-6 py-10 text-sm text-muted-foreground">{t("loading")}</div> : null}

            {!loading ? (
              managedSkills.length === 0 ? (
                <div className="rounded-[24px] border border-dashed border-border bg-background px-6 py-10 text-center">
                  <h3 className="text-base font-semibold text-foreground">{t("skillsEmptyTitle")}</h3>
                  <p className="mt-2 text-sm leading-7 text-muted-foreground">{t("skillsEmptyDescription")}</p>
                </div>
              ) : (
                <div className="overflow-hidden rounded-[24px] border border-border bg-background/60">
                  {managedSkills.map((skill, index) => {
                    const byTool = distributionByTool(skill.distribution);
                    return (
                      <article
                        key={skill.id}
                        className={`flex flex-col gap-4 px-4 py-4 transition-colors hover:bg-muted/50 lg:flex-row lg:items-center lg:justify-between ${
                          index !== managedSkills.length - 1 ? "border-b border-border" : ""
                        }`}
                      >
                        <div className="grid min-w-0 flex-1 gap-2 lg:grid-cols-[minmax(180px,0.9fr)_minmax(220px,1.4fr)_minmax(180px,1fr)] lg:items-center">
                          <div className="min-w-0">
                            <div className="truncate text-base font-semibold text-foreground">{skill.name}</div>
                            <div className="mt-1">{sourceBadge(skill.source)}</div>
                          </div>
                          <p className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-sm text-muted-foreground">
                            {skill.description || t("skillsNoDescription")}
                          </p>
                          <div className="flex min-w-0 flex-wrap items-center gap-2">
                            <span className={`inline-flex rounded-full border px-2.5 py-1 text-[11px] font-medium ${skillStatusTone(skill.status)}`}>
                              {statusLabel(t, skill.status)}
                            </span>
                            <span className="text-xs text-muted-foreground">
                              {t("skillsSummaryConnected", {
                                defaultValue: "{{count}} CLIs connected",
                                count: countEnabledTools(skill.distribution),
                              })}
                            </span>
                          </div>
                        </div>

                        <div className="flex flex-col gap-3 lg:w-[360px] lg:items-end">
                          <div className="flex flex-wrap gap-1.5 lg:justify-end">
                            {TOOLS.map((tool) => {
                              const state = toolIndicatorState(t, tool, Boolean(dashboard?.tool_availability[tool]), byTool.get(tool));
                              return (
                                <span
                                  key={`${skill.id}:${tool}`}
                                  title={state.title}
                                  aria-label={state.ariaLabel}
                                  className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${state.pillClass}`}
                                >
                                  <span className={`h-1.5 w-1.5 rounded-full ${state.dotClass}`} />
                                  {state.label}
                                </span>
                              );
                            })}
                          </div>
                          <div className="flex items-center gap-2">
                            <DetailsButton label={t("details", { defaultValue: "Details" })} onClick={() => setSelectedSkill(skill)} />
                          </div>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )
            ) : null}
          </div>
        </section>

        {selectedSkill ? (
          <ModalShell title={selectedSkill.name} subtitle={selectedSkill.description || t("skillsNoDescription")} closeLabel={t("close")} onClose={() => setSelectedSkill(null)}>
            <div className="space-y-5">
              <div className="flex flex-wrap items-center gap-2">
                {sourceBadge(selectedSkill.source, "sm")}
                <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${skillStatusTone(selectedSkill.status)}`}>{statusLabel(t, selectedSkill.status)}</span>
                {selectedSkill.library.path ? <IconButton label={t("skillsOpenFolder")} onClick={() => void handleOpen(selectedSkill.library.path)} /> : null}
                {selectedSkill.source.readme_url ? <IconButton label={t("skillsHubOpenReadme", { defaultValue: "Open README" })} onClick={() => void handleOpen(selectedSkill.source.readme_url ?? "")} /> : null}
              </div>

              <DetailSection title={t("skillsCliInstallSection", { defaultValue: "CLI installs" })}>
                <div className="space-y-3">
                  {TOOLS.map((tool) => {
                    const distribution = distributionByTool(selectedSkill.distribution).get(tool);
                    const available = Boolean(dashboard?.tool_availability[tool]);
                    const state = toolIndicatorState(t, tool, available, distribution);
                    const comparisonState = distribution?.comparison_state ?? "missing";
                    const currentMethod = distribution?.method === "copy" ? "copy" : "symlink";
                    return (
                      <div key={tool} className={`rounded-2xl border px-4 py-3 ${available ? "border-border/70 bg-card" : "border-border/60 bg-muted/50 text-muted-foreground"}`}>
                        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <span title={state.title} aria-label={state.ariaLabel} className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium ${state.pillClass}`}>
                                <span className={`h-1.5 w-1.5 rounded-full ${state.dotClass}`} />
                                {TOOL_LABELS[tool]}
                              </span>
                              <span className={`inline-flex rounded-full border px-2.5 py-1 text-[11px] font-medium ${available ? comparisonTone(comparisonState) : "border-border bg-muted text-muted-foreground"}`}>
                                {comparisonLabel(t, comparisonState)}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {t("skillsInstallMethodCurrent", {
                                  defaultValue: "Current: {{method}}",
                                  method: distribution?.installed_path ? installMethodLabel(t, currentMethod) : "-",
                                })}
                              </span>
                            </div>
                            <div className="mt-2 text-xs text-muted-foreground">
                              {distribution?.installed_path
                                ? t("skillsCliInstalledByMethod", {
                                    defaultValue: "Installed with {{method}}.",
                                    method: installMethodLabel(t, currentMethod),
                                  })
                                : t("skillsCliNotInstalledForSkill", { defaultValue: "Not installed for this CLI." })}
                              {!available ? ` ${t("skillsToolCliUnavailableDetail", { defaultValue: "CLI executable not detected." })}` : ""}
                            </div>
                          </div>

                          <div className="flex flex-wrap items-center gap-2">
                            {distribution?.installed_path ? (
                              <IconButton
                                label={t("skillsOpenInstalled", { defaultValue: "Open install location" })}
                                onClick={() => {
                                  void openDirectoryLocation(distribution.installed_path ?? "").catch(() => setError(t("skillsOpenFailed")));
                                }}
                              />
                            ) : null}
                            {distribution ? (
                              <div className="inline-flex overflow-hidden rounded-xl border border-border bg-background">
                                {(["symlink", "copy"] as const).map((method) => (
                                  <button
                                    key={method}
                                    type="button"
                                    onClick={() => void setInstallMethod(selectedSkill, distribution, method)}
                                    disabled={mutating || (distribution.installed_path ? currentMethod === method : false)}
                                    className={`min-h-10 cursor-pointer px-3.5 py-2 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-70 ${
                                      distribution.installed_path && currentMethod === method ? "bg-accent text-white" : "text-foreground hover:bg-muted"
                                    }`}
                                  >
                                    {installMethodLabel(t, method)}
                                  </button>
                                ))}
                              </div>
                            ) : null}
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </DetailSection>
            </div>
          </ModalShell>
        ) : null}

        {discoverOpen ? (
          <ModalShell
            title={t("skillsHubDiscoverTab", { defaultValue: "Discover / Import" })}
            subtitle={t("skillsHubDiscoverDescription", {
              defaultValue: "Import local skills that already exist, then review repository discoveries separately.",
            })}
            closeLabel={t("close")}
            onClose={() => setDiscoverOpen(false)}
          >
            <div className="space-y-5">
              <div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="text-sm font-semibold text-foreground">{t("skillsHubDiscoverLocalTitle", { defaultValue: "Existing local skills" })}</div>
                  <IconButton label={t("skillsHubToolbarImportExisting", { defaultValue: "Import Existing" })} onClick={() => void importExisting()} disabled={mutating} />
                </div>
                <div className="mt-3 overflow-hidden rounded-[24px] border border-border bg-background/60">
                  {localDiscoveries.length === 0 ? (
                    <div className="px-6 py-8 text-sm text-muted-foreground">{t("skillsHubDiscoverLocalEmpty", { defaultValue: "No importable local skills were found." })}</div>
                  ) : (
                    localDiscoveries.map((skill, index) => (
                      <article
                        key={`${skill.origin_tool}:${skill.path}`}
                        className={`flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between ${
                          index !== localDiscoveries.length - 1 ? "border-b border-border" : ""
                        }`}
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <h3 className="text-base font-semibold text-foreground">{skill.name}</h3>
                            <span className={`inline-flex rounded-full px-2.5 py-1 text-[11px] font-medium ${toolTone(skill.origin_tool)}`}>
                              {TOOL_LABELS[skill.origin_tool]}
                            </span>
                          </div>
                          <p className="mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm text-muted-foreground">
                            {skill.description || t("skillsNoDescription")}
                          </p>
                          <div className="mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm text-muted-foreground">{skill.path}</div>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <IconButton label={primaryActionLabel(t, skill.primary_action)} onClick={() => void importDiscovered(skill)} disabled={mutating} kind="primary" />
                          <IconButton label={t("skillsOpenFolder")} onClick={() => void handleOpen(skill.path)} />
                        </div>
                      </article>
                    ))
                  )}
                </div>
              </div>

              <div>
                <div className="text-sm font-semibold text-foreground">{t("skillsHubDiscoverRepoTitle", { defaultValue: "Repository discoveries" })}</div>
                <div className="mt-3 space-y-3">
                  {repoDiscoveries.length === 0 ? (
                    <div className="rounded-[24px] border border-dashed border-border bg-background px-6 py-8 text-sm text-muted-foreground">
                      {t("skillsHubDiscoverRepoEmpty", { defaultValue: "No repository sources have been refreshed yet." })}
                    </div>
                  ) : (
                    repoDiscoveries.map((group) => (
                      <section key={group.source_id} className="overflow-hidden rounded-[24px] border border-border bg-background/60">
                        <div className="flex flex-col gap-2 border-b border-border px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
                          <div>
                            <div className="text-sm font-semibold text-foreground">{group.source_label}</div>
                            <div className="text-xs text-muted-foreground">{group.skill_count} skills</div>
                          </div>
                          {group.error ? <div className="text-sm text-red-400">{group.error}</div> : null}
                        </div>
                        <div>
                          {group.skills.map((skill, index) => (
                            <article
                              key={`${group.source_id}:${skill.path}`}
                              className={`flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between ${
                                index !== group.skills.length - 1 ? "border-b border-border" : ""
                              }`}
                            >
                              <div className="min-w-0 flex-1">
                                <div className="text-sm font-semibold text-foreground">{skill.name}</div>
                                <div className="mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm text-muted-foreground">
                                  {skill.description || t("skillsNoDescription")}
                                </div>
                                <div className="mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-xs text-muted-foreground">{skill.path}</div>
                              </div>
                              {skill.readme_url ? <IconButton label={t("skillsHubOpenReadme", { defaultValue: "Open README" })} onClick={() => void handleOpen(skill.readme_url)} /> : null}
                            </article>
                          ))}
                        </div>
                      </section>
                    ))
                  )}
                </div>
              </div>
            </div>
          </ModalShell>
        ) : null}

        {sourcesOpen ? (
          <ModalShell
            title={t("skillsHubSourcesTab", { defaultValue: "Sources" })}
            subtitle={t("skillsHubSourcesDescription", {
              defaultValue: "Configure repository sources, refresh them, and keep track of how many skills each source exposes.",
            })}
            closeLabel={t("close")}
            onClose={() => setSourcesOpen(false)}
          >
            <div className="space-y-5">
              <section className="rounded-[24px] border border-border bg-background/60 p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="text-sm font-semibold text-foreground">{t("skillsHubSourcesAddTitle", { defaultValue: "Add repository source" })}</div>
                  <IconButton label={t("skillsHubToolbarRefresh", { defaultValue: "Refresh Sources" })} onClick={() => void refreshSources()} disabled={mutating} />
                </div>
                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <input value={sourceForm.owner} onChange={(event) => setSourceForm((current) => ({ ...current, owner: event.target.value }))} placeholder="owner" className={INPUT_CLASS} />
                  <input value={sourceForm.repo} onChange={(event) => setSourceForm((current) => ({ ...current, repo: event.target.value }))} placeholder="repo" className={INPUT_CLASS} />
                  <input value={sourceForm.branch} onChange={(event) => setSourceForm((current) => ({ ...current, branch: event.target.value }))} placeholder="branch" className={INPUT_CLASS} />
                  <input value={sourceForm.subpath} onChange={(event) => setSourceForm((current) => ({ ...current, subpath: event.target.value }))} placeholder="subpath (optional)" className={INPUT_CLASS} />
                </div>
                <div className="mt-4 flex flex-wrap items-center gap-3">
                  <label className="inline-flex items-center gap-2 text-sm text-foreground">
                    <input
                      type="checkbox"
                      checked={sourceForm.enabled}
                      onChange={(event) => setSourceForm((current) => ({ ...current, enabled: event.target.checked }))}
                      className="h-4 w-4 rounded border-border accent-current"
                    />
                    {t("skillsHubSourceEnabled", { defaultValue: "Enabled" })}
                  </label>
                  <IconButton label={t("skillsHubSourceAddButton", { defaultValue: "Add source" })} onClick={() => void createRepoSource()} disabled={mutating} kind="primary" />
                </div>
              </section>

              <div className="overflow-hidden rounded-[24px] border border-border bg-background/60">
                {sources.length === 0 ? (
                  <div className="px-6 py-8 text-sm text-muted-foreground">{t("skillsHubSourcesEmpty", { defaultValue: "No repository sources configured yet." })}</div>
                ) : (
                  sources.map((source, index) => (
                    <article
                      key={source.id}
                      className={`flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between ${
                        index !== sources.length - 1 ? "border-b border-border" : ""
                      }`}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="text-base font-semibold text-foreground">{source.label}</h3>
                          <span className="inline-flex rounded-full bg-sky-500/12 px-2.5 py-1 text-[11px] font-medium text-sky-300">
                            {source.skill_count} skills
                          </span>
                        </div>
                        <div className="mt-1 text-sm text-muted-foreground">
                          {source.enabled ? t("skillsHubSourceEnabled", { defaultValue: "Enabled" }) : t("skillsBindingStatusDisabled", { defaultValue: "Not enabled" })}
                        </div>
                        {source.last_error ? <div className="mt-1 text-sm text-red-400">{source.last_error}</div> : null}
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <IconButton label={t("refresh")} onClick={() => void refreshSources()} disabled={mutating} />
                        <IconButton label={t("delete")} onClick={() => void deleteRepoSource(source.id)} disabled={mutating} kind="danger" />
                      </div>
                    </article>
                  ))
                )}
              </div>
            </div>
          </ModalShell>
        ) : null}
      </div>
    </div>
  );
}
