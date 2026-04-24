# Responsive UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the desktop app UI fill the entire window on fullscreen and prevent chart label/bar overlap at any size.

**Architecture:** Convert from a max-width-capped scrollable layout to a flex-based viewport-filling layout. Charts use `height: 100%` to fill their flex containers instead of fixed pixel values. ECharts options use `containLabel` and `hideOverlap` to prevent label crowding.

**Tech Stack:** React 18, TypeScript, Tailwind CSS v4, ECharts via echarts-for-react

**Spec:** `docs/superpowers/specs/2026-04-17-responsive-ui-design.md`

---

### Task 1: Convert Layout Shell to Viewport-Filling Flex Layout

**Files:**
- Modify: `src/components/Layout.tsx:43-78`

- [ ] **Step 1: Update root container and header**

In `src/components/Layout.tsx`, change the root div and header inner div:

```tsx
// Line 44: change root from min-h-screen to h-screen flex
// Before:
<div className="min-h-screen bg-background">

// After:
<div className="h-screen flex flex-col bg-background min-w-[900px]">
```

```tsx
// Line 46: remove max-w-[1200px] from header inner div
// Before:
<div className="mx-auto max-w-[1200px] px-6 py-3 flex items-center justify-between">

// After:
<div className="px-6 py-3 flex items-center justify-between">
```

- [ ] **Step 2: Update main element**

```tsx
// Line 75: remove max-w-[1200px], add flex-1 and overflow-auto
// Before:
<main className="mx-auto max-w-[1200px] px-6 py-6">

// After:
<main className="flex-1 overflow-auto px-6 py-6">
```

- [ ] **Step 3: Verify build compiles**

Run: `cd /Users/harrison/Documents/harrison_project/agent-usage-desktop && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add src/components/Layout.tsx
git commit -m "refactor: convert layout to viewport-filling flex layout

Remove max-w-[1200px] cap, use h-screen + flex-col so content
fills the entire Tauri window on fullscreen."
```

---

### Task 2: Update ChartCard to Fill Parent Container

**Files:**
- Modify: `src/components/ChartCard.tsx:4-48`

- [ ] **Step 1: Remove height prop from interface and default**

```tsx
// Lines 4-8: remove height from props interface
// Before:
interface ChartCardProps {
  title: string;
  option: object;
  className?: string;
  height?: number;
}

// After:
interface ChartCardProps {
  title: string;
  option: object;
  className?: string;
}
```

- [ ] **Step 2: Update component signature and remove height parameter**

```tsx
// Line 25: remove height from destructuring
// Before:
export default function ChartCard({ title, option, className, height = 260 }: ChartCardProps) {

// After:
export default function ChartCard({ title, option, className }: ChartCardProps) {
```

- [ ] **Step 3: Update card container to flex column layout**

```tsx
// Line 43: add flex flex-col, wrap ECharts in flex-1 container
// Before:
  return (
    <div className={`bg-card border border-border rounded-xl p-4 shadow-sm ${className || ""}`}>
      <h3 className="text-xs font-medium text-muted-foreground mb-2">{title}</h3>
      <ReactECharts option={themed()} style={{ height }} notMerge={true} />
    </div>
  );

// After:
  return (
    <div className={`bg-card border border-border rounded-xl p-4 shadow-sm flex flex-col ${className || ""}`}>
      <h3 className="text-xs font-medium text-muted-foreground mb-2">{title}</h3>
      <div className="flex-1 min-h-0">
        <ReactECharts option={themed()} style={{ height: '100%', width: '100%' }} notMerge={true} />
      </div>
    </div>
  );
```

- [ ] **Step 4: Verify build compiles**

Run: `cd /Users/harrison/Documents/harrison_project/agent-usage-desktop && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add src/components/ChartCard.tsx
git commit -m "refactor: make ChartCard fill parent container height

Remove fixed height prop, use flex-1 + height:100% so ECharts
fills whatever space the parent provides."
```

---

### Task 3: Convert Dashboard to Flex-Based Chart Sizing

**Files:**
- Modify: `src/pages/Dashboard.tsx:77-101` (skeleton)
- Modify: `src/pages/Dashboard.tsx:216-288` (main layout)

- [ ] **Step 1: Update Dashboard outer wrapper**

```tsx
// Line 217: add flex classes to root
// Before:
    <div className="space-y-4">

// After:
    <div className="flex flex-col flex-1 min-h-0 gap-4">
```

- [ ] **Step 2: Update main grid to flex-1**

```tsx
// Line 234: add flex-1 min-h-0 to the grid
// Before:
        <div className="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-5">

// After:
        <div className="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-5 flex-1 min-h-0">
```

- [ ] **Step 3: Update right panel chart containers**

