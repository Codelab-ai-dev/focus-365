# Bitácora de sesión — Rebanada 32: App instalable como PWA (móvil)

**Fecha:** 2026-06-18
**Estado al cierre:** Mergeada a `main` y pusheada. **Smoke de producción pendiente del deploy manual.**
**Rama:** `plan-32-pwa` (mezclada `--no-ff` y borrada).

## Qué se entregó

La SPA ahora es una **PWA instalable**: "Agregar a pantalla de inicio" la abre
**standalone** (sin barra del navegador), con ícono propio y splash. Carga al
instante y abre **sin conexión** (precache del app shell; los datos siguen
pidiendo red). Chrome móvil pulido (theme-color claro/oscuro, áreas seguras del
notch, meta tags iOS/Android). Cuando hay versión nueva, un **toast** avisa
("Hay una actualización · Recargar"). **Sin** push (rebanada aparte).

## Arquitectura

- **`vite-plugin-pwa` v1.3.0** (Workbox): genera `manifest.webmanifest` y `sw.js`
  (precache de 17 entradas + `navigateFallback: /index.html` para que las rutas de
  TanStack abran offline). Sin runtime caching de la API (offline = solo shell).
- **Íconos** neo-brutalistas desde SVG fuente (`icon.svg` con marco, `icon-maskable.svg`
  con zona segura) → `pwa-192/512`, `512-maskable`, `apple-touch-icon` vía
  `scripts/gen-icons.sh` (rsvg-convert). Commiteados.
- **`ui/UpdateToast.tsx`**: hook `useRegisterSW` (`registerType: 'prompt'`); avisa de
  update (botón Recargar → `updateServiceWorker(true)`) y de offlineReady. Montado en
  `__root.tsx`. No renderiza nada si no hay nada que avisar.
- **`index.html`**: viewport `viewport-fit=cover`, theme-color con media claro/oscuro,
  apple-touch-icon, apple-mobile-web-app-*. **`index.css`**: padding de `env(safe-area-inset-*)`.
- **nginx**: `Cache-Control: no-cache` para `sw.js`/`registerSW.js`/`manifest.webmanifest`
  (para detectar versiones nuevas; los assets con hash van con caché larga).

## Commits

`íconos` · `config vite-plugin-pwa` · `UpdateToast` · `safe areas + nginx + smoke` · merge.

## Decisiones / hallazgos

- **`virtual:pwa-register/react` en Vitest:** el `vi.mock` solo no bastaba — Vite
  intenta resolver el módulo virtual en transform-time y el plugin PWA no está en
  `vitest.config.ts`. Se agregó un **alias** a un stub (`src/test/pwa-register-stub.ts`)
  para que el import resuelva; los tests lo sobreescriben con `vi.mock`. (Era el
  riesgo (c) anticipado en el plan.)
- **`noUnusedLocals`:** el hook devuelve setters que no usábamos todos; se omitió
  `setNeedRefresh` en la desestructuración para no romper el build.
- **`nginx -t` local:** falla por el upstream `api` (no resuelve fuera de la red de
  compose), no por el bloque nuevo — la sintaxis parsea bien antes de esa
  validación; la validación real es en el deploy.
- **Offline = solo shell** y **push afuera** por decisión de alcance.

## Verificación al cierre

- `npm run build` limpio; PWA genera `manifest.webmanifest`, `sw.js`, `workbox-*.js`
  y los íconos en `dist/`.
- Suite front completa: **174/174** verde (incluye 3 tests de `UpdateToast`).
- **Smoke producción:** pendiente del deploy manual. `scripts/smoke-r32.sh` verifica
  que el manifest sirve y contiene "Focus 365", que `/sw.js` responde 200 con
  `Cache-Control: no-cache`, y que `/pwa-192.png` y `/pwa-512.png` responden 200.
  La instalación real y el standalone se prueban a mano en el celular.

## Limitaciones honestas

- **iOS:** instalación y standalone funcionan, pero varias features PWA solo se
  habilitan con la app instalada desde Safari; push queda afuera.
- **Offline = solo shell:** sin conexión abre la interfaz, los datos muestran error.
- El SW solo corre en **HTTPS** (prod) o `localhost` (probar con `npm run preview`,
  no con el dev server).

## Backlog restante

Notificaciones push (PWA) — evolución mayor; backups off-site (B2).
