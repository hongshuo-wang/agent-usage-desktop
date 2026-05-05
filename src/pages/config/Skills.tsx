import { openPath as open, revealItemInDir } from "@tauri-apps/plugin-opener";
import { useEffect, useState, type ReactNode } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import ConfirmPanel, { type AffectedFile } from "../../components/ConfirmPanel";
import { TOOL_LABELS, TOOLS, type ToolTarget } from "../../components/ToolTargets";
import { fetchRaw, mutateAPI } from "../../lib/api";

type SyncMethod = "symlink" | "copy";
type SourceKind = "global" | "local";
type RowStatus = "ok" | "problem";
type DefinitionStatus = "valid" | "missing_path" | "not_directory" | "missing_skill_md" | "invalid_skill_md" | "";
type SyncState = "in_sync" | "can_import_to_global" | "can_sync_to_cli" | "managed_using_local" | "definition_error";
type PrimaryAction = "none" | "import_to_global" | "sync_to_cli" | "override_global";
type IssueScope = "global_definition" | "local_definition" | "actual_install";

type SkillsCLIIssue = {
  scope: IssueScope;
  code: string;
  path: string;
  message_key: string;
  details?: Record<string, string>;
};

type ActualInstall = {
  path: string;
  hash: string;
  method: SyncMethod;
  valid: boolean;
  problem_reason: string;
};

type SkillsCLIOverview = {
  tool: ToolTarget;
  library_path: string;
  cli: {
    available: boolean;
    command: string;
    message: string;
  };
  tool_available: boolean;
  summary: {
    visible_skills: number;
    global_bindings: number;
    local_bindings: number;
    issue_count: number;
  };
  skills: SkillsCLISkill[];
};

type SkillsCLISkill = {
  id: number;
  name: string;
  description: string;
  managed: boolean;
  source_kind: SourceKind;
  status: RowStatus;
  is_valid: boolean;
  problem_reason: string;
  definition_status?: DefinitionStatus;
  sync_state: SyncState;
  primary_action: PrimaryAction;
  action_source_path: string;
  action_target_path: string;
  can_delete: boolean;
  delete_path: string;
  issues: SkillsCLIIssue[];
  binding: {
    enabled: boolean;
    method: SyncMethod;
    source_kind: SourceKind;
    variant_id: number;
    local_source_path: string;
    local_source_hash: string;
    actual: ActualInstall[];
  };
  global: {
    present: boolean;
    valid: boolean;
    definition_status?: DefinitionStatus;
    problem_reason: string;
    current_variant_id: number;
    current_path: string;
    current_hash: string;
  };
  local: {
    present: boolean;
    valid: boolean;
    definition_status?: DefinitionStatus;
    problem_reason: string;
    path: string;
    hash: string;
    origin_tool: string;
  };
};

type MutationResponse = {
  affected_files: AffectedFile[];
};

type ImportManagedSkillResponse = MutationResponse & {
  skill_id: number;
  variant_id: number;
  created_new: boolean;
};

function normalizeSkill(skill: SkillsCLISkill): SkillsCLISkill {
  return {
    ...skill,
    issues: Array.isArray(skill.issues) ? skill.issues : [],
    binding: {
      ...skill.binding,
      actual: Array.isArray(skill.binding?.actual) ? skill.binding.actual : [],
    },
    global: {
      ...skill.global,
    },
    local: {
      ...skill.local,
    },
  };
}

function normalizeOverview(overview: SkillsCLIOverview): SkillsCLIOverview {
  return {
    ...overview,
    skills: Array.isArray(overview.skills) ? overview.skills.map(normalizeSkill) : [],
  };
}

const SECONDARY_BUTTON =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-border bg-background px-4 py-2 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const PRIMARY_BUTTON =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-xl bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const DANGER_BUTTON =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-red-500/30 px-4 py-2 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-60";
const GHOST_BUTTON =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border border-border/70 px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60";
const INPUT_CLASS =
  "w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors focus:border-accent focus:ring-4 focus:ring-accent/10";

