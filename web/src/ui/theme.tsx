import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { motion } from "framer-motion";

type Theme = "light" | "dark";
const STORAGE_KEY = "focus365-theme";

const ThemeContext = createContext<{ theme: Theme; toggle: () => void } | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY) === "dark" ? "dark" : "light";
    } catch {
      return "light";
    }
  });

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    try {
      localStorage.setItem(STORAGE_KEY, theme);
    } catch {
      // ignore; the attribute is already set above
    }
  }, [theme]);

  const toggle = () => setTheme((t) => (t === "light" ? "dark" : "light"));

  return <ThemeContext.Provider value={{ theme, toggle }}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme debe usarse dentro de ThemeProvider");
  return ctx;
}

// ThemeToggle es el chip 🌙/☀️ del TopBar; el icono rota al cambiar.
export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const label = theme === "light" ? "Cambiar a tema oscuro" : "Cambiar a tema claro";
  return (
    <button
      onClick={toggle}
      aria-label={label}
      className="rounded-full border-2 border-ink bg-surface px-2 py-0.5 text-sm shadow-brutal-sm transition-transform hover:-translate-y-[1px]"
    >
      <motion.span
        key={theme}
        initial={{ rotate: -180, opacity: 0 }}
        animate={{ rotate: 0, opacity: 1 }}
        transition={{ duration: 0.3, ease: "easeOut" }}
        className="inline-block"
      >
        {theme === "light" ? "🌙" : "☀️"}
      </motion.span>
    </button>
  );
}
