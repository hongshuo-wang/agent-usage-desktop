import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchRaw } from "../lib/api";

type SyncStatusResponse = {
  changes_count: number;
  conflicts: number;
};

type ViewState = "loading" | "synced" | "conflicts" | "error";

export default function SyncStatus() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<SyncStatusResponse | null>(null);
  const [viewState, setViewState] = useState<ViewState>("loading");

  useEffect(() => {
    let active = true;

    const loadStatus = async () => {
      try {
        const next = await fetchRaw<SyncStatusResponse>("config/sync/status");
        if (!active) {
          return;
        }

        setStatus(next);
        setViewState(next.conflicts > 0 || next.changes_count > 0 ? "conflicts" : "synced");
      } catch {
        if (active) {
          setViewState("error");
        }
      }
    };

    loadStatus();

    return () => {
      active = false;
    };
  }, []);

  const conflictCount = (status?.conflicts ?? 0) + (status?.changes_count ?? 0);
  const isSynced = viewState === "synced";
  const dotClass =
    viewState === "conflicts"
      ? "bg-orange-500"
      : viewState === "error"
        ? "bg-muted-foreground"
        : "bg-emerald-500";

  let text = t("loading");
  if (isSynced) {
    text = `${t("synced")} · ${t("noConflicts")}`;
  } else if (viewState === "conflicts") {
    text = `${conflictCount} ${t("conflict")}`;
  } else if (viewState === "error") {
    text = t("syncStatus");
  }

  return (
    <div className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-sm text-muted-foreground">
      <span className={`h-2.5 w-2.5 rounded-full ${dotClass}`} aria-hidden="true" />
      <span>{text}</span>
    </div>
  );
}
