interface StatCardProps {
  label: string;
  value: string;
  color: string;
}

export default function StatCard({ label, value, color }: StatCardProps) {
  return (
    <div className="relative bg-card border border-border rounded-xl p-4 shadow-sm hover:shadow-md transition-shadow overflow-hidden">
      <div className={`absolute left-0 top-0 w-1 h-full`} style={{ backgroundColor: color }} />
      <div className="text-xs text-muted-foreground font-semibold uppercase tracking-wide">{label}</div>
      <div className="text-2xl font-bold mt-1 font-mono">{value}</div>
    </div>
  );
}
