import { ReactNode } from "react";

const VARIANTS = {
  accent: "bg-accent text-[#16130e]",
  money: "bg-money-bg text-money-fg",
  sky: "bg-sky-bg text-sky-fg",
  sun: "bg-sun-bg text-sun-fg",
  danger: "bg-danger-bg text-danger-fg",
  plain: "bg-surface text-ink",
} as const;

const SIZES = {
  sm: "px-2 py-0.5 text-xs",
  md: "px-3 py-1 text-sm",
} as const;

export function Chip({
  variant = "plain",
  size = "sm",
  className = "",
  children,
}: {
  variant?: keyof typeof VARIANTS;
  size?: keyof typeof SIZES;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={`inline-block rounded-md border-2 border-ink font-bold shadow-brutal-sm ${VARIANTS[variant]} ${SIZES[size]} ${className}`}
    >
      {children}
    </span>
  );
}
