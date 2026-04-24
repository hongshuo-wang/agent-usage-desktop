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

function summarizeMethods(targets: Partial<Record<ToolTarget, SkillTarget>>): string {
  const methods = Array.from(
    new Set(
      TOOLS.map((tool) => targets?.[tool])
        .filter((target): target is SkillTarget => Boolean(target?.enabled))
        .map((target) => target.method)
    )
  );

  if (methods.length === 0) {
    return "—";
  }
  return methods.join(", ");
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
  const [form, setForm] = useState<FormState>(() => createEmptyForm());
  const [loading, setLoading] = useState(true);
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

  const loadSkills = async (nextSelectedID?: number | "new" | null) => {
    setLoading(true);
    setError(null);

    try {
      const data = await fetchRaw<Skill[]>("config/skills");
      setSkills(data);

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

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
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

      <div className="grid flex-1 min-h-0 gap-4 xl:grid-cols-[minmax(420px,0.95fr)_minmax(0,1.05fr)]">
        <aside className="flex min-h-0 flex-col rounded-xl border border-border bg-card">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
            <h3 className="text-sm font-semibold text-foreground">{t("skills")}</h3>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                disabled
                title={t("comingSoon", "Coming soon")}
                className="rounded-lg border border-border px-3 py-1.5 text-sm text-muted-foreground opacity-60"
              >
                {t("importFromGitHub", "Import from GitHub")}
              </button>
              <button
                type="button"
                onClick={startCreate}
                className="rounded-lg bg-accent px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-accent/90"
              >
                {t("create")}
              </button>
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto p-3">
            {loading ? (
              <div className="px-2 py-6 text-sm text-muted-foreground">{t("loading")}</div>
            ) : skills.length === 0 && !isCreating ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                {t("noSkills")}
              </div>
            ) : (
              <div className="space-y-3">
                {isCreating ? (
                  <button
                    type="button"
                    className="w-full rounded-lg border border-accent bg-accent/10 px-3 py-3 text-left text-sm text-foreground"
                  >
                    {t("create")}
                  </button>
                ) : null}

                {skills.map((skill) => {
                  const selected = skill.id === selectedID;

                  return (
                    <div
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
                      className={`rounded-lg border px-3 py-3 text-left transition-colors ${
                        selected
                          ? "border-accent bg-accent/10 text-foreground"
                          : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
                      }`}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium">{skill.name}</div>
                          <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                            {skill.description || "—"}
                          </div>
                        </div>
                        <span
                          className={`shrink-0 rounded-full px-2 py-0.5 text-xs ${
                            skill.enabled
                              ? "bg-accent text-white"
                              : "border border-border text-muted-foreground"
                          }`}
                        >
                          {skill.enabled ? t("enabled") : t("disabled")}
                        </span>
                      </div>

                      <div className="mt-3 break-all rounded-md border border-border bg-background/60 px-2 py-1.5 text-xs text-muted-foreground">
                        {skill.source_path}
                      </div>

                      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        <span className="rounded-md border border-border px-2 py-1">
                          {t("syncMethod")}: {summarizeMethods(skill.targets)}
                        </span>
                        <button
                          type="button"
                          onClick={(event) => {
                            event.stopPropagation();
                            openFolder(skill.source_path);
                          }}
                          className="rounded-md border border-border px-2 py-1 transition-colors hover:text-foreground"
                        >
                          {t("openFolder")}
                        </button>
                      </div>

                      <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                        {TOOLS.map((tool) => {
                          const target = skill.targets?.[tool];
                          return (
                            <div key={tool} className="flex items-center gap-2">
                              <input
                                type="checkbox"
                                checked={Boolean(target?.enabled)}
                                readOnly
                                tabIndex={-1}
                                className="h-3.5 w-3.5 rounded border-border accent-accent"
                              />
                              <span>
                                {TOOL_LABELS[tool]}
                                {target?.enabled ? ` · ${target.method}` : ""}
                              </span>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </aside>

        <main className="min-h-0 overflow-y-auto rounded-xl border border-border bg-card p-5">
          {!showEditor ? (
            <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
              {t("noSkills")}
            </div>
          ) : (
            <div className="max-w-3xl space-y-6">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-base font-semibold text-foreground">
                  {isCreating ? t("create") : t("edit")}
                </h3>
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

              <div className="grid gap-4 md:grid-cols-2">
                <label className="block space-y-2">
                  <span className="text-sm font-medium text-foreground">{t("profileName")}</span>
                  <input
                    type="text"
                    value={form.name}
                    onChange={(event) => updateForm({ name: event.target.value })}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                  />
                </label>
              </div>

              <label className="block space-y-2">
                <span className="text-sm font-medium text-foreground">{t("sourcePath")}</span>
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
                <span className="text-sm font-medium text-foreground">{t("description")}</span>
                <textarea
                  value={form.description}
                  onChange={(event) => updateForm({ description: event.target.value })}
                  rows={4}
                  className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                />
              </label>

              <section className="space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <h4 className="text-sm font-semibold text-foreground">{t("syncTargets")}</h4>
                  <label className="flex items-center gap-2 text-sm text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={form.enabled}
                      onChange={(event) => updateForm({ enabled: event.target.checked })}
                      className="h-4 w-4 rounded border-border accent-accent"
                    />
                    <span>{t("enabled")}</span>
                  </label>
                </div>
                <ToolTargets targets={toolSelection} onChange={updateTargetSelection} />
                {enabledTargets.length > 0 ? (
                  <div className="grid gap-4 md:grid-cols-2">
                    {enabledTargets.map((tool) => (
                      <label key={tool} className="block space-y-2">
                        <span className="text-sm font-medium text-foreground">
                          {TOOL_LABELS[tool]} · {t("syncMethod")}
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
              </div>
            </div>
          )}
        </main>
      </div>

      {pendingAction ? (
        <ConfirmPanel
          title={pendingAction.title}
          affectedFiles={pendingAction.affectedFiles}
          onCancel={() => setPendingAction(null)}
          onConfirm={pendingAction.type === "save" ? confirmSave : confirmDelete}
          confirmLabel={pendingAction.confirmLabel}
          loading={submitting}
        />
      ) : null}
    </div>
  );
}
