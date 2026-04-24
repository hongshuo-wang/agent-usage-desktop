const TOOLS = ["claude", "codex", "opencode", "openclaw"] as const;
export type ToolTarget = (typeof TOOLS)[number];

const TOOL_LABELS: Record<ToolTarget, string> = {
  claude: "Claude Code",
  codex: "Codex",
  opencode: "OpenCode",
  openclaw: "OpenClaw",
};

type ToolTargetsProps = {
  targets: Partial<Record<ToolTarget, boolean>>;
  onChange: (targets: Partial<Record<ToolTarget, boolean>>) => void;
};

export default function ToolTargets({ targets, onChange }: ToolTargetsProps) {
  return (
    <div className="flex flex-wrap gap-2">
      {TOOLS.map((tool) => {
        const checked = Boolean(targets[tool]);

        return (
          <label
            key={tool}
            className={`flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors ${
              checked
                ? "border-accent bg-accent text-white"
                : "border-border text-muted-foreground hover:text-foreground"
            }`}
          >
            <input
              type="checkbox"
              checked={checked}
              onChange={() => onChange({ ...targets, [tool]: !checked })}
              className="h-4 w-4 rounded border-border accent-current"
            />
            <span>{TOOL_LABELS[tool]}</span>
          </label>
        );
      })}
    </div>
  );
}

export { TOOLS, TOOL_LABELS };
