import { useEffect, useState } from "react";
import { animate, useReducedMotion } from "framer-motion";

// Stat: etiqueta uppercase + número display con contador animado al montar.
// Con reduced-motion el valor aparece directo (sin cuenta).
export function Stat({
  label,
  value,
  prefix = "",
  suffix = "",
  format,
  className = "",
}: {
  label: string;
  value: number;
  prefix?: string;
  suffix?: string;
  format?: (n: number) => string;
  className?: string;
}) {
  const reduced = useReducedMotion();
  const [display, setDisplay] = useState(reduced ? value : 0);

  useEffect(() => {
    if (reduced) {
      setDisplay(value);
      return;
    }
    const controls = animate(0, value, {
      duration: 0.6,
      ease: "easeOut",
      onUpdate: (v) => setDisplay(Math.round(v)),
    });
    return () => controls.stop();
  }, [value, reduced]);

  const text = format ? format(display) : String(display);
  return (
    <div className={className}>
      <div className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">{label}</div>
      <div className="font-display text-2xl font-bold tracking-tight">
        {prefix}
        {text}
        {suffix}
      </div>
    </div>
  );
}
