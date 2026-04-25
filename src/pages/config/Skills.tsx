import { openPath as open } from "@tauri-apps/plugin-opener";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import ConfirmPanel, { type AffectedFile } from "../../components/ConfirmPanel";
import SyncStatus from "../../components/SyncStatus";
import ToolTargets, {
  TOOL_LABELS,
  TOOLS,
  type ToolTarget,
} from "../../components/ToolTargets";
import { fetchRaw, mutateAPI } from "../../lib/api";

type SyncMethod = "symlink" | "copy";

type SkillTarget = {
  method: SyncMethod;
  enabled: boolean;
};

type Skill = {
  id: number;
  name: string;
  source_path: string;
  description: string;
  enabled: boolean;
  targets: Partial<Record<ToolTarget, SkillTarget>>;
  created_at: string;
};

type FormState = {
  name: string;
  sourcePath: string;
  description: string;
  enabled: boolean;
  targets: Partial<Record<ToolTarget, SkillTarget>>;
};

type SkillConnectionState = "connected" | "notConnected" | "unassigned";

type CLISkillGroup = {
  key: SkillConnectionState;
  label: string;
  description: string;
  items: Skill[];
};

type MutationResponse = {
  affected_files: AffectedFile[];
};

type CreateMutationResponse = MutationResponse & {
  id: number;
};

type ConfigFileInfo = {
  path: string;
  tool: ToolTarget | string;
};

type SkillInventoryEntry = {
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
};

type SkillConflict = {
  name: string;
  library: SkillInventoryEntry;
  external: SkillInventoryEntry;
};

type SkillInventory = {
  library_path: string;
  cli: {
    available: boolean;
    command: string;
    message: string;
  };
  library: SkillInventoryEntry[];
  discovered: SkillInventoryEntry[];
  conflicts: SkillConflict[];
  summary: {
    library_count: number;
    discovered_count: number;
    importable_count: number;
    conflict_count: number;
  };
};

type ImportSkillsResponse = MutationResponse & {
  imported_count: number;
  skipped_count: number;
  conflicts: SkillConflict[];
};

type PendingAction =
  | {
      type: "save";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
      mode: "create" | "edit";
      skillID: number | null;
      snapshot: FormState;
    }
  | {
      type: "delete";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
      snapshot: Skill;
    }
  | {
      type: "import";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
    }
  | {
      type: "resolveConflict";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
      payload: {
        name: string;
        tool: string;
        library_path: string;
        external_path: string;
        direction: "external_over_library" | "library_over_external";
      };
    };