function skillKey(skill: SkillsCLISkill) {
  return skill.id > 0 ? `managed:${skill.id}` : `local:${skill.name.toLowerCase()}:${skill.local.path}`;
}

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

function getOpenCandidates(path: string) {
  const candidates = [path.trim(), getFolderPath(path)];
  return candidates.filter((candidate, index, all) => candidate && all.indexOf(candidate) === index);
}

function sourceLabel(t: TFunction, sourceKind: SourceKind) {
  return t(sourceKind === "global" ? "skillsBindingGlobal" : "skillsBindingLocal");
}

function statusLabel(t: TFunction, status: RowStatus) {
  return status === "problem" ? t("skillsStatusProblem") : t("skillsStatusOk");
}

function syncStateLabel(t: TFunction, syncState: SyncState) {
  switch (syncState) {
    case "can_import_to_global":
      return t("skillsSyncStateCanImportToGlobal");
    case "can_sync_to_cli":
      return t("skillsSyncStateCanSyncToCli");
    case "managed_using_local":
      return t("skillsSyncStateManagedUsingLocal");
    case "definition_error":
      return t("skillsSyncStateDefinitionError");
    default:
      return t("skillsSyncStateInSync");
  }
}

function syncStateTone(syncState: SyncState) {
  switch (syncState) {
    case "can_import_to_global":
      return "border-sky-500/25 bg-sky-500/10 text-sky-700 dark:text-sky-300";
    case "can_sync_to_cli":
      return "border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300";
    case "managed_using_local":
      return "border-violet-500/25 bg-violet-500/10 text-violet-700 dark:text-violet-300";
    case "definition_error":
      return "border-red-500/25 bg-red-500/10 text-red-700 dark:text-red-300";
    default:
      return "border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
  }
}

function primaryActionLabel(t: TFunction, action: PrimaryAction) {
  switch (action) {
    case "import_to_global":
      return t("skillsActionImportToGlobal");
    case "sync_to_cli":
      return t("skillsActionSyncToCli");
    case "override_global":
      return t("skillsActionOverrideGlobal");
    default:
      return t("skillsNoAction");
  }
}

function primaryActionDoneLabel(t: TFunction, action: PrimaryAction) {
  switch (action) {
    case "import_to_global":
      return t("skillsActionImportToGlobalDone");
    case "sync_to_cli":
      return t("skillsActionSyncToCliDone");
    case "override_global":
      return t("skillsActionOverrideGlobalDone");
    default:
      return "";
  }
}

function issueMessage(t: TFunction, issue?: SkillsCLIIssue | null) {
  if (!issue) {
    return "";
  }
  return t(issue.message_key, {
    path: issue.path,
    expected_path: issue.details?.expected_path ?? "",
    expected_hash: issue.details?.expected_hash ?? "",
    actual_path: issue.details?.actual_path ?? "",
    actual_hash: issue.details?.actual_hash ?? "",
    source_kind: issue.details?.source_kind ?? "",
    defaultValue: issue.code,
  });
}

function issueScopeLabel(t: TFunction, scope: IssueScope) {
  switch (scope) {
    case "global_definition":
      return t("skillsIssueScopeGlobalDefinition");
    case "local_definition":
      return t("skillsIssueScopeLocalDefinition");
    default:
      return t("skillsIssueScopeActualInstall");
  }
}

function definitionStatusLabel(t: TFunction, status?: DefinitionStatus) {
  switch (status) {
    case "valid":
      return t("skillsDefinitionStatusValid");
    case "missing_path":
      return t("skillsDefinitionStatusMissingPath");
    case "not_directory":
      return t("skillsDefinitionStatusNotDirectory");
    case "missing_skill_md":
      return t("skillsDefinitionStatusMissingSkillFile");
    case "invalid_skill_md":
      return t("skillsDefinitionStatusInvalidSkillFile");
    default:
      return "-";
  }
}

