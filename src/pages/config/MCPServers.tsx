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

type MCPServer = {
  id: number;
  name: string;
  command: string;
  args: string;
  env: string;
  enabled: boolean;
  targets: Partial<Record<ToolTarget, boolean>>;
  created_at: string;
};

type ConfigFileInfo = {
  path: string;
  tool: ToolTarget | string;
  description: string;
  doc_url: string;
  exists: boolean;
};

type FormState = {
  name: string;
  command: string;
  args: string;
  env: string;
  enabled: boolean;
  targets: Partial<Record<ToolTarget, boolean>>;
};

type MutationResponse = {
  affected_files: AffectedFile[];
};

type CreateMutationResponse = MutationResponse & {
  id: number;
};

type PendingAction =
  | {
      type: "save";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
      mode: "create" | "edit";
      serverID: number | null;
      snapshot: FormState;
    }
  | {
      type: "delete";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
      snapshot: MCPServer;
    };

const emptyForm: FormState = {
  name: "",
  command: "",
  args: "[]",
  env: "{}",
  enabled: true,
  targets: {},
};

function serverToForm(server: MCPServer): FormState {
  return {
    name: server.name,
    command: server.command,
    args: server.args?.trim() ? server.args : "[]",
    env: server.env?.trim() ? server.env : "{}",
    enabled: server.enabled,
    targets: server.targets ?? {},
  };
}

function normalizeJSONText(value: string, fallback: string): string {
  return value.trim() ? value : fallback;
}

function isMCPConfigFile(file: ConfigFileInfo): boolean {
  const description = file.description.toLowerCase();

  if (file.tool === "claude") {
    return file.path.endsWith(".claude.json") || description.includes("mcp");
  }

  if (file.tool === "codex") {
    return file.path.endsWith("config.toml") || (
      description.includes("config") && !description.includes("auth")
    );
  }

  return true;
}

function buildAffectedFilesPreview(
  files: ConfigFileInfo[],
  targets: Partial<Record<ToolTarget, boolean>>,
  operation: "write" | "delete"
): AffectedFile[] {
  return files
    .filter((file) => TOOLS.includes(file.tool as ToolTarget))
    .filter((file) => Boolean(targets[file.tool as ToolTarget]))
    .filter(isMCPConfigFile)
    .map((file) => ({
      path: file.path,
      tool: file.tool,
      operation,
    }))
    .filter((file, index, list) =>
      list.findIndex((entry) => entry.path === file.path && entry.tool === file.tool) === index
    );
}

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) {
    return "Name is required.";
  }
  if (!form.command.trim()) {
    return "Command is required.";
  }

  try {
    const parsed = JSON.parse(normalizeJSONText(form.args, "[]")) as unknown;
    if (!Array.isArray(parsed) || parsed.some((value) => typeof value !== "string")) {
      return "Arguments must be a JSON array.";
    }
  } catch {
    return "Arguments must be a valid JSON array.";
  }

  try {
    const parsed = JSON.parse(normalizeJSONText(form.env, "{}")) as unknown;
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
      return "Environment variables must be a JSON object.";
    }
    if (Object.values(parsed).some((value) => typeof value !== "string")) {
      return "Environment variables must be a JSON object.";
    }
  } catch {
    return "Environment variables must be a valid JSON object.";
  }

  return null;
}

