import { useMemo } from "react";

interface SparklineProps {
  data: number[];
  color: string;
  height?: number;
}

export default function Sparkline({ data, color, height = 32 }: SparklineProps) {
  const path = useMemo(() => {
    if (data.length < 2) return "";
    const max = Math.max(...data) || 1;
    const min = Math.min(...data);
    const range = max - min || 1;
    const w = 100;
    const h = height;
    const step = w / (data.length - 1);
    const pts = data.map((v, i) => ({
      x: i * step,
      y: h - ((v - min) / range) * (h * 0.85) - h * 0.075,
    }));
    // smooth curve
    let d = `M${pts[0].x},${pts[0].y}`;
    for (let i = 1; i < pts.length; i++) {
      const cp = (pts[i].x - pts[i - 1].x) / 2;
      d += ` C${pts[i - 1].x + cp},${pts[i - 1].y} ${pts[i].x - cp},${pts[i].y} ${pts[i].x},${pts[i].y}`;
    }
    return d;
  }, [data, height]);

  const areaPath = useMemo(() => {
    if (!path) return "";
    const w = 100;
    return `${path} L${w},${height} L0,${height} Z`;
  }, [path, height]);

  if (data.length === 0) return null;

  // Single data point: show a flat dashed line
  if (data.length === 1) {
    const y = height / 2;
    return (
      <svg viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" className="w-full" style={{ height }}>
        <line x1="0" y1={y} x2="100" y2={y} stroke={color} strokeWidth={1} strokeDasharray="4 3" vectorEffect="non-scaling-stroke" strokeOpacity={0.5} />
      </svg>
    );
  }

  return (
    <svg viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" className="w-full" style={{ height }}>
      <defs>
        <linearGradient id={`sg-${color.replace("#", "")}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.2} />
          <stop offset="100%" stopColor={color} stopOpacity={0.02} />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#sg-${color.replace("#", "")})`} />
      <path d={path} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
