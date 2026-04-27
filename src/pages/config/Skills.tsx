import { openPath as open } from "@tauri-apps/plugin-opener";
import { useEffect, useMemo, useState } from "react";
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

type ImportManagedSkillResponse = MutationResponse & {
  skill_id: number;
  variant_id: number;
  created_new: boolean;
};

type FilterMode = "all" | "issues" | "managed" | "unmanaged";
type SkillsAPIFlavor = "overview" | "legacy";

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

const PRIMARY_BUTTON =
  "inline-flex min-h-11 items-center justify-center rounded-xl bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const SECONDARY_BUTTON =
  "inline-flex min-h-11 items-center justify-center rounded-xl border border-border px-4 py-2 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const GHOST_BUTTON =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-border/70 px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
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

function normalizeSkillName(name: string) {
  return name.trim().toLowerCase();
}

function issueTone(issueCount: number) {
  if (issueCount > 0) {
    return "border-amber-500/30 bg-amber-500/8";
  }
  return "border-border bg-background/80";
}

function getStatusLabel(t: (key: string) => string, status: SkillStatus) {
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

function matchesFilter(skill: SkillOverviewItem, filter: FilterMode) {
  if (filter === "issues") {
    return skill.issues.length > 0;
  }
  if (filter === "managed") {
    return skill.managed;
  }
  if (filter === "unmanaged") {
    return !skill.managed;
  }
  return true;
}

function MetricCard({
  title,
  value,
  helper,
}: {
  title: string;
  value: number | string;
  helper: string;
}) {
  return (
    <div className="rounded-2xl border border-border bg-background/80 p-4 shadow-sm">
      <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
        {title}
      </div>
      <div className="mt-3 text-3xl font-semibold text-foreground">{value}</div>
      <div className="mt-2 text-sm leading-6 text-muted-foreground">{helper}</div>
    </div>
  );
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
      className={`inline-flex min-h-11 items-center gap-2 rounded-full border px-4 py-2 text-sm transition-colors ${
        active
          ? "border-accent bg-accent text-white"
          : "border-border bg-background text-muted-foreground hover:text-foreground"
      }`}
    >
      <span>{label}</span>
      <span className={`rounded-full px-2 py-0.5 text-xs ${active ? "bg-white/20" : "bg-muted"}`}>
        {count}
      </span>
    </button>
  );
}

function ToolPill({ tool, state, t }: { tool: ToolTarget; state?: ToolState; t: (key: string) => string }) {
  const status = state?.status ?? "not_installed";
  return (
    <div className="min-w-[112px] rounded-xl border border-border/80 bg-background px-3 py-2">
      <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
        {TOOL_LABELS[tool]}
      </div>
      <div className="mt-1 text-sm font-medium text-foreground">{getStatusLabel(t, status)}</div>
      {state?.enabled ? (
        <div className="mt-1 text-xs text-muted-foreground">{pathTail(state.selected_path)}</div>
      ) : null}
    </div>
  );
}

function SkillsCLIStatusChip({
  available,
  loading,
  t,
}: {
  available?: boolean;
  loading: boolean;
  t: (key: string) => string;
}) {
  const installed = Boolean(available);
  const label = loading ? t("loading") : installed ? t("skillsCliInstalled") : t("skillsCliNotInstalled");

  return (
    <div className="group relative">
      <button
        type="button"
        className={`inline-flex min-h-11 items-center rounded-full border px-3 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
          installed
            ? "border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300"
            : "border-border bg-background/70 text-muted-foreground"
        }`}
        aria-label={t("skillsCliTooltipTitle")}
      >
        {label}
      </button>
      <div className="pointer-events-none absolute left-0 top-full z-20 mt-2 w-80 rounded-2xl border border-border bg-card p-4 text-left opacity-0 shadow-lg transition-opacity duration-200 group-hover:opacity-100 group-focus-within:opacity-100">
        <div className="text-sm font-semibold text-foreground">{t("skillsCliTooltipTitle")}</div>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">{t("skillsCliTooltipBody")}</p>
        <div className="mt-3 rounded-xl border border-border/70 bg-background px-3 py-2 font-mono text-xs text-foreground">
          npx skills add hongshuo-wang/agent-usage-desktop -y
        </div>
        <p className="mt-2 text-xs leading-5 text-muted-foreground">{t("skillsCliTooltipHint")}</p>
      </div>
    </div>
  );
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
    summary,
    skills: items,
  };
}

