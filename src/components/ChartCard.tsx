import { useState, useEffect, useCallback, useRef } from "react";
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

/** Axis labels stay short; tooltips carry the exact number. */
export function compactNumber(value: number): string {
  for (const [limit, unit] of [[1e9, "B"], [1e6, "M"], [1e3, "K"]] as const) {
    if (Math.abs(value) < limit) continue;
    const scaled = value / limit;
    const text = Math.abs(scaled) >= 10 ? scaled.toFixed(0) : scaled.toFixed(1);
    return `${text.replace(/\.0$/, "")}${unit}`;
  }
  return String(value);
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

    chartRef.current = echarts.init(container, undefined, { renderer: "canvas" });
    return () => {
      chartRef.current?.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    chartRef.current?.setOption(themed(), true);
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
      for (const { event, listener } of listeners) {
        chart.off(event, listener);
      }
    };
  }, [onEvents]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver(() => {
      chartRef.current?.resize();
    });
    ro.observe(container);
    return () => ro.disconnect();
  }, []);

  return (
    <div className={`flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg bg-card p-3.5 ${className || ""}`}>
      {title ? (
        <h3 className="mb-2 text-xs font-semibold text-muted-foreground">{title}</h3>
      ) : null}
      <div ref={containerRef} className="flex-1 min-h-0" />
    </div>
  );
}
