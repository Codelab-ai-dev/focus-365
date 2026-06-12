# Plan 12 — Rediseño UI neo-brutalista (sistema + shell + dashboard + auth) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sistema de diseño neo-brutalista completo (tokens claro/oscuro, primitivas animadas con framer-motion) aplicado al TopBar, dashboard y login/registro.

**Architecture:** CSS variables semánticas (`:root` / `[data-theme="dark"]`) mapeadas en Tailwind; primitivas en `web/src/ui/` (una por archivo); las rutas componen primitivas. La paleta vieja (`ink/sand/amber/...`) coexiste hasta el cierre de la R13.

**Tech Stack:** React 18 + Tailwind + framer-motion (nueva dep) + TanStack Router/Query + Vitest.

**Spec:** `docs/superpowers/specs/2026-06-11-plan-12-rediseno-ui-design.md` (cubre R12 y R13; este plan es R12).

**Reglas transversales:**
- NO cambiar ningún texto visible al usuario ni roles/aria existentes (los tests los asser­tan). La marca «Focus 365» se versaliza con CSS `uppercase`, el nodo de texto sigue siendo `Focus 365`.
- Las primitivas usan SOLO tokens semánticos (`border-ink`, `bg-surface`…), nunca hex ni la paleta vieja. Excepción: el texto sobre `accent` es siempre `text-[#16130e]` (regla del spec).
- Comandos: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run [archivo]` y `npm run build`. Commits en español terminando con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Rama: `plan-12-rediseno-ui` desde `master` antes de la Task 1 (NO worktree).

---

### Task 1: Fundaciones — framer-motion, tokens CSS, Tailwind, fuentes

**Files:**
- Modify: `web/package.json` (vía npm install)
- Modify: `web/src/index.css`
- Modify: `web/tailwind.config.js`

- [ ] **Step 1: Instalar framer-motion**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npm install framer-motion
```

- [ ] **Step 2: Reemplazar `web/src/index.css` por:**

```css
@import url("https://fonts.googleapis.com/css2?family=Inter:wght@400;500;700;800&family=Space+Grotesk:wght@500;700&display=swap");
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    --c-bg: #f4f0e6;
    --c-surface: #ffffff;
    --c-ink: #16130e;
    --c-muted: #6b6258;
    --c-shadow: #16130e;
    --c-accent: #ff7a3d;
    --c-money-bg: #9fd89a; --c-money-fg: #16130e;
    --c-sky-bg:   #9ec7f5; --c-sky-fg:   #16130e;
    --c-sun-bg:   #f5d76e; --c-sun-fg:   #16130e;
    --c-danger-bg:#f5a3a3; --c-danger-fg:#16130e;
  }
  [data-theme="dark"] {
    --c-bg: #191613;
    --c-surface: #221e1a;
    --c-ink: #f4f0e6;
    --c-muted: #a89b8c;
    --c-shadow: #000000;
    --c-accent: #ff7a3d;
    --c-money-bg: #1f2b1e; --c-money-fg: #9fd89a;
    --c-sky-bg:   #1c2533; --c-sky-fg:   #9ec7f5;
    --c-sun-bg:   #2e2812; --c-sun-fg:   #f5d76e;
    --c-danger-bg:#2e1a1a; --c-danger-fg:#f5a3a3;
  }

  html, body, #root {
    @apply h-full;
  }
  body {
    background-color: var(--c-bg);
    color: var(--c-ink);
    /* Papel punteado sutil: refuerza el aire de cuaderno sin estorbar. */
    background-image: radial-gradient(
      color-mix(in srgb, var(--c-ink) 7%, transparent) 1px,
      transparent 1px
    );
    background-size: 22px 22px;
    transition: background-color 200ms ease, color 200ms ease;
    @apply font-sans antialiased;
  }

  /* Foco visible del lenguaje: anillo accent desplazado. */
  :focus-visible {
    outline: 2px solid var(--c-accent);
    outline-offset: 2px;
  }
}

/* Fuego de racha: flicker orgánico, lo único que anima en loop. */
@keyframes flicker {
  0%, 100% { transform: scale(1) rotate(-2deg); }
  50%      { transform: scale(1.12) rotate(2deg); }
}
.animate-flicker {
  display: inline-block;
  animation: flicker 1.6s ease-in-out infinite;
}
@media (prefers-reduced-motion: reduce) {
  .animate-flicker { animation: none; }
}
```

