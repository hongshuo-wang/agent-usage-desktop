import { useState, useEffect, useCallback, useRef } from "react";
import { compactNumber } from "./chartUtils";
import * as echarts from "echarts/core";
import { BarChart, LineChart, PieChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

echarts.use([BarChart, LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

interface ChartCardProps {
  title?: string;
  option: object;
  className?: string;
  onEvents?: Record<string, (params: { name?: string }) => void>;
}

function useIsDark() {
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark")
  );
  useEffect(() => {
    const obs = new MutationObserver(() => {
      setDark(document.documentElement.classList.contains("dark"));
    });
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => obs.disconnect();
  }, []);
  return dark;
}

export default function ChartCard({ title, option, className, onEvents }: ChartCardProps) {
  const isDark = useIsDark();

  const themed = useCallback(() => {
    const styles = getComputedStyle(document.documentElement);
    const css = (name: string, fallback: string) => styles.getPropertyValue(name).trim() || fallback;
    const textColor = css("--color-muted-foreground", isDark ? "#a1a1a6" : "#6e6e73");
    const axisColor = css("--color-border", isDark ? "#3a3a3c" : "#dedee2");
    const base = option as Record<string, unknown>;
    const baseXAxis = (base.xAxis as Record<string, unknown>) || {};
    const themeAxis = (axis: Record<string, unknown>) => {
      const axisLine = (axis.axisLine as Record<string, unknown>) || {};
      const axisLineStyle = (axisLine.lineStyle as Record<string, unknown>) || {};
      const splitLine = (axis.splitLine as Record<string, unknown>) || {};
      const splitLineStyle = (splitLine.lineStyle as Record<string, unknown>) || {};
      const axisLabel = (axis.axisLabel as Record<string, unknown>) || {};
      const isCategory = axis.type === "category" || axis.type === "time";
      return {
        ...axis,
        // Gridlines are the only chrome left holding the plot together, so the
        // axis line and ticks go — defaults first so a chart can opt back in.
        axisLine: { show: false, ...axisLine, lineStyle: { ...axisLineStyle, color: axisColor } },
        axisTick: { show: false, ...((axis.axisTick as object) || {}) },
        axisLabel: {
          fontSize: 11,
          ...axisLabel,
          color: textColor,
          ...(isCategory || axisLabel.formatter ? {} : { formatter: compactNumber }),
        },
        splitLine: {
          show: !isCategory,
          ...splitLine,
          lineStyle: { color: axisColor, type: "solid" as const, opacity: 0.55, ...splitLineStyle },
        },
      };
    };
    const baseYAxis = base.yAxis;
    return {
      ...base,
      backgroundColor: "transparent",
      textStyle: {
        color: textColor,
        fontFamily: "-apple-system, BlinkMacSystemFont, sans-serif",
      },
      tooltip: {
        backgroundColor: css("--color-card", isDark ? "#242426" : "#ffffff"),
        borderColor: axisColor,
        borderWidth: 1,
        padding: [8, 10],
        extraCssText: "border-radius:8px;box-shadow:0 6px 20px rgba(0,0,0,0.14);",
        ...((base.tooltip as object) || {}),
        textStyle: { color: css("--color-foreground", isDark ? "#f5f5f7" : "#1d1d1f"), fontSize: 12 },
      },
      legend: {
        icon: "circle",
        itemWidth: 8,
        itemHeight: 8,
        itemGap: 14,
        ...(base.legend as object || {}),
        textStyle: { color: textColor, fontSize: 11, ...(((base.legend as Record<string, unknown>)?.textStyle as object) || {}) },
      },
      xAxis: themeAxis(baseXAxis),
      yAxis: Array.isArray(baseYAxis)
        ? baseYAxis.map((axis) => themeAxis((axis as Record<string, unknown>) || {}))
        : themeAxis((baseYAxis as Record<string, unknown>) || {}),
    };
  }, [option, isDark]);

  const chartRef = useRef<echarts.ECharts | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const existing = echarts.getInstanceByDom(container);
    if (existing && !existing.isDisposed()) existing.dispose();
    const chart = echarts.init(container, undefined, { renderer: "canvas" });
    chartRef.current = chart;
    return () => {
      // Keep cleanup tied to the instance created by this effect. A later HMR
      // or StrictMode mount must never be disposed by an older cleanup.
      if (chartRef.current === chart) chartRef.current = null;
      if (!chart.isDisposed()) chart.dispose();
    };
  }, []);

  useEffect(() => {
    const chart = chartRef.current;
    // StrictMode mounts, unmounts and remounts in one commit; a stale effect
    // must not write to the instance that its cleanup already disposed.
    if (!chart || chart.isDisposed()) return;
    chart.setOption(themed(), true);
  }, [themed]);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart || !onEvents) return;
    const listeners = Object.entries(onEvents).map(([event, handler]) => {
      const listener = (...args: unknown[]) => handler((args[0] || {}) as { name?: string });
      chart.on(event, listener);
      return { event, listener };
    });
    return () => {
      if (chart.isDisposed()) return;
      for (const { event, listener } of listeners) {
        chart.off(event, listener);
      }
    };
  }, [onEvents]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver(() => {
      const chart = chartRef.current;
      if (chart && !chart.isDisposed()) chart.resize();
    });
    ro.observe(container);
    return () => ro.disconnect();
  }, []);

  return (
    <div className={`flex min-h-0 min-w-0 flex-col ${className || ""}`}>
      {title ? (
        <h3 className="mb-2 truncate text-2xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</h3>
      ) : null}
      <div ref={containerRef} className="min-h-0 flex-1" />
    </div>
  );
}