function getDefaultSyncMethod(): SyncMethod {
  if (typeof navigator === "undefined") {
    return "symlink";
  }

  const platform =
    (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform ??
    navigator.platform ??
    navigator.userAgent;

  return /windows/i.test(platform) ? "copy" : "symlink";
}

function createEmptyForm(): FormState {
  const defaultMethod = getDefaultSyncMethod();

  return {
    name: "",
    sourcePath: "",
    description: "",
    enabled: true,
    targets: Object.fromEntries(
      TOOLS.map((tool) => [
        tool,
        {
          enabled: false,
          method: defaultMethod,
        },
      ])
    ) as Partial<Record<ToolTarget, SkillTarget>>,
  };
}

function skillToForm(skill: Skill): FormState {
  const defaultMethod = getDefaultSyncMethod();
  const targets = Object.fromEntries(
    TOOLS.map((tool) => {
      const target = skill.targets?.[tool];

      return [
        tool,
        {
          enabled: Boolean(target?.enabled),
          method: target?.method ?? defaultMethod,
        },
      ];
    })
  ) as Partial<Record<ToolTarget, SkillTarget>>;

  return {
    name: skill.name,
    sourcePath: skill.source_path,
    description: skill.description ?? "",
    enabled: skill.enabled,
    targets,
  };
}

function cloneSkill(skill: Skill): Skill {
  return {
    ...skill,
    targets: Object.fromEntries(
      Object.entries(skill.targets ?? {}).map(([tool, target]) => [
        tool,
        target ? { ...target } : target,
      ])
    ) as Partial<Record<ToolTarget, SkillTarget>>,
  };
}

function buildTargets(
  targets: Partial<Record<ToolTarget, SkillTarget>>
): Record<string, SkillTarget> {
  return Object.fromEntries(
    TOOLS.map((tool) => [
      tool,
      {
        method: targets[tool]?.method ?? getDefaultSyncMethod(),
        enabled: Boolean(targets[tool]?.enabled),
      },
    ])
  );
}

function basename(path: string): string {
  const normalized = path.replace(/[\\/]+$/, "");
  const lastSeparator = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  return lastSeparator >= 0 ? normalized.slice(lastSeparator + 1) : normalized;
}

function dirname(path: string): string {
  const lastSeparator = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return lastSeparator >= 0 ? path.slice(0, lastSeparator) : "";
}

function joinPath(base: string, child: string): string {
  const separator = base.includes("\\") ? "\\" : "/";
  return base ? `${base.replace(/[\\/]+$/, "")}${separator}${child}` : child;
}

function inferSkillInstallRoot(tool: ToolTarget, files: ConfigFileInfo[]): string | null {
  const firstPath = files.find((file) => file.tool === tool)?.path;
  if (!firstPath) {
    return null;
  }

  switch (tool) {
    case "claude":
      return joinPath(dirname(firstPath), "skills");
    case "codex": {
      const codexDir = dirname(firstPath);
      return joinPath(joinPath(dirname(codexDir), ".agents"), "skills");
    }
    case "opencode":
    case "openclaw":
      return joinPath(dirname(firstPath), "skills");
    default:
      return null;
  }
}

function buildSkillPreviewEntries(
  files: ConfigFileInfo[],
  sourcePath: string,
  targets: Partial<Record<ToolTarget, SkillTarget>>,
  createEntry: (path: string, tool: ToolTarget) => AffectedFile
): AffectedFile[] {
  const skillName = basename(sourcePath);
  return TOOLS.filter((tool) => Boolean(targets[tool]?.enabled)).reduce<AffectedFile[]>(
    (preview, tool) => {
      const root = inferSkillInstallRoot(tool, files);
      if (!root) {
        return preview;
      }
      preview.push(createEntry(joinPath(root, skillName), tool));
      return preview;
    },
    []
  );
}

function buildSavePreview(
  files: ConfigFileInfo[],
  sourcePath: string,
  targets: Partial<Record<ToolTarget, SkillTarget>>
): AffectedFile[] {
  return buildSkillPreviewEntries(files, sourcePath, targets, (path, tool) => ({
    path,
    tool,
    operation: targets[tool]?.method ?? getDefaultSyncMethod(),
  }));
}

function buildDeletePreview(
  files: ConfigFileInfo[],
  sourcePath: string,
  targets: Partial<Record<ToolTarget, SkillTarget>>
): AffectedFile[] {
  return buildSkillPreviewEntries(files, sourcePath, targets, (path, tool) => ({
    path,
    tool,
    operation: "delete",
  }));
}

function getEnabledTools(targets: Partial<Record<ToolTarget, SkillTarget>>): ToolTarget[] {
  return TOOLS.filter((tool) => Boolean(targets?.[tool]?.enabled));
}

function getConnectedToolCount(skills: Skill[], tool: ToolTarget): number {
  return skills.filter((skill) => Boolean(skill.targets?.[tool]?.enabled)).length;
}

function getDiscoveredToolCount(entries: SkillInventoryEntry[], tool: ToolTarget): number {
  return entries.filter((entry) => entry.tool === tool && !entry.represented).length;
}

function getInitialTool(skills: Skill[], inventory: SkillInventory | null): ToolTarget {
  const connectedTool = TOOLS.find((tool) => getConnectedToolCount(skills, tool) > 0);
  if (connectedTool) {
    return connectedTool;
  }

  const discoveredTool = TOOLS.find((tool) =>
    getDiscoveredToolCount(inventory?.discovered ?? [], tool) > 0
  );
  return discoveredTool ?? "codex";
}

function matchesSkillQuery(skill: Skill, query: string): boolean {
  if (!query.trim()) {
    return true;
  }

  const haystack = [skill.name, skill.description, skill.source_path]
    .join("\n")
    .toLowerCase();

  return haystack.includes(query.trim().toLowerCase());
}

function matchesInventoryQuery(entry: SkillInventoryEntry, query: string): boolean {
  if (!query.trim()) {
    return true;
  }

  const haystack = [entry.name, entry.description, entry.path].join("\n").toLowerCase();
  return haystack.includes(query.trim().toLowerCase());
}

function getConnectionState(skill: Skill, tool: ToolTarget): SkillConnectionState {
  if (skill.targets?.[tool]?.enabled) {
    return "connected";
  }
  if (getEnabledTools(skill.targets).length === 0) {
    return "unassigned";
  }
  return "notConnected";
}

function buildCurrentCLIGroups(
  skills: Skill[],
  tool: ToolTarget,
  query: string,
  labels: Record<SkillConnectionState, string>,
  descriptions: Record<SkillConnectionState, string>
): CLISkillGroup[] {
  const filtered = skills.filter((skill) => matchesSkillQuery(skill, query));

  return (["connected", "notConnected", "unassigned"] as SkillConnectionState[]).map((key) => ({
    key,
    label: labels[key],
    description: descriptions[key],
    items: filtered.filter((skill) => getConnectionState(skill, tool) === key),
  }));
}

function getToolTranslationKey(tool: ToolTarget): string {
  switch (tool) {
    case "claude":
      return "claudeCode";
    case "codex":
      return "codex";
    case "opencode":
      return "openCode";
    case "openclaw":
      return "openClaw";
    default:
      return tool;
  }
}

function getInventoryToolLabel(
  tool: string,
  labels: Record<ToolTarget, string>
): string {
  return TOOLS.includes(tool as ToolTarget) ? labels[tool as ToolTarget] : tool;
}

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) {
    return "Name is required.";
  }
  if (!form.sourcePath.trim()) {
    return "Source path is required.";
  }
  const invalidTarget = TOOLS.find((tool) => {
    const method = form.targets[tool]?.method;
    return method !== undefined && method !== "symlink" && method !== "copy";
  });
  if (invalidTarget) {
    return "Sync method must be symlink or copy.";
  }
  return null;
}