export default function MCPServers() {
  const { t } = useTranslation();
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [selectedID, setSelectedID] = useState<number | "new" | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [previewingAction, setPreviewingAction] = useState<"save" | "delete" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null);

  const selectedServer = useMemo(
    () => servers.find((server) => server.id === selectedID) ?? null,
    [selectedID, servers]
  );
  const isCreating = selectedID === "new";
  const showEditor = isCreating || selectedServer !== null;

  const loadServers = async (nextSelectedID?: number | "new" | null) => {
    setLoading(true);
    setError(null);

    try {
      const data = await fetchRaw<MCPServer[]>("config/mcp");
      setServers(data);

      if (nextSelectedID !== undefined) {
        setSelectedID(nextSelectedID);
        if (nextSelectedID === "new" || nextSelectedID === null) {
          setForm(emptyForm);
        } else {
          const nextServer = data.find((server) => server.id === nextSelectedID);
          setForm(nextServer ? serverToForm(nextServer) : emptyForm);
        }
        return;
      }

      const currentStillExists = data.some((server) => server.id === selectedID);
      const nextServer = currentStillExists
        ? data.find((server) => server.id === selectedID) ?? null
        : data[0] ?? null;

      setSelectedID(nextServer?.id ?? null);
      setForm(nextServer ? serverToForm(nextServer) : emptyForm);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadServers();
  }, []);

  const updateForm = (updates: Partial<FormState>) => {
    setForm((current) => ({ ...current, ...updates }));
  };

  const selectServer = (server: MCPServer) => {
    setSelectedID(server.id);
    setForm(serverToForm(server));
    setError(null);
    setStatus(null);
  };

  const startCreate = () => {
    setSelectedID("new");
    setForm(emptyForm);
    setError(null);
    setStatus(null);
  };

  const previewAffectedFiles = async (
    targets: Partial<Record<ToolTarget, boolean>>,
    operation: "write" | "delete"
  ) => {
    const files = await fetchRaw<ConfigFileInfo[]>("config/files");
    return buildAffectedFilesPreview(files, targets, operation);
  };

  const requestSave = async () => {
    const validationError = validateForm(form);
    if (validationError) {
      setError(validationError);
      return;
    }

    setPreviewingAction("save");
    setError(null);
    setStatus(null);

    try {
      const affectedFiles = await previewAffectedFiles(form.targets, "write");
      const snapshot = {
        ...form,
        targets: { ...form.targets },
      };
      setPendingAction({
        type: "save",
        title: `${isCreating ? t("create") : t("save")} ${t("mcpServers")}`,
        confirmLabel: isCreating ? t("create") : t("save"),
        affectedFiles,
        mode: isCreating ? "create" : "edit",
        serverID: selectedServer?.id ?? null,
        snapshot,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setPreviewingAction(null);
    }
  };

  const requestDelete = async () => {
    if (!selectedServer) {
      return;
    }

    setPreviewingAction("delete");
    setError(null);
    setStatus(null);

    try {
      const affectedFiles = await previewAffectedFiles(selectedServer.targets ?? {}, "delete");
      setPendingAction({
        type: "delete",
        title: `${t("delete")} ${selectedServer.name}`,
        confirmLabel: t("delete"),
        affectedFiles,
        snapshot: {
          ...selectedServer,
          targets: { ...(selectedServer.targets ?? {}) },
        },
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
        command: pendingAction.snapshot.command.trim(),
        args: normalizeJSONText(pendingAction.snapshot.args, "[]"),
        env: normalizeJSONText(pendingAction.snapshot.env, "{}"),
      };

      let affectedFiles: AffectedFile[] = [];

      if (pendingAction.mode === "create") {
        const response = await mutateAPI<CreateMutationResponse>("POST", "config/mcp", {
          ...payload,
          enabled: pendingAction.snapshot.enabled,
          targets: pendingAction.snapshot.targets,
        });
        affectedFiles = response.affected_files ?? [];
        await loadServers(response.id);
      } else if (pendingAction.serverID !== null) {
        const updateResponse = await mutateAPI<MutationResponse>(
          "PUT",
          `config/mcp/${pendingAction.serverID}`,
          {
            ...payload,
            enabled: pendingAction.snapshot.enabled,
            targets: pendingAction.snapshot.targets,
          }
        );
        affectedFiles = [...(updateResponse.affected_files ?? [])];
        await loadServers(pendingAction.serverID);
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
        `config/mcp/${pendingAction.snapshot.id}`
      );
      await loadServers();
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
          <h2 className="text-lg font-semibold text-foreground">{t("mcpServers")}</h2>
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

      <div className="grid flex-1 min-h-0 gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border p-4">
            <h3 className="text-sm font-semibold text-foreground">{t("mcpServers")}</h3>
            <button
              type="button"
              onClick={startCreate}
              className="rounded-lg bg-accent px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-accent/90"
            >
              {t("create")}
            </button>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto p-3">
            {loading ? (
              <div className="px-2 py-6 text-sm text-muted-foreground">{t("loading")}</div>
            ) : servers.length === 0 && !isCreating ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                {t("noMCPServers")}
              </div>
            ) : (
              <div className="space-y-2">
                {isCreating ? (
                  <button
                    type="button"
                    className="w-full rounded-lg border border-accent bg-accent/10 px-3 py-3 text-left text-sm text-foreground"
                  >
                    {t("create")}
                  </button>
                ) : null}

                {servers.map((server) => {
                  const selected = server.id === selectedID;

                  return (
                    <div
                      key={server.id}
                      role="button"
                      tabIndex={0}
                      onClick={() => selectServer(server)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          selectServer(server);
                        }
                      }}
                      className={`w-full rounded-lg border px-3 py-3 text-left transition-colors ${
                        selected
                          ? "border-accent bg-accent/10 text-foreground"
                          : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">{server.name}</span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-xs ${
                            server.enabled
                              ? "bg-accent text-white"
                              : "border border-border text-muted-foreground"
                          }`}
                        >
                          {server.enabled ? t("enabled") : t("disabled")}
                        </span>
                      </div>
                      <div className="mt-1 truncate text-xs text-muted-foreground">
                        {server.command}
                      </div>
                      <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                        {TOOLS.map((tool) => (
                          <div key={tool} className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              checked={Boolean(server.targets?.[tool])}
                              readOnly
                              tabIndex={-1}
                              className="h-3.5 w-3.5 rounded border-border accent-accent"
                            />
                            <span>{TOOL_LABELS[tool]}</span>
                          </div>
                        ))}
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
              {t("noMCPServers")}
            </div>
          ) : (
            <div className="max-w-3xl space-y-6">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-base font-semibold text-foreground">
                  {isCreating ? t("create") : t("edit")}
                </h3>
                {!isCreating && selectedServer ? (
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

                <label className="block space-y-2">
                  <span className="text-sm font-medium text-foreground">{t("command")}</span>
                  <input
                    type="text"
                    value={form.command}
                    onChange={(event) => updateForm({ command: event.target.value })}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                  />
                </label>
              </div>

              <label className="block space-y-2">
                <span className="text-sm font-medium text-foreground">{t("arguments")}</span>
                <textarea
                  value={form.args}
                  onChange={(event) => updateForm({ args: event.target.value })}
                  rows={5}
                  className="w-full rounded-lg border border-border bg-background px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-accent"
                />
              </label>

              <label className="block space-y-2">
                <span className="text-sm font-medium text-foreground">{t("envVars")}</span>
                <textarea
                  value={form.env}
                  onChange={(event) => updateForm({ env: event.target.value })}
                  rows={6}
                  className="w-full rounded-lg border border-border bg-background px-3 py-2 font-mono text-sm outline-none transition-colors focus:border-accent"
                />
              </label>

              <section className="space-y-3">
                <h4 className="text-sm font-semibold text-foreground">{t("syncTargets")}</h4>
                <ToolTargets
                  targets={form.targets}
                  onChange={(targets) => updateForm({ targets })}
                />
              </section>

              <label className="flex items-center gap-3 rounded-lg border border-border bg-background/40 px-4 py-3">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(event) => updateForm({ enabled: event.target.checked })}
                  className="h-4 w-4 rounded border-border accent-accent"
                />
                <span className="text-sm text-foreground">{t("enabled")}</span>
              </label>

              <div className="flex flex-wrap items-center gap-3 border-t border-border pt-5">
                <button
                  type="button"
                  onClick={requestSave}
                  disabled={submitting || previewingAction !== null}
                  className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {previewingAction === "save"
                    ? t("loading")
                    : isCreating
                      ? t("create")
                      : t("save")}
                </button>

                {!isCreating && selectedServer ? (
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
          confirmLabel={pendingAction.confirmLabel}
          onCancel={() => setPendingAction(null)}
          onConfirm={pendingAction.type === "delete" ? confirmDelete : confirmSave}
          loading={submitting}
        />
      ) : null}
    </div>
  );
}
