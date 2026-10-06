import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  hint?: ReactNode;
  actions?: ReactNode;
}

/** Gives every route a consistent heading without repeating the shell branding. */
export default function PageHeader({ title, hint, actions }: PageHeaderProps) {
  return (
    <header className="flex min-w-0 flex-wrap items-center justify-between gap-x-5 gap-y-3">
      <div className="min-w-0">
        <h1 className="truncate text-xl font-semibold tracking-tight">{title}</h1>
        {hint ? <p className="mt-0.5 min-w-0 text-xs text-muted-foreground">{hint}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </header>
  );
}
