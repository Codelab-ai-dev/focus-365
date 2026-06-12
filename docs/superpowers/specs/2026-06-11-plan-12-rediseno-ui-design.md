# Plan 12/13 — Rediseño UI neo-brutalista — Diseño

**Fecha:** 2026-06-11
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Rediseño visual completo de la web con estética **neo-brutalista**: bordes
gruesos de tinta, sombras duras desplazadas, chips de color vivos, tipografía
display pesada y animaciones notorias con propósito. **Dos temas** (claro crema
por defecto y oscuro carbón) con toggle persistido. Cero cambios de
funcionalidad: todo lo construido en las rebanadas 1–11 sigue funcionando
igual; solo cambia la piel y el movimiento.

**Decisiones (acordadas en brainstorming con mockups en el navegador):**
- **Dirección:** neo-brutalista (opción B de tres direcciones evaluadas).
- **Temas:** claro `#f4f0e6` por defecto + oscuro `#191613`, toggle 🌙/☀️ en el
  TopBar, preferencia en localStorage.
- **Tipografía:** Space Grotesk (display: títulos, números, marca) + Inter
  (cuerpo), vía Google Fonts.
- **Animación:** «notoria pero con propósito» — transiciones de página,
  cascadas, contadores, micro-interacciones físicas; nada de parallax ni
  confetti. `prefers-reduced-motion` degrada todo a fades.
- **Dependencia nueva:** `framer-motion` (aprobada).
- **Arquitectura:** tokens semánticos con CSS variables (enfoque A) + sistema
  de primitivas propio en `web/src/ui/`.
- **Ejecución en dos rebanadas:** R12 (sistema + shell + dashboard + auth) y
  R13 (las 6 rutas restantes). Este spec cubre ambas.

**Fuera de alcance:** cambios de funcionalidad o de API, responsive más allá
de lo que ya existe (la app ya es mobile-first simple), modo de alto
contraste, i18n.

## 2. Tokens y temas

CSS variables definidas en `index.css` bajo `:root` (claro) y
`[data-theme="dark"]`; Tailwind las expone como colores semánticos
(`tailwind.config.js` → `colors: { bg: "var(--c-bg)", ... }`). La paleta
`ink/sand/amber/money/streak` actual se elimina al final de R13 (R12 la deja
coexistir mientras migran las rutas).

| Token | Claro (crema) | Oscuro (carbón) | Uso |
|-------|---------------|------------------|-----|
| `--c-bg` | `#f4f0e6` | `#191613` | fondo de página |
| `--c-surface` | `#ffffff` | `#221e1a` | tarjetas |
| `--c-ink` | `#16130e` | `#f4f0e6` | texto principal y bordes |
| `--c-muted` | `#6b6258` | `#a89b8c` | texto secundario |
| `--c-shadow` | `#16130e` | `#000000` | sombra dura |
| `--c-accent` | `#ff7a3d` | `#ff7a3d` | naranja de marca (texto sobre él: siempre `#16130e`) |
| `--c-money-bg` / `--c-money-fg` | `#9fd89a` / `#16130e` | `#1f2b1e` / `#9fd89a` | chip finanzas |
| `--c-sky-bg` / `--c-sky-fg` | `#9ec7f5` / `#16130e` | `#1c2533` / `#9ec7f5` | chip info |
| `--c-sun-bg` / `--c-sun-fg` | `#f5d76e` / `#16130e` | `#2e2812` / `#f5d76e` | chip hábitos |
| `--c-danger-bg` / `--c-danger-fg` | `#f5a3a3` / `#16130e` | `#2e1a1a` / `#f5a3a3` | errores |

Reglas del lenguaje: bordes `2.5px solid var(--c-ink)` (Tailwind
`border-[2.5px] border-ink`), sombra dura `4px 4px 0 var(--c-shadow)` (tiles
chicos `3px 3px 0`), radio 8px, sin sombras difusas ni gradientes (salvo el
fuego de racha). `html` lleva `transition: background-color 200ms` y las
superficies igual, para que el cambio de tema se sienta deliberado.

**ThemeProvider** (`web/src/ui/theme.tsx`): estado `"light" | "dark"`, aplica
`data-theme` en `document.documentElement`, persiste en
`localStorage["focus365-theme"]`, claro por defecto. Exporta `useTheme` y
`ThemeToggle` (botón chip con 🌙/☀️ que rota al cambiar).

## 3. Tipografía

`index.css` importa Space Grotesk (500, 700) junto al Inter existente
(400/500/700/800). Tailwind: `fontFamily.display = ["Space Grotesk", ...]`,
`fontFamily.sans` queda Inter. La display se usa en: marca del TopBar, h1/h2
de páginas, números de Stat, montos, y los botones primarios. Tracking
apretado (`tracking-tight`) en display grande.

## 4. Primitivas (`web/src/ui/`)

Una pieza por archivo, con tests de comportamiento donde hay lógica:

