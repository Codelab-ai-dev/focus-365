# Rebanada 32 — App instalable como PWA (móvil) · Diseño

**Fecha:** 2026-06-18
**Estado:** Diseño aprobado.

## Resumen

Convertir la SPA (Vite 5 + React, servida por nginx en HTTPS, mismo origen que la
API) en una **PWA instalable**, con **app shell offline** y **chrome móvil**
pulido. **Sin** notificaciones push (queda como rebanada aparte).

Tooling: **`vite-plugin-pwa`** (Workbox por debajo) — genera el manifest y el
service worker, precachea el bundle, y expone el hook de actualización. Casi no
agrega código propio.

## Alcance

- **Instalable:** "Agregar a pantalla de inicio" → abre standalone (sin barra del
  navegador), ícono propio, splash básico.
- **Offline = solo app shell:** la app abre al instante y sin conexión muestra la
  interfaz; los datos siguen pidiendo red y, sin ella, muestran su estado de error
  normal. **No** se cachean respuestas de la API.
- **Chrome móvil:** theme-color, áreas seguras (notch), meta tags iOS/Android.
- **Actualización:** aviso sutil (toast) cuando hay versión nueva; el usuario toca
  para recargar.
- **Fuera de alcance:** push/notificaciones; runtime caching de datos de la API;
  background sync; offline de datos.

## Componentes

### 1. Dependencia y configuración de Vite

- Agregar `vite-plugin-pwa` a `devDependencies` de `web/package.json`.
- En `web/vite.config.ts`, agregar el plugin `VitePWA({...})`:
  - `registerType: 'prompt'` (aviso de actualización, no auto-silencioso).
  - `manifest`: ver sección 2.
  - `includeAssets`: los íconos y `favicon`.
  - `workbox.navigateFallback: '/index.html'` para que las rutas SPA abran offline.
  - `workbox.globPatterns`: `**/*.{js,css,html,svg,png,woff2}` (precache del shell).
  - `devOptions.enabled: false` (el SW solo en build; en dev molesta).

### 2. Manifest

Definido inline en la config del plugin (genera `manifest.webmanifest`):

```json
{
  "name": "Focus 365",
  "short_name": "Focus 365",
  "description": "Tu vida en orden: hábitos, metas, finanzas y entrenamiento.",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "orientation": "portrait",
  "lang": "es",
  "background_color": "#f4f0e6",
  "theme_color": "#ff7a3d",
  "icons": [
    { "src": "/pwa-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/pwa-512.png", "sizes": "512x512", "type": "image/png" },
    { "src": "/pwa-512-maskable.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable" }
  ]
}
```

### 3. Íconos

- Fuente: un SVG neo-brutalista en `web/public/icon.svg` — una "F" en bloque, tinta
  `#16130e` sobre fondo naranja `#ff7a3d`, con borde grueso (coherente con el
  lenguaje `border-2 border-ink`). La versión maskable tiene padding (zona segura
  ~20%) para que Android no recorte la letra.
- Script `web/scripts/gen-icons.sh`: renderiza desde el SVG a
  `web/public/pwa-192.png`, `web/public/pwa-512.png` y
  `web/public/pwa-512-maskable.png` usando `rsvg-convert` (o `magick` como
  fallback). Los PNG resultantes se commitean (no se generan en el build de Docker).
- También `web/public/favicon.svg` y `apple-touch-icon` (180x180) desde la misma
  fuente.

### 4. Service worker y aviso de actualización

- El SW lo genera Workbox (precache + `navigateFallback`). Sin runtime caching.
- Nuevo `web/src/ui/UpdateToast.tsx`:
  - Usa `useRegisterSW` de `virtual:pwa-register/react`.
  - Cuando `needRefresh` es true, muestra un toast neo-brutalista fijo abajo
    (`fixed bottom-4`, `border-2 border-ink bg-surface shadow-brutal`): texto
    *"Hay una actualización"* + botón *"Recargar"* que llama `updateServiceWorker(true)`.
  - Cuando `offlineReady` es true, muestra brevemente *"Lista para usar sin conexión"*
    con un botón de cerrar.
  - Si ni `needRefresh` ni `offlineReady`, no renderiza nada.
- Montar `<UpdateToast />` en `web/src/routes/__root.tsx` junto a `<TopBar />` y
  `<Outlet />`.

### 5. `index.html` (chrome móvil)

Agregar al `<head>`:
- `<meta name="theme-color" content="#ff7a3d" media="(prefers-color-scheme: light)">`
  y un segundo con el color oscuro para dark (`#191613`).
- `<meta name="apple-mobile-web-app-capable" content="yes">`,
  `<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">`,
  `<meta name="apple-mobile-web-app-title" content="Focus 365">`.
- `<link rel="apple-touch-icon" href="/apple-touch-icon.png">`.
- `<link rel="icon" href="/favicon.svg" type="image/svg+xml">`.
- Cambiar el viewport a `width=device-width, initial-scale=1.0, viewport-fit=cover`.

CSS de áreas seguras en `src/index.css`: usar `env(safe-area-inset-*)` donde el
contenido toca los bordes (p.ej. padding inferior del body/topbar en standalone),
sin romper el layout en navegador normal.

### 6. nginx (deploy)

En `web/nginx.conf`, antes del `location /`, agregar un bloque para que el service
worker y el manifest no queden cacheados con caché larga (así el navegador detecta
versiones nuevas), manteniendo caché larga solo para assets con hash:

```nginx
    # El service worker y el manifest deben revalidarse siempre.
    location = /sw.js              { add_header Cache-Control "no-cache"; }
    location = /registerSW.js      { add_header Cache-Control "no-cache"; }
    location = /manifest.webmanifest { add_header Cache-Control "no-cache"; }
```

(Los assets con hash en el nombre los revisiona Workbox; no requieren reglas extra.)

## Manejo de errores

- Si el navegador no soporta service workers, `vite-plugin-pwa` degrada con
  gracia: la app funciona como SPA normal, sin instalación ni offline. No hay que
  hacer nada especial.
- El toast de actualización solo aparece cuando hay una versión nueva detectada; si
  el registro del SW falla, la app sigue andando (sin toast).

## Pruebas

- `web/src/ui/UpdateToast.test.tsx` (Vitest): mockeando `virtual:pwa-register/react`,
  - renderiza el aviso y el botón "Recargar" cuando `needRefresh` es true, y al
    tocar llama a `updateServiceWorker`;
  - no renderiza nada cuando `needRefresh` y `offlineReady` son false.
- **Verificación de build:** `npm run build` produce `dist/manifest.webmanifest`,
  `dist/sw.js` y los íconos en `dist/`. El SW en sí no se unit-testea (es Workbox,
  build-time); se valida con el build y un smoke manual de instalación en el celular.

## Verificación / smoke

`scripts/smoke-r32.sh`: contra producción, verifica que
`GET /manifest.webmanifest` responde 200 con `application/manifest+json` (o JSON) y
contiene `"Focus 365"`; que `GET /sw.js` responde 200 con `Cache-Control: no-cache`;
y que los íconos `/pwa-192.png` y `/pwa-512.png` responden 200. (La instalación real
y el standalone se prueban manualmente en el celular.)

## Limitaciones honestas

- **iOS:** la instalación y el standalone funcionan, pero iOS habilita varias
  features PWA solo si la app está **instalada** desde Safari; push queda afuera.
- **Offline = solo shell:** sin conexión la app abre pero los datos muestran error.
- El SW solo corre en **HTTPS** (prod) o `localhost`; para probarlo local hay que
  usar `npm run preview` (build), no el dev server.
