# Plan 13 — Rediseño UI neo-brutalista (parte 2: las 6 rutas + limpieza) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrar check-in, finanzas, entrenamiento, disciplina, metas y asistente al sistema neo-brutalista de la R12 y eliminar la paleta vieja.

**Architecture:** Reskin mecánico guiado por un diccionario de transformación (clase vieja → primitiva/clase nueva); cero cambios de lógica, datos, textos o accesibilidad. Cierra con la limpieza de Tailwind (paleta vieja fuera) y dos mejoras de `Stat` heredadas de la review de R12.

**Tech Stack:** React + Tailwind + framer-motion + primitivas de `web/src/ui/` (R12).

**Spec:** `docs/superpowers/specs/2026-06-11-plan-12-rediseno-ui-design.md` (§6 rutas 5-10, §9).

**Reglas transversales (aplican a TODAS las tareas):**
1. **Antes de editar una ruta, leer su archivo completo y su test.** Los textos visibles, aria-labels, roles y la lógica (queries, mutaciones, estados) son INVARIANTES. Si un test falla tras el reskin, se restaura el texto/estructura — nunca se cambia la aserción.
2. **Único cambio de harness permitido:** envolver el render del test en `<MotionConfig reducedMotion="always">` (import de framer-motion) si la ruta ganó animaciones de Reveal/Stat que hagan asíncrono lo que era síncrono.
3. Verificación por tarea: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build` — **102/102 + build limpio** (o más si la tarea agrega tests).
4. Commits en español terminando con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
5. Rama: `plan-13-rediseno-ui-rutas` desde `master` antes de la Task 1 (NO worktree).

## Diccionario de transformación (el corazón del reskin)

| Viejo | Nuevo |
|-------|-------|
| Contenedor `rounded-* border border-ink-700 bg-ink-900/800 p-*` (tarjeta/sección/form) | `<Card className="p-4">` (import `@/ui/Card`); si envuelve un link clickeable, `<Card interactive>` dentro del `<Link className="block">` |
| `border-ink-700` suelto (divisores, bordes) | `border-ink` con `border-2` |
| `text-sand-100` | quitar (se hereda `text-ink` del root) o `text-ink` si hace falta explícito |
| `text-sand-400` | `text-muted` |
| `text-amber-brand` (acentos/links) | links: `font-bold text-ink underline decoration-accent decoration-2 underline-offset-2`; acentos no-link: `text-accent` |
| `bg-amber-brand ... text-ink-950` (botón) | `<Button>` (primary); botones secundarios/outline → `<Button variant="ghost">` |
| `<input>`/`<select>` con clases viejas | `<Input>` (`@/ui/Input`); para `<select>`/`<textarea>` aplicar las mismas clases del Input literal: `w-full rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm text-ink outline-none transition-shadow focus:shadow-brutal-sm` |
| `text-money` (montos positivos/income) | dentro de fila/tarjeta: `<Chip variant="money" size="sm">` envolviendo el monto, o `text-money-fg` SOLO si el contenedor ya es `bg-money-bg` |
| `text-streak` para **errores** | banda danger: `rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm` |
| `text-streak` para **racha/fuego/overdue** | `<Chip variant="danger">` (overdue) o `text-accent font-bold` (rachas) |
| `h1`/títulos de página | `font-display text-xl font-bold tracking-tight` |
| Números/montos grandes | agregar `font-display tracking-tight` |
| Wrapper de página `<div className="p-6 ...">` | `<PageTransition><div className="mx-auto max-w-3xl p-6 ...">` (asistente conserva su `max-w-xl`) |
| Listas/grids principales (hábitos, movimientos, metas, ejercicios, sesiones) | envolver en `<Reveal>` con cada item en `<RevealItem>` (cascada) — solo la lista principal de cada página, no todo |
| Estados "Cargando…" | conservar texto, clase `text-muted` |

Etiquetas pequeñas tipo overline (donde ya existan títulos de sección): `text-[10px] font-bold uppercase tracking-[0.12em] text-muted`.

---

### Task 1: `Stat` — `hideLabel` y animar desde el valor actual (nits de R12)

**Files:**
- Modify: `web/src/ui/Stat.tsx`
- Modify: `web/src/ui/animated.test.tsx`
- Modify: `web/src/routes/index.tsx` (reemplazar el truco `[&>div:first-child]:hidden`)

- [ ] **Step 1: Tests que fallan.** En `animated.test.tsx`, dentro del describe de Stat:

```tsx
it("hideLabel oculta la etiqueta", () => {
  renderStill(<Stat label="Racha" value={5} hideLabel />);
  expect(screen.queryByText("Racha")).toBeNull();
  expect(screen.getByText("5")).toBeInTheDocument();
});