```tsx
// Line 278: right panel becomes flex with min-h-0
// Before:
          <div className="flex flex-col gap-3 min-w-0">
            <ChartCard title={t("tokenUsage")} option={tokensOption} height={200} />
            <div className="grid grid-cols-1 sm:grid-cols-[3fr_2fr] gap-3">
              <ChartCard title={t("costTrend")} option={costOption} height={160} />
              <ChartCard title={t("costByModel")} option={pieOption} height={160} />
            </div>
          </div>

// After:
          <div className="flex flex-col gap-3 min-w-0 min-h-0">
            <ChartCard title={t("tokenUsage")} option={tokensOption} className="flex-[2] min-h-[160px]" />
            <div className="grid grid-cols-1 sm:grid-cols-[3fr_2fr] gap-3 flex-[1] min-h-[120px]">
              <ChartCard title={t("costTrend")} option={costOption} />
              <ChartCard title={t("costByModel")} option={pieOption} />
            </div>
          </div>
```

- [ ] **Step 4: Update skeleton loader to match flex layout**

```tsx
// Lines 79-101: update DashboardSkeleton
// Before:
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-5">
      <div className="space-y-4">
        ...
      </div>
      <div className="space-y-3">
        <Skeleton className="h-[200px] rounded-xl" />
        <div className="grid grid-cols-1 sm:grid-cols-[3fr_2fr] gap-3">
          <Skeleton className="h-[160px] rounded-xl" />
          <Skeleton className="h-[160px] rounded-xl" />
        </div>
      </div>
    </div>
  );

// After:
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-5 flex-1 min-h-0">
      <div className="space-y-4">
        ...
      </div>
      <div className="flex flex-col gap-3 min-h-0">
        <Skeleton className="flex-[2] min-h-[160px] rounded-xl" />
        <div className="grid grid-cols-1 sm:grid-cols-[3fr_2fr] gap-3 flex-[1] min-h-[120px]">
          <Skeleton className="min-h-[120px] rounded-xl" />
          <Skeleton className="min-h-[120px] rounded-xl" />
        </div>
      </div>
    </div>
  );
```

- [ ] **Step 5: Verify build compiles**

Run: `cd /Users/harrison/Documents/harrison_project/agent-usage-desktop && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 6: Commit**

```bash
git add src/pages/Dashboard.tsx
git commit -m "refactor: convert dashboard charts to flex-based sizing

