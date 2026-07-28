# Apple 风格使用分析界面改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将使用分析页改造成 Token 优先的 macOS 原生风格工作台，同时保留当前新版的筛选、趋势、吞吐和会话下钻能力。

**Architecture:** 保持后端 API 与 `Dashboard` 的请求/并发保护不变，新增一个纯展示函数生成可验证的用量摘要，并将 Token 主摘要与洞察拆成两个小型组件。`Layout`、全局颜色 token、工具栏和 ECharts 选项在现有 React/Tailwind v4 架构内渐进调整，不引入新依赖。

**Tech Stack:** React 19、TypeScript 6、Tailwind CSS v4、ECharts 6、React Router 7、Vitest、Testing Library、Tauri v2。

---

## File Map

- Create: `src/lib/dashboardPresentation.ts` — 从现有时间序列和排行生成可验证的最高点、主要模型与主要项目。
- Create: `src/lib/dashboardPresentation.test.ts` — 覆盖峰值计算、并列规则、空数据和不变性。
- Create: `src/components/dashboard/TokenSummary.tsx` — Token 主指标和低权重辅助指标。
- Create: `src/components/dashboard/UsageInsight.tsx` — 最高用量、主要模型/项目及会话下钻入口。
- Create: `src/components/dashboard/DashboardSummary.test.tsx` — 两个新组件的语义、层级与交互测试。
- Modify: `src/components/Layout.tsx` — macOS 风格侧栏、窄窗口导航和当前态语义。
- Modify: `src/components/Layout.test.tsx` — 导航当前态、桌面/移动结构和单一主滚动容器测试。
- Modify: `src/components/TimeRangeSelector.tsx` — 将现有查询能力收敛为紧凑工具栏样式。
- Modify: `src/components/TimeRangeSelector.test.tsx` — 保护筛选弹层、刷新和键盘关闭行为。
- Modify: `src/pages/Dashboard.tsx` — 接入摘要组件和洞察，调整页面顺序、骨架和图表样式。
- Modify: `src/pages/Dashboard.test.tsx` — 保护 Token 优先层级、洞察下钻、状态与原有功能。
- Modify: `src/components/ChartCard.tsx` — 统一图表文字、网格线、tooltip 和 resize 表现。
- Modify: `src/lib/utils.ts` — 把多彩高饱和图表色改为蓝色主序列和克制的辅助色。
- Modify: `src/lib/utils.test.ts` — 固定新的图表调色板契约。
- Modify: `src/lib/locales/zh.json` — 新增洞察和辅助指标文案。
- Modify: `src/lib/locales/en.json` — 新增对应英文文案。
- Modify: `src/lib/locales/locales.test.ts` — 保证中英文 key 对齐。
- Modify: `src/styles/globals.css` — 系统字体、中性表面、Apple 蓝、焦点、响应式与 reduced-motion 基线。

### Task 1: Add a verifiable dashboard presentation model

**Files:**
- Create: `src/lib/dashboardPresentation.ts`
- Create: `src/lib/dashboardPresentation.test.ts`

- [ ] **Step 1: Write the failing presentation tests**