export default function Skills() {
  const { t } = useTranslation();
  const [skills, setSkills] = useState<Skill[]>([]);
  const [selectedID, setSelectedID] = useState<number | "new" | null>(null);
  const [activeTool, setActiveTool] = useState<ToolTarget | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [form, setForm] = useState<FormState>(() => createEmptyForm());
  const [loading, setLoading] = useState(true);
  const [inventory, setInventory] = useState<SkillInventory | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [previewingAction, setPreviewingAction] = useState<"save" | "delete" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null);

  const selectedSkill = useMemo(
    () => skills.find((skill) => skill.id === selectedID) ?? null,
    [selectedID, skills]
  );
  const isCreating = selectedID === "new";
  const showEditor = isCreating || selectedSkill !== null;
  const currentTool = activeTool ?? "codex";
  const toolLabels = useMemo(
    () =>
      Object.fromEntries(
        TOOLS.map((tool) => [tool, t(getToolTranslationKey(tool), TOOL_LABELS[tool])])
      ) as Record<ToolTarget, string>,
    [t]
  );
  const groupLabels = useMemo(
    () => ({
      connected: t("connected", "Connected"),
      notConnected: t("notConnected", "Not Connected"),
      unassigned: t("unassigned", "Unassigned"),
    }),
    [t]
  );
  const groupDescriptions = useMemo(
    () => ({
      connected: t("connectedDescription", "Already installed for the current CLI."),
      notConnected: t(
        "notConnectedDescription",
        "Available to connect, but not installed for the current CLI yet."
      ),
      unassigned: t("unassignedDescription", "Not connected to any CLI yet."),
    }),
    [t]
  );
  const discoveredToolCounts = useMemo(
    () =>
      Object.fromEntries(
        TOOLS.map((tool) => [tool, getDiscoveredToolCount(inventory?.discovered ?? [], tool)])
      ) as Record<ToolTarget, number>,
    [inventory]
  );
  const toolCounts = useMemo(
    () =>
      Object.fromEntries(
        TOOLS.map((tool) => [
          tool,
          skills.length > 0 ? getConnectedToolCount(skills, tool) : discoveredToolCounts[tool],
        ])
      ) as Record<ToolTarget, number>,
    [discoveredToolCounts, skills]
  );
  const currentCLIGroups = useMemo(
    () =>
      buildCurrentCLIGroups(skills, currentTool, searchQuery, groupLabels, groupDescriptions),
    [skills, currentTool, searchQuery, groupLabels, groupDescriptions]
  );
  const importableDiscovered = inventory?.discovered.filter((entry) => entry.importable) ?? [];
  const conflicts = inventory?.conflicts ?? [];
  const currentToolDiscovered = useMemo(
    () =>
      (inventory?.discovered ?? []).filter(
        (entry) =>
          entry.tool === currentTool && !entry.represented && matchesInventoryQuery(entry, searchQuery)
      ),
    [currentTool, inventory, searchQuery]
  );

  const loadSkills = async (nextSelectedID?: number | "new" | null) => {
    setLoading(true);
    setError(null);

    try {
      const [data, inventoryData] = await Promise.all([
        fetchRaw<Skill[]>("config/skills"),
        fetchRaw<SkillInventory>("config/skills/inventory"),
      ]);
      setSkills(data);
      setInventory(inventoryData);
      if (activeTool === null) {
        setActiveTool(getInitialTool(data, inventoryData));
      }

      if (nextSelectedID !== undefined) {
        setSelectedID(nextSelectedID);
        if (nextSelectedID === "new" || nextSelectedID === null) {
          setForm(createEmptyForm());
        } else {
          const nextSkill = data.find((skill) => skill.id === nextSelectedID);
          setForm(nextSkill ? skillToForm(nextSkill) : createEmptyForm());
        }
        return;
      }

      const currentStillExists = data.some((skill) => skill.id === selectedID);
      const nextSkill = currentStillExists
        ? data.find((skill) => skill.id === selectedID) ?? null
        : data[0] ?? null;

      setSelectedID(nextSkill?.id ?? null);
      setForm(nextSkill ? skillToForm(nextSkill) : createEmptyForm());
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setLoading(false);
    }
  };

  const loadInventory = async () => {
    setError(null);
    try {
      setInventory(await fetchRaw<SkillInventory>("config/skills/inventory"));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    }
  };

  useEffect(() => {
    loadSkills();
  }, []);

  const updateForm = (updates: Partial<FormState>) => {
    setForm((current) => ({ ...current, ...updates }));
  };

  const enabledTargets = useMemo(
    () => TOOLS.filter((tool) => Boolean(form.targets[tool]?.enabled)),
    [form.targets]
  );
  const hasVisibleSkills = useMemo(
    () => currentCLIGroups.some((group) => group.items.length > 0),
    [currentCLIGroups]
  );

  const toolSelection = useMemo(
    () =>
      Object.fromEntries(
        TOOLS.map((tool) => [tool, Boolean(form.targets[tool]?.enabled)])
      ) as Partial<Record<ToolTarget, boolean>>,
    [form.targets]
  );

  const updateTargetSelection = (nextTargets: Partial<Record<ToolTarget, boolean>>) => {
    const defaultMethod = getDefaultSyncMethod();
    setForm((current) => ({
      ...current,
      targets: Object.fromEntries(
        TOOLS.map((tool) => [
          tool,
          {
            enabled: Boolean(nextTargets[tool]),
            method: current.targets[tool]?.method ?? defaultMethod,
          },
        ])
      ) as Partial<Record<ToolTarget, SkillTarget>>,
    }));
  };

  const updateTargetMethod = (tool: ToolTarget, method: SyncMethod) => {
    setForm((current) => ({
      ...current,
      targets: {
        ...current.targets,
        [tool]: {
          enabled: Boolean(current.targets[tool]?.enabled),
          method,
        },
      },
    }));
  };

  const selectSkill = (skill: Skill) => {
    setSelectedID(skill.id);
    setForm(skillToForm(skill));
    setError(null);
    setStatus(null);
  };

  const startCreate = () => {
    setSelectedID("new");
    setForm(createEmptyForm());
    setError(null);
    setStatus(null);
  };

  const openFolder = async (path: string) => {
    if (!path.trim()) {
      setError(`${t("sourcePath")} is required.`);
      return;
    }

    setError(null);
    try {
      await open(path);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    }
  };

  const requestSave = async () => {
    const validationError = validateForm(form);
    if (validationError) {
      setError(validationError);
      return;
    }

    const snapshot = {
      ...form,
      targets: Object.fromEntries(
        TOOLS.map((tool) => [
          tool,
          form.targets[tool]
            ? {
                ...form.targets[tool],
              }
            : form.targets[tool],
        ])
      ) as Partial<Record<ToolTarget, SkillTarget>>,
    };
    setError(null);
    setStatus(null);
    setPreviewingAction("save");
    try {
      const files = await fetchRaw<ConfigFileInfo[]>("config/files");
      setPendingAction({
        type: "save",
        title: `${isCreating ? t("create") : t("save")} ${t("skills")}`,
        confirmLabel: isCreating ? t("create") : t("save"),
        affectedFiles: buildSavePreview(files, snapshot.sourcePath, snapshot.targets),
        mode: isCreating ? "create" : "edit",
        skillID: selectedSkill?.id ?? null,
        snapshot,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setPreviewingAction(null);
    }
  };

  const requestDelete = async () => {
    if (!selectedSkill) {
      return;
    }

    const snapshot = cloneSkill(selectedSkill);
    setError(null);
    setStatus(null);
    setPreviewingAction("delete");
    try {
      const files = await fetchRaw<ConfigFileInfo[]>("config/files");
      setPendingAction({
        type: "delete",
        title: `${t("delete")} ${selectedSkill.name}`,
        confirmLabel: t("delete"),
        affectedFiles: buildDeletePreview(files, snapshot.source_path, snapshot.targets),
        snapshot,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setPreviewingAction(null);
    }
  };

  const confirmSave = async () => {
    if (!pendingAction || pendingAction.type !== "save") {
      return;
    }

    const validationError = validateForm(pendingAction.snapshot);
    if (validationError) {
      setError(validationError);
      setPendingAction(null);
      return;
    }

    setSubmitting(true);
    setError(null);
    setStatus(null);

    try {
      const payload = {
        name: pendingAction.snapshot.name.trim(),
        source_path: pendingAction.snapshot.sourcePath.trim(),
        description: pendingAction.snapshot.description.trim(),
        enabled: pendingAction.snapshot.enabled,
        targets: buildTargets(pendingAction.snapshot.targets),
      };

      let affectedFiles: AffectedFile[] = [];

      if (pendingAction.mode === "create") {
        const response = await mutateAPI<CreateMutationResponse>("POST", "config/skills", payload);
        affectedFiles = response.affected_files ?? [];
        await loadSkills(response.id);
      } else if (pendingAction.skillID !== null) {
        const response = await mutateAPI<MutationResponse>(
          "PUT",
          `config/skills/${pendingAction.skillID}`,
          payload
        );
        affectedFiles = response.affected_files ?? [];
        await loadSkills(pendingAction.skillID);
      }

      setPendingAction(null);
      setStatus(`${t("synced")} · ${affectedFiles.length} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSubmitting(false);
    }
  };

  const confirmDelete = async () => {
    if (!pendingAction || pendingAction.type !== "delete") {
      return;
    }

    setSubmitting(true);
    setError(null);
    setStatus(null);

    try {
      const response = await mutateAPI<MutationResponse>(
        "DELETE",
        `config/skills/${pendingAction.snapshot.id}`
      );
      await loadSkills();
      setPendingAction(null);
      setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSubmitting(false);
    }
  };

  const requestImportDiscovered = () => {
    const importable = inventory?.discovered.filter((entry) => entry.importable) ?? [];
    setError(null);
    setStatus(null);
    setPendingAction({
      type: "import",
      title: t("importDiscoveredSkills", "Import discovered skills"),
      confirmLabel: t("importSkills", "Import skills"),
      affectedFiles: importable.map((entry) => ({
        path: `${inventory?.library_path ?? ""}/${basename(entry.path)}`,
        tool: "global",
        operation: "import",
      })),
    });
  };

  const requestResolveConflict = (
    conflict: SkillConflict,
    direction: "external_over_library" | "library_over_external"
  ) => {
    const usingExternal = direction === "external_over_library";
    setError(null);
    setStatus(null);
    setPendingAction({
      type: "resolveConflict",
      title: usingExternal
        ? t("useToolVersion", "Use tool version")
        : t("useGlobalVersion", "Use global version"),
      confirmLabel: usingExternal
        ? t("replaceGlobal", "Replace global")
        : t("preferGlobal", "Prefer global"),
      affectedFiles: [
        {
          path: usingExternal ? conflict.library.path : conflict.external.path,
          tool: usingExternal ? "global" : conflict.external.tool,
          operation: usingExternal ? "replace" : "prefer_global",
        },
      ],
      payload: {
        name: conflict.name,
        tool: conflict.external.tool,
        library_path: conflict.library.path,
        external_path: conflict.external.path,
        direction,
      },
    });
  };

  const confirmImport = async () => {
    setSubmitting(true);
    setError(null);
    setStatus(null);
    try {
      const response = await mutateAPI<ImportSkillsResponse>("POST", "config/skills/import", {});
      await loadSkills();
      setPendingAction(null);
      setStatus(
        `${t("imported", "Imported")} ${response.imported_count} · ${response.skipped_count} ${t(
          "conflict",
          "Conflict"
        )}`
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSubmitting(false);
    }
  };

  const confirmResolveConflict = async () => {
    if (!pendingAction || pendingAction.type !== "resolveConflict") {
      return;
    }
    setSubmitting(true);
    setError(null);
    setStatus(null);
    try {
      const response = await mutateAPI<MutationResponse>(
        "POST",
        "config/skills/conflicts/resolve",
        pendingAction.payload
      );
      await loadSkills();
      setPendingAction(null);
      setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSubmitting(false);
    }
  };

  const confirmPendingAction = () => {
    if (!pendingAction) {
      return;
    }
    if (pendingAction.type === "save") {
      confirmSave();
      return;
    }
    if (pendingAction.type === "delete") {
      confirmDelete();
      return;
    }
    if (pendingAction.type === "import") {
      confirmImport();
      return;
    }
    confirmResolveConflict();
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-y-auto pb-4 pr-1">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-foreground">{t("skills")}</h2>
          <p className="text-sm text-muted-foreground">{t("syncTargets")}</p>
        </div>
        <SyncStatus />
      </div>

      {error ? (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-500">
          {error}
        </div>
      ) : null}
      {status ? (
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-500">
          {status}
        </div>
      ) : null}

      <section className="rounded-xl border border-border bg-card p-3.5">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-semibold text-foreground">
                {t("globalSkillLibrary", "Global skill library")}
              </h3>
              <span
                className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                  inventory?.cli.available
                    ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                    : "border border-border text-muted-foreground"
                }`}
              >
                npx skills · {inventory?.cli.available ? t("enabled") : t("disabled")}
              </span>
            </div>
            <p className="break-all text-xs text-muted-foreground">
              {inventory?.library_path ?? "~/.agent-usage/skills"}
            </p>
            <p className="text-xs text-muted-foreground">{inventory?.cli.message ?? "—"}</p>
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={loadInventory}
              className="rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              {t("refresh")}
            </button>
            <button
              type="button"
              onClick={requestImportDiscovered}
              disabled={!inventory || inventory.summary.importable_count === 0 || submitting}
              className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {t("importNonConflicting", "Import non-conflicting")}
            </button>
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-2 xl:grid-cols-4">
          {[
            [t("library", "Library"), inventory?.summary.library_count ?? 0],
            [t("discovered", "Discovered"), inventory?.summary.discovered_count ?? 0],
            [t("importable", "Importable"), inventory?.summary.importable_count ?? 0],
            [t("conflict"), inventory?.summary.conflict_count ?? 0],
          ].map(([label, value]) => (
            <div
              key={label}
              className="rounded-lg border border-border/70 bg-background/60 px-3 py-2"
            >
              <div className="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                {label}
              </div>
              <div className="mt-1 text-sm font-semibold text-foreground">{value}</div>
            </div>
          ))}
        </div>
      </section>

      <div
        className={`grid gap-3 ${
          conflicts.length
            ? "xl:grid-cols-[minmax(0,1.2fr)_minmax(300px,0.8fr)]"
            : ""
        }`}
      >
        <section className="rounded-xl border border-border bg-card p-3.5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-foreground">
                {t("discoveredSkills", "Discovered skills")}
              </h3>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("importable", "Importable")} · {importableDiscovered.length}
              </p>
            </div>
            <span className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
              {inventory?.summary.discovered_count ?? 0}
            </span>
          </div>

          <div className="mt-3 max-h-60 overflow-y-auto pr-1">
            {importableDiscovered.length ? (
              <div className="grid gap-2 md:grid-cols-2">
                {importableDiscovered.map((entry) => (
                  <div
                    key={`${entry.tool}:${entry.path}`}
                    className="rounded-lg border border-border/70 bg-background/40 px-3 py-2.5"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-foreground">
                          {entry.name}
                        </div>
                        <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                          {entry.description || "—"}
                        </div>
                      </div>
                      <span className="shrink-0 rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
                        {getInventoryToolLabel(entry.tool, toolLabels)}
                      </span>
                    </div>
                    <div className="mt-2 break-all text-[11px] leading-5 text-muted-foreground">
                      {entry.path}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="rounded-lg border border-dashed border-border px-3 py-6 text-sm text-muted-foreground">
                {t("noSkills")}
              </div>
            )}
          </div>
        </section>

        {conflicts.length ? (
          <section className="rounded-xl border border-red-500/20 bg-card p-3.5">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-foreground">
                  {t("skillConflicts", "Skill conflicts")}
                </h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("conflict")} · {conflicts.length}
                </p>
              </div>
              <span className="rounded-full border border-red-500/30 px-2 py-0.5 text-[11px] text-red-500">
                {conflicts.length}
              </span>
            </div>

            <div className="mt-3 max-h-60 space-y-2 overflow-y-auto pr-1">
              {conflicts.map((conflict) => (
                <div
                  key={`${conflict.external.tool}:${conflict.external.path}`}
                  className="rounded-lg border border-red-500/20 bg-red-500/5 p-3"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="text-sm font-medium text-foreground">{conflict.name}</div>
                        <span className="rounded-full border border-red-500/30 px-2 py-0.5 text-[11px] text-red-500">
                          {getInventoryToolLabel(conflict.external.tool, toolLabels)}
                        </span>
                      </div>
                      <div className="mt-2 break-all text-[11px] leading-5 text-muted-foreground">
                        {t("sourcePath")}: {conflict.external.path}
                      </div>
                      <div className="mt-1 break-all text-[11px] leading-5 text-muted-foreground">
                        {t("globalSkillLibrary", "Global skill library")}: {conflict.library.path}
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button
                        type="button"
                        onClick={() =>
                          requestResolveConflict(conflict, "external_over_library")
                        }
                        className="rounded-lg border border-border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
                      >
                        {t("useToolVersion", "Use tool version")}
                      </button>
                      <button
                        type="button"
                        onClick={() =>
                          requestResolveConflict(conflict, "library_over_external")
                        }
                        className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-accent/90"
                      >
                        {t("useGlobalVersion", "Use global version")}
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </section>
        ) : null}
      </div>

      <section className="rounded-xl border border-border bg-card p-2.5">
        <div className="px-1 pb-2">
          <h4 className="text-sm font-semibold text-foreground">{t("skills")}</h4>
          <p className="mt-1 text-xs text-muted-foreground">
            {skills.length === 0 && (inventory?.summary.discovered_count ?? 0) > 0
              ? t(
                  "currentCliDiscoveredFallback",
                  "No imported skills yet — showing discovered skills for each CLI."
                )
              : t("currentCliListDescription", "Manage skills in the current CLI context.")}
          </p>
        </div>

        <div className="rounded-xl border border-border/70 bg-background/50 p-1">
          <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-4">
          {TOOLS.map((tool) => {
            const active = tool === currentTool;

            return (
              <button
                key={tool}
                type="button"
                onClick={() => setActiveTool(tool)}
                className={`min-w-0 rounded-lg px-3 py-2 text-left transition-colors ${
                  active
                    ? "bg-card text-foreground shadow-sm ring-1 ring-accent/20"
                    : "text-muted-foreground hover:bg-background hover:text-foreground"
                }`}
              >
                <div className="flex items-center justify-between gap-3">
                  <span className="truncate text-sm font-medium">{toolLabels[tool]}</span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-[11px] ${
                      active
                        ? "bg-accent text-white"
                        : "border border-border text-muted-foreground"
                    }`}
                  >
                    {toolCounts[tool]}
                  </span>
                </div>
              </button>
            );
          })}
        </div>
        </div>
      </section>

      <div className="grid min-h-[500px] gap-3 xl:flex-1 xl:min-h-0 xl:grid-cols-[minmax(340px,0.84fr)_minmax(0,1.16fr)] xl:grid-rows-[minmax(0,1fr)]">
        <aside className="flex min-h-[320px] min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card xl:min-h-0">
          <div className="border-b border-border p-3.5">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-semibold text-foreground">{toolLabels[currentTool]}</h3>
              <button
                type="button"
                onClick={startCreate}
                className="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-accent/90"
              >
                {t("create")}
              </button>
            </div>

            <input
              type="text"
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              placeholder={t(
                "skillSearchPlaceholder",
                "Search skills by name, description, or path..."
              )}
              className="mt-3 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
            />
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto p-2.5 pr-2">
            {loading ? (
              <div className="px-2 py-6 text-sm text-muted-foreground">{t("loading")}</div>
            ) : skills.length === 0 && !isCreating ? (
              <div className="space-y-4">
                {currentToolDiscovered.length ? (
                  <section className="space-y-2">
                    <div className="px-1">
                      <div className="flex items-center justify-between gap-3">
                        <div className="flex items-center gap-2">
                          <span className="h-2 w-2 rounded-full bg-emerald-500" />
                          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                            {t("detectedInCurrentCli", "Discovered in current CLI")}
                          </h4>
                        </div>
                        <span className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
                          {currentToolDiscovered.length}
                        </span>
                      </div>
                      <p className="mt-1 text-[11px] leading-5 text-muted-foreground">
                        {toolLabels[currentTool]} ·{" "}
                        {t(
                          "currentCliDiscoveredDescription",
                          "Found on the selected CLI but not imported into your library yet."
                        )}
                      </p>
                    </div>

                    <div className="space-y-2">
                      {currentToolDiscovered.map((entry) => (
                        <article
                          key={`${entry.tool}:${entry.path}`}
                          className="rounded-lg border border-border/70 bg-background/50 px-3 py-2.5"
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="truncate text-sm font-medium text-foreground">
                                {entry.name}
                              </div>
                              <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                                {entry.description || "—"}
                              </div>
                            </div>
                            <span
                              className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] ${
                                entry.conflict
                                  ? "border border-red-500/30 bg-red-500/10 text-red-500"
                                  : "border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                              }`}
                            >
                              {entry.conflict
                                ? t("conflict", "Conflict")
                                : t("importable", "Importable")}
                            </span>
                          </div>

                          <div className="mt-2 break-all text-[11px] leading-5 text-muted-foreground">
                            {entry.path}
                          </div>

                          <div className="mt-2 flex flex-wrap gap-2">
                            <button
                              type="button"
                              onClick={() => openFolder(entry.path)}
                              className="rounded-md border border-border px-2.5 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
                            >
                              {t("openSource", "Open Source")}
                            </button>
                          </div>
                        </article>
                      ))}
                    </div>
                  </section>
                ) : null}

                <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                  {currentToolDiscovered.length
                    ? t(
                        "importToManageHere",
                        "Import them from the section above to manage them here."
                      )
                    : searchQuery.trim()
                      ? t("noMatchingSkills", "No matching skills.")
                      : t("noSkills")}
                </div>
              </div>
            ) : !hasVisibleSkills && searchQuery.trim() ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                {t("noMatchingSkills", "No matching skills.")}
              </div>
            ) : (
              <div className="space-y-4">
                {currentCLIGroups.map((group) => (
                  <section key={group.key} className="space-y-2">
                    <div className="px-1">
                      <div className="flex items-center justify-between gap-3">
                        <div className="flex items-center gap-2">
                          <span
                            className={`h-2 w-2 rounded-full ${
                              group.key === "connected"
                                ? "bg-accent"
                                : group.key === "notConnected"
                                  ? "bg-amber-500"
                                  : "bg-slate-400"
                            }`}
                          />
                          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                            {group.label}
                          </h4>
                        </div>
                        <span className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
                          {group.items.length}
                        </span>
                      </div>
                      <p className="mt-1 text-[11px] leading-5 text-muted-foreground">
                        {group.description}
                      </p>
                    </div>

                    {group.items.length === 0 ? (
                      <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-muted-foreground">
                        {t("emptyGroupState", "No skills in this group yet.")}
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {group.items.map((skill) => {
                          const selected = skill.id === selectedID;
                          const currentTarget = skill.targets[currentTool];
                          const otherTools = getEnabledTools(skill.targets).filter(
                            (tool) => tool !== currentTool
                          );

                          return (
                            <article
                              key={skill.id}
                              role="button"
                              tabIndex={0}
                              onClick={() => selectSkill(skill)}
                              onKeyDown={(event) => {
                                if (event.key === "Enter" || event.key === " ") {
                                  event.preventDefault();
                                  selectSkill(skill);
                                }
                              }}
                              className={`rounded-lg border px-3 py-2.5 text-left transition-colors ${
                                selected
                                  ? "border-accent bg-accent/10 text-foreground shadow-sm"
                                  : "border-border/70 bg-background/50 text-muted-foreground hover:border-border hover:bg-background/80 hover:text-foreground"
                              }`}
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0">
                                  <div className="truncate text-sm font-medium text-foreground">
                                    {skill.name}
                                  </div>
                                  <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                                    {skill.description || "—"}
                                  </div>
                                </div>
                                {currentTarget?.enabled ? (
                                  <span className="shrink-0 rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent">
                                    {currentTarget.method}
                                  </span>
                                ) : null}
                              </div>

                              <div className="mt-2 flex flex-wrap gap-1.5 text-[11px]">
                                <span className="rounded-full border border-border px-2 py-0.5 text-muted-foreground">
                                  {skill.enabled ? t("enabled") : t("disabled")}
                                </span>
                                {otherTools.slice(0, 2).map((tool) => (
                                  <span
                                    key={tool}
                                    className="rounded-full border border-border px-2 py-0.5 text-muted-foreground"
                                  >
                                    {toolLabels[tool]}
                                  </span>
                                ))}
                                {otherTools.length > 2 ? (
                                  <span className="rounded-full border border-border px-2 py-0.5 text-muted-foreground">
                                    +{otherTools.length - 2}
                                  </span>
                                ) : null}
                              </div>

                              <div className="mt-2 min-w-0 overflow-hidden break-all rounded-md border border-border bg-background/70 px-2 py-1.5 text-[11px] leading-5 text-muted-foreground">
                                {skill.source_path}
                              </div>

                              <div className="mt-2 flex flex-wrap gap-2">
                                <button
                                  type="button"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    openFolder(skill.source_path);
                                  }}
                                  className="rounded-md border border-border px-2.5 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
                                >
                                  {t("openSource", "Open Source")}
                                </button>
                              </div>
                            </article>
                          );
                        })}
                      </div>
                    )}
                  </section>
                ))}
              </div>
            )}
          </div>
        </aside>

        <main className="flex min-h-[380px] min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card xl:min-h-0">
          <div className="border-b border-border px-4 py-3.5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-foreground">
                  {isCreating ? t("create") : t("edit")}
                </h3>
                <p className="mt-1 text-xs text-muted-foreground">{toolLabels[currentTool]}</p>
              </div>
              {!isCreating ? (
                <span
                  className={`rounded-full px-3 py-1 text-xs font-medium ${
                    form.enabled
                      ? "bg-accent text-white"
                      : "border border-border text-muted-foreground"
                  }`}
                >
                  {form.enabled ? t("enabled") : t("disabled")}
                </span>
              ) : null}
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
            {!showEditor ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("noSkills")}
              </div>
            ) : (
              <div className="max-w-3xl space-y-6">
                <section className="space-y-4">
                  <div className="grid gap-4 md:grid-cols-2">
                    <label className="block space-y-2">
                      <span className="text-sm font-medium text-foreground">
                        {t("skillName", "Skill Name")}
                      </span>
                      <input
                        type="text"
                        value={form.name}
                        onChange={(event) => updateForm({ name: event.target.value })}
                        className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                      />
                    </label>
                  </div>

                  <label className="block space-y-2">
                    <span className="text-sm font-medium text-foreground">
                      {t("sourcePath")}
                    </span>
                    <div className="flex gap-2">
                      <input
                        type="text"
                        value={form.sourcePath}
                        onChange={(event) => updateForm({ sourcePath: event.target.value })}
                        className="min-w-0 flex-1 rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                      />
                      <button
                        type="button"
                        onClick={() => openFolder(form.sourcePath)}
                        className="shrink-0 rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
                      >
                        {t("openFolder")}
                      </button>
                    </div>
                  </label>

                  <label className="block space-y-2">
                    <span className="text-sm font-medium text-foreground">
                      {t("description")}
                    </span>
                    <textarea
                      value={form.description}
                      onChange={(event) => updateForm({ description: event.target.value })}
                      rows={4}
                      className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                    />
                  </label>
                </section>

                <section className="rounded-xl border border-border bg-background/40 p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h4 className="text-sm font-semibold text-foreground">
                        {toolLabels[currentTool]}
                      </h4>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t(
                          "currentCliSettingsDescription",
                          "Configure how this skill syncs to the selected CLI."
                        )}
                      </p>
                    </div>
                    <label className="flex items-center gap-2 text-sm text-muted-foreground">
                      <input
                        type="checkbox"
                        checked={Boolean(form.targets[currentTool]?.enabled)}
                        onChange={(event) =>
                          updateTargetSelection({
                            ...toolSelection,
                            [currentTool]: event.target.checked,
                          })
                        }
                        className="h-4 w-4 rounded border-border accent-accent"
                      />
                      <span>{t("connected", "Connected")}</span>
                    </label>
                  </div>

                  {form.targets[currentTool]?.enabled ? (
                    <label className="mt-4 block space-y-2">
                      <span className="text-sm font-medium text-foreground">
                        {t("syncMethod")}
                      </span>
                      <select
                        value={form.targets[currentTool]?.method ?? getDefaultSyncMethod()}
                        onChange={(event) =>
                          updateTargetMethod(currentTool, event.target.value as SyncMethod)
                        }
                        className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                      >
                        <option value="symlink">{t("symlink")}</option>
                        <option value="copy">{t("copy")}</option>
                      </select>
                    </label>
                  ) : (
                    <p className="mt-4 text-sm text-muted-foreground">
                      {t(
                        "notConnectedDescription",
                        "Available to connect, but not installed for the current CLI yet."
                      )}
                    </p>
                  )}
                </section>

                <section className="space-y-3">
                  <div>
                    <h4 className="text-sm font-semibold text-foreground">
                      {t("otherCliSummary", "Other CLI summary")}
                    </h4>
                    <div className="mt-2 flex flex-wrap gap-2">
                      {TOOLS.filter((tool) => tool !== currentTool).map((tool) => {
                        const target = form.targets[tool];

                        return (
                          <span
                            key={tool}
                            className="rounded-full border border-border px-2 py-1 text-xs text-muted-foreground"
                          >
                            {toolLabels[tool]} ·{" "}
                            {target?.enabled
                              ? `${t("connected", "Connected")} · ${target.method}`
                              : t("notConnected", "Not Connected")}
                          </span>
                        );
                      })}
                    </div>
                  </div>

                  <details className="overflow-hidden rounded-lg border border-border bg-background/40">
                    <summary className="cursor-pointer px-4 py-3 text-sm font-medium text-foreground">
                      {t("advancedCliSettings", "Advanced CLI settings")}
                    </summary>
                    <div className="space-y-4 border-t border-border px-4 py-4">
                      <ToolTargets targets={toolSelection} onChange={updateTargetSelection} />
                      {enabledTargets.length > 0 ? (
                        <div className="grid gap-4 md:grid-cols-2">
                          {enabledTargets.map((tool) => (
                            <label key={tool} className="block space-y-2">
                              <span className="text-sm font-medium text-foreground">
                                {toolLabels[tool]} · {t("syncMethod")}
                              </span>
                              <select
                                value={form.targets[tool]?.method ?? getDefaultSyncMethod()}
                                onChange={(event) =>
                                  updateTargetMethod(tool, event.target.value as SyncMethod)
                                }
                                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                              >
                                <option value="symlink">{t("symlink")}</option>
                                <option value="copy">{t("copy")}</option>
                              </select>
                            </label>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </details>
                </section>

                <section className="space-y-3">
                  <h4 className="text-sm font-semibold text-foreground">
                    {t("globalStatus", "Global status")}
                  </h4>
                  <label className="flex items-center gap-2 text-sm text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={form.enabled}
                      onChange={(event) => updateForm({ enabled: event.target.checked })}
                      className="h-4 w-4 rounded border-border accent-accent"
                    />
                    <span>{t("enabled")}</span>
                  </label>
                </section>

                <div className="flex flex-wrap items-center gap-3 border-t border-border pt-5">
                  <button
                    type="button"
                    onClick={requestSave}
                    disabled={submitting || previewingAction !== null}
                    className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {previewingAction === "save" || submitting
                      ? t("loading")
                      : isCreating
                        ? t("create")
                        : t("save")}
                  </button>
                  {!isCreating && selectedSkill ? (
                    <button
                      type="button"
                      onClick={requestDelete}
                      disabled={submitting || previewingAction !== null}
                      className="rounded-lg border border-red-500/40 px-4 py-2 text-sm text-red-500 transition-colors hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {previewingAction === "delete" ? t("loading") : t("delete")}
                    </button>
                  ) : null}
                  <button
                    type="button"
                    onClick={() => openFolder(form.sourcePath)}
                    className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {t("openFolder")}
                  </button>
                </div>
              </div>
            )}
          </div>
        </main>
      </div>

      {pendingAction ? (
        <ConfirmPanel
          title={pendingAction.title}
          affectedFiles={pendingAction.affectedFiles}
          onCancel={() => setPendingAction(null)}
          onConfirm={confirmPendingAction}
          confirmLabel={pendingAction.confirmLabel}
          loading={submitting}
        />
      ) : null}
    </div>
  );
}