Charts now use flex-[2]/flex-[1] with min-height floors instead
of fixed pixel heights. They grow to fill available space on
fullscreen while maintaining minimum usable sizes."
```

---

### Task 4: Fix ECharts Options to Prevent Label Overlap

**Files:**
- Modify: `src/pages/Dashboard.tsx:174-210` (chart options)
- Modify: `src/components/ChartCard.tsx:28-39` (themed function)

- [ ] **Step 1: Update themed() in ChartCard to deep-merge axisLabel**

The `themed()` function in ChartCard.tsx currently overwrites `axisLabel` entirely. We need it to merge so that `hideOverlap` from Dashboard options survives.

```tsx
// In ChartCard.tsx, update the themed callback (lines 28-39):
// Before:
  const themed = useCallback(() => {
    const textColor = isDark ? "#a3a3a3" : "#737373";
    const axisLine = isDark ? "#262626" : "#e5e5e5";
    const base = option as Record<string, unknown>;
    return {
      ...base,
      backgroundColor: "transparent",
      tooltip: { ...(base.tooltip as object || {}), backgroundColor: isDark ? "#262626" : "#fff", borderColor: axisLine, textStyle: { color: isDark ? "#e5e5e5" : "#171717", fontSize: 12 } },
      legend: { ...(base.legend as object || {}), textStyle: { color: textColor, fontSize: 11 } },
      xAxis: { ...(base.xAxis as object || {}), axisLine: { lineStyle: { color: axisLine } }, axisLabel: { color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
      yAxis: { ...(base.yAxis as object || {}), axisLine: { show: false }, axisLabel: { color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
    };
  }, [option, isDark]);

// After:
  const themed = useCallback(() => {
    const textColor = isDark ? "#a3a3a3" : "#737373";
    const axisLine = isDark ? "#262626" : "#e5e5e5";
    const base = option as Record<string, unknown>;
    const baseXAxis = (base.xAxis as Record<string, unknown>) || {};
    const baseYAxis = (base.yAxis as Record<string, unknown>) || {};
    return {
      ...base,
      backgroundColor: "transparent",
      tooltip: { ...(base.tooltip as object || {}), backgroundColor: isDark ? "#262626" : "#fff", borderColor: axisLine, textStyle: { color: isDark ? "#e5e5e5" : "#171717", fontSize: 12 } },
      legend: { ...(base.legend as object || {}), textStyle: { color: textColor, fontSize: 11 } },
      xAxis: { ...baseXAxis, axisLine: { lineStyle: { color: axisLine } }, axisLabel: { ...(baseXAxis.axisLabel as object || {}), color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
      yAxis: { ...baseYAxis, axisLine: { show: false }, axisLabel: { ...(baseYAxis.axisLabel as object || {}), color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
    };
  }, [option, isDark]);
```

- [ ] **Step 2: Update tokensOption grid and axis labels**

```tsx
// In Dashboard.tsx, lines 174-186:
// Before:
  const tokensOption = tokensData?.labels ? {
    tooltip: { trigger: "axis" },
    legend: { data: [t("input"), t("output"), t("cacheRead"), t("cacheCreate")] },
    grid: { left: 40, right: 12, top: 36, bottom: 24 },
    xAxis: { type: "category", data: tokensData.labels },
    yAxis: { type: "value" },
    ...
  } : {};

// After:
  const tokensOption = tokensData?.labels ? {
    tooltip: { trigger: "axis" },
    legend: { type: "scroll", data: [t("input"), t("output"), t("cacheRead"), t("cacheCreate")] },
    grid: { left: 8, right: 8, top: 36, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: tokensData.labels, axisLabel: { hideOverlap: true } },
    yAxis: { type: "value" },
    ...
  } : {};
```

- [ ] **Step 3: Update costOption grid and axis labels**

```tsx
// In Dashboard.tsx, lines 188-198:
// Before:
  const costOption = costData?.series ? {
    tooltip: { trigger: "axis" },
    legend: { data: costData.series.map((s) => s.model) },
    grid: { left: 40, right: 12, top: 36, bottom: 24 },
    xAxis: { type: "category", data: costData.labels },
    yAxis: { type: "value" },
    ...
  } : {};

// After:
  const costOption = costData?.series ? {
    tooltip: { trigger: "axis" },
    legend: { type: "scroll", data: costData.series.map((s) => s.model) },
    grid: { left: 8, right: 8, top: 36, bottom: 4, containLabel: true },
    xAxis: { type: "category", data: costData.labels, axisLabel: { hideOverlap: true } },
    yAxis: { type: "value" },
    ...
  } : {};
```

- [ ] **Step 4: Update pieOption labels to prevent overflow**

```tsx
// In Dashboard.tsx, lines 200-210:
// Before:
  const pieOption = pieData.length ? {
    tooltip: { trigger: "item" },
    series: [{
      type: "pie", radius: ["40%", "70%"],
      data: pieData.map((d, i) => ({
        name: d.model, value: d.cost,
        itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
      })),
      label: { formatter: "{b}: {d}%" },
    }],
  } : {};

// After:
  const pieOption = pieData.length ? {
    tooltip: { trigger: "item" },
    series: [{
      type: "pie", radius: ["40%", "70%"],
      data: pieData.map((d, i) => ({
        name: d.model, value: d.cost,
        itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
      })),
      label: { formatter: "{b}: {d}%", overflow: "truncate", width: 80 },
    }],
  } : {};
```

- [ ] **Step 5: Verify build compiles**

Run: `cd /Users/harrison/Documents/harrison_project/agent-usage-desktop && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 6: Commit**

```bash
git add src/pages/Dashboard.tsx src/components/ChartCard.tsx
git commit -m "fix: prevent chart label overlap with containLabel and hideOverlap

- ECharts grid uses containLabel:true for auto label spacing
- X-axis labels use hideOverlap to skip crowded labels
- Legend uses type:scroll for many-model scenarios
- Pie labels use overflow:truncate to prevent bleed
- themed() deep-merges axisLabel so Dashboard options survive"
```

---

### Task 5: Visual Verification

- [ ] **Step 1: Start dev server**

Tell the user to run: `cd /Users/harrison/Documents/harrison_project/agent-usage-desktop && npx tauri dev`

- [ ] **Step 2: Verify at default window size (1200x800)**

Check:
- Dashboard fills the window width (no empty margins on sides)
- Charts fill vertical space proportionally
- Left stats panel at 260px, charts fill remaining width
- No label overlap on charts
- Skeleton loader matches chart layout during loading

- [ ] **Step 3: Verify at fullscreen**

Check:
- Content expands to fill entire screen
- Charts grow proportionally (token chart ~2x height of bottom charts)
- No excessive whitespace anywhere
- Chart labels remain readable, no overlap
- Pie chart labels don't bleed outside container

- [ ] **Step 4: Verify at minimum size (900x600)**

Check:
- Content doesn't overflow or break
- Charts respect min-height floors (160px / 120px)
- Scrollbar appears if content exceeds viewport

- [ ] **Step 5: Verify Sessions and Settings pages**

Check:
- Sessions table fills full width
- Settings page renders correctly
- Navigation header spans full width

- [ ] **Step 6: Final commit if any fixes needed**

If visual issues are found, fix and commit with descriptive message.