function presenceLabel(t: TFunction, present: boolean, valid: boolean) {
  if (!present) {
    return t("skillsPresenceMissing");
  }
  return valid ? t("skillsPresenceAvailable") : t("skillsPresenceInvalid");
}

function currentSourcePath(skill: SkillsCLISkill) {
  return skill.source_kind === "global" ? skill.global.current_path : skill.local.path;
}

function primaryIssue(skill: SkillsCLISkill) {
  return (skill.issues ?? [])[0] ?? null;
}

function scopeSummary(t: TFunction, summary?: SkillsCLIOverview["summary"]) {
  const issues = summary?.issue_count ?? 0;
  if (issues === 0) {
    return t("skillsCliFocusSummaryNoIssues", { count: summary?.visible_skills ?? 0 });
  }
  return t("skillsCliFocusSummary", {
    count: summary?.visible_skills ?? 0,
    issues,
  });
}

function SyncBadge({ t, syncState }: { t: TFunction; syncState: SyncState }) {
  return (
    <span className={`inline-flex rounded-full border px-2.5 py-1 text-[11px] font-medium ${syncStateTone(syncState)}`}>
      {syncStateLabel(t, syncState)}
    </span>
  );
}

function ProblemIndicator({
  t,
  issue,
}: {
  t: TFunction;
  issue?: SkillsCLIIssue | null;
}) {
  if (!issue) {
    return null;
  }
  const tooltip = issueMessage(t, issue) || t("skillsStatusProblem");
  return (
    <div className="group relative inline-flex">
      <span
        className="inline-flex h-6 min-w-6 items-center justify-center rounded-full border border-red-500/30 bg-red-500/10 px-2 text-[11px] font-semibold text-red-600 dark:text-red-300"
        aria-label={tooltip}
        title={tooltip}
      >
        !
      </span>
      <div className="pointer-events-none absolute left-1/2 top-full z-10 mt-2 hidden w-72 -translate-x-1/2 rounded-xl border border-border bg-card px-3 py-2 text-xs leading-5 text-foreground shadow-lg group-hover:block group-focus-within:block">
        {tooltip}
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
      <h3 className="text-base font-semibold text-foreground">{title}</h3>
      <div className="mt-3 space-y-3 text-sm">{children}</div>
    </section>
  );
}

function PathValue({ value }: { value: string }) {
  return <div className="mt-1.5 break-all text-sm leading-6 text-foreground">{value || "-"}</div>;
}