- [ ] **Step 3: Ampliar `web/tailwind.config.js`** (la paleta vieja se conserva tal cual; se agregan los tokens — `ink` y `money` pasan de valor simple/objeto a objeto con `DEFAULT` para no romper clases viejas):

```js
/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // -- Paleta vieja (rutas sin migrar; se elimina al cierre de R13) --
        ink: {
          DEFAULT: "var(--c-ink)", // token semántico nuevo
          950: "#13110f",
          900: "#1c1814",
          800: "#241f1a",
          700: "#2a2520",
        },
        sand: {
          100: "#f5ede0",
          400: "#a89b8c",
        },
        amber: { brand: "#e0a458" },
        money: {
          DEFAULT: "#5ca86b", // viejo text-money
          bg: "var(--c-money-bg)",
          fg: "var(--c-money-fg)",
        },
        streak: "#e8763e",
        // -- Tokens semánticos del rediseño --
        bg: "var(--c-bg)",
        surface: "var(--c-surface)",
        muted: "var(--c-muted)",
        accent: "var(--c-accent)",
        sky: { bg: "var(--c-sky-bg)", fg: "var(--c-sky-fg)" },
        sun: { bg: "var(--c-sun-bg)", fg: "var(--c-sun-fg)" },
        danger: { bg: "var(--c-danger-bg)", fg: "var(--c-danger-fg)" },
      },
      boxShadow: {
        brutal: "4px 4px 0 var(--c-shadow)",
        "brutal-sm": "3px 3px 0 var(--c-shadow)",
        "brutal-lg": "6px 6px 0 var(--c-shadow)",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        display: ["Space Grotesk", "Inter", "sans-serif"],
      },
    },
  },
  plugins: [],
};
```

- [ ] **Step 4: Verificar que nada se rompe**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
```
Esperado: 87/87 y build verde (solo se agregaron tokens; ninguna ruta cambió).

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/src/index.css web/tailwind.config.js
git commit -m "feat(web): tokens neo-brutalistas claro/oscuro, fuentes y framer-motion"
```

---

### Task 2: ThemeProvider + ThemeToggle + wiring en main

**Files:**
- Create: `web/src/ui/theme.tsx`
- Create: `web/src/ui/theme.test.tsx`
- Modify: `web/src/main.tsx`

- [ ] **Step 1: Tests que fallan** — `web/src/ui/theme.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, ThemeToggle, useTheme } from "./theme";

function Probe() {
  const { theme } = useTheme();
  return <span data-testid="theme">{theme}</span>;
}

describe("ThemeProvider", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("arranca en claro por defecto y aplica data-theme", () => {
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>
    );
    expect(screen.getByTestId("theme").textContent).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("lee la preferencia guardada al montar", () => {
    localStorage.setItem("focus365-theme", "dark");
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("el toggle cambia el tema, el atributo y persiste", async () => {
    render(
      <ThemeProvider>
        <ThemeToggle />
        <Probe />
      </ThemeProvider>
    );
    await userEvent.click(screen.getByRole("button", { name: "Cambiar a tema oscuro" }));
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem("focus365-theme")).toBe("dark");
    // El aria-label refleja el próximo estado.
    expect(screen.getByRole("button", { name: "Cambiar a tema claro" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Verificar que fallan** (`npx vitest run src/ui/theme.test.tsx` → import error).

- [ ] **Step 3: Implementar** `web/src/ui/theme.tsx`:

```tsx
import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { motion } from "framer-motion";

type Theme = "light" | "dark";
const STORAGE_KEY = "focus365-theme";

const ThemeContext = createContext<{ theme: Theme; toggle: () => void } | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() =>
    localStorage.getItem(STORAGE_KEY) === "dark" ? "dark" : "light"
  );

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem(STORAGE_KEY, theme);
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
```

- [ ] **Step 4: Wiring en `web/src/main.tsx`** (ThemeProvider por fuera, MotionConfig para reduced-motion):

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { MotionConfig } from "framer-motion";
import { router } from "./router";
import { AuthProvider } from "./lib/auth";
import { ThemeProvider } from "./ui/theme";
import "./index.css";

const queryClient = new QueryClient();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <MotionConfig reducedMotion="user">
        <QueryClientProvider client={queryClient}>
          <AuthProvider>
            <RouterProvider router={router} />
          </AuthProvider>
        </QueryClientProvider>
      </MotionConfig>
    </ThemeProvider>
  </React.StrictMode>
);
```

