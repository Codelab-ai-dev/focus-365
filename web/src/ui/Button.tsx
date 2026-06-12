import { ButtonHTMLAttributes } from "react";
import { motion } from "framer-motion";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost";
};

// El press físico del lenguaje: el botón se hunde en su sombra al click
// (translate del motion + active:shadow-none de CSS).
export function Button({ variant = "primary", className = "", children, ...rest }: Props) {
  const skin =
    variant === "primary" ? "bg-accent text-[#16130e]" : "bg-surface text-ink";
  return (
    <motion.button
      whileTap={{ x: 2, y: 2 }}
      transition={{ duration: 0.1 }}
      className={`rounded-lg border-[2.5px] border-ink px-4 py-2 font-display text-sm font-bold shadow-brutal active:shadow-none disabled:opacity-60 ${skin} ${className}`}
      {...(rest as object)}
    >
      {children}
    </motion.button>
  );
}