```ts
import { describe, expect, it } from "vitest";
import { buildDashboardInsight } from "./dashboardPresentation";

describe("buildDashboardInsight", () => {
  it("finds the highest token bucket and keeps rankings independent", () => {
    const rows = [
      { date: "2025-01-01", input_tokens: 10, output_tokens: 20, cache_read: 5, cache_create: 1 },
      { date: "2025-01-02 15:00", input_tokens: 30, output_tokens: 40, cache_read: 20, cache_create: 10 },
    ];
    const models = [
      { key: "sonnet", total_tokens: 80, total_cost: 1, sessions: 2, calls: 3, cache_hit_rate: 0.5, unknown_price: false },
      { key: "opus", total_tokens: 20, total_cost: 1, sessions: 1, calls: 1, cache_hit_rate: 0, unknown_price: false },
    ];
    const projects = [
      { key: "console", total_tokens: 60, total_cost: 1, sessions: 2, calls: 3, cache_hit_rate: 0.5, unknown_price: false },
    ];

    expect(buildDashboardInsight(rows, models, projects)).toEqual({
      peak: { timestamp: "2025-01-02 15:00", day: "2025-01-02", totalTokens: 100 },
      topModel: { key: "sonnet", totalTokens: 80 },
      topProject: { key: "console", totalTokens: 60 },
    });
    expect(rows[1].input_tokens).toBe(30);
  });

  it("uses the earliest bucket for equal peaks and omits missing facts", () => {
    const rows = [
      { date: "2025-01-01", input_tokens: 5, output_tokens: 5, cache_read: 0, cache_create: 0 },
      { date: "2025-01-02", input_tokens: 10, output_tokens: 0, cache_read: 0, cache_create: 0 },
    ];

    expect(buildDashboardInsight(rows, [], [])).toEqual({
      peak: { timestamp: "2025-01-01", day: "2025-01-01", totalTokens: 10 },
      topModel: null,
      topProject: null,
    });
    expect(buildDashboardInsight([], [], [])).toEqual({ peak: null, topModel: null, topProject: null });
  });
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `npm test -- src/lib/dashboardPresentation.test.ts`

Expected: FAIL because `./dashboardPresentation` does not exist.

- [ ] **Step 3: Implement the pure presentation model**

```ts
import type { TokensRow, UsageBreakdown } from "./types";

export type DashboardInsight = {
  peak: { timestamp: string; day: string; totalTokens: number } | null;
  topModel: { key: string; totalTokens: number } | null;
  topProject: { key: string; totalTokens: number } | null;
};

function rowTotal(row: TokensRow): number {
  return row.input_tokens + row.output_tokens + row.cache_read + row.cache_create;
}

function topBreakdown(rows: UsageBreakdown[]): { key: string; totalTokens: number } | null {
  const top = rows.reduce<UsageBreakdown | null>((best, row) => (
    !best || row.total_tokens > best.total_tokens ? row : best
  ), null);
  return top ? { key: top.key, totalTokens: top.total_tokens } : null;
}

