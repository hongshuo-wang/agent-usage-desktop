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

  return (
    <section
      aria-labelledby="dashboard-insight-heading"
      data-testid="dashboard-band-insight"
      className="dashboard-insight"
    >
      <h2 id="dashboard-insight-heading" className="text-sm font-semibold">{t("usageOverview")}</h2>
      <div className="dashboard-insight-facts">
        {peak && (
          <button
            type="button"
            aria-label={t("peakUsage")}
            title={t("viewRelatedSessions")}
            onClick={() => onOpenDay(peak.day)}
            className="focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span>{t("peakUsage")}</span>
            <strong>{peak.timestamp}</strong>
            <small>{fmtTokens(peak.totalTokens)} {t("tokens")}</small>
          </button>
        )}
        {topModel && (
          <button
            type="button"
            aria-label={t("topModel")}
            title={t("viewRelatedSessions")}
            onClick={() => onOpenModel(topModel.key)}
            className="focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span>{t("topModel")}</span>
            <strong>{topModel.key}</strong>
            <small>{fmtTokens(topModel.totalTokens)} {t("tokens")}</small>
          </button>
        )}
        {topProject && (
          <button
            type="button"
            aria-label={t("topProject")}
            title={t("viewRelatedSessions")}
            onClick={() => onOpenProject(topProject.key)}
            className="focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            <span>{t("topProject")}</span>
            <strong>{topProject.key}</strong>
            <small>{fmtTokens(topProject.totalTokens)} {t("tokens")}</small>
          </button>
        )}
      </div>
    </section>
  );
}