export default function SkillsPage() {
  const { t } = useTranslation();
  const [overview, setOverview] = useState<SkillOverview | null>(null);
  const [apiFlavor, setApiFlavor] = useState<SkillsAPIFlavor>("overview");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [mutatingTool, setMutatingTool] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
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
        return next.skills[0] ? selectionKey(next.skills[0]) : null;
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsLoadFailed"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadOverview();
  }, []);

  const visibleSkills = useMemo(() => {
    return (overview?.skills ?? []).filter(
      (skill) => matchesQuery(skill, query) && matchesFilter(skill, filter)
    );
  }, [filter, overview, query]);

  const selectedSkill = useMemo(() => {
    return visibleSkills.find((skill) => selectionKey(skill) === selectedKey)
      ?? overview?.skills.find((skill) => selectionKey(skill) === selectedKey)
      ?? null;
  }, [overview, selectedKey, visibleSkills]);

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

  const filteredCounts = useMemo(() => {
    const skills = overview?.skills ?? [];
    return {
      all: skills.length,
      issues: skills.filter((skill) => skill.issues.length > 0).length,
      managed: skills.filter((skill) => skill.managed).length,
      unmanaged: skills.filter((skill) => !skill.managed).length,
    };
  }, [overview]);

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
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsCreateFailed"));
    } finally {
      setSaving(false);
    }
  };

  const openFolder = async (path: string) => {
    if (!path) {
      return;
    }
    try {
      await open(path);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsOpenFailed"));
    }
  };

  return (
    <div className="h-full overflow-y-auto pr-1">
      <div className="flex flex-col gap-6 pb-6">
        <div className="rounded-[28px] border border-border bg-[radial-gradient(circle_at_top_left,_rgba(59,130,246,0.12),_transparent_35%),linear-gradient(135deg,rgba(255,255,255,0.92),rgba(255,255,255,0.78))] p-6 shadow-sm dark:bg-[radial-gradient(circle_at_top_left,_rgba(59,130,246,0.18),_transparent_35%),linear-gradient(135deg,rgba(18,24,38,0.96),rgba(18,24,38,0.88))]">
          <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
            <div className="max-w-3xl space-y-3">
              <div className="inline-flex rounded-full border border-accent/20 bg-accent/8 px-3 py-1 text-xs font-medium text-accent">
                {t("skillsPageBadge")}
              </div>
              <div>
                <h1 className="text-2xl font-semibold tracking-tight text-foreground">{t("skills")}</h1>
                <p className="mt-2 max-w-2xl text-sm leading-7 text-muted-foreground">
                  {t("skillsOverviewDescription")}
                </p>
              </div>
              <div className="flex flex-wrap gap-3">
                <SyncStatus />
                <SkillsCLIStatusChip available={overview?.cli.available} loading={loading && !overview} t={t} />
              </div>
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
                <button type="button" onClick={() => void openFolder(overview.library_path)} className={PRIMARY_BUTTON}>
                  {t("skillsOpenLibrary")}
                </button>
              ) : null}
            </div>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <MetricCard
            title={t("skillsMetricLibrary")}
            value={overview?.summary.managed_skills ?? 0}
            helper={t("skillsMetricLibraryHelp")}
          />
          <MetricCard
            title={t("skillsMetricConnected")}
            value={overview?.summary.connected_tools ?? 0}
            helper={t("skillsMetricConnectedHelp")}
          />
          <MetricCard
            title={t("skillsMetricIssues")}
            value={overview?.summary.issue_count ?? 0}
            helper={t("skillsMetricIssuesHelp")}
          />
          <MetricCard
            title={t("skillsMetricUnmanaged")}
            value={overview?.summary.unmanaged_skills ?? 0}
            helper={t("skillsMetricUnmanagedHelp")}
          />
        </div>

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

        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.3fr)_minmax(360px,0.9fr)]">
          <section className="space-y-4">
          <div className="rounded-[24px] border border-border bg-card p-4 shadow-sm">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="space-y-1">
                <h2 className="text-lg font-semibold text-foreground">{t("skillsPrimaryListTitle")}</h2>
                <p className="text-sm text-muted-foreground">{t("skillsPrimaryListDescription")}</p>
              </div>

              <div className="flex flex-col gap-3 lg:items-end">
                <input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("skillsSearchPlaceholderNew")}
                  className={`${INPUT_CLASS} min-w-[260px]`}
                />
                <div className="flex flex-wrap gap-2">
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
                </div>
              </div>
            </div>
          </div>

          {loading ? (
            <div className="rounded-[24px] border border-border bg-card px-6 py-10 text-sm text-muted-foreground">
              {t("loading")}
            </div>
          ) : visibleSkills.length === 0 ? (
            <div className="rounded-[24px] border border-dashed border-border bg-card px-6 py-10 text-center">
              <h3 className="text-base font-semibold text-foreground">{t("skillsEmptyTitle")}</h3>
              <p className="mt-2 text-sm leading-7 text-muted-foreground">{t("skillsEmptyDescription")}</p>
            </div>
          ) : (
            <div className="space-y-3">
              {visibleSkills.map((skill) => {
                const active = selectionKey(skill) === selectedKey;
                return (
                  <button
                    key={selectionKey(skill)}
                    type="button"
                    onClick={() => setSelectedKey(selectionKey(skill))}
                    className={`w-full rounded-[24px] border p-5 text-left shadow-sm transition-all ${
                      active
                        ? "border-accent/40 bg-accent/6 ring-2 ring-accent/20"
                        : `${issueTone(skill.issues.length)} hover:border-accent/20 hover:bg-muted/30`
                    }`}
                  >
                    <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                      <div className="min-w-0 space-y-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="text-lg font-semibold text-foreground">{skill.name}</h3>
                          <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                            {skill.managed ? t("skillsManagedLabel") : t("skillsUnmanagedLabel")}
                          </span>
                          {skill.issues.length > 0 ? (
                            <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                              {t("skillsIssuesBadge", { count: skill.issues.length })}
                            </span>
                          ) : null}
                        </div>
                        <p className="text-sm leading-7 text-muted-foreground">
                          {skill.description || t("skillsNoDescription")}
                        </p>
                        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
                          <div className="rounded-2xl border border-border/70 bg-background/90 p-3">
                            <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                              {t("skillsLibraryColumn")}
                            </div>
                            <div className="mt-2 text-sm font-medium text-foreground">
                              {skill.library.present ? pathTail(skill.library.path) : t("skillsNotInLibrary")}
                            </div>
                            <div className="mt-1 text-xs text-muted-foreground">
                              {skill.library.path || t("skillsLibraryMissingHint")}
                            </div>
                          </div>
                          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
                            {TOOLS.map((tool) => (
                              <ToolPill key={tool} tool={tool} state={skill.tools[tool]} t={t} />
                            ))}
                          </div>
                        </div>
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
          </section>

          <aside className="xl:sticky xl:top-6 xl:self-start">
            <div className="rounded-[28px] border border-border bg-card p-5 shadow-sm">
            {!selectedSkill ? (
              creating ? (
                <div className="space-y-6">
                  <div className="space-y-3">
                    <h2 className="text-lg font-semibold text-foreground">{t("skillsCreateTitle")}</h2>
                    <p className="text-sm leading-7 text-muted-foreground">
                      {t("skillsCreateDescription")}
                    </p>
                  </div>

                  <div className="space-y-4 rounded-2xl border border-border bg-background/70 p-4">
                    <div className="space-y-3">
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("skillName")}
                        </label>
                        <input
                          value={form.name}
                          onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                          className={INPUT_CLASS}
                        />
                      </div>
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("description")}
                        </label>
                        <textarea
                          value={form.description}
                          onChange={(event) =>
                            setForm((current) => ({ ...current, description: event.target.value }))
                          }
                          rows={3}
                          className={`${INPUT_CLASS} resize-y`}
                        />
                      </div>
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("sourcePath")}
                        </label>
                        <input
                          value={form.sourcePath}
                          onChange={(event) =>
                            setForm((current) => ({ ...current, sourcePath: event.target.value }))
                          }
                          className={INPUT_CLASS}
                        />
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-3">
                      <button
                        type="button"
                        onClick={() => void createSkill()}
                        disabled={!form.name.trim() || !form.sourcePath.trim() || saving}
                        className={PRIMARY_BUTTON}
                      >
                        {saving ? t("loading") : t("skillsCreateNew")}
                      </button>
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
                    </div>
                  </div>
                </div>
              ) : (
                <div className="space-y-4 rounded-2xl border border-dashed border-border bg-background/50 px-5 py-8">
                  <h2 className="text-lg font-semibold text-foreground">{t("skillsDetailTitle")}</h2>
                  <p className="text-sm leading-7 text-muted-foreground">{t("skillsDetailEmpty")}</p>
                  <button
                    type="button"
                    onClick={() => {
                      setCreating(true);
                      setForm(createFormState(null));
                    }}
                    className={PRIMARY_BUTTON}
                  >
                    {t("skillsCreateNew")}
                  </button>
                </div>
              )
            ) : (
              <div className="space-y-6">
                <div className="space-y-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        {t("skillsDetailTitle")}
                      </div>
                      <h2 className="mt-2 text-2xl font-semibold tracking-tight text-foreground">
                        {selectedSkill.name}
                      </h2>
                    </div>
                    {selectedSkill.managed ? (
                      <button
                        type="button"
                        onClick={() => setPendingDelete(selectedSkill)}
                        className="inline-flex min-h-10 items-center justify-center rounded-lg border border-red-500/30 px-3 py-2 text-sm text-red-500 transition-colors hover:bg-red-500/10"
                      >
                        {t("delete")}
                      </button>
                    ) : null}
                  </div>
                  <p className="text-sm leading-7 text-muted-foreground">
                    {selectedSkill.description || t("skillsNoDescription")}
                  </p>
                </div>

                <div className="rounded-2xl border border-border bg-background/70 p-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        {t("skillsLibraryColumn")}
                      </div>
                      <div className="mt-2 text-sm font-medium text-foreground">
                        {selectedSkill.library.path || t("skillsNotInLibrary")}
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
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
                          onClick={() =>
                            void importManaged(selectedSkill, selectedSkill.library.path, "global")
                          }
                          disabled={mutatingTool === "global"}
                          className={PRIMARY_BUTTON}
                        >
                          {t("skillsAdoptLibrary")}
                        </button>
                      ) : null}
                    </div>
                  </div>
                </div>

                {selectedSkill.issues.length > 0 ? (
                  <div className="space-y-3 rounded-2xl border border-amber-500/30 bg-amber-500/8 p-4">
                    <div className="space-y-1">
                      <h3 className="text-base font-semibold text-foreground">{t("skillsIssuesTitle")}</h3>
                      <p className="text-sm leading-6 text-muted-foreground">
                        {t("skillsIssuesDescription")}
                      </p>
                    </div>
                    <div className="space-y-2">
                      {Object.entries(toolIssueSummary(selectedSkill.issues)).map(([code, tools]) => (
                        <div
                          key={code}
                          className="rounded-xl border border-amber-500/20 bg-background/80 px-3 py-2"
                        >
                          <div className="text-sm font-medium text-foreground">
                            {getStatusLabel(t, code as SkillStatus)}
                          </div>
                          <div className="mt-1 text-xs text-muted-foreground">
                            {tools.map((tool) => TOOL_LABELS[tool as ToolTarget] ?? tool).join(" / ")}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : null}

                {selectedSkill.managed ? (
                  <div className="space-y-4 rounded-2xl border border-border bg-background/70 p-4">
                    <div className="space-y-1">
                      <h3 className="text-base font-semibold text-foreground">{t("skillsEditSection")}</h3>
                      <p className="text-sm leading-6 text-muted-foreground">{t("skillsEditDescription")}</p>
                    </div>
                    <div className="space-y-3">
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("skillName")}
                        </label>
                        <input
                          value={form.name}
                          onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                          className={INPUT_CLASS}
                        />
                      </div>
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("description")}
                        </label>
                        <textarea
                          value={form.description}
                          onChange={(event) =>
                            setForm((current) => ({ ...current, description: event.target.value }))
                          }
                          rows={3}
                          className={`${INPUT_CLASS} resize-y`}
                        />
                      </div>
                      <div>
                        <label className="mb-1.5 block text-sm font-medium text-foreground">
                          {t("sourcePath")}
                        </label>
                        <input
                          value={form.sourcePath}
                          onChange={(event) =>
                            setForm((current) => ({ ...current, sourcePath: event.target.value }))
                          }
                          className={INPUT_CLASS}
                        />
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-3">
                      <button
                        type="button"
                        onClick={() => void saveSkill()}
                        disabled={!hasUnsavedChanges || saving}
                        className={PRIMARY_BUTTON}
                      >
                        {saving ? t("loading") : t("save")}
                      </button>
                      <button
                        type="button"
                        onClick={() => setForm(createFormState(selectedSkill))}
                        disabled={!hasUnsavedChanges || saving}
                        className={SECONDARY_BUTTON}
                      >
                        {t("resetChanges")}
                      </button>
                    </div>
                  </div>
                ) : null}

                {selectedSkill.variants.length > 0 ? (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between gap-3">
                      <h3 className="text-base font-semibold text-foreground">{t("skillsVariantSection")}</h3>
                      <span className="text-sm text-muted-foreground">{selectedSkill.variants.length}</span>
                    </div>
                    <div className="space-y-3">
                      {selectedSkill.variants.map((variant) => (
                        <div key={`${variant.id}-${variant.source_path}`} className="rounded-2xl border border-border bg-background/70 p-4">
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
                              <div className="mt-2 break-all text-sm font-medium text-foreground">
                                {variant.source_path}
                              </div>
                              <div className="mt-1 text-xs text-muted-foreground">
                                hash {hashPreview(variant.hash)}
                              </div>
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

                <div className="space-y-4">
                  <div className="space-y-1">
                    <h3 className="text-base font-semibold text-foreground">{t("skillsToolSection")}</h3>
                    <p className="text-sm leading-6 text-muted-foreground">{t("skillsToolDescription")}</p>
                  </div>
                  <div className="space-y-4">
                    {TOOLS.map((tool) => {
                      const state = selectedSkill.tools[tool];
                      const canSelectVariants = selectedSkill.managed && selectedSkill.variants.length > 0;
                      return (
                        <div key={tool} className="rounded-2xl border border-border bg-background/70 p-4">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <div className="text-sm font-semibold text-foreground">{TOOL_LABELS[tool]}</div>
                              <div className="mt-1 text-sm text-muted-foreground">
                                {getStatusLabel(t, state?.status ?? "not_installed")}
                              </div>
                            </div>
                            <div className="flex flex-wrap gap-2">
                              <button
                                type="button"
                                onClick={() =>
                                  void mutateToolTarget(
                                    tool,
                                    {
                                      enabled: !(state?.enabled ?? false),
                                      selected_variant_id:
                                        state?.selected_variant_id ||
                                        selectedSkill.library.variant_id ||
                                        selectedSkill.variants[0]?.id ||
                                        0,
                                    },
                                    state?.enabled ? t("skillsDisconnected") : t("skillsConnected")
                                  )
                                }
                                disabled={!selectedSkill.managed || mutatingTool === tool || !canSelectVariants}
                                className={SECONDARY_BUTTON}
                              >
                                {state?.enabled ? t("disconnect") : t("connect")}
                              </button>
                              {state?.actual[0]?.path ? (
                                <button
                                  type="button"
                                  onClick={() => void openFolder(state.actual[0].path)}
                                  className={GHOST_BUTTON}
                                >
                                  {t("skillsOpenInstalled")}
                                </button>
                              ) : null}
                            </div>
                          </div>

                          {selectedSkill.managed ? (
                            <div className="mt-4 space-y-3">
                              <div className="flex flex-wrap gap-2">
                                {(["symlink", "copy"] as SyncMethod[]).map((method) => (
                                  <button
                                    key={method}
                                    type="button"
                                    onClick={() =>
                                      void mutateToolTarget(tool, { method }, t("skillsMethodUpdated"))
                                    }
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
                                {selectedSkill.variants.map((variant) => {
                                  const active = state?.selected_variant_id === variant.id && state?.enabled;
                                  return (
                                    <button
                                      key={`${tool}-${variant.id}`}
                                      type="button"
                                      onClick={() =>
                                        void mutateToolTarget(
                                          tool,
                                          { enabled: true, selected_variant_id: variant.id },
                                          t("skillsVariantApplied")
                                        )
                                      }
                                      disabled={mutatingTool === tool}
                                      className={`flex min-h-11 items-center justify-between rounded-xl border px-3 py-2 text-left text-sm transition-colors ${
                                        active
                                          ? "border-accent bg-accent/8 text-foreground"
                                          : "border-border bg-background hover:bg-muted"
                                      }`}
                                    >
                                      <span className="min-w-0">
                                        <span className="block truncate font-medium">
                                          {pathTail(variant.source_path)}
                                        </span>
                                        <span className="block text-xs text-muted-foreground">
                                          hash {hashPreview(variant.hash)}
                                        </span>
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

                          {!selectedSkill.managed && state?.actual[0]?.path && apiFlavor === "overview" ? (
                            <div className="mt-4">
                              <button
                                type="button"
                                onClick={() => void importManaged(selectedSkill, state.actual[0].path, tool)}
                                disabled={mutatingTool === tool}
                                className={PRIMARY_BUTTON}
                              >
                                {t("skillsAdoptFromTool", { tool: TOOL_LABELS[tool] })}
                              </button>
                            </div>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                </div>

                {!selectedSkill.managed && selectedSkill.discovered.length > 0 ? (
                  <div className="space-y-3 rounded-2xl border border-border bg-background/70 p-4">
                    <h3 className="text-base font-semibold text-foreground">{t("skillsDiscoveredSection")}</h3>
                    <div className="space-y-2">
                      {selectedSkill.discovered.map((entry) => (
                        <div
                          key={`${entry.tool}-${entry.path}`}
                          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/70 bg-background px-3 py-2"
                        >
                          <div className="min-w-0 flex-1">
                            <div className="text-sm font-medium text-foreground">
                              {entry.tool === "global" ? t("skillsLibraryColumn") : entry.tool}
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
            )}
            </div>
          </aside>
        </div>

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