- [ ] **Step 5: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/ui/theme.tsx web/src/ui/theme.test.tsx web/src/main.tsx
git commit -m "feat(web): ThemeProvider con persistencia y ThemeToggle animado"
```

---

### Task 3: Primitivas estáticas — Card, Chip, Button, Input

**Files:**
- Create: `web/src/ui/Card.tsx`, `web/src/ui/Chip.tsx`, `web/src/ui/Button.tsx`, `web/src/ui/Input.tsx`
- Create: `web/src/ui/primitives.test.tsx`

(Nota: `Input` no estaba en la lista del spec §4; se agrega como 9.ª primitiva porque login/registro y las rutas de R13 comparten el mismo input — DRY.)

- [ ] **Step 1: Tests que fallan** — `web/src/ui/primitives.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Card } from "./Card";
import { Chip } from "./Chip";
import { Button } from "./Button";
import { Input } from "./Input";

describe("Card", () => {
  it("renderiza children y acepta className extra", () => {
    render(<Card className="extra">contenido</Card>);
    const el = screen.getByText("contenido");
    expect(el.className).toContain("extra");
    expect(el.className).toContain("border-ink");
  });

  it("interactive agrega el hover de levantamiento", () => {
    render(<Card interactive>hover</Card>);
    expect(screen.getByText("hover").className).toContain("hover:");
  });
});

describe("Chip", () => {
  it("aplica la variante de color", () => {
    render(<Chip variant="money">+$3,200</Chip>);
    expect(screen.getByText("+$3,200").className).toContain("bg-money-bg");
  });
});

describe("Button", () => {
  it("dispara onClick y respeta disabled", async () => {
    const onClick = vi.fn();
    const { rerender } = render(<Button onClick={onClick}>Guardar</Button>);
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    expect(onClick).toHaveBeenCalledTimes(1);

    rerender(<Button onClick={onClick} disabled>Guardar</Button>);
    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled();
  });

  it("la variante ghost usa surface", () => {
    render(<Button variant="ghost">Cancelar</Button>);
    expect(screen.getByRole("button", { name: "Cancelar" }).className).toContain("bg-surface");
  });
});

describe("Input", () => {
  it("propaga props y escribe", async () => {
    render(<Input aria-label="Email" placeholder="Email" />);
    const input = screen.getByLabelText("Email");
    await userEvent.type(input, "a@b.com");
    expect((input as HTMLInputElement).value).toBe("a@b.com");
  });
});
```

- [ ] **Step 2: Verificar que fallan** (imports inexistentes).

- [ ] **Step 3: Implementar.**

`web/src/ui/Card.tsx`:

```tsx
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
```

`web/src/ui/Chip.tsx`:

```tsx
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
```

`web/src/ui/Button.tsx`:

```tsx
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
```

(Si TypeScript pelea con los tipos de eventos entre `ButtonHTMLAttributes` y
`motion.button`, usar `HTMLMotionProps<"button">` de framer-motion como base
del tipo Props en vez de `ButtonHTMLAttributes` — comportamiento idéntico.)

`web/src/ui/Input.tsx`:

```tsx
import { InputHTMLAttributes, forwardRef } from "react";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className = "", ...rest }, ref) {
    return (
      <input
        ref={ref}
        className={`w-full rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm text-ink outline-none transition-shadow focus:shadow-brutal-sm ${className}`}
        {...rest}
      />
    );
  }
);
```

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/primitives.test.tsx && npm run build
git add web/src/ui
git commit -m "feat(web): primitivas Card, Chip, Button e Input del lenguaje neo-brutalista"
```

---

### Task 4: Primitivas animadas — Stat, ProgressBar, PageTransition, Reveal

**Files:**
- Create: `web/src/ui/Stat.tsx`, `web/src/ui/ProgressBar.tsx`, `web/src/ui/PageTransition.tsx`, `web/src/ui/Reveal.tsx`
- Create: `web/src/ui/animated.test.tsx`

- [ ] **Step 1: Tests que fallan** — `web/src/ui/animated.test.tsx` (con `MotionConfig reducedMotion="always"` los valores finales son inmediatos y deterministas):