| Primitiva | Qué hace |
|-----------|----------|
| `theme.tsx` | ThemeProvider + useTheme + ThemeToggle (con test: toggle cambia atributo y persiste) |
| `Card.tsx` | superficie con borde/sombra del lenguaje; prop `as`/`className`; hover: `translate(-2px,-2px)` y sombra crece a `6px 6px 0` |
| `Button.tsx` | variantes `primary` (accent) y `ghost`; press físico: `translate(2px,2px)` + sombra a `0 0 0`; estados disabled |
| `Chip.tsx` | variantes `accent/money/sky/sun/danger/plain`; tamaño `sm/md` |
| `Stat.tsx` | etiqueta uppercase + número display con contador animado al montar (de 0 al valor, ~0.8s, respeta reduced-motion); acepta prefijo/sufijo (`$`, `%`, `días`) |
| `ProgressBar.tsx` | riel con borde, relleno animado al porcentaje (spring suave) |
| `PageTransition.tsx` | wrapper de ruta: `opacity 0→1` + `y 12→0` en 250ms ease-out |
| `Reveal.tsx` | contenedor stagger (60ms entre hijos) para grids/listas |

Todas usan solo tokens semánticos (nunca hex). `framer-motion` se importa solo
dentro de `ui/` (las rutas componen primitivas, no animan a mano), con una
excepción permitida: animaciones puntuales ya existentes del chat.

## 5. Animación (reglas)

- `MotionConfig reducedMotion="user"` en la raíz (`main.tsx`): con
  `prefers-reduced-motion`, framer-motion elimina transforms y deja fades.
- Página: PageTransition en cada ruta.
- Dashboard: tiles en cascada (Reveal), contadores (Stat), fuego de racha con
  flicker sutil (keyframes CSS, 1.6s).
- Botones/tarjetas: hover/press físicos (ver §4).
- Chat: burbujas nuevas entran con slide-in corto; el streaming existente no
  cambia.
- Toggle de tema: icono rota 180° al cambiar.
- Duraciones 150–300ms, easings ease-out; nada anima en loop salvo el fuego.

## 6. Migración de pantallas

**R12 — sistema + shell + primera impresión:**
1. Tokens, fuentes, ThemeProvider, las 8 primitivas, MotionConfig.
2. **TopBar:** marca «FOCUS 365» como sticker rotado (-1°) sobre chip de tinta,
   nav como chips (activo = chip accent), ThemeToggle, logout ghost.
3. **Dashboard:** hero de racha (tarjeta accent grande con fuego y contador) +
   grid de tiles de color (finanzas=money, check-in=sky, hábitos=sun,
   entrenamiento=plain, metas=plain) en cascada; banda de IA como tarjeta
   surface con chip "IA".
4. **Login/Registro:** tarjeta central sobre el crema/carbón, inputs con borde
   grueso y focus accent, botón primary con press físico.

**R13 — las 6 rutas restantes:**
5. **Check-in:** los tres valores como steppers/chips grandes seleccionables.
6. **Finanzas:** resumen del ciclo con Stats, movimientos como filas-tarjeta
   (monto en display, income=money / expense=danger), form en Card.
7. **Entrenamiento:** catálogo y sesiones con Cards; series como chips.
8. **Disciplina:** hábitos como filas-tarjeta con check grande satisfactorio
   (animación de marca al completar) y racha con chip sun.
9. **Metas:** ProgressBar animada por meta, overdue con chip danger.
10. **Asistente:** burbujas con el lenguaje (usuario=accent suave,
    asistente=surface con borde), tarjeta de acción con los mismos
    Button/Chip, input con borde grueso.

Mientras dura la migración (entre R12 y R13), las rutas no migradas siguen
usando la paleta vieja — ambas coexisten en Tailwind hasta el cierre de R13,
que elimina la paleta vieja y cualquier clase huérfana.

## 7. Manejo de errores y accesibilidad

- Errores inline existentes pasan a chip/banda `danger` — mismos textos y
  semántica.
- Focus visible: anillo de 2px accent desplazado (`outline-offset`) en todos
  los interactivos.
- Contraste: los pares fg/bg de chips cumplen AA en ambos temas (texto tinta
  sobre pasteles claros; texto vibrante sobre fondos oscurecidos).
- `prefers-reduced-motion` (ver §5).
- El toggle de tema tiene `aria-label` («Cambiar a tema oscuro/claro»).

## 8. Testing

- **Los 87 tests existentes siguen verdes sin cambios de aserciones** (assertan
  texto/roles; si alguno asserta clases, se ajusta justificadamente). Donde las
  rutas se envuelvan en providers nuevos, los tests de ruta agregan el wrapper.
- Nuevos: ThemeProvider (default claro, toggle aplica `data-theme` y persiste,
  lee localStorage al montar), Stat (muestra el valor final), Button
  (variantes y disabled), Chip (variantes), ProgressBar (porcentaje correcto).
- `npm run build` estricto en verde al cierre de cada rebanada.
- Smoke visual manual: capturas en claro y oscuro de dashboard y una ruta
  interior, en docker, antes de cada merge.

## 9. Criterios de aceptación

- La app completa se ve neo-brutalista coherente en ambos temas; el toggle
  cambia todo al instante y la preferencia sobrevive recargas.
- Animaciones presentes y con propósito (página, cascada, contadores, press
  físico, fuego de racha) y desactivables vía `prefers-reduced-motion`.
- Ninguna funcionalidad de las rebanadas 1–11 se rompe: suites backend y
  frontend completas en verde, smokes E2E existentes pasan.
- Al cierre de R13 no queda ningún uso de la paleta vieja.
