/** Shortens a bucket label for chart axes; the tooltip keeps the full value. */
export function shortBucket(label: string): string {
  const match = label.match(/^\d{4}-(\d{2}-\d{2})(?:[ T](\d{1,2})(?::(\d{2}))?)?$/);
  if (!match) return label;
  const [, day, hour, minute] = match;
  if (hour === undefined) return day;
  return `${day} ${hour.padStart(2, "0")}:${minute ?? "00"}`;
}

export function fmtTokens(n: number): string {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(1) + "K";
  return String(n);
}

export function fmtCost(n: number): string {
  if (n >= 1) return "$" + n.toFixed(2);
  return "$" + n.toFixed(4);
}

export function localDateStr(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export type TimePreset = "today" | "thisWeek" | "thisMonth" | "thisYear" | "last3d" | "last7d" | "last30d" | "custom";

export function getTimeRange(preset: TimePreset, customFrom?: string, customTo?: string): { from: string; to: string } {
  const now = new Date();
  const todayStr = localDateStr(now);
  switch (preset) {
    case "today": return { from: todayStr, to: todayStr };
    case "thisWeek": {
      const d = new Date(now);
      d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return { from: localDateStr(d), to: todayStr };
    }
    case "thisMonth": return { from: todayStr.slice(0, 8) + "01", to: todayStr };
    case "thisYear": return { from: todayStr.slice(0, 5) + "01-01", to: todayStr };
    case "last3d": { const d = new Date(now); d.setDate(d.getDate() - 2); return { from: localDateStr(d), to: todayStr }; }
    case "last7d": { const d = new Date(now); d.setDate(d.getDate() - 6); return { from: localDateStr(d), to: todayStr }; }
    case "last30d": { const d = new Date(now); d.setDate(d.getDate() - 29); return { from: localDateStr(d), to: todayStr }; }
    case "custom": return { from: customFrom || todayStr, to: customTo || todayStr };
  }
}

export function relativeTime(ts: string, t: (key: string) => string): string {
  if (!ts) return "-";
  const d = new Date(ts.replace(" ", "T").replace(" +0000 UTC", "Z"));
  if (isNaN(d.getTime())) return ts.slice(0, 16);
  const diff = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diff < 60) return t("justNow") || "just now";
  if (diff < 3600) return Math.floor(diff / 60) + (t("mAgo") || "m ago");
  if (diff < 86400) return Math.floor(diff / 3600) + (t("hAgo") || "h ago");
  if (diff < 604800) return Math.floor(diff / 86400) + (t("dAgo") || "d ago");
  return d.toLocaleDateString();
}

/**
 * Mid-luminance series palette so every colour stays legible on both the light
 * and the dark surface. Index order is semantic: 0 input, 1 output,
 * 2 cache create, 3 cache read, 5 requests/min. Hues follow the logo.
 */
export const CHART_COLORS = [
  "#1677ea", // input — logo meter blue
  "#2ec4a6", // output — logo teal
  "#f0a02c", // cache create — amber
  "#8b95a6", // cache read — slate
  "#7a63d6", // violet
  "#ff7968", // requests/min — logo coral
  "#3aa6e8", // sky
  "#c96aa8", // plum
];
