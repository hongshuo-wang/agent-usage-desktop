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
    <section data-testid="dashboard-band-core" className="panel">
      <div className="grid min-w-0 gap-x-8 gap-y-5 py-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
        <div className="min-w-0 lg:border-r lg:border-border lg:pr-8">
          <p className="label">{t("totalTokens")}</p>
          <strong
            data-testid="primary-token-total"
            className="metric mt-1.5 block text-[2.25rem] leading-none text-accent"
          >
            {fmtTokens(stats.total_tokens)}
          </strong>
          <p className="mt-2.5 truncate text-xs text-muted-foreground">
            {rangeDetail} · {stats.total_calls} {t("apiCalls")}
          </p>
        </div>

        <dl
          data-testid="secondary-metrics"
          className="grid min-w-0 grid-cols-2 gap-2.5 lg:grid-cols-4"
        >
          <div className="inset min-w-0 px-3 py-2.5">
            <dt className="label truncate">{t("sessions")}</dt>
            <dd className="metric mt-1 text-lg">{stats.total_sessions}</dd>
          </div>
          <div className="inset min-w-0 px-3 py-2.5">
            <dt className="label truncate">{t("userMessages")}</dt>
            <dd className="metric mt-1 text-lg">{stats.total_prompts}</dd>
          </div>
          <div className="inset min-w-0 px-3 py-2.5">
            <dt className="label truncate">{t("cacheServedRatio")}</dt>
            <dd className="metric mt-1 text-lg">{(stats.cache_hit_rate * 100).toFixed(1)}%</dd>
          </div>
          <div className="inset min-w-0 px-3 py-2.5">
            <dt className="label truncate">{t("localCostEstimate")}</dt>
            <dd data-testid="estimated-cost" className="metric mt-1 text-lg text-muted-foreground">
              {fmtCost(stats.total_cost)}
            </dd>
          </div>
        </dl>
      </div>
    </section>
  );
}
