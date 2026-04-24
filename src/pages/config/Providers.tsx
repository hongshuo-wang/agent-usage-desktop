import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import ConfirmPanel, { type AffectedFile } from "../../components/ConfirmPanel";
import SyncStatus from "../../components/SyncStatus";
import ToolTargets, { type ToolTarget } from "../../components/ToolTargets";
import { fetchRaw, mutateAPI } from "../../lib/api";

type Profile = {
  id: number;
  name: string;
  is_active: boolean;
  config: string;
  has_api_key: boolean;
  tool_targets: Partial<Record<ToolTarget, boolean>>;
  created_at: string;
  updated_at: string;
};

type ProviderConfig = {
  api_key?: string;
  base_url?: string;
  model?: string;
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
  apiKey: string;
  baseUrl: string;
  model: string;
  toolTargets: Partial<Record<ToolTarget, boolean>>;
};

type ActivateResponse = {
  affected_files: AffectedFile[];
};

type ProfileMutationResponse = {
  id?: number;
  affected_files: AffectedFile[];
};

const emptyForm: FormState = {
  name: "",
  apiKey: "",
  baseUrl: "",
  model: "",
  toolTargets: {},
};

function parseProviderConfig(config: string): ProviderConfig {
  try {
    const parsed = JSON.parse(config) as ProviderConfig;
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

function profileToForm(profile: Profile): FormState {
  const config = parseProviderConfig(profile.config);

  return {
    name: profile.name,
    apiKey: "",
    baseUrl: config.base_url ?? "",
    model: config.model ?? "",
    toolTargets: profile.tool_targets ?? {},
  };
}

function buildConfig(form: FormState): string {
  return JSON.stringify({
    api_key: form.apiKey,
    base_url: form.baseUrl,
    model: form.model,
  });
}

function isProviderActivationFile(file: ConfigFileInfo): boolean {
  if (file.tool !== "claude") {
    return true;
  }

  return !(
    file.path.endsWith(".claude.json") ||
    file.description.toLowerCase().includes("mcp")
  );
}

export default function Providers() {
  const { t } = useTranslation();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [selectedID, setSelectedID] = useState<number | "new" | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [activating, setActivating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [confirmFiles, setConfirmFiles] = useState<AffectedFile[] | null>(null);

  const selectedProfile = useMemo(
    () => profiles.find((profile) => profile.id === selectedID) ?? null,
    [profiles, selectedID]
  );
  const isCreating = selectedID === "new";

  const loadProfiles = async (nextSelectedID?: number | "new") => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchRaw<Profile[]>("config/profiles");
      setProfiles(data);

      if (nextSelectedID !== undefined) {
        setSelectedID(nextSelectedID);
        if (nextSelectedID === "new") {
          setForm(emptyForm);
        } else {
          const nextProfile = data.find((profile) => profile.id === nextSelectedID);
          setForm(nextProfile ? profileToForm(nextProfile) : emptyForm);
        }
        return;
      }

      const currentStillExists = data.some((profile) => profile.id === selectedID);
      const nextProfile = currentStillExists
        ? data.find((profile) => profile.id === selectedID) ?? null
        : data.find((profile) => profile.is_active) ?? data[0] ?? null;

      setSelectedID(nextProfile?.id ?? null);
      setForm(nextProfile ? profileToForm(nextProfile) : emptyForm);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProfiles();
  }, []);

  const selectProfile = (profile: Profile) => {
    setSelectedID(profile.id);
    setForm(profileToForm(profile));
    setError(null);
    setStatus(null);
  };

  const startCreate = () => {
    setSelectedID("new");
    setForm(emptyForm);
    setError(null);
    setStatus(null);
  };

  const updateForm = (updates: Partial<FormState>) => {
    setForm((current) => ({ ...current, ...updates }));
  };

  const validateForm = () => {
    if (!form.name.trim()) {
      return `${t("profileName")} is required.`;
    }
    if (!form.apiKey.trim() && (isCreating || !selectedProfile?.has_api_key)) {
      return `${t("apiKey")} is required.`;
    }
    return null;
  };

  const saveProfile = async () => {
    const validationError = validateForm();
    if (validationError) {
      setError(validationError);
      return;
    }

    setSaving(true);
    setError(null);
    setStatus(null);
    try {
      if (isCreating) {
        const response = await mutateAPI<ProfileMutationResponse>("POST", "config/profiles", {
          name: form.name.trim(),
          config: buildConfig(form),
          tool_targets: form.toolTargets,
        });
        await loadProfiles(response.id);
        setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
      } else if (selectedProfile) {
        const response = await mutateAPI<ProfileMutationResponse>("PUT", `config/profiles/${selectedProfile.id}`, {
          name: form.name.trim(),
          config: buildConfig(form),
          tool_targets: form.toolTargets,
        });
        await loadProfiles(selectedProfile.id);
        setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async () => {
    if (!selectedProfile) {
      return;
    }

    setSaving(true);
    setError(null);
    setStatus(null);
    try {
      const response = await mutateAPI<ProfileMutationResponse>("DELETE", `config/profiles/${selectedProfile.id}`);
      await loadProfiles();
      setStatus(`${t("synced")} · ${(response.affected_files ?? []).length} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setSaving(false);
    }
  };

  const previewActivation = async () => {
    if (!selectedProfile) {
      return;
    }

    setActivating(true);
    setError(null);
    setStatus(null);
    try {
      const files = await fetchRaw<ConfigFileInfo[]>("config/files");
      const enabledTargets = selectedProfile.tool_targets ?? {};
      setConfirmFiles(
        files
          .filter((file) => Boolean(enabledTargets[file.tool as ToolTarget]))
          .filter(isProviderActivationFile)
          .map((file) => ({
            path: file.path,
            tool: file.tool,
            operation: "write",
          }))
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setActivating(false);
    }
  };

  const confirmActivation = async () => {
    if (!selectedProfile) {
      return;
    }

    setActivating(true);
    setError(null);
    try {
      const response = await mutateAPI<ActivateResponse>(
        "POST",
        `config/profiles/${selectedProfile.id}/activate`
      );
      setConfirmFiles(null);
      await loadProfiles(selectedProfile.id);
      const affectedCount = response.affected_files?.length ?? 0;
      setStatus(`${t("profileActivated")} · ${affectedCount} ${t("affectedFiles")}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("syncStatus"));
    } finally {
      setActivating(false);
    }
  };

  const showEditor = isCreating || selectedProfile !== null;
  const canActivate = Boolean(selectedProfile && !selectedProfile.is_active);

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-foreground">{t("providers")}</h2>
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

      <div className="grid flex-1 min-h-0 gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border p-4">
            <h3 className="text-sm font-semibold text-foreground">{t("profiles", "Profiles")}</h3>
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
            ) : profiles.length === 0 && !isCreating ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                {t("noProfiles")}
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
                {profiles.map((profile) => {
                  const selected = profile.id === selectedID;
                  return (
                    <button
                      key={profile.id}
                      type="button"
                      onClick={() => selectProfile(profile)}
                      className={`w-full rounded-lg border px-3 py-3 text-left transition-colors ${
                        selected
                          ? "border-accent bg-accent/10 text-foreground"
                          : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">{profile.name}</span>
                        {profile.is_active ? (
                          <span className="rounded-full bg-accent px-2 py-0.5 text-xs text-white">
                            {t("active")}
                          </span>
                        ) : null}
                      </div>
                      <div className="mt-1 truncate text-xs text-muted-foreground">
                        {new Date(profile.updated_at).toLocaleString()}
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </aside>

        <main className="min-h-0 overflow-y-auto rounded-xl border border-border bg-card p-5">
          {!showEditor ? (
            <div className="rounded-lg border border-dashed border-border px-4 py-8 text-sm text-muted-foreground">
              {t("noProfiles")}
            </div>
          ) : (
            <div className="max-w-2xl space-y-6">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-base font-semibold text-foreground">
                  {isCreating ? t("create") : t("edit")}
                </h3>
                {selectedProfile?.is_active ? (
                  <span className="rounded-full bg-accent px-3 py-1 text-xs font-medium text-white">
                    {t("active")}
                  </span>
                ) : null}
              </div>

              <div className="space-y-4">
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
                  <span className="text-sm font-medium text-foreground">{t("apiKey")}</span>
                  <input
                    type="password"
                    value={form.apiKey}
                    placeholder={selectedProfile?.has_api_key ? "••••••••••••••••" : ""}
                    onChange={(event) => updateForm({ apiKey: event.target.value })}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                  />
                  {selectedProfile?.has_api_key ? (
                    <p className="text-xs text-muted-foreground">
                      {t("apiKey")} is stored securely. Leave blank to keep the existing key.
                    </p>
                  ) : null}
                </label>

                <label className="block space-y-2">
                  <span className="text-sm font-medium text-foreground">{t("baseUrl")}</span>
                  <input
                    type="url"
                    value={form.baseUrl}
                    onChange={(event) => updateForm({ baseUrl: event.target.value })}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                  />
                </label>

                <label className="block space-y-2">
                  <span className="text-sm font-medium text-foreground">{t("model")}</span>
                  <input
                    type="text"
                    value={form.model}
                    onChange={(event) => updateForm({ model: event.target.value })}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-accent"
                  />
                </label>
              </div>

              <section className="space-y-3">
                <div>
                  <h4 className="text-sm font-semibold text-foreground">{t("syncTargets")}</h4>
                </div>
                <ToolTargets
                  targets={form.toolTargets}
                  onChange={(toolTargets) => updateForm({ toolTargets })}
                />
              </section>

              <div className="flex flex-wrap items-center gap-3 border-t border-border pt-5">
                <button
                  type="button"
                  onClick={saveProfile}
                  disabled={saving || activating}
                  className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {saving ? t("loading") : isCreating ? t("create") : t("save")}
                </button>
                {!isCreating && selectedProfile ? (
                  <>
                    <button
                      type="button"
                      onClick={previewActivation}
                      disabled={!canActivate || saving || activating}
                      className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {activating ? t("loading") : t("activate")}
                    </button>
                    <button
                      type="button"
                      onClick={deleteProfile}
                      disabled={saving || activating}
                      className="rounded-lg border border-red-500/40 px-4 py-2 text-sm text-red-500 transition-colors hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {t("delete")}
                    </button>
                  </>
                ) : null}
              </div>
            </div>
          )}
        </main>
      </div>

      {confirmFiles ? (
        <ConfirmPanel
          title={t("confirmChanges")}
          affectedFiles={confirmFiles}
          onCancel={() => setConfirmFiles(null)}
          onConfirm={confirmActivation}
          confirmLabel={t("activate")}
          loading={activating}
        />
      ) : null}
    </div>
  );
}
