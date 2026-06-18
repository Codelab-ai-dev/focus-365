export type ChartPoint = { label: string; value: number };

// BarChart dibuja barras SVG (neo-brutalista: relleno accent, borde ink) a partir
// de una serie {label, value}. Alto proporcional al máximo de la serie.
export function BarChart({
  data,
  unit = "",
  className = "",
}: {
  data: ChartPoint[];
  unit?: string;
  className?: string;
}) {
  if (data.length === 0) {
    return <p className={`text-xs text-muted ${className}`}>sin datos</p>;
  }
  const max = Math.max(...data.map((d) => d.value), 1);
  const W = 100;
  const H = 60;
  const gap = data.length > 1 ? 2 : 0;
  const barW = (W - gap * (data.length - 1)) / data.length;
  const aria =
    "Gráfico de barras: " +
    data.map((d) => `${d.label} ${Math.round(d.value)}${unit}`).join(", ");

  return (
    <div className={className}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={aria} className="h-24 w-full">
        {data.map((d, i) => {
          const h = (d.value / max) * (H - 1);
          const x = i * (barW + gap);
          return (
            <rect
              key={i}
              x={x}
              y={H - h}
              width={barW}
              height={h}
              className="fill-accent stroke-ink"
              strokeWidth={0.5}
            >
              <title>{`${d.label}: ${Math.round(d.value)}${unit}`}</title>
            </rect>
          );
        })}
      </svg>
      <div className="mt-1 flex gap-[2px] text-[8px] text-muted">
        {data.map((d, i) => (
          <span key={i} className="flex-1 truncate text-center">
            {d.label}
          </span>
        ))}
      </div>
    </div>
  );
}
