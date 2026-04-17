# Desktop UI Responsive Layout Redesign

## Problem

The desktop app UI has two critical issues:
1. **No fullscreen adaptation** — Layout is capped at `max-w-[1200px]`, wasting space on larger screens
2. **Chart overlap/crowding** — Fixed pixel heights (200px/160px) and fixed ECharts grid margins cause labels and bars to overlap, especially with many data points or models

## Solution: Flex-based Viewport Fill

Convert the app from a scrollable web-style layout to a desktop-native viewport-filling layout using CSS flexbox.

### 1. Layout Shell (Layout.tsx)

**Before:**
```
<div min-h-screen>
  <header> <div max-w-[1200px]> ... </div> </header>
  <main max-w-[1200px]> {children} </main>
</div>
```

**After:**
```
<div h-screen flex flex-col min-w-[900px]>
  <header> <div px-6> ... </div> </header>
  <main flex-1 overflow-auto px-6 py-6> {children} </main>
</div>
```

Changes:
- Remove `max-w-[1200px]` from both header and main
- Root becomes `h-screen flex flex-col` — fills the Tauri window exactly
- Main becomes `flex-1 overflow-auto` — takes remaining vertical space, scrolls if content overflows
- Add `min-w-[900px]` to match Tauri's minWidth constraint

### 2. Dashboard Page (Dashboard.tsx)

**Grid layout:**
- Outer wrapper becomes `flex flex-col flex-1 min-h-0` (min-h-0 is critical for flex children to shrink)
- TimeRangeSelector stays as-is (fixed height)
- Main grid becomes `grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-5 flex-1 min-h-0`

**Right panel (charts):**
- Becomes `flex flex-col gap-3 min-w-0 min-h-0`
- Token Usage chart: `flex-[2] min-h-[160px]` (takes 2/3 of space)
- Bottom chart row: `flex-[1] min-h-[120px]` with internal `grid grid-cols-1 sm:grid-cols-[3fr_2fr] gap-3 h-full`
- Each ChartCard fills its container height

**Left panel:**
- Stays at 260px fixed width (appropriate for stats display)
- No height changes needed — it scrolls with the page if needed

**Skeleton loader:**
- Mirror the flex layout: replace fixed `h-[200px]`/`h-[160px]` with `flex-[2] min-h-[160px]` and `flex-[1] min-h-[120px]`

### 3. ChartCard Component (ChartCard.tsx)

**Before:**
```tsx
<ReactECharts option={themed()} style={{ height }} notMerge={true} />
```

**After:**
```tsx
<div className="flex-1 min-h-0">
  <ReactECharts option={themed()} style={{ height: '100%', width: '100%' }} notMerge={true} />
</div>
```

Changes:
- Remove `height` prop (no longer needed)
- ECharts renders at 100% of parent container
- Card itself becomes `flex flex-col` so the chart area fills remaining space after the title

### 4. ECharts Options Anti-Overlap (Dashboard.tsx)

**Grid margins:**
```ts
// Before
grid: { left: 40, right: 12, top: 36, bottom: 24 }

// After
grid: { left: 8, right: 8, top: 36, bottom: 4, containLabel: true }
```
`containLabel: true` lets ECharts auto-calculate space for axis labels.

**Legend scrolling:**
```ts
legend: { type: 'scroll', ... }
```
Prevents legend items from overflowing when many models are present.

**X-axis label overflow:**
```ts
xAxis: {
  axisLabel: {
    hideOverlap: true,    // auto-hide overlapping labels
  }
}
```

**Pie chart label overflow:**
```ts
label: {
  formatter: "{b}: {d}%",
  overflow: "truncate",
  width: 80
}
```

### 5. Minimum Size Protection

- Tauri config already enforces `minWidth: 900, minHeight: 600`
- CSS `min-w-[900px]` on root prevents content compression
- Chart containers have `min-h-[120px]`/`min-h-[160px]` floors

### 6. Files Changed

| File | Changes |
|------|---------|
| `src/components/Layout.tsx` | Remove max-width, add h-screen flex layout |
| `src/pages/Dashboard.tsx` | Flex-based chart sizing, ECharts option fixes |
| `src/components/ChartCard.tsx` | Remove fixed height prop, use 100% fill |

### 7. Not Changed

- Sessions page — benefits automatically from Layout max-width removal
- Settings page — same
- No new dependencies
- No data logic changes
- Tauri config unchanged
