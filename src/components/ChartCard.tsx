import { useState, useEffect, useCallback, useRef } from "react";
import * as echarts from "echarts/core";
import { BarChart, PieChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

echarts.use([BarChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

interface ChartCardProps {
  title: string;
  option: object;
  className?: string;
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

export default function ChartCard({ title, option, className }: ChartCardProps) {
  const isDark = useIsDark();

  const themed = useCallback(() => {
    const textColor = isDark ? "#a8a8a8" : "#737373";
    const axisLine = isDark ? "#2e2e2e" : "#e5e5e5";
    const base = option as Record<string, unknown>;
    const baseXAxis = (base.xAxis as Record<string, unknown>) || {};
    const baseYAxis = (base.yAxis as Record<string, unknown>) || {};
    return {
      ...base,
      backgroundColor: "transparent",
      tooltip: { ...(base.tooltip as object || {}), backgroundColor: isDark ? "#1a1a1a" : "#fff", borderColor: axisLine, textStyle: { color: isDark ? "#ededed" : "#171717", fontSize: 12 } },
      legend: { ...(base.legend as object || {}), textStyle: { color: textColor, fontSize: 11 } },
      xAxis: { ...baseXAxis, axisLine: { lineStyle: { color: axisLine } }, axisLabel: { ...(baseXAxis.axisLabel as object || {}), color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
      yAxis: { ...baseYAxis, axisLine: { show: false }, axisLabel: { ...(baseYAxis.axisLabel as object || {}), color: textColor, fontSize: 11 }, splitLine: { lineStyle: { color: axisLine, type: "dashed" as const } } },
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
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver(() => {
      chartRef.current?.resize();
    });
    ro.observe(container);
    return () => ro.disconnect();
  }, []);

  return (
    <div className={`bg-card border border-border rounded-xl p-3 shadow-sm flex flex-col min-w-0 min-h-0 overflow-hidden ${className || ""}`}>
      <h3 className="text-xs font-medium text-muted-foreground mb-1.5">{title}</h3>
      <div ref={containerRef} className="flex-1 min-h-0" />
    </div>
  );
}
