import { useTranslation } from "react-i18next";
import type { DashboardStats } from "../../lib/types";
import { fmtCost, fmtTokens } from "../../lib/utils";

type TokenSummaryProps = {
  stats: DashboardStats;
  rangeDetail: string;
};

export default function TokenSummary({ stats, rangeDetail }: TokenSummaryProps) {
  const { t } = useTranslation();

  return (
    <section
      data-testid="dashboard-band-core"
      className="dashboard-summary"
    >
      <div className="dashboard-token-hero">
        <p className="dashboard-eyebrow">{t("totalTokens")}</p>
        <strong data-testid="primary-token-total" className="dashboard-token-value">
          {fmtTokens(stats.total_tokens)}
        </strong>
        <p className="dashboard-range">
          {rangeDetail} · {stats.total_calls} {t("apiCalls")}
        </p>
      </div>

      <dl
        data-testid="secondary-metrics"
        className="dashboard-secondary-metrics"
      >
        <div className="dashboard-secondary-metric">
          <dt>{t("sessions")}</dt>
          <dd>{stats.total_sessions}</dd>
        </div>
        <div className="dashboard-secondary-metric">
          <dt>{t("userMessages")}</dt>
          <dd>{stats.total_prompts}</dd>
        </div>
        <div className="dashboard-secondary-metric">
          <dt>{t("cacheServedRatio")}</dt>
          <dd>{(stats.cache_hit_rate * 100).toFixed(1)}%</dd>
        </div>
        <div className="dashboard-secondary-metric dashboard-cost-metric">
          <dt>{t("localCostEstimate")}</dt>
          <dd data-testid="estimated-cost" className="text-muted-foreground">
            {fmtCost(stats.total_cost)}
          </dd>
        </div>
      </dl>
    </section>
  );
}