```tsx
import { describe, it, expect } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MotionConfig } from "framer-motion";
import { Stat } from "./Stat";
import { ProgressBar } from "./ProgressBar";
import { PageTransition } from "./PageTransition";
import { Reveal, RevealItem } from "./Reveal";

function renderStill(ui: React.ReactNode) {
  return render(<MotionConfig reducedMotion="always">{ui}</MotionConfig>);
}

describe("Stat", () => {
  it("muestra etiqueta y el valor final con sufijo", async () => {
    renderStill(<Stat label="Racha actual" value={12} suffix=" días" />);
    expect(screen.getByText("Racha actual")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("12 días")).toBeInTheDocument());
  });

  it("acepta un formateador (montos)", async () => {
    renderStill(
      <Stat label="Neto" value={320000} format={(n) => `$${(n / 100).toLocaleString("en-US")}`} />
    );
    await waitFor(() => expect(screen.getByText("$3,200")).toBeInTheDocument());
  });
});

describe("ProgressBar", () => {
  it("expone el porcentaje vía role progressbar", () => {
    renderStill(<ProgressBar value={60} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "60");
  });

  it("clampa fuera de rango", () => {
    renderStill(<ProgressBar value={140} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
  });
});

describe("PageTransition y Reveal", () => {
  it("renderizan children", () => {
    renderStill(
      <PageTransition>
        <Reveal>
          <RevealItem>uno</RevealItem>
          <RevealItem>dos</RevealItem>
        </Reveal>
      </PageTransition>
    );
    expect(screen.getByText("uno")).toBeInTheDocument();
    expect(screen.getByText("dos")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar.**

`web/src/ui/Stat.tsx`:

```tsx
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
```

`web/src/ui/ProgressBar.tsx`:

```tsx
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
```

`web/src/ui/PageTransition.tsx`:

```tsx
import { ReactNode } from "react";
import { motion } from "framer-motion";

