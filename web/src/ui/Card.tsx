import { ReactNode } from "react";

// Card es la superficie base del lenguaje: borde grueso de tinta, sombra dura.
// interactive agrega el gesto de levantarse al hover (para tiles clickeables).
export function Card({
  interactive = false,
  className = "",
  children,
}: {
  interactive?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const lift = interactive
    ? "transition-all duration-150 hover:-translate-x-[2px] hover:-translate-y-[2px] hover:shadow-brutal-lg"
    : "";
  return (
    <div
      className={`rounded-lg border-[2.5px] border-ink bg-surface shadow-brutal ${lift} ${className}`}
    >
      {children}
    </div>
  );
}
