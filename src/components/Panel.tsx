import type { ReactNode } from "react";

interface PanelProps {
  title?: ReactNode;
  hint?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
  testId?: string;
}

/** The single card surface used by every dashboard band. */
export default function Panel({
  title,
  hint,
  actions,
  children,
  className = "",
  bodyClassName = "panel-body",
  testId,
}: PanelProps) {
  const hasHead = Boolean(title || hint || actions);
  return (
    <section data-testid={testId} className={`panel ${className}`}>
      {hasHead ? (
        <div className="panel-head">
          <div className="min-w-0">
            {title ? <h2 className="panel-title">{title}</h2> : null}
            {hint ? <p className="panel-hint mt-0.5">{hint}</p> : null}
          </div>
          {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={bodyClassName}>{children}</div>
    </section>
  );
}
