import { openPath as open } from "@tauri-apps/plugin-opener";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import ConfirmPanel, { type AffectedFile } from "../../components/ConfirmPanel";
import SyncStatus from "../../components/SyncStatus";
import { fetchRaw, mutateAPI } from "../../lib/api";

type ConfigFileInfo = {
  path: string;
  tool: string;
  description: string;
  doc_url: string;
  exists: boolean;
};

type SyncChange = {
  tool: string;
  file_path: string;
  old_hash: string;
  new_hash: string;
  exists: boolean;
};

type SyncStatusResponse = {
  changes_count: number;
  conflicts: number;
  changes?: SyncChange[];
};

type BackupRecordRaw = {
  ID?: number;
  id?: number;
  Tool?: string;
  tool?: string;
  FilePath?: string;
  file_path?: string;
  BackupPath?: string;
  backup_path?: string;
  Slot?: number;
  slot?: number;
  CreatedAt?: string;
  created_at?: string;
  TriggerType?: string;
  trigger_type?: string;
};

type BackupRecord = {
  id: number;
  tool: string;
  filePath: string;
  backupPath: string;
  slot: number | null;
  createdAt: string;
  triggerType: string;
};

type BackupMutationResponse = {
  affected_files: AffectedFile[];
};

type PendingAction =
  | {
      type: "manual-backup";
      title: string;
      confirmLabel: string;
      affectedFiles: AffectedFile[];
    }
  | {
      type: "restore";
      title: string;
      confirmLabel: string;
      backup: BackupRecord;
      affectedFiles: AffectedFile[];
    };

type ResultPanel = {
  title: string;
  affectedFiles: AffectedFile[];
};

function getFileKey(tool: string, filePath: string) {
  return `${tool}::${filePath}`;
}

function getFolderPath(filePath: string) {
  const normalized = filePath.trim().replace(/[\\/]+$/, "");
  if (!normalized) {
    return ".";
  }

  if (normalized === "/") {
    return "/";
  }

  if (/^[A-Za-z]:\\?$/.test(normalized)) {
    return normalized.endsWith("\\") ? normalized : `${normalized}\\`;
  }

  const lastSeparator = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  if (lastSeparator === -1) {
    return ".";
  }
  if (lastSeparator === 0) {
    return normalized[0] === "/" ? "/" : ".";
  }

  const driveRootMatch = normalized.slice(0, lastSeparator + 1).match(/^[A-Za-z]:\\$/);
  if (driveRootMatch) {
    return driveRootMatch[0];
  }

  return normalized.slice(0, lastSeparator);
}

function normalizeBackupRecord(record: BackupRecordRaw): BackupRecord | null {
  const id = record.id ?? record.ID;
  const tool = record.tool ?? record.Tool;
  const filePath = record.file_path ?? record.FilePath;
  const backupPath = record.backup_path ?? record.BackupPath ?? "";
  const slotValue = record.slot ?? record.Slot;
  const createdAt = record.created_at ?? record.CreatedAt;
  const triggerType = record.trigger_type ?? record.TriggerType ?? "manual";

  if (typeof id !== "number" || !tool || !filePath || !createdAt) {
    return null;
  }

  return {
    id,
    tool,
    filePath,
    backupPath,
    slot: typeof slotValue === "number" ? slotValue : null,
    createdAt,
    triggerType,
  };
}

function formatHash(hash: string) {
  return hash ? hash.slice(0, 12) : "—";
}

function buildConflictDiff(change: SyncChange) {
  return [
    "--- ours / last synced",
    `- hash: ${change.old_hash || "unknown"}`,
    `- exists: yes`,
    "+++ external / current",
    `+ hash: ${change.new_hash || "unknown"}`,
    `+ exists: ${change.exists ? "yes" : "no"}`,
    `! status: ${change.exists ? "hash mismatch" : "missing externally"}`,
  ].join("\n");
}