it("al cambiar value muestra el valor nuevo (sin recontar desde 0)", async () => {
  const { rerender } = renderStill(<Stat label="N" value={10} />);
  await waitFor(() => expect(screen.getByText("10")).toBeInTheDocument());
  rerender(
    <MotionConfig reducedMotion="always">
      <Stat label="N" value={25} />
    </MotionConfig>
  );
  await waitFor(() => expect(screen.getByText("25")).toBeInTheDocument());
});
```

- [ ] **Step 2: Verificar que fallan** (`npx vitest run src/ui/animated.test.tsx` — prop inexistente).

- [ ] **Step 3: Implementar.** `Stat.tsx` completo nuevo:

```tsx
import { useEffect, useRef, useState } from "react";
import { animate } from "framer-motion";
import { useReducedMotionConfig } from "framer-motion";

// Stat: etiqueta uppercase + número display con contador animado. hideLabel
// permite usarlo dentro de tiles que ya ponen su propio título. Al cambiar
// value, anima desde el valor mostrado (no recuenta desde 0).
export function Stat({
  label,
  value,
  prefix = "",
  suffix = "",
  format,
  hideLabel = false,
  className = "",
}: {
  label: string;
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
```

(Nota: conservar el import de `useReducedMotionConfig` tal como quedó en R12 — si el archivo actual lo importa en una sola línea junto a `animate`, mantener ese estilo.)

- [ ] **Step 4: Reemplazar el truco en `index.tsx`** — en `StreakHero` y `FinanceCard`, cambiar `className="[&>div:first-child]:hidden"` por la prop `hideLabel` (quitar la clase, agregar `hideLabel`).

- [ ] **Step 5: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/ui/Stat.tsx web/src/ui/animated.test.tsx web/src/routes/index.tsx
git commit -m "refactor(web): Stat con hideLabel y animación desde el valor actual"
```

---

### Task 2: Check-in

**Files:**
- Modify: `web/src/routes/check-in.tsx`
- Test: `web/src/routes/check-in.test.tsx` (solo harness si hace falta, regla 2)

- [ ] **Step 1:** Leer `check-in.tsx` y `check-in.test.tsx` completos; listar los textos/labels asser­tados.
- [ ] **Step 2:** Aplicar el diccionario: `PageTransition` + contenedor centrado; el form en `Card`; los tres valores (ánimo/energía/disciplina) conservan su mecánica actual (sliders/steppers/inputs según lo que exista) pero con la piel nueva — si son `<input type="number"|"range">` usar `Input`/clases del diccionario; labels como overline; nota como `Input`/textarea con clases del diccionario; submit con `<Button>`; errores como banda danger; mensaje de éxito (si existe) con `<Chip variant="money">`.
- [ ] **Step 3:** `npx vitest run src/routes/check-in.test.tsx` → verde sin cambiar aserciones; luego suite completa + build.
- [ ] **Step 4:** Commit: `git add web/src/routes/check-in.tsx web/src/routes/check-in.test.tsx && git commit -m "feat(web): check-in con el lenguaje neo-brutalista"`

---

### Task 3: Finanzas

**Files:**
- Modify: `web/src/routes/finanzas.tsx`
- Test: `web/src/routes/finanzas.test.tsx`

- [ ] **Step 1:** Leer archivo + test; listar invariantes.
- [ ] **Step 2:** Aplicar el diccionario, además:
  - El resumen del ciclo (ingresos/gastos/neto) usa `<Stat>` con `format={formatMXN}` (los montos llegan en centavos) — el neto en una `Card` con `bg-money-bg text-money-fg` si verde / `bg-danger-bg text-danger-fg` si rojo (mismo criterio que el FinanceCard del dashboard).
  - La lista de movimientos en `<Reveal>`: cada movimiento es una fila-`Card` (`p-3`, flex justify-between) con el monto en `font-display font-bold` — income con `<Chip variant="money">+{monto}</Chip>`, expense con `<Chip variant="danger">−{monto}</Chip>` SOLO si el formato actual del texto lo permite sin cambiarlo (si el test asserta el string exacto del monto, conservar el string exacto y poner el chip alrededor).
  - El form de alta en `Card` con `Input`/select del diccionario y `<Button>`.
- [ ] **Step 3:** Suite + build. **Step 4:** Commit `feat(web): finanzas con el lenguaje neo-brutalista`.

---

### Task 4: Entrenamiento

**Files:**
- Modify: `web/src/routes/entrenamiento.tsx`
- Test: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1:** Leer archivo + test; listar invariantes.
- [ ] **Step 2:** Diccionario, además: catálogo de ejercicios y sesiones como `Card`s en `<Reveal>`; series/repeticiones como `<Chip variant="sky" size="sm">`; botones de acción (`agregar serie`, `terminar sesión`, etc.) con `Button`/`Button variant="ghost"` según jerarquía (el primario de la pantalla es uno solo).
- [ ] **Step 3:** Suite + build. **Step 4:** Commit `feat(web): entrenamiento con el lenguaje neo-brutalista`.

---

### Task 5: Disciplina (hábitos) — con el check satisfactorio

**Files:**
- Modify: `web/src/routes/disciplina.tsx`
- Test: `web/src/routes/disciplina.test.tsx`

- [ ] **Step 1:** Leer archivo + test; listar invariantes (incluido cómo se marca hoy un hábito: botón/checkbox y sus textos/aria).
- [ ] **Step 2:** Diccionario, además: cada hábito es una fila-`Card` en `<Reveal>`; la racha del hábito como `<Chip variant="sun" size="sm">🔥 N</Chip>`; el control de marcar conserva su elemento y aria actuales pero con esta piel — si es un botón, este markup (adaptando SOLO clases, no semántica):

```tsx
<motion.button
  whileTap={{ scale: 0.85 }}
  /* conservar el onClick / aria-label / role / texto actuales */
  className={`grid h-9 w-9 place-items-center rounded-md border-[2.5px] border-ink text-lg font-bold shadow-brutal-sm transition-colors ${
    done ? "bg-accent text-[#16130e]" : "bg-surface text-muted"
  }`}
>
  {done ? "✓" : ""}
</motion.button>
```

  (import `{ motion }` de framer-motion; `done` es el booleano que ya use el componente). Si el control actual NO es un botón (p. ej. checkbox), conservar el elemento y solo re-estilizar.
- [ ] **Step 3:** Suite + build (regla 2 de harness si la cascada hace asíncrona la lista). **Step 4:** Commit `feat(web): disciplina con check satisfactorio neo-brutalista`.

---

### Task 6: Metas

**Files:**
- Modify: `web/src/routes/metas.tsx`
- Test: `web/src/routes/metas.test.tsx`

- [ ] **Step 1:** Leer archivo + test; listar invariantes.
- [ ] **Step 2:** Diccionario, además: cada meta es una `Card` en `<Reveal>` con su progreso en `<ProgressBar value={progress} className="mt-2" />` (la barra vieja se elimina; si el porcentaje se muestra como texto, conservar el texto exacto al lado); overdue con `<Chip variant="danger">`; el form de creación/edición con `Input`/`Button`.
- [ ] **Step 3:** Suite + build. **Step 4:** Commit `feat(web): metas con barras de progreso animadas`.

---

### Task 7: Asistente (chat + tarjeta de acción)

**Files:**
- Modify: `web/src/routes/asistente.tsx`
- Test: `web/src/routes/asistente.test.tsx`

- [ ] **Step 1:** Leer archivo + test (7 tests; assertan burbujas, streaming, tarjeta de acción con botones Confirmar/Cancelar y estados). TODO el comportamiento es invariante.
- [ ] **Step 2:** Aplicar:
  - Burbujas: usuario → `self-end rounded-lg border-2 border-ink bg-accent/30 px-3 py-2 text-sm shadow-brutal-sm`; asistente → `self-start rounded-lg border-2 border-ink bg-surface px-3 py-2 text-sm shadow-brutal-sm`; «Pensando…» igual que asistente pero `text-muted`.
  - La burbuja parcial del streaming usa la misma piel del asistente (solo cambian clases, la lógica de `streaming` queda intacta).
  - `ActionCard`: el contenedor pasa a `mt-2 rounded-lg border-2 border-ink bg-bg p-3 text-sm shadow-brutal-sm`; el título conserva su texto con `font-display font-bold`; los botones Confirmar/Cancelar pasan a `<Button className="px-3 py-1 text-xs">` y `<Button variant="ghost" className="px-3 py-1 text-xs">` (textos exactos); «✓ Hecha» → `<Chip variant="money" size="sm">✓ Hecha</Chip>`; «Cancelada» → `text-xs text-muted` como hoy.
  - Input + botón Enviar → `Input` y `Button` (aria-label «Mensaje» y texto «Enviar» intactos); error inline → banda danger.
  - Wrapper `PageTransition` (conserva `max-w-xl`).
- [ ] **Step 3:** Suite + build (los 7 tests del asistente y los de streaming deben pasar sin cambios de aserción; harness regla 2 si hace falta). **Step 4:** Commit `feat(web): asistente con burbujas y tarjeta de acción neo-brutalistas`.

---

### Task 8: Limpieza — adiós paleta vieja

**Files:**
- Modify: `web/tailwind.config.js`
- Modify: `web/src/index.css` (si quedara algo viejo)
- Sweep: `web/src/**/*.tsx`

- [ ] **Step 1: Barrido de huérfanos.**

```bash
cd /Users/gustavo/Desktop/focus-365/web && grep -rn "ink-950\|ink-900\|ink-800\|ink-700\|sand-100\|sand-400\|amber-brand\|text-streak\|bg-streak\|text-money[^-]\|bg-money[^-]" src/ --include="*.tsx" | grep -v test
```
Cada hit restante se migra con el diccionario (deberían ser cero o casi tras las Tasks 2-7). Repetir hasta cero hits (excluyendo tests, que no usan clases).

- [ ] **Step 2: Quitar la paleta vieja de `tailwind.config.js`** — el bloque `colors` queda SOLO con los tokens semánticos:

```js
colors: {
  bg: "var(--c-bg)",
  surface: "var(--c-surface)",
  ink: "var(--c-ink)",
  muted: "var(--c-muted)",
  accent: "var(--c-accent)",
  money: { bg: "var(--c-money-bg)", fg: "var(--c-money-fg)" },
  sky: { bg: "var(--c-sky-bg)", fg: "var(--c-sky-fg)" },
  sun: { bg: "var(--c-sun-bg)", fg: "var(--c-sun-fg)" },
  danger: { bg: "var(--c-danger-bg)", fg: "var(--c-danger-fg)" },
},
```
(`ink` pasa de objeto a valor simple: verificar con el grep del Step 1 que ya nadie usa `ink-NNN`. `boxShadow`/`fontFamily` quedan igual.)

- [ ] **Step 3:** Suite completa + build. **Step 4:** Commit `refactor(web): fuera la paleta vieja — solo tokens semánticos`.

---

### Task 9: Verificación final, smoke visual y cierre

- [ ] **Step 1:** Suites completas (frontend + backend) como en R12.
- [ ] **Step 2:** Rebuild docker + `/tmp/smoke_actions.sh` (8/8).
- [ ] **Step 3:** Smoke visual del usuario: las 6 rutas en ambos temas.
- [ ] **Step 4:** Review final holística (subagente), nits, merge `--no-ff`, bitácora.

---

## Notas para el ejecutor

- Este plan es un **reskin guiado por diccionario**: el implementador DEBE leer la ruta y su test antes de tocar nada, y el diccionario manda sobre cualquier criterio propio. Ante ambigüedad real (un patrón que el diccionario no cubre), reportar DONE_WITH_CONCERNS describiendo la decisión tomada.
- Los tests son la red: 102 en verde al inicio; cada tarea termina en verde. El único cambio de harness permitido es el wrapper `MotionConfig reducedMotion="always"`.
- jerarquía de botones: un solo `Button` primary por pantalla; el resto ghost.
