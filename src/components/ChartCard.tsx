import ReactECharts from "echarts-for-react";

interface ChartCardProps {
  title: string;
  option: object;
  className?: string;
}

export default function ChartCard({ title, option, className }: ChartCardProps) {
  return (
    <div className={`bg-card border border-border rounded-xl p-4 shadow-sm ${className || ""}`}>
      <h3 className="text-sm font-semibold mb-3">{title}</h3>
      <ReactECharts option={option} style={{ height: 260 }} notMerge={true} />
    </div>
  );
}