export default function FilesBackups() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<ConfigFileInfo[]>([]);
  const [syncStatus, setSyncStatus] = useState<SyncStatusResponse | null>(null);
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [expandedConflictKey, setExpandedConflictKey] = useState<string | null>(null);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [loadingBackups, setLoadingBackups] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [resolvingKey, setResolvingKey] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resultPanel, setResultPanel] = useState<ResultPanel | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const loadFilesAndStatus = async (showSpinner = true) => {
    if (showSpinner) {
      setLoadingFiles(true);
    } else {
      setRefreshing(true);
    }

    try {
      const [nextFiles, nextStatus] = await Promise.all([
        fetchRaw<ConfigFileInfo[]>("config/files"),
        fetchRaw<SyncStatusResponse>("config/sync/status"),
      ]);
      setFiles(nextFiles);
      setSyncStatus(nextStatus);
    } finally {
      setLoadingFiles(false);
      setRefreshing(false);
    }
  };

  const loadBackups = async () => {
    setLoadingBackups(true);
    try {
      const response = await fetchRaw<BackupRecordRaw[]>("config/backups");
      setBackups(response.map(normalizeBackupRecord).filter((item): item is BackupRecord => item !== null));
    } finally {
      setLoadingBackups(false);
    }
  };

  const reloadAll = async (showSpinner = false) => {
    try {
      await Promise.all([loadFilesAndStatus(showSpinner), loadBackups()]);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
      throw err;
    }
  };

  useEffect(() => {
    void reloadAll(true).catch(() => {});
  }, []);

  const changesByKey = useMemo(() => {
    const entries: Array<[string, SyncChange]> = (syncStatus?.changes ?? []).map((change) => [
      getFileKey(change.tool, change.file_path),
      change,
    ]);
    return new Map<string, SyncChange>(entries);
  }, [syncStatus]);

  const openFolder = async (filePath: string) => {
    const folderPath = getFolderPath(filePath);
    if (!folderPath) {
      setError(t("syncStatus"));
      return;
    }

    setError(null);
    try {
      await open(folderPath);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    }
  };

  const resolveConflict = async (
    tool: string,
    filePath: string,
    strategy: "keep_external" | "keep_ours"
  ) => {
    const key = getFileKey(tool, filePath);
    setResolvingKey(key);
    setError(null);
    setStatus(null);

    try {
      await mutateAPI<{ status: "ok" }>("POST", "config/sync/resolve", {
        tool,
        file_path: filePath,
        strategy,
      });
      await reloadAll();
      setExpandedConflictKey(null);
      setStatus(t("synced"));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setResolvingKey(null);
    }
  };

  const requestManualBackup = () => {
    setError(null);
    setStatus(null);
    setPendingAction({
      type: "manual-backup",
      title: t("manualBackup"),
      confirmLabel: t("manualBackup"),
      affectedFiles: files.map((file) => ({
        path: file.path,
        tool: file.tool,
        operation: "backup",
      })),
    });
  };

  const requestRestore = (backup: BackupRecord) => {
    setError(null);
    setStatus(null);
    setPendingAction({
      type: "restore",
      title: t("restore"),
      confirmLabel: t("restore"),
      backup,
      affectedFiles: [
        {
          path: backup.filePath,
          tool: backup.tool,
          operation: "restore",
        },
      ],
    });
  };

  const confirmPendingAction = async () => {
    if (!pendingAction) {
      return;
    }

    setSubmitting(true);
    setError(null);
    setStatus(null);

    try {
      if (pendingAction.type === "manual-backup") {
        await reloadAll();
      }

      const response =
        pendingAction.type === "manual-backup"
          ? await mutateAPI<BackupMutationResponse>("POST", "config/backups")
          : await mutateAPI<BackupMutationResponse>(
              "POST",
              `config/backups/${pendingAction.backup.id}/restore`
            );

      setPendingAction(null);
      setResultPanel({
        title: pendingAction.type === "manual-backup" ? t("manualBackup") : t("restore"),
        affectedFiles: response.affected_files ?? [],
      });
      await reloadAll();
      setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSubmitting(false);
    }
  };

  const filesErrorState = !loadingFiles && files.length === 0 && !syncStatus;
  const conflictsCount =
    syncStatus?.changes !== undefined
      ? syncStatus.changes.length
      : Math.max(syncStatus?.changes_count ?? 0, syncStatus?.conflicts ?? 0);

  return (
    <div className="h-full overflow-y-auto pr-1">
      <div className="flex flex-col gap-4 pb-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold text-foreground">{t("filesBackups")}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{t("syncStatus")}</p>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <SyncStatus />
            <button
              type="button"
              onClick={requestManualBackup}
              disabled={loadingFiles || submitting || refreshing}
              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting && pendingAction?.type === "manual-backup" ? t("loading") : t("manualBackup")}
            </button>
          </div>
        </div>

        {error ? (
          <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-500">
            {error}
          </div>
        ) : null}

        {status ? (
          <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-600 dark:text-emerald-400">
            {status}
          </div>
        ) : null}

      <div className="space-y-4">
        <section className="rounded-xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-foreground">{t("filesBackups")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {refreshing
                  ? t("loading")
                  : conflictsCount > 0
                    ? `${conflictsCount} ${t("conflict")}`
                    : t("synced")}
              </p>
            </div>
            <button
              type="button"
              onClick={() => reloadAll()}
              disabled={refreshing || loadingFiles || submitting}
              className="rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            >
              {refreshing ? t("loading") : t("syncStatus")}
            </button>
          </div>

          <div className="mt-4">
            {loadingFiles ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("loading")}
              </div>
            ) : filesErrorState ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("syncStatus")}
              </div>
            ) : files.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("noConflicts")}
              </div>
            ) : (
              <div className="grid gap-3 2xl:grid-cols-2">
                {files.map((file) => {
                  const key = getFileKey(file.tool, file.path);
                  const change = changesByKey.get(key);
                  const isConflict = Boolean(change);
                  const isExpanded = expandedConflictKey === key;
                  const rowClass = isConflict
                    ? "border-orange-500/40 bg-orange-500/10"
                    : "border-border bg-background/40";

                  return (
                    <div key={key} className={`min-w-0 rounded-xl border ${rowClass}`}>
                      <div
                        role={isConflict ? "button" : undefined}
                        tabIndex={isConflict ? 0 : -1}
                        onClick={() => {
                          if (isConflict) {
                            setExpandedConflictKey(isExpanded ? null : key);
                          }
                        }}
                        onKeyDown={(event) => {
                          if (!isConflict) {
                            return;
                          }
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            setExpandedConflictKey(isExpanded ? null : key);
                          }
                        }}
                        className={`grid gap-3 p-4 ${
                          isConflict ? "cursor-pointer" : ""
                        }`}
                      >
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div className="text-sm font-semibold text-foreground">{file.tool}</div>
                          <span
                            className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                              isConflict
                                ? "bg-orange-500 text-white"
                                : "border border-border text-muted-foreground"
                            }`}
                          >
                            {isConflict ? t("conflict") : t("synced")}
                          </span>
                        </div>
                        <div className="min-w-0 space-y-1">
                          <div className="break-words text-sm text-foreground">{file.path}</div>
                          {!file.exists ? (
                            <div className="mt-1 text-xs text-muted-foreground">Not found</div>
                          ) : null}
                        </div>
                        <div className="text-sm text-muted-foreground">{file.description || "—"}</div>
                        <div className="flex flex-wrap items-center gap-3">
                          {file.doc_url ? (
                            <a
                              href={file.doc_url}
                              target="_blank"
                              rel="noreferrer"
                              onClick={(event) => event.stopPropagation()}
                              className="text-sm text-accent underline-offset-2 hover:underline"
                            >
                              {t("docLink")}
                            </a>
                          ) : (
                            <span className="text-sm text-muted-foreground">—</span>
                          )}
                          <button
                            type="button"
                            onClick={(event) => {
                              event.stopPropagation();
                              openFolder(file.path);
                            }}
                            className="rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
                          >
                            {t("openFolder")}
                          </button>
                        </div>
                      </div>

                      {isConflict && isExpanded && change ? (
                        <div className="border-t border-orange-500/30 px-4 py-4">
                          <div className="rounded-lg border border-orange-500/20 bg-background/70 p-4">
                            <div className="mb-3 flex items-center justify-between gap-3">
                              <div className="text-sm font-medium text-foreground">Diff</div>
                              <div className="text-xs text-muted-foreground">
                                {formatHash(change.old_hash)} → {formatHash(change.new_hash)}
                              </div>
                            </div>
                            <pre className="overflow-x-auto rounded-lg border border-border bg-background p-3 font-mono text-xs leading-6 text-muted-foreground whitespace-pre-wrap">
                              {buildConflictDiff(change)}
                            </pre>
                          </div>

                          <div className="mt-4 flex flex-wrap gap-3">
                            <button
                              type="button"
                              onClick={() => resolveConflict(file.tool, file.path, "keep_external")}
                              disabled={resolvingKey === key}
                              className="rounded-lg border border-orange-500/40 px-4 py-2 text-sm text-orange-600 transition-colors hover:bg-orange-500/10 disabled:cursor-not-allowed disabled:opacity-60 dark:text-orange-300"
                            >
                              {resolvingKey === key ? t("loading") : t("keepExternal")}
                            </button>
                            <button
                              type="button"
                              onClick={() => resolveConflict(file.tool, file.path, "keep_ours")}
                              disabled={resolvingKey === key}
                              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
                            >
                              {resolvingKey === key ? t("loading") : t("keepOurs")}
                            </button>
                          </div>
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </section>

        <section className="rounded-xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-foreground">{t("backupHistory")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">{backups.length} items</p>
            </div>
          </div>

          <div className="mt-4">
            {loadingBackups ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("loading")}
              </div>
            ) : backups.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
                {t("noBackups")}
              </div>
            ) : (
              <div className="space-y-3">
                {backups.map((backup) => (
                  <div
                    key={backup.id}
                    className="grid min-w-0 gap-3 rounded-xl border border-border bg-background/40 p-4 lg:grid-cols-[180px_90px_minmax(0,1fr)_120px_70px_100px]"
                  >
                    <div className="text-sm text-foreground">
                      {new Date(backup.createdAt).toLocaleString()}
                    </div>
                    <div className="text-sm font-medium text-foreground">{backup.tool}</div>
                    <div className="min-w-0 break-words text-sm text-muted-foreground">{backup.filePath}</div>
                    <div className="text-sm text-muted-foreground">{backup.triggerType || "—"}</div>
                    <div className="text-sm text-muted-foreground">
                      {backup.slot === null ? "—" : backup.slot}
                    </div>
                    <div className="flex justify-start lg:justify-end">
                      <button
                        type="button"
                        onClick={() => requestRestore(backup)}
                        disabled={submitting}
                        className="rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        {t("restore")}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>
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

      {resultPanel ? (
        <ConfirmPanel
          title={resultPanel.title}
          affectedFiles={resultPanel.affectedFiles}
          onCancel={() => setResultPanel(null)}
          onConfirm={() => setResultPanel(null)}
          confirmLabel={t("save")}
        />
      ) : null}
      </div>
    </div>
  );
}
