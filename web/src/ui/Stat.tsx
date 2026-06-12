import { useEffect, useRef, useState } from "react";
import { animate, useReducedMotionConfig } from "framer-motion";

// Stat: etiqueta uppercase + número display con contador animado. hideLabel
// permite usarlo dentro de tiles que ya ponen su propio título. Al cambiar
// value, anima desde el valor mostrado (no recuenta desde 0).
export function Stat({
  label = "",
  value,
  prefix = "",
  suffix = "",
  format,
  hideLabel = false,
  className = "",
}: {
  label?: string;
  value: number;
  prefix?: string;
  suffix?: string;
  format?: (n: number) => string;
  hideLabel?: boolean;
  className?: string;
}) {
  const reduced = useReducedMotionConfig();
  const [display, setDisplay] = useState(reduced ? value : 0);
  const shown = useRef(display);
  shown.current = display;

  useEffect(() => {
    if (reduced) {
      setDisplay(value);
      return;
    }
    const controls = animate(shown.current, value, {
      duration: 0.6,
      ease: "easeOut",
      onUpdate: (v) => setDisplay(Math.round(v)),
    });
    return () => controls.stop();
  }, [value, reduced]);

  const text = format ? format(display) : String(display);
  return (
    <div className={className}>
      {!hideLabel && (
        <div className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">{label}</div>
      )}
      <div className="font-display text-2xl font-bold tracking-tight">
        {prefix}
        {text}
        {suffix}
      </div>
    </div>
  );
}
