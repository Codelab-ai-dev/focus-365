# Bitácora de sesión — Rebanada 12: Rediseño UI neo-brutalista (parte 1)

**Fecha:** 2026-06-12
**Estado al cierre:** Completada y mergeada a `master`.
**Rama de trabajo:** `plan-12-rediseno-ui` (mezclada con `--no-ff` y borrada).
**Merge commit:** `92036d4`

## Qué se entregó

Primera mitad del rediseño neo-brutalista (spec compartido con la R13):
- **Sistema de diseño completo:** tokens CSS claro (crema, default) / oscuro
  (carbón) con mapeo semántico en Tailwind, Space Grotesk + Inter, papel
  punteado de fondo, focus visible accent, `framer-motion` con
  `MotionConfig reducedMotion="user"`.
- **9 primitivas en `web/src/ui/`:** theme (Provider + toggle 🌙/☀️ persistido
  con anti-flash en `index.html`), Card (hover de levantamiento), Button
  (press físico), Chip (6 variantes), Input, Stat (contador animado),
  ProgressBar, PageTransition, Reveal/RevealItem (cascada).
- **Pantallas migradas:** TopBar (marca sticker rotada, nav de chips, toggle),
  dashboard (hero de racha naranja con fuego flicker + tiles de color en
  cascada), login y registro.
- Las 6 rutas restantes quedan con la piel vieja sobre el fondo nuevo hasta la
  R13 (la paleta vieja coexiste en Tailwind a propósito).

- **Spec (R12+R13):** `docs/superpowers/specs/2026-06-11-plan-12-rediseno-ui-design.md`
- **Plan R12:** `docs/superpowers/plans/2026-06-12-plan-12-rediseno-ui.md`

## Proceso de diseño

Brainstorming con el **visual companion** (mockups en el navegador): se
evaluaron 3 direcciones (Brasa, Neo-brutalista, Aurora) → eligió neo-brutalista;
se mostró la variante dark → ambos temas con toggle, claro por defecto; se
eligió Space Grotesk entre 3 voces tipográficas renderizadas. Mockups
persistidos en `.superpowers/brainstorm/`.

## Commits (en orden)

| Commit | Tarea |
|--------|-------|
| `dd817e7` | Tokens, fuentes, framer-motion, Tailwind semántico |
| `90c9f97` | Fix de tests date-flaky (`/12/` → `/12 días/`, matcheaba la fecha del día 12) |
| `978db27` | ThemeProvider + ThemeToggle + wiring (MotionConfig) |
| `f51c67c` | Card, Chip, Button, Input |
| `799fa97` | Stat, ProgressBar, PageTransition, Reveal |
| `ea18f13` | TopBar |
| `3463f52` | Login y registro |
| `74e2727` | Dashboard (+ fix: `useReducedMotionConfig` en Stat) |
| `7a0a8a3` | Nits de la review final |
| `92036d4` | Merge a master |

## Decisiones / desviaciones de la sesión

- **Ejecución:** subagent-driven (Sonnet por tarea, diffs verificados).
  Review final con Opus: `APPROVED_WITH_NITS`, resueltos en `7a0a8a3`:
  1. **`__root.tsx` con `bg-ink-950` fijo** (Important): el layout raíz pintaba
     un panel oscuro detrás de todo, tapando el crema del tema claro → ahora
     `bg-bg text-ink`.
  2. **Anti-flash de tema:** script inline en `index.html` que aplica
     `data-theme=dark` antes del bundle (el effect llegaba tarde y producía un
     destello claro animado).
  3. **localStorage blindado** con try/catch (modo privado no tumba la app).
- La review verificó contraste AA de todos los pares de chips en ambos temas
  (mínimo 7.15:1), que `whileTap` no dispara en botones disabled, y la
  paridad exacta de los textos del dashboard contra la versión vieja.
- `Stat` usa `useReducedMotionConfig` (no `useReducedMotion`): respeta el
  `MotionConfig` del contexto — clave para tests deterministas con
  `reducedMotion="always"`.
- Hallazgo de la Task 1: dos tests del dashboard eran **date-flaky** (regex
  `/12/` matcheaba «viernes, 12 de junio»); fallaban solo los días 12.
  Corregidos a `/12 días/`.
- Nit pendiente para R13: el truco `[&>div:first-child]:hidden` para ocultar
  la etiqueta de `Stat` es frágil; mejor un prop `hideLabel`. Y si la R13 usa
  `Stat` con valores vivos, animar desde el valor actual (hoy re-cuenta de 0).

## Verificación al cierre

- Frontend: **102/102** Vitest (21 archivos; 15 tests nuevos del sistema) +
  build estricto. Backend intacto.
- Docker reconstruido; smoke funcional de acciones IA **8/8** sobre la UI nueva.
- Smoke visual del usuario en ambos temas: aprobado («se ve increíble»).
- Árbol limpio, rama borrada, servidor de mockups detenido.

## Fuera de alcance (siguiente: R13)

Migrar check-in, finanzas, entrenamiento, disciplina, metas y asistente a las
primitivas; eliminar la paleta vieja de Tailwind y cualquier clase huérfana;
los dos nits de `Stat`.