export function buildDashboardInsight(
  tokens: TokensRow[],
  models: UsageBreakdown[],
  projects: UsageBreakdown[],
): DashboardInsight {
  const peakRow = tokens.reduce<TokensRow | null>((best, row) => (
    !best || rowTotal(row) > rowTotal(best) ? row : best
  ), null);

  return {
    peak: peakRow ? {
      timestamp: peakRow.date,
      day: peakRow.date.match(/^\d{4}-\d{2}-\d{2}/)?.[0] ?? peakRow.date,
      totalTokens: rowTotal(peakRow),
    } : null,
    topModel: topBreakdown(models),
    topProject: topBreakdown(projects),
  };
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `npm test -- src/lib/dashboardPresentation.test.ts`

Expected: 2 tests PASS.

- [ ] **Step 5: Commit the presentation model**

```bash
git add src/lib/dashboardPresentation.ts src/lib/dashboardPresentation.test.ts
git commit -m "feat: add usage insight presentation model"
```

### Task 2: Build the Token-first summary and insight components

**Files:**
- Create: `src/components/dashboard/TokenSummary.tsx`
- Create: `src/components/dashboard/UsageInsight.tsx`
- Create: `src/components/dashboard/DashboardSummary.test.tsx`
- Modify: `src/lib/locales/zh.json`
- Modify: `src/lib/locales/en.json`
- Modify: `src/lib/locales/locales.test.ts`

- [ ] **Step 1: Write failing component and locale tests**

Create `DashboardSummary.test.tsx` with the complete component coverage:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { DashboardStats } from "../../lib/types";
import TokenSummary from "./TokenSummary";
import UsageInsight from "./UsageInsight";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

const stats: DashboardStats = {
  total_tokens: 410, total_cost: 1.2345, priced_cost_usd: 1, unpriced_records: 2,
  legacy_cost_usd: 0.2345, pricing_last_synced_at: "2025-01-02T05:06:07Z",
  total_sessions: 7, total_prompts: 13, total_calls: 19, cache_hit_rate: 0.25,
  input_tokens: 111, output_tokens: 222, cache_read: 33, cache_create: 44,
};

describe("dashboard summary", () => {
  it("puts tokens before muted secondary metrics and renders no invented comparison", () => {
    render(<TokenSummary stats={stats} rangeDetail="2025-01-01 至 2025-01-07" />);
    const primary = screen.getByTestId("primary-token-total");
    expect(primary).toHaveTextContent("410");
    expect(primary.compareDocumentPosition(screen.getByTestId("secondary-metrics")))
      .toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(screen.getByTestId("estimated-cost")).toHaveClass("text-muted-foreground");
    expect(screen.queryByText(/较上周|last period/i)).not.toBeInTheDocument();
  });

  it("opens independently verifiable peak, model, and project sessions", async () => {
    const user = userEvent.setup();
    const onOpenDay = vi.fn();
    const onOpenModel = vi.fn();
    const onOpenProject = vi.fn();
    render(
      <UsageInsight
        insight={{
          peak: { timestamp: "2025-01-03 12:00", day: "2025-01-03", totalTokens: 410 },
          topModel: { key: "sonnet", totalTokens: 200 },
          topProject: { key: "console", totalTokens: 150 },
        }}
        onOpenDay={onOpenDay}
        onOpenModel={onOpenModel}
        onOpenProject={onOpenProject}
      />,
    );
    await user.click(screen.getByRole("button", { name: "peakUsage" }));
    await user.click(screen.getByRole("button", { name: "topModel" }));
    await user.click(screen.getByRole("button", { name: "topProject" }));
    expect(onOpenDay).toHaveBeenCalledWith("2025-01-03");
    expect(onOpenModel).toHaveBeenCalledWith("sonnet");
    expect(onOpenProject).toHaveBeenCalledWith("console");
  });

  it("renders nothing when no insight fact is available", () => {
    const { container } = render(<UsageInsight insight={{ peak: null, topModel: null, topProject: null }} onOpenDay={vi.fn()} onOpenModel={vi.fn()} onOpenProject={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });
});
```

Extend `locales.test.ts` so the new keys must exist in both files:

```ts
for (const key of ["usageOverview", "peakUsage", "topModel", "topProject", "viewRelatedSessions", "cacheServedRatio"]) {
  expect(zh).toHaveProperty(key);
  expect(en).toHaveProperty(key);
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `npm test -- src/components/dashboard/DashboardSummary.test.tsx src/lib/locales/locales.test.ts`

Expected: FAIL because the components and new locale keys do not exist.

- [ ] **Step 3: Implement `TokenSummary`**

```tsx
import { useTranslation } from "react-i18next";
import type { DashboardStats } from "../../lib/types";
import { fmtCost, fmtTokens } from "../../lib/utils";

export default function TokenSummary({ stats, rangeDetail }: { stats: DashboardStats; rangeDetail: string }) {
  const { t } = useTranslation();
  const metrics = [
    { key: "sessions", value: String(stats.total_sessions) },
    { key: "userMessages", value: String(stats.total_prompts) },
    { key: "cacheServedRatio", value: `${(stats.cache_hit_rate * 100).toFixed(1)}%` },
  ];

  return (
    <section data-testid="dashboard-band-core" className="dashboard-summary">
      <div className="dashboard-token-hero">
        <p className="dashboard-eyebrow">{t("totalTokens")}</p>
        <strong data-testid="primary-token-total" className="dashboard-token-value">{fmtTokens(stats.total_tokens)}</strong>
        <p className="dashboard-range">{rangeDetail} · {stats.total_calls} {t("apiCalls")}</p>
      </div>
      <dl data-testid="secondary-metrics" className="dashboard-secondary-metrics">
        {metrics.map((metric) => (
          <div key={metric.key} className="dashboard-secondary-metric">
            <dt>{t(metric.key)}</dt><dd>{metric.value}</dd>
          </div>
        ))}
        <div className="dashboard-secondary-metric dashboard-cost-metric">
          <dt>{t("localCostEstimate")}</dt>
          <dd data-testid="estimated-cost" className="text-muted-foreground">{fmtCost(stats.total_cost)}</dd>
        </div>
      </dl>
    </section>
  );
}
```

- [ ] **Step 4: Implement `UsageInsight`**

```tsx
import { useTranslation } from "react-i18next";
import type { DashboardInsight } from "../../lib/dashboardPresentation";
import { fmtTokens } from "../../lib/utils";

type Props = {
  insight: DashboardInsight;
  onOpenDay: (day: string) => void;
  onOpenModel: (model: string) => void;
  onOpenProject: (project: string) => void;
};

export default function UsageInsight({ insight, onOpenDay, onOpenModel, onOpenProject }: Props) {
  const { t } = useTranslation();
  if (!insight.peak && !insight.topModel && !insight.topProject) return null;
  return (
    <section data-testid="dashboard-band-insight" className="dashboard-insight" aria-labelledby="usage-insight-title">
      <h2 id="usage-insight-title">{t("usageOverview")}</h2>
      <div className="dashboard-insight-facts">
        {insight.peak && <button type="button" onClick={() => onOpenDay(insight.peak!.day)} aria-label={t("peakUsage")}><span>{t("peakUsage")}</span><strong>{insight.peak.timestamp}</strong><small>{fmtTokens(insight.peak.totalTokens)} {t("tokens")}</small></button>}
        {insight.topModel && <button type="button" onClick={() => onOpenModel(insight.topModel!.key)} aria-label={t("topModel")}><span>{t("topModel")}</span><strong>{insight.topModel.key}</strong><small>{fmtTokens(insight.topModel.totalTokens)} {t("tokens")}</small></button>}
        {insight.topProject && <button type="button" onClick={() => onOpenProject(insight.topProject!.key)} aria-label={t("topProject")}><span>{t("topProject")}</span><strong>{insight.topProject.key}</strong><small>{fmtTokens(insight.topProject.totalTokens)} {t("tokens")}</small></button>}
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Add exact locale copy**

Add to `zh.json`:

```json
"usageOverview": "用量概览",
"peakUsage": "最高用量",
"topModel": "主要模型",
"topProject": "主要项目",
"viewRelatedSessions": "查看相关会话",
"cacheServedRatio": "缓存服务比例"
```

Add to `en.json`:

```json
"usageOverview": "Usage overview",
"peakUsage": "Peak usage",
"topModel": "Top model",
"topProject": "Top project",
"viewRelatedSessions": "View related sessions",
"cacheServedRatio": "Cache served ratio"
```

- [ ] **Step 6: Run focused tests and verify GREEN**

Run: `npm test -- src/components/dashboard/DashboardSummary.test.tsx src/lib/locales/locales.test.ts`

Expected: component tests and locale parity tests PASS.

- [ ] **Step 7: Commit the summary components**

```bash
git add src/components/dashboard src/lib/locales/zh.json src/lib/locales/en.json src/lib/locales/locales.test.ts
git commit -m "feat: add token-first usage summary"
```

### Task 3: Refine the application shell and visual tokens

**Files:**
- Modify: `src/components/Layout.tsx:33-107`
- Modify: `src/components/Layout.test.tsx`
- Modify: `src/styles/globals.css:1-48`
- Modify: `src/lib/utils.ts:52`
- Modify: `src/lib/utils.test.ts`

- [ ] **Step 1: Write failing shell and palette tests**

Add assertions to `Layout.test.tsx`:

```tsx
render(<MemoryRouter initialEntries={["/"]}><Layout><div>content</div></Layout></MemoryRouter>);
expect(screen.getByTestId("desktop-navigation")).toBeInTheDocument();
expect(screen.getByTestId("mobile-navigation")).toBeInTheDocument();
for (const link of screen.getAllByRole("link", { name: "title" })) {
  expect(link).toHaveAttribute("aria-current", "page");
}
expect(screen.getAllByRole("link", { name: "sessionLog" })[0]).not.toHaveAttribute("aria-current");
expect(screen.getAllByRole("main")).toHaveLength(1);
expect(screen.getByRole("main")).toHaveClass("overflow-y-auto");
```

Add to `utils.test.ts`:

```ts
expect(CHART_COLORS).toEqual([
  "#0071e3", "#5ac8fa", "#64d2ff", "#8e8e93",
  "#34c759", "#ff9f0a", "#5856d6", "#af52de",
]);
```

- [ ] **Step 2: Run focused tests and verify RED**

Run: `npm test -- src/components/Layout.test.tsx src/lib/utils.test.ts`

Expected: FAIL because navigation test IDs/current-page semantics and the blue palette are absent.

- [ ] **Step 3: Add shell semantics and stable navigation structure**

In both desktop and mobile link maps, add:

```tsx
aria-current={isActive ? "page" : undefined}
```

Add `data-testid="desktop-navigation"` to the desktop `<nav>` and `data-testid="mobile-navigation"` to the mobile `<nav>`. Keep the existing routes and theme/language handlers unchanged. Change active navigation classes from `bg-accent-dim` to `bg-muted text-foreground`, keep 8px radii, and use `focus-visible:ring-2 focus-visible:ring-accent` on every link and icon button.

- [ ] **Step 4: Replace the global visual tokens and chart palette**

Use these exact light/dark tokens in `globals.css`:

```css
@theme {
  --color-background: #f5f5f7;
  --color-foreground: #1d1d1f;
  --color-card: #ffffff;
  --color-card-foreground: #1d1d1f;
  --color-border: #dedee2;
  --color-muted: #e8e8ed;
  --color-muted-foreground: #6e6e73;
  --color-accent: #0071e3;
  --color-accent-hover: #0077ed;
  --color-green: #248a3d;
  --color-accent-dim: rgba(0, 113, 227, 0.1);
}

:root {
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", sans-serif;
  letter-spacing: 0;
}

.dark {
  --color-background: #161617;
  --color-foreground: #f5f5f7;
  --color-card: #242426;
  --color-card-foreground: #f5f5f7;
  --color-border: #3a3a3c;
  --color-muted: #303033;
  --color-muted-foreground: #a1a1a6;
  --color-accent: #0a84ff;
  --color-accent-hover: #409cff;
  --color-green: #30d158;
  --color-accent-dim: rgba(10, 132, 255, 0.14);
}
```

Replace `CHART_COLORS` with the array asserted by the test.

- [ ] **Step 5: Run focused tests and build**

Run: `npm test -- src/components/Layout.test.tsx src/lib/utils.test.ts && npm run build`

Expected: focused tests PASS and Vite build completes without TypeScript errors.

- [ ] **Step 6: Commit the shell and tokens**

```bash
git add src/components/Layout.tsx src/components/Layout.test.tsx src/styles/globals.css src/lib/utils.ts src/lib/utils.test.ts
git commit -m "style: adopt native desktop visual system"
```

### Task 4: Integrate the Token-first dashboard hierarchy

**Files:**
- Modify: `src/pages/Dashboard.tsx:1-620`
- Modify: `src/pages/Dashboard.test.tsx`

- [ ] **Step 1: Replace old band-order expectations with failing hierarchy tests**

Update the first Dashboard test to assert the approved order:

```tsx
expect(await screen.findByTestId("primary-token-total")).toHaveTextContent("410");
expect(screen.getAllByTestId(/^dashboard-band-/).map((band) => band.dataset.testid)).toEqual([
  "dashboard-band-core",
  "dashboard-band-insight",
  "dashboard-band-analysis",
  "dashboard-band-detail",
]);
const core = screen.getByTestId("dashboard-band-core");
expect(within(core).getByTestId("estimated-cost")).toHaveTextContent("$1.23");
expect(within(core).getByTestId("estimated-cost")).toHaveClass("text-muted-foreground");
expect(within(core).queryByText(/last period|较上周/i)).not.toBeInTheDocument();
```

Add drill-down tests:

```tsx
it.each([
  ["peakUsage", "from=2025-01-03"],
  ["topModel", "model=sonnet"],
  ["topProject", "project=console"],
])("opens sessions from insight %s", async (name, query) => {
  renderDashboard();
  await userEvent.setup().click(await screen.findByRole("button", { name }));
  expect(screen.getByTestId("location")).toHaveTextContent(query);
});
```

- [ ] **Step 2: Run Dashboard tests and verify RED**

Run: `npm test -- src/pages/Dashboard.test.tsx`

Expected: FAIL because the new summary and insight bands are not integrated.

- [ ] **Step 3: Integrate the presentation model and components**

Add imports:

```tsx
import TokenSummary from "../components/dashboard/TokenSummary";
import UsageInsight from "../components/dashboard/UsageInsight";
import { buildDashboardInsight } from "../lib/dashboardPresentation";
```

Create the memoized model beside chart options:

```tsx
const usageInsight = useMemo(() => buildDashboardInsight(
  data?.tokens || [],
  data?.models || [],
  data?.projects || [],
), [data?.tokens, data?.models, data?.projects]);
```

Replace the old five-column `Metric` section with:

```tsx
<TokenSummary stats={stats} rangeDetail={rangeDetail} />
<UsageInsight
  insight={usageInsight}
  onOpenDay={(day) => openSessions({ from: day, to: day })}
  onOpenModel={(model) => openSessions({ model })}
  onOpenProject={(project) => openSessions({ project })}
/>
```

Delete the now-unused local `Metric` component. Keep `BandTitle`, `BreakdownRows`, `ModelUsageRows`, throughput state and every existing request callback intact.

- [ ] **Step 4: Reframe the page header and content bands**

Place this header immediately before `TimeRangeSelector`:

```tsx
<header className="dashboard-page-header">
  <div>
    <p className="dashboard-eyebrow">{rangeDetail}</p>
    <h1>{t("title")}</h1>
  </div>
</header>
```

Keep the analysis order as trend + model usage, followed by Agent composition + project ranking + throughput. Replace translucent band fills (`bg-card/40`, `bg-card/25`) with the named dashboard classes defined in Task 5 so visual hierarchy comes from spacing and surfaces rather than alternating stripes.

- [ ] **Step 5: Update the skeleton to match final geometry**

Replace the generic loop with stable summary, insight, analysis and detail placeholders:

```tsx
<div className="dashboard-skeleton" aria-label="loading">
  <section className="dashboard-summary"><Skeleton className="h-5 w-28" /><Skeleton className="mt-3 h-12 w-52" /><Skeleton className="h-28 w-full" /></section>
  <Skeleton className="h-20 w-full" />
  <div className="grid gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]"><Skeleton className="h-80" /><Skeleton className="h-80" /></div>
  <div className="grid gap-5 xl:grid-cols-3"><Skeleton className="h-64" /><Skeleton className="h-64" /><Skeleton className="h-64" /></div>
</div>
```

- [ ] **Step 6: Run Dashboard tests and build**

Run: `npm test -- src/pages/Dashboard.test.tsx && npm run build`

Expected: all Dashboard tests PASS, including existing concurrency, throughput and query-context tests; build completes.

- [ ] **Step 7: Commit the hierarchy integration**

```bash
git add src/pages/Dashboard.tsx src/pages/Dashboard.test.tsx
git commit -m "feat: prioritize tokens in usage dashboard"
```

### Task 5: Polish toolbar, chart surfaces, states, and responsive layout

**Files:**
- Modify: `src/components/TimeRangeSelector.tsx:100-190`
- Modify: `src/components/TimeRangeSelector.test.tsx`
- Modify: `src/components/ChartCard.tsx:1-120`
- Modify: `src/pages/Dashboard.test.tsx`
- Modify: `src/styles/globals.css`

- [ ] **Step 1: Add failing state and interaction coverage**

Keep the existing TimeRangeSelector interaction tests and add:

```tsx
expect(screen.getByRole("button", { name: "refresh" })).toHaveAttribute("title", "refresh");
expect(screen.getByRole("button", { name: "editQuery" })).toHaveAttribute("aria-expanded", "false");
await user.click(screen.getByRole("button", { name: "editQuery" }));
expect(screen.getByRole("dialog", { name: "queryEditor" })).toBeInTheDocument();
await user.keyboard("{Escape}");
expect(screen.queryByRole("dialog", { name: "queryEditor" })).not.toBeInTheDocument();
```

Add Dashboard state tests using `defaultAPIResponse` overrides:

```tsx
it("shows the composed empty state when every usage result is empty", async () => {
  vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
    if (path === "stats") return { ...stats, total_tokens: 0, total_calls: 0 };
    if (path === "usage-breakdown" || path === "tokens-over-time") return [];
    return defaultAPIResponse(path, params);
  });
  renderDashboard();
  expect(await screen.findByText("noUsageData")).toBeInTheDocument();
  expect(screen.getByText("noUsageDataDetail")).toBeInTheDocument();
});

it("keeps successful overview content when throughput fails", async () => {
  vi.mocked(fetchAPI).mockImplementation(async (path, params) => {
    if (path === "throughput") throw new Error("throughput unavailable");
    return defaultAPIResponse(path, params);
  });
  renderDashboard();
  expect(await screen.findByTestId("primary-token-total")).toHaveTextContent("410");
  expect(await screen.findByText("throughput unavailable")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run focused tests and verify RED where markup changed**

Run: `npm test -- src/components/TimeRangeSelector.test.tsx src/pages/Dashboard.test.tsx`

Expected: new state tests PASS against preserved behavior; toolbar assertions FAIL until the compact markup is finalized.

- [ ] **Step 3: Apply the compact toolbar styling without changing handlers**

Keep every callback and dialog field. Change only structural classes:

- outer summary row: `rounded-lg border border-border/80 bg-card px-3 py-2`;
- current query label: sentence case with `text-[11px] font-medium text-muted-foreground` and no uppercase tracking;
- edit button: 32px height, 8px radius, neutral border, blue focus ring;
- refresh button: stable `h-8 w-8`, icon-only with existing title and `aria-label`;
- active chips: 6px radius instead of full pills so they read as filters, not badges;
- dialog: maximum 8px container radius and a restrained `shadow-[0_18px_60px_rgba(0,0,0,0.18)]`.

- [ ] **Step 4: Add dashboard and chart CSS classes**

Append the exact responsive structure to `globals.css`:

```css
.dashboard-page-header { display:flex; align-items:flex-end; justify-content:space-between; gap:1rem; padding:.25rem .25rem .5rem; }
.dashboard-page-header h1 { margin-top:.25rem; font-size:1.75rem; line-height:1.1; font-weight:700; letter-spacing:0; }
.dashboard-eyebrow,.dashboard-range { color:var(--color-muted-foreground); font-size:.75rem; }
.dashboard-summary { display:grid; grid-template-columns:minmax(0,1.15fr) minmax(20rem,1fr); gap:2rem; align-items:end; padding:1.5rem; border:1px solid var(--color-border); border-radius:.5rem; background:var(--color-card); }
.dashboard-token-value { display:block; margin-top:.35rem; font-size:3.5rem; line-height:1; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:0; }
.dashboard-secondary-metrics { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:1px; overflow:hidden; border:1px solid var(--color-border); border-radius:.5rem; background:var(--color-border); }
.dashboard-secondary-metric { min-width:0; padding:.75rem; background:var(--color-card); }
.dashboard-secondary-metric dt { color:var(--color-muted-foreground); font-size:.6875rem; }
.dashboard-secondary-metric dd { margin-top:.25rem; font-size:1rem; font-weight:600; font-variant-numeric:tabular-nums; }
.dashboard-cost-metric dd { font-size:.875rem; font-weight:500; }
.dashboard-insight { padding:1rem 1.25rem; border-radius:.5rem; background:var(--color-muted); }
.dashboard-insight h2 { font-size:.75rem; font-weight:600; }
.dashboard-insight-facts { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:.75rem; margin-top:.75rem; }
.dashboard-insight-facts button { min-width:0; padding:.625rem; border-radius:.375rem; text-align:left; transition:background-color .2s ease,transform .2s ease; }
.dashboard-insight-facts button:hover { background:var(--color-card); }
.dashboard-insight-facts button:active { transform:translateY(1px); }
.dashboard-insight-facts span,.dashboard-insight-facts small { display:block; color:var(--color-muted-foreground); font-size:.6875rem; }
.dashboard-insight-facts strong { display:block; overflow:hidden; margin:.2rem 0; text-overflow:ellipsis; white-space:nowrap; font-size:.8125rem; }
@media (max-width: 767px) {
  .dashboard-summary { grid-template-columns:minmax(0,1fr); gap:1.25rem; padding:1rem; }
  .dashboard-insight-facts { grid-template-columns:minmax(0,1fr); }
  .dashboard-token-value { font-size:2.5rem; }
}
```

- [ ] **Step 5: Refine `ChartCard` theme defaults**

Keep renderer setup, event registration and ResizeObserver unchanged. At the start of `themed()`, resolve the active CSS tokens and use them for axes, legends, and tooltips:

```ts
const styles = getComputedStyle(document.documentElement);
const css = (name: string) => styles.getPropertyValue(name).trim();
const textColor = css("--color-muted-foreground");
const axisColor = css("--color-border");
const base = option as Record<string, unknown>;
const baseXAxis = (base.xAxis as Record<string, unknown>) || {};
const themeAxis = (axis: Record<string, unknown>) => ({
  ...axis,
  axisLine: { ...(axis.axisLine as object || {}), lineStyle: { color: axisColor } },
  axisLabel: { ...(axis.axisLabel as object || {}), color: textColor, fontSize: 11 },
  splitLine: { ...(axis.splitLine as object || {}), lineStyle: { color: axisColor, type: "dashed" as const } },
});
const baseYAxis = base.yAxis;
return {
  ...base,
  backgroundColor: "transparent",
  textStyle: { color: textColor, fontFamily: "-apple-system, BlinkMacSystemFont, sans-serif" },
  tooltip: { ...(base.tooltip as object || {}), backgroundColor: css("--color-card"), borderColor: axisColor, textStyle: { color: css("--color-foreground"), fontSize: 12 } },
  legend: { ...(base.legend as object || {}), textStyle: { color: textColor, fontSize: 11 } },
  xAxis: themeAxis(baseXAxis),
  yAxis: Array.isArray(baseYAxis)
    ? baseYAxis.map((axis) => themeAxis((axis as Record<string, unknown>) || {}))
    : themeAxis((baseYAxis as Record<string, unknown>) || {}),
};
```

This returns fresh axis objects so the caller-provided `option` object is not mutated.

- [ ] **Step 6: Run frontend tests and build**

Run: `npm test && npm run build && git diff --check`

Expected: all Vitest suites PASS, build succeeds, and `git diff --check` has no output.

- [ ] **Step 7: Commit toolbar and responsive polish**

```bash
git add src/components/TimeRangeSelector.tsx src/components/TimeRangeSelector.test.tsx src/components/ChartCard.tsx src/pages/Dashboard.test.tsx src/styles/globals.css
git commit -m "style: polish usage dashboard interactions"
```

### Task 6: End-to-end verification and visual QA

**Files:**
- Verify only; fix failures in the file that owns the behavior.

- [ ] **Step 1: Run all automated checks**

Run:

```bash
npm test
npm run build
go test ./...
git diff --check
```

Expected: every command exits 0; Vitest and Go report no failing tests; diff check emits no output.

- [ ] **Step 2: Build the local sidecar for the current macOS architecture**

Run:

```bash
mkdir -p src-tauri/binaries
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .
```

Expected: `src-tauri/binaries/agent-usage-aarch64-apple-darwin` exists and is executable. If `uname -m` returns `x86_64`, use `GOARCH=amd64` and output `agent-usage-x86_64-apple-darwin` instead.

- [ ] **Step 3: Start the desktop application**

Run: `npx tauri dev`

Expected: the Tauri window opens, the sidecar health check succeeds, and the usage dashboard loads without console errors.

- [ ] **Step 4: Capture and inspect the visual matrix**

Capture the usage page at these states and sizes:

- 1440 × 900, Chinese, light;
- 1440 × 900, English, dark;
- 900 × 700, Chinese, light;
- 390 × 844 equivalent narrow window, English, dark.

For each capture verify: Token is the largest number; cost is secondary; navigation current state is visible; no text overlaps; no page-level horizontal scrollbar; chart canvases are nonblank; loading/empty/error content does not resize controls; model/project/peak buttons show hover, focus and pressed feedback.

- [ ] **Step 5: Re-run checks after visual fixes**

Run:

```bash
npm test
npm run build
go test ./...
git diff --check
git status --short
```

Expected: all checks pass and status contains only intentional source/test changes or is clean after commits.

- [ ] **Step 6: Commit any final visual corrections**

If Step 4 required corrections:

```bash
git add src src-tauri
git commit -m "fix: refine dashboard responsive layout"
```

Do not commit generated sidecar binaries unless the repository already tracks the matching target binary.
