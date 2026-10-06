export function compactNumber(value: number): string {
  for (const [limit, unit] of [[1e9, "B"], [1e6, "M"], [1e3, "K"]] as const) {
    if (Math.abs(value) < limit) continue;
    const scaled = value / limit;
    const text = Math.abs(scaled) >= 10 ? scaled.toFixed(0) : scaled.toFixed(1);
    return `${text.replace(/\.0$/, "")}${unit}`;
  }
  return String(value);
}
