import { useState, useEffect, useCallback, useRef } from "react";
import * as echarts from "echarts/core";
import { BarChart, LineChart, PieChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

echarts.use([BarChart, LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

interface ChartCardProps {
  title: string;
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
    const textColor = isDark ? "#a8a8a8" : "#737373";
    const axisLine = isDark ? "#2e2e2e" : "#e5e5e5";
    const base = option as Record<string, unknown>;
    const baseXAxis = (base.xAxis as Record<string, unknown>) || {};
    const themeYAxis = (axis: Record<string, unknown>) => ({
      ...axis,
      axisLine: { show: false },
      axisLabel: { ...(axis.axisLabel as object || {}), color: textColor, fontSize: 11 },
      splitLine: { ...(axis.splitLine as object || {}), lineStyle: { color: axisLine, type: "dashed" as const } },
    });
    const baseYAxis = base.yAxis;
    return {
      ...base,
      backgroundColor: "transparent",
      tooltip: { ...(base.tooltip as object || {}), backgroundColor: isDark ? "#1a1a1a" : "#fff", borderColor: axisLine, textStyle: { color: isDark ? "#ededed" : "#171717", fontSize: 12 } },
      legend: { ...(base.legend as object || {}), textStyle: { color: textColor, fontSize: 11 } },
      xAxis: { ...baseXAxis, axisLine: { lineStyle: { color: axisLine } }, axisLabel: { ...(baseXAxis.axisLabel as object || {}), color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
      yAxis: Array.isArray(baseYAxis)
        ? baseYAxis.map((axis) => themeYAxis((axis as Record<string, unknown>) || {}))
        : themeYAxis((baseYAxis as Record<string, unknown>) || {}),
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
    <div className={`flex min-h-0 min-w-0 flex-col overflow-hidden rounded-md bg-card/70 p-3 ${className || ""}`}>
      <h3 className="text-xs font-medium text-muted-foreground mb-1.5">{title}</h3>
      <div ref={containerRef} className="flex-1 min-h-0" />
    </div>
  );
}
