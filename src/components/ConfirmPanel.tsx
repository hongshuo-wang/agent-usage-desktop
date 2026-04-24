import { useEffect, useId, useRef } from "react";
import { useTranslation } from "react-i18next";

export type AffectedFile = {
  path: string;
  tool: string;
  operation: string;
  diff?: string;
};

type ConfirmPanelProps = {
  title?: string;
  affectedFiles: AffectedFile[];
  onConfirm: () => void;
  onCancel: () => void;
  confirmLabel?: string;
  cancelLabel?: string;
  loading?: boolean;
};

export default function ConfirmPanel({
  title,
  affectedFiles,
  onConfirm,
  onCancel,
  confirmLabel,
  cancelLabel,
  loading = false,
}: ConfirmPanelProps) {
  const { t } = useTranslation();
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const cancelButtonRef = useRef<HTMLButtonElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    previousFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;

    const focusTarget = cancelButtonRef.current ?? panelRef.current;
    focusTarget?.focus();

    return () => {
      previousFocusRef.current?.focus();
    };
  }, []);

  useEffect(() => {
    if (loading) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCancel();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [loading, onCancel]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="max-h-[85vh] w-full max-w-3xl overflow-hidden rounded-xl border border-border bg-card shadow-xl"
      >
        <div className="border-b border-border px-5 py-4">
          <h2 id={titleId} className="text-base font-semibold text-foreground">
            {title ?? t("confirmChanges")}
          </h2>
        </div>

        <div className="space-y-4 overflow-y-auto px-5 py-4">
          <div>
            <h3 className="mb-3 text-sm font-semibold text-foreground">{t("affectedFiles")}</h3>
            {affectedFiles.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
                {t("noConflicts")}
              </div>
            ) : (
              <div className="space-y-3">
                {affectedFiles.map((file, index) => (
                  <div
                    key={`${file.path}-${file.tool}-${file.operation}-${index}`}
                    className="rounded-lg border border-border bg-background/60 p-4"
                  >
                    <div className="break-all text-sm font-medium text-foreground">{file.path}</div>
                    <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
                      <span className="rounded-md border border-border px-2 py-1">{file.tool}</span>
                      <span className="rounded-md border border-border px-2 py-1">{file.operation}</span>
                    </div>
                    {file.diff ? (
                      <pre className="mt-3 overflow-x-auto rounded-lg border border-border bg-background p-3 text-xs text-muted-foreground whitespace-pre-wrap">
                        {file.diff}
                      </pre>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="flex items-center justify-end gap-3 border-t border-border px-5 py-4">
          <button
            ref={cancelButtonRef}
            type="button"
            onClick={onCancel}
            disabled={loading}
            className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
          >
            {cancelLabel ?? t("cancel")}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={loading}
            className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {loading ? t("loading") : confirmLabel ?? t("confirmChanges")}
          </button>
        </div>
      </div>
    </div>
  );
}
