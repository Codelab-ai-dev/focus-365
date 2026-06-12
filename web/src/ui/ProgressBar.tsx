import { motion, useReducedMotion } from "framer-motion";

export function ProgressBar({ value, className = "" }: { value: number; className?: string }) {
  const reduced = useReducedMotion();
  const pct = Math.max(0, Math.min(100, Math.round(value)));
  return (
    <div
      role="progressbar"
      aria-valuenow={pct}
      aria-valuemin={0}
      aria-valuemax={100}
      className={`h-3 overflow-hidden rounded-md border-2 border-ink bg-surface ${className}`}
    >
      <motion.div
        className="h-full bg-accent"
        initial={reduced ? false : { width: 0 }}
        animate={{ width: `${pct}%` }}
        transition={{ type: "spring", stiffness: 120, damping: 20 }}
      />
    </div>
  );
}