// Entrada estándar de página: fade + slide-up corto.
export function PageTransition({ children }: { children: ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}
```

`web/src/ui/Reveal.tsx`:

```tsx
import { ReactNode } from "react";
import { motion } from "framer-motion";

const container = {
  hidden: {},
  show: { transition: { staggerChildren: 0.06 } },
};

const item = {
  hidden: { opacity: 0, y: 10 },
  show: { opacity: 1, y: 0, transition: { duration: 0.25, ease: "easeOut" } },
};

// Reveal + RevealItem: cascada con stagger para grids de tarjetas.
export function Reveal({ className = "", children }: { className?: string; children: ReactNode }) {
  return (
    <motion.div className={className} variants={container} initial="hidden" animate="show">
      {children}
    </motion.div>
  );
}

export function RevealItem({ className = "", children }: { className?: string; children: ReactNode }) {
  return (
    <motion.div className={className} variants={item}>
      {children}
    </motion.div>
  );
}
```

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/ && npm run build
git add web/src/ui
git commit -m "feat(web): primitivas animadas Stat, ProgressBar, PageTransition y Reveal"
```

---

### Task 5: TopBar neo-brutalista

**Files:**
- Modify: `web/src/components/TopBar.tsx`
- Modify: `web/src/components/TopBar.test.tsx`

- [ ] **Step 1: Ampliar el test (falla primero).** En `TopBar.test.tsx`:

1. El `renderBar` envuelve en ThemeProvider (el toggle lo necesita):

```tsx
import { ThemeProvider } from "@/ui/theme";
// ...
// @ts-ignore router de prueba
render(
  <ThemeProvider>
    <RouterProvider router={router} />
  </ThemeProvider>
);
```

2. Test nuevo dentro del describe:

```tsx
it("incluye el toggle de tema", async () => {
  mockAuth.user = { id: "u1", email: "a@b.com", name: "Ana" };
  renderBar();
  expect(
    await screen.findByRole("button", { name: /Cambiar a tema/ })
  ).toBeInTheDocument();
});
```

- [ ] **Step 2: Verificar que falla** (`npx vitest run src/components/TopBar.test.tsx` — no hay toggle).

- [ ] **Step 3: Reescribir `TopBar.tsx`** (textos intactos: «Focus 365», labels de nav, «Salir»):

```tsx
import { Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth";
import { ThemeToggle } from "@/ui/theme";

const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
  { to: "/asistente", label: "Asistente" },
];

// TopBar es la barra de navegación persistente. Sólo se muestra con usuario;
// en /login y /register useAuth devuelve user null y no se renderiza.
export function TopBar() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <nav className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b-[2.5px] border-ink bg-bg px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Link
          to="/"
          className="inline-block -rotate-1 rounded-sm bg-ink px-2 py-0.5 font-display text-sm font-bold uppercase tracking-tight text-bg shadow-brutal-sm transition-transform hover:rotate-1"
        >
          Focus 365
        </Link>
        <div className="flex flex-wrap gap-1.5 text-sm">
          {LINKS.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={
                pathname === l.to
                  ? "rounded-md border-2 border-ink bg-accent px-2 py-0.5 font-bold text-[#16130e] shadow-brutal-sm"
                  : "rounded-md border-2 border-transparent px-2 py-0.5 font-medium text-muted transition-colors hover:border-ink hover:text-ink"
              }
            >
              {l.label}
            </Link>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-3">
        <ThemeToggle />
        <button
          onClick={logout}
          className="text-sm font-medium text-muted transition-colors hover:text-ink"
        >
          Salir
        </button>
      </div>
    </nav>
  );
}
```

- [ ] **Step 4: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/components
git commit -m "feat(web): TopBar neo-brutalista con marca sticker, nav de chips y toggle de tema"
```

---

### Task 6: Login y Registro

**Files:**
- Modify: `web/src/routes/login.tsx`
- Modify: `web/src/routes/register.tsx`
- (Tests existentes `login.test.tsx` sin cambios de aserciones: textos/labels se conservan.)

- [ ] **Step 1: Reescribir el cuerpo visual de `login.tsx`** (lógica intacta — mismos estados, handlers, textos y aria-labels):

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/login")({ component: LoginPage });

function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await login(email, password);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al iniciar sesión");
    }
  }

  return (
    <PageTransition>
      <div className="flex min-h-screen items-center justify-center p-6">
        <Card className="w-full max-w-sm p-6">
          <form onSubmit={onSubmit} className="space-y-4">
            <span className="inline-block -rotate-1 rounded-sm bg-ink px-2 py-0.5 font-display text-xl font-bold uppercase tracking-tight text-bg shadow-brutal-sm">
              Focus 365
            </span>
            <p className="text-sm text-muted">Inicia sesión para continuar.</p>
            <Input
              aria-label="Email" type="email" placeholder="Email" value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Input
              aria-label="Contraseña" type="password" placeholder="Contraseña" value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {error && (
              <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full">
              Entrar
            </Button>
            <p className="text-center text-xs text-muted">
              ¿Sin cuenta?{" "}
              <Link to="/register" className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2">
                Regístrate
              </Link>
            </p>
          </form>
        </Card>
      </div>
    </PageTransition>
  );
}
```

- [ ] **Step 2: `register.tsx` igual** — leer el archivo actual y aplicar la misma transformación: `PageTransition` + `Card` + `Input` por cada campo (conservando aria-labels y textos exactos: «Nombre», «Email», «Contraseña», el botón y el link a login), error como banda `danger`, botón primario full-width.

- [ ] **Step 3: Suite + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/routes/login.tsx web/src/routes/register.tsx
git commit -m "feat(web): login y registro con el lenguaje neo-brutalista"
```

---

### Task 7: Dashboard — hero de racha, tiles de color en cascada

**Files:**
- Modify: `web/src/routes/index.tsx`
- Modify: `web/src/routes/index.test.tsx` (solo el wrapper de render)

- [ ] **Step 1: Ajustar el harness del test (falla después, al cambiar la página, si falta).** En `index.test.tsx`, envolver el render con `MotionConfig reducedMotion="always"` para que el contador de Stat sea determinista:

```tsx
import { MotionConfig } from "framer-motion";
// en renderPage(), alrededor del QueryClientProvider:
render(
  <MotionConfig reducedMotion="always">
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  </MotionConfig>
);
```

NO cambiar ninguna aserción: los textos del dashboard nuevo son los mismos.

- [ ] **Step 2: Reescribir `index.tsx`.** Misma lógica de datos (queries, guard, estados de carga/error con sus textos exactos: «Cargando tu día…», «No pudimos cargar tu día.», «Reintentar»); cambia la composición visual:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getDashboard, todayString, type Snapshot } from "@/lib/dashboard";
import { formatMXN } from "@/lib/finances";
import { getInsight } from "@/lib/ai";
import { Card } from "@/ui/Card";
import { Chip } from "@/ui/Chip";
import { Stat } from "@/ui/Stat";
import { Button } from "@/ui/Button";
import { PageTransition } from "@/ui/PageTransition";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/")({ component: DashboardPage });

function DashboardPage() {
  const { user } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const query = useQuery({
    queryKey: ["dashboard", todayString()],
    queryFn: getDashboard,
    enabled: !!user,
  });

  if (!user) return null;

  if (query.isLoading) {
    return <p className="p-6 text-muted">Cargando tu día…</p>;
  }

  if (query.isError || !query.data) {
    return (
      <div className="p-6">
        <p className="w-fit rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
          No pudimos cargar tu día.
        </p>
        <Button variant="ghost" className="mt-3" onClick={() => query.refetch()}>
          Reintentar
        </Button>
      </div>
    );
  }

  const s = query.data;
  const fecha = new Date().toLocaleDateString("es-MX", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });

  return (
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <AIBand />
        <p className="mt-4 text-sm text-muted">
          Hola, <span className="font-bold text-ink">{user.name}</span> · {fecha} ·{" "}
          {s.dimensions_active} dimensiones en marcha
        </p>

        <Reveal className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <RevealItem>
            <StreakHero s={s} />
          </RevealItem>
          <RevealItem>
            <FinanceCard s={s} />
          </RevealItem>
        </Reveal>

        <Reveal className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <RevealItem><MoodCard s={s} /></RevealItem>
          <RevealItem><CheckinCard s={s} /></RevealItem>
          <RevealItem><TrainingCard s={s} /></RevealItem>
          <RevealItem><GoalsCard s={s} /></RevealItem>
        </Reveal>
      </div>
    </PageTransition>
  );
}

function AIBand() {
  const { user } = useAuth();
  const insightQ = useQuery({
    queryKey: ["ai-insight", todayString()],
    queryFn: getInsight,
    enabled: !!user,
    // Si la IA falla, degradamos al placeholder sin reintentar: la banda nunca
    // debe quedarse cargando ni golpear repetidamente un endpoint caído.
    retry: false,
  });

  let content = "✦ Tu insight del día llega pronto";
  if (insightQ.isLoading) {
    content = "✦ Generando tu insight…";
  } else if (insightQ.data?.available && insightQ.data.content) {
    content = `✦ ${insightQ.data.content}`;
  }
  return (
    <Link to="/asistente" className="block">
      <Card interactive className="flex items-center gap-3 px-4 py-3">
        <Chip variant="accent">IA</Chip>
        <span className="text-sm font-bold">{content}</span>
      </Card>
    </Link>
  );
}

// Tile envuelve Card interactiva en un Link; bg permite pintar el tile entero
// con un chip-color (los hijos ponen el texto con su fg correspondiente).
function Tile({
  to,
  title,
  bg = "",
  children,
}: {
  to: string;
  title: string;
  bg?: string;
  children: React.ReactNode;
}) {
  return (
    <Link to={to} className="block h-full">
      <Card interactive className={`flex h-full min-h-[72px] flex-col gap-1 p-4 ${bg}`}>
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] opacity-70">
          {title}
        </span>
        {children}
      </Card>
    </Link>
  );
}

function StreakHero({ s }: { s: Snapshot }) {
  return (
    <Link to="/disciplina" className="block h-full">
      <Card interactive className="flex h-full flex-col justify-between bg-accent p-5 text-[#16130e]">
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] opacity-70">
          🔥 Racha
        </span>
        {s.streak.total === 0 ? (
          <span className="text-sm font-medium opacity-80">Sin hábitos aún</span>
        ) : (
          <>
            <div className="flex items-end gap-2">
              <Stat label="" value={s.streak.best_current} suffix=" días" className="[&>div:first-child]:hidden" />
              <span className="animate-flicker text-2xl">🔥</span>
            </div>
            <span className="text-xs font-bold opacity-80">
              {s.streak.done_today}/{s.streak.total} hábitos hoy
            </span>
          </>
        )}
      </Card>
    </Link>
  );
}

function FinanceCard({ s }: { s: Snapshot }) {
  const bg =
    s.finance.status === "verde"
      ? "bg-money-bg text-money-fg"
      : s.finance.status === "rojo"
        ? "bg-danger-bg text-danger-fg"
        : "";
  return (
    <Tile to="/finanzas" title="Superávit del ciclo" bg={bg}>
      <Stat
        label=""
        value={s.finance.net}
        format={formatMXN}
        className="[&>div:first-child]:hidden"
      />
      <span className="text-xs font-bold opacity-70">
        {s.finance.cycle} · {s.finance.status}
      </span>
    </Tile>
  );
}

function Bar({ value }: { value: number }) {
  // value 1-10 → ancho proporcional.
  return (
    <div className="h-2 w-full overflow-hidden rounded-md border-2 border-ink bg-surface">
      <div className="h-full bg-accent" style={{ width: `${value * 10}%` }} />
    </div>
  );
}

function MoodCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/check-in" title="Ánimo / Energía" bg="bg-sky-bg text-sky-fg">
      {s.checkin == null ? (
        <span className="text-xs opacity-80">Sin check-in hoy</span>
      ) : (
        <div className="flex flex-col gap-1">
          <Bar value={s.checkin.mood} />
          <Bar value={s.checkin.energy} />
        </div>
      )}
    </Tile>
  );
}

function CheckinCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/check-in" title="Check-in de hoy" bg="bg-sun-bg text-sun-fg">
      {s.checkin?.present ? (
        <span className="text-xs font-bold">Hecho ✓ · disciplina {s.checkin.discipline}</span>
      ) : (
        <span className="text-xs opacity-80">Pendiente</span>
      )}
    </Tile>
  );
}

function TrainingCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/entrenamiento" title="Entreno de hoy">
      <span className="text-xs font-bold text-muted">
        {s.training.trained_today ? `${s.training.type} ✓` : "Sin entreno hoy"}
      </span>
    </Tile>
  );
}

function GoalsCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/metas" title="Metas activas">
      <span className="text-xs font-bold text-muted">
        {s.goals.active} activas · {s.goals.avg_progress}% prom.
      </span>
      {s.goals.overdue > 0 && (
        <Chip variant="danger" className="mt-1 w-fit">
          {s.goals.overdue} vencida(s)
        </Chip>
      )}
    </Tile>
  );
}
```

Notas para el implementador:
- El truco `[&>div:first-child]:hidden` oculta la etiqueta interna de `Stat` cuando el Tile ya pone su propio título — los textos visibles quedan idénticos a los actuales (p. ej. «12 días», «$3,200.00», «2/4 hábitos hoy»).
- `formatMXN` ya formatea centavos; `Stat` cuenta el número crudo y `format` lo presenta en cada frame.
- Antes de tocar la página, correr `npx vitest run src/routes/index.test.tsx` y revisar qué textos asser­ta, para no romperlos.

- [ ] **Step 3: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/routes/index.tsx web/src/routes/index.test.tsx
git commit -m "feat(web): dashboard neo-brutalista con hero de racha, tiles de color y cascada"
```

---

### Task 8: Verificación final, smoke visual y cierre

- [ ] **Step 1: Suites completas**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```

- [ ] **Step 2: Rebuild docker + smoke funcional**

```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"; cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
/tmp/smoke_actions.sh   # las acciones del chat siguen vivas (8/8)
```
(Requiere `dangerouslyDisableSandbox: true`.)

- [ ] **Step 3: Smoke visual** — pedirle al usuario que abra `http://localhost:5174`, haga login y revise dashboard + login en ambos temas (toggle del TopBar). Las rutas no migradas (check-in, finanzas, etc.) se verán con la piel vieja sobre el fondo nuevo — es el estado transicional esperado hasta R13.

- [ ] **Step 4: Cierre** — review final holística (subagente), nits, merge `--no-ff` a `master` con superpowers:finishing-a-development-branch, bitácora en `docs/superpowers/sesiones/`.

---

## Notas para el ejecutor

- Los tests existentes asser­tan textos y roles. Si uno falla tras un restyle, casi siempre es un texto que cambió — restaurar el texto, no el test. Únicos cambios de harness permitidos: wrapper `ThemeProvider` (TopBar) y `MotionConfig reducedMotion="always"` (index).
- En jsdom no existe `color-mix` ni importan las fuentes: nada de eso se asserta.
- framer-motion en jsdom funciona; con `MotionConfig reducedMotion="always"` los valores finales son síncronos.
- La paleta vieja sigue en Tailwind a propósito (rutas de R13); no «limpiar».
