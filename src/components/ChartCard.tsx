import { useState, useEffect, useCallback } from "react";
import ReactECharts from "echarts-for-react";

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

  return (
    <div className={`bg-card border border-border rounded-xl p-4 shadow-sm flex flex-col ${className || ""}`}>
      <h3 className="text-xs font-medium text-muted-foreground mb-2">{title}</h3>
      <div className="flex-1 min-h-0">
        <ReactECharts option={themed()} style={{ height: '100%', width: '100%' }} notMerge={true} />
      </div>
    </div>
  );
}
