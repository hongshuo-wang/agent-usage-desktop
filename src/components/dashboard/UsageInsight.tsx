import { useTranslation } from "react-i18next";
import type { DashboardInsight } from "../../lib/dashboardPresentation";
import { fmtTokens } from "../../lib/utils";

type UsageInsightProps = {
  insight: DashboardInsight;
  onOpenDay: (day: string) => void;
  onOpenModel: (model: string) => void;
  onOpenProject: (project: string) => void;
};

export default function UsageInsight({ insight, onOpenDay, onOpenModel, onOpenProject }: UsageInsightProps) {
  const { t } = useTranslation();
  const { peak, topModel, topProject } = insight;

  if (peak === null && topModel === null && topProject === null) return null;

  const facts = [
    peak && { key: "peakUsage", value: peak.timestamp, tokens: peak.totalTokens, onClick: () => onOpenDay(peak.day) },
    topModel && { key: "topModel", value: topModel.key, tokens: topModel.totalTokens, onClick: () => onOpenModel(topModel.key) },
    topProject && { key: "topProject", value: topProject.key, tokens: topProject.totalTokens, onClick: () => onOpenProject(topProject.key) },
  ].filter((fact) => fact !== null);

  return (
    <section
      aria-labelledby="dashboard-insight-heading"
      data-testid="dashboard-band-insight"
      className="panel"
    >
      <div className="panel-head">
        <h2 id="dashboard-insight-heading" className="panel-title">{t("usageOverview")}</h2>
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-2.5 pb-2 sm:grid-cols-3">
        {facts.map((fact) => (
          <button
            key={fact.key}
            type="button"
            aria-label={t(fact.key)}
            title={t("viewRelatedSessions")}
            onClick={fact.onClick}
            className="inset min-w-0 px-3.5 py-3 text-left transition-colors hover:bg-accent-dim focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span className="block truncate text-2xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t(fact.key)}
            </span>
            <strong className="mt-1 block truncate text-[13px] font-semibold">{fact.value}</strong>
            <small className="mt-0.5 block truncate text-2xs tabular-nums text-muted-foreground">
              {fmtTokens(fact.tokens)} {t("tokens")}
            </small>
          </button>
        ))}
      </div>
    </section>
  );
}