function InfoGrid({
  items,
}: {
  items: Array<{ label: string; value: ReactNode }>;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {items.map((item) => (
        <div key={item.label}>
          <div className="text-xs font-medium text-muted-foreground">{item.label}</div>
          <div className="mt-1 text-foreground">{item.value}</div>
        </div>
      ))}
    </div>
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
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/45 px-4 py-6">
      <div className="max-h-[90vh] w-full max-w-4xl overflow-y-auto rounded-[28px] border border-border bg-card p-5 shadow-2xl sm:p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold text-foreground">{title}</h2>
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

export default function SkillsPage() {
  const { t } = useTranslation();
  const [overviews, setOverviews] = useState<Partial<Record<ToolTarget, SkillsCLIOverview>>>({});
  const [scope, setScope] = useState<ToolTarget>("codex");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [mutating, setMutating] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<SkillsCLISkill | null>(null);

  useEffect(() => {
    void loadAll();
  }, []);

  async function loadAll(selected?: string | null) {
    setLoading(true);
    setError("");
    try {
      const results = await Promise.all(
        TOOLS.map(async (tool) => [tool, await fetchRaw<SkillsCLIOverview>(`config/skills/cli/${tool}`)] as const)
      );
      const next: Partial<Record<ToolTarget, SkillsCLIOverview>> = {};
      for (const [tool, overview] of results) {
        next[tool] = normalizeOverview(overview);
      }
      setOverviews(next);
      if (selected === null) {
        setSelectedKey(null);
      } else if (selected) {
        setSelectedKey(selected);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsLoadFailed"));
    } finally {
      setLoading(false);
    }
  }

  async function openFolder(path: string) {
    for (const candidate of getOpenCandidates(path)) {
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
    setError(t("skillsOpenFailed"));
  }

  async function importToGlobal(skill: SkillsCLISkill) {
    if (!skill.local.path) {
      setError(t("skillsNoLocalVersion"));
      return;
    }
    setMutating(true);
    setError("");
    setMessage("");
    try {
      await mutateAPI<ImportManagedSkillResponse>("POST", "config/skills/import-managed", {
        skill_id: skill.id || 0,
        name: skill.name,
        tool: scope,
        source_path: skill.local.path,
      });
      setMessage(primaryActionDoneLabel(t, "import_to_global"));
      await loadAll(skill.id > 0 ? skillKey(skill) : null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutating(false);
    }
  }

  async function syncToCurrentCLI(skill: SkillsCLISkill) {
    if (!skill.global.current_path) {
      setError(t("skillsNoGlobalVersion"));
      return;
    }
    setMutating(true);
    setError("");
    setMessage("");
    try {
      let skillID = skill.id;
      if (skillID <= 0) {
        const imported = await mutateAPI<ImportManagedSkillResponse>("POST", "config/skills/import-managed", {
          skill_id: 0,
          name: skill.name,
          tool: "global",
          source_path: skill.global.current_path,
        });
        skillID = imported.skill_id;
      }
      await mutateAPI<MutationResponse>("PUT", `config/skills/${skillID}/bindings/${scope}`, {
        enabled: true,
        method: skill.binding.method || "symlink",
        source_kind: "global",
      });
      setMessage(primaryActionDoneLabel(t, "sync_to_cli"));
      await loadAll(skillID > 0 && skill.id > 0 ? skillKey(skill) : null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutating(false);
    }
  }

  async function overrideGlobal(skill: SkillsCLISkill) {
    if (!skill.local.path) {
      setError(t("skillsNoLocalVersion"));
      return;
    }
    setMutating(true);
    setError("");
    setMessage("");
    try {
      const imported = await mutateAPI<ImportManagedSkillResponse>("POST", "config/skills/import-managed", {
        skill_id: skill.id || 0,
        name: skill.name,
        tool: scope,
        source_path: skill.local.path,
      });
      await mutateAPI<MutationResponse>("POST", `config/skills/${imported.skill_id}/current-variant`, {
        variant_id: imported.variant_id,
      });
      setMessage(primaryActionDoneLabel(t, "override_global"));
      await loadAll(skill.id > 0 ? skillKey(skill) : null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("skillsActionFailed"));
    } finally {
      setMutating(false);
    }
  }

  async function performPrimaryAction(skill: SkillsCLISkill) {
    switch (skill.primary_action) {
      case "import_to_global":
        await importToGlobal(skill);
        break;
      case "sync_to_cli":
        await syncToCurrentCLI(skill);
        break;
      case "override_global":
        await overrideGlobal(skill);
        break;
      default:
        break;
    }
  }

  async function confirmDelete() {
    if (!pendingDelete) {
      return;
    }
    const deletingManagedRecord = pendingDelete.id > 0;
    setMutating(true);
    setError("");
    setMessage("");
    try {
      if (deletingManagedRecord) {
        await mutateAPI<MutationResponse>("DELETE", `config/skills/${pendingDelete.id}`);
      } else {
        await mutateAPI<MutationResponse>("POST", "config/skills/delete-path", {
          path: pendingDelete.delete_path,
        });
      }
      setPendingDelete(null);
      setSelectedKey(null);
      setMessage(t(deletingManagedRecord ? "skillsDeletedManaged" : "skillsDeletedDirectory"));
      await loadAll(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(deletingManagedRecord ? "skillsDeleteManagedFailed" : "skillsDeleteDirectoryFailed"));
    } finally {
      setMutating(false);
    }
  }

  const currentOverview = overviews[scope] ?? null;
  const visibleSkills = (currentOverview?.skills ?? []).filter((skill) => {
    if (!query.trim()) {
      return true;
    }
    const search = query.trim().toLowerCase();
    return [
      skill.name,
      skill.description,
      skill.local.path,
      skill.global.current_path,
      skill.action_source_path,
      skill.action_target_path,
      skill.sync_state,
      skill.primary_action,
      ...(skill.issues ?? []).flatMap((issue) => [issue.path, issueScopeLabel(t, issue.scope), issueMessage(t, issue)]),
    ]
      .join("\n")
      .toLowerCase()
      .includes(search);
  });
  const selectedSkill =
    visibleSkills.find((skill) => skillKey(skill) === selectedKey) ??
    currentOverview?.skills.find((skill) => skillKey(skill) === selectedKey) ??
    null;
  const summary = currentOverview?.summary;

  return (
    <div className="h-full overflow-y-auto pr-1">
      <div className="flex flex-col gap-4 pb-6">
        <section className="relative rounded-[28px] border border-border bg-card px-5 py-5 sm:px-6 sm:py-6">
          <div className="pointer-events-none absolute inset-0 overflow-hidden rounded-[28px]">
            <div className="absolute -left-10 -top-10 h-32 w-32 rounded-full bg-accent/10 blur-3xl" />
            <div className="absolute right-0 top-0 h-28 w-28 rounded-full bg-sky-500/10 blur-3xl" />
          </div>
          <div className="relative flex flex-col gap-5 xl:flex-row xl:items-end xl:justify-between">
            <div className="min-w-0">
              <h1 className="text-3xl font-semibold tracking-tight text-foreground sm:text-[2.05rem]">{t("skills")}</h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">{t("skillsCliOverviewDescription")}</p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {currentOverview?.library_path ? (
                <button type="button" onClick={() => void openFolder(currentOverview.library_path)} className={GHOST_BUTTON}>
                  {t("skillsOpenGlobalLibrary")}
                </button>
              ) : null}
              <button type="button" onClick={() => void loadAll(selectedKey)} className={SECONDARY_BUTTON}>
                {t("refresh")}
              </button>
            </div>
          </div>
        </section>

        {error ? (
          <div className="rounded-2xl border border-red-500/25 bg-red-500/8 px-4 py-3 text-sm text-red-600 dark:text-red-300">{error}</div>
        ) : null}
        {message ? (
          <div className="rounded-2xl border border-emerald-500/25 bg-emerald-500/8 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-300">{message}</div>
        ) : null}

        <section className="rounded-[28px] border border-border bg-card p-4 sm:p-5">
          <div className="flex min-w-max items-end gap-2 overflow-x-auto border-b border-border">
            {TOOLS.map((tool) => {
              const overview = overviews[tool];
              return (
                <button
                  key={tool}
                  type="button"
                  onClick={() => {
                    setScope(tool);
                    setSelectedKey(null);
                  }}
                  className={`inline-flex min-h-11 cursor-pointer items-center gap-3 rounded-t-2xl border border-b-0 px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
                    scope === tool
                      ? "border-border bg-card text-foreground"
                      : "border-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                  }`}
                >
                  <span className="text-sm font-semibold">{TOOL_LABELS[tool]}</span>
                  <span className={`rounded-full px-2 py-0.5 text-[11px] ${scope === tool ? "bg-accent text-white" : "bg-muted text-muted-foreground"}`}>
                    {overview?.summary.visible_skills ?? 0}
                  </span>
                </button>
              );
            })}
          </div>

          <div className="space-y-5 pt-5">
            <div className="space-y-1">
              <div className="text-xl font-semibold text-foreground">{TOOL_LABELS[scope]}</div>
              <p className="text-sm leading-6 text-muted-foreground">{scopeSummary(t, summary)}</p>
            </div>

            <div className="w-full lg:max-w-xl">
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("skillsSearchPlaceholderNew")}
                className={INPUT_CLASS}
              />
            </div>

            {loading ? (
              <div className="rounded-[24px] border border-border bg-background px-6 py-10 text-sm text-muted-foreground">{t("loading")}</div>
            ) : visibleSkills.length === 0 ? (
              <div className="rounded-[24px] border border-dashed border-border bg-background px-6 py-10 text-center">
                <h3 className="text-base font-semibold text-foreground">{t("skillsEmptyTitle")}</h3>
                <p className="mt-2 text-sm leading-7 text-muted-foreground">{t("skillsEmptyDescription")}</p>
              </div>
            ) : (
              <div className="space-y-3">
                {visibleSkills.map((skill) => {
                  const issue = primaryIssue(skill);
                  const path = currentSourcePath(skill);

                  return (
                    <article
                      key={skillKey(skill)}
                      className="rounded-[24px] border border-border bg-background/80 p-4 transition-all hover:border-accent/20 hover:bg-muted/20 hover:shadow-sm"
                    >
                      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                        <div className="min-w-0 flex-1 space-y-3">
                          <div className="flex flex-wrap items-center gap-2">
                            <h3 className="text-lg font-semibold text-foreground">{skill.name}</h3>
                            <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                              {sourceLabel(t, skill.source_kind)}
                            </span>
                            <SyncBadge t={t} syncState={skill.sync_state} />
                            <ProblemIndicator t={t} issue={skill.status === "problem" ? issue : null} />
                          </div>

                          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                            <div className="rounded-[18px] border border-border/70 bg-card px-3 py-3">
                              <div className="text-xs font-medium text-muted-foreground">{t("skillsGlobalVersionState")}</div>
                              <div className="mt-2 text-sm text-foreground">{presenceLabel(t, skill.global.present, skill.global.valid)}</div>
                            </div>
                            <div className="rounded-[18px] border border-border/70 bg-card px-3 py-3">
                              <div className="text-xs font-medium text-muted-foreground">{t("skillsCurrentCliVersionState", { tool: TOOL_LABELS[scope] })}</div>
                              <div className="mt-2 text-sm text-foreground">{presenceLabel(t, skill.local.present, skill.local.valid)}</div>
                            </div>
                          </div>

                          <div className="rounded-[18px] border border-border/70 bg-card px-3 py-3">
                            <div className="text-xs font-medium text-muted-foreground">{t("skillsNextAction")}</div>
                            <div className="mt-2 text-sm leading-6 text-foreground">{primaryActionLabel(t, skill.primary_action)}</div>
                          </div>

                          {issue ? (
                            <div className="rounded-[18px] border border-red-500/20 bg-red-500/5 px-3 py-3">
                              <div className="text-xs font-medium text-red-600 dark:text-red-300">{issueScopeLabel(t, issue.scope)}</div>
                              <div className="mt-2 text-sm leading-6 text-foreground">{issueMessage(t, issue)}</div>
                            </div>
                          ) : null}

                          <div className="rounded-[18px] border border-border/70 bg-card px-3 py-3">
                            <div className="text-xs font-medium text-muted-foreground">{t("skillsCurrentVersion")}</div>
                            <div className="mt-2 break-all text-sm text-foreground">{path || "-"}</div>
                          </div>
                        </div>

                        <div className="flex shrink-0 flex-col items-start gap-2 lg:items-end">
                          <div className="flex flex-wrap items-center gap-2 lg:justify-end">
                            {skill.primary_action !== "none" ? (
                              <button type="button" onClick={() => void performPrimaryAction(skill)} disabled={mutating} className={PRIMARY_BUTTON}>
                                {primaryActionLabel(t, skill.primary_action)}
                              </button>
                            ) : null}
                            {skill.can_delete ? (
                              <button type="button" onClick={() => setPendingDelete(skill)} disabled={mutating} className={DANGER_BUTTON}>
                                {t("delete")}
                              </button>
                            ) : null}
                            <button type="button" onClick={() => setSelectedKey(skillKey(skill))} className={SECONDARY_BUTTON}>
                              {t("skillsActionViewDetails")}
                            </button>
                            {path ? (
                              <button type="button" onClick={() => void openFolder(path)} className={GHOST_BUTTON}>
                                {t("skillsOpenFolder")}
                              </button>
                            ) : null}
                          </div>
                        </div>
                      </div>
                    </article>
                  );
                })}
              </div>
            )}
          </div>
        </section>

        {selectedSkill ? (
          <ModalShell
            title={selectedSkill.name}
            subtitle={selectedSkill.description || t("skillsNoDescription")}
            closeLabel={t("close")}
            onClose={() => setSelectedKey(null)}
          >
            <div className="space-y-5">
              <DetailSection title={t("skillsSectionSync")}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <SyncBadge t={t} syncState={selectedSkill.sync_state} />
                    <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                      {sourceLabel(t, selectedSkill.source_kind)}
                    </span>
                    <ProblemIndicator t={t} issue={selectedSkill.status === "problem" ? primaryIssue(selectedSkill) : null} />
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {selectedSkill.primary_action !== "none" ? (
                      <button type="button" onClick={() => void performPrimaryAction(selectedSkill)} disabled={mutating} className={PRIMARY_BUTTON}>
                        {primaryActionLabel(t, selectedSkill.primary_action)}
                      </button>
                    ) : null}
                    {selectedSkill.can_delete ? (
                      <button type="button" onClick={() => setPendingDelete(selectedSkill)} disabled={mutating} className={DANGER_BUTTON}>
                        {t("delete")}
                      </button>
                    ) : null}
                  </div>
                </div>
                <InfoGrid
                  items={[
                    { label: t("skillsStatus"), value: statusLabel(t, selectedSkill.status) },
                    { label: t("skillsDefinitionStatus"), value: definitionStatusLabel(t, selectedSkill.definition_status) },
                    { label: t("skillsSyncState"), value: syncStateLabel(t, selectedSkill.sync_state) },
                    { label: t("skillsNextAction"), value: primaryActionLabel(t, selectedSkill.primary_action) },
                  ]}
                />
                {selectedSkill.action_source_path || selectedSkill.action_target_path ? (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div>
                      <div className="text-xs font-medium text-muted-foreground">{t("skillsActionSourcePath")}</div>
                      <PathValue value={selectedSkill.action_source_path} />
                    </div>
                    <div>
                      <div className="text-xs font-medium text-muted-foreground">{t("skillsActionTargetPath")}</div>
                      <PathValue value={selectedSkill.action_target_path} />
                    </div>
                  </div>
                ) : null}
              </DetailSection>

              {(selectedSkill.issues ?? []).length > 0 ? (
                <DetailSection title={t("skillsIssuesTitle")}>
                  <div className="space-y-3">
                    {(selectedSkill.issues ?? []).map((issue, index) => (
                      <div key={`${issue.scope}:${issue.code}:${issue.path || index}`} className="rounded-2xl border border-border/70 bg-card px-4 py-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="rounded-full border border-border px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                            {issueScopeLabel(t, issue.scope)}
                          </span>
                          <span className="rounded-full border border-red-500/30 bg-red-500/10 px-2.5 py-1 text-[11px] font-medium text-red-600 dark:text-red-300">
                            {issueMessage(t, issue)}
                          </span>
                        </div>
                        <div className="mt-3 grid gap-3 sm:grid-cols-2">
                          <div>
                            <div className="text-xs font-medium text-muted-foreground">{t("skillsIssueType")}</div>
                            <div className="mt-1 text-foreground">{issue.code}</div>
                          </div>
                          <div>
                            <div className="text-xs font-medium text-muted-foreground">{t("skillsIssuePath")}</div>
                            <PathValue value={issue.path || issue.details?.expected_path || issue.details?.actual_path || ""} />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </DetailSection>
              ) : null}

              <DetailSection title={t("skillsSectionGlobal")}>
                {selectedSkill.global.current_path ? (
                  <>
                    <InfoGrid
                      items={[
                        { label: t("skillsDefinitionStatus"), value: definitionStatusLabel(t, selectedSkill.global.definition_status) },
                        { label: t("skillsStatus"), value: presenceLabel(t, selectedSkill.global.present, selectedSkill.global.valid) },
                      ]}
                    />
                    <div>
                      <div className="text-xs font-medium text-muted-foreground">{t("skillsGlobalCurrentPath")}</div>
                      <PathValue value={selectedSkill.global.current_path} />
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button type="button" onClick={() => void openFolder(selectedSkill.global.current_path)} className={GHOST_BUTTON}>
                        {t("skillsOpenFolder")}
                      </button>
                    </div>
                  </>
                ) : (
                  <div className="text-muted-foreground">{t("skillsNoGlobalVersion")}</div>
                )}
              </DetailSection>

              <DetailSection title={t("skillsSectionLocal")}>
                {selectedSkill.local.path ? (
                  <>
                    <InfoGrid
                      items={[
                        { label: t("skillsDefinitionStatus"), value: definitionStatusLabel(t, selectedSkill.local.definition_status) },
                        { label: t("skillsStatus"), value: presenceLabel(t, selectedSkill.local.present, selectedSkill.local.valid) },
                      ]}
                    />
                    <div>
                      <div className="text-xs font-medium text-muted-foreground">{t("skillsLocalPath")}</div>
                      <PathValue value={selectedSkill.local.path} />
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button type="button" onClick={() => void openFolder(selectedSkill.local.path)} className={GHOST_BUTTON}>
                        {t("skillsOpenFolder")}
                      </button>
                    </div>
                  </>
                ) : (
                  <div className="text-muted-foreground">{t("skillsNoLocalVersion")}</div>
                )}
              </DetailSection>

              {(((selectedSkill.binding.actual ?? []).length > 0) || selectedSkill.binding.method) ? (
                <details className="rounded-[24px] border border-border bg-background/50 p-4">
                  <summary className="cursor-pointer list-none text-sm font-semibold text-foreground">
                    {t("skillsSectionMoreInfo")}
                  </summary>
                  <div className="mt-4 space-y-4 text-sm">
                    {selectedSkill.binding.method ? (
                      <div>
                        <div className="text-xs font-medium text-muted-foreground">{t("skillsBindingMethod")}</div>
                        <div className="mt-1 text-foreground">{selectedSkill.binding.method}</div>
                      </div>
                    ) : null}
                    {(selectedSkill.binding.actual ?? []).length > 0 ? (
                      <div>
                        <div className="text-xs font-medium text-muted-foreground">{t("skillsInstalledPath")}</div>
                        <div className="mt-2 space-y-2">
                          {(selectedSkill.binding.actual ?? []).map((install) => (
                            <div key={install.path} className="rounded-xl border border-border/70 bg-background px-3 py-3">
                              <div className="break-all text-foreground">{install.path}</div>
                              <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                                <span>{install.method}</span>
                                {!install.valid && install.problem_reason ? <span>{t("skillsPresenceInvalid")}</span> : null}
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                </details>
              ) : null}
            </div>
          </ModalShell>
        ) : null}

        {pendingDelete ? (
          <ConfirmPanel
            title={t(pendingDelete.id > 0 ? "skillsDeleteManagedConfirmTitle" : "skillsDeleteDirectoryConfirmTitle")}
            affectedFiles={[
              {
                path: pendingDelete.delete_path || currentSourcePath(pendingDelete) || pendingDelete.name,
                tool: pendingDelete.source_kind === "global" ? "global" : scope,
                operation: "delete",
              },
            ]}
            confirmLabel={t("delete")}
            loading={mutating}
            onCancel={() => setPendingDelete(null)}
            onConfirm={() => void confirmDelete()}
          />
        ) : null}
      </div>
    </div>
  );
}
