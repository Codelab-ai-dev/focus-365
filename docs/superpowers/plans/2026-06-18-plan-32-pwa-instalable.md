# App instalable como PWA (móvil) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convertir la SPA en una PWA instalable, con app shell offline, chrome móvil y aviso de actualización — sin push.

**Architecture:** `vite-plugin-pwa` (Workbox) genera el manifest y el service worker (precache del app shell + navigateFallback para rutas SPA). Un componente `UpdateToast` usa el hook `useRegisterSW` para avisar de nuevas versiones. Íconos neo-brutalistas generados desde un SVG. nginx sirve el SW/manifest sin caché larga.

**Tech Stack:** Vite 5 + React 18 + vite-plugin-pwa (Workbox) + Vitest + nginx.

---

## Notas de entorno (leer antes de empezar)

- Trabajás en `/Users/gustavo/Desktop/focus-365`. El front está en `web/`.
- Comandos front: `cd /Users/gustavo/Desktop/focus-365/web && npm run build` (tsc + vite build), `npm test -- <archivo>` (Vitest puntual).
- `vitest.config.ts` está SEPARADO de `vite.config.ts` y **no** incluye el plugin PWA → en tests, el módulo `virtual:pwa-register/react` se mockea con `vi.mock`.
- Herramientas de imagen disponibles en la máquina: `rsvg-convert`, `magick`/`convert`, `sips`. El script de íconos usa `rsvg-convert` con fallback a `magick`.
- Colores de marca: accent naranja `#ff7a3d`, tinta `#16130e`, fondo claro `#f4f0e6`, fondo oscuro `#191613`.
- La carpeta `web/public/` no existe todavía; los archivos estáticos de Vite van ahí y se copian tal cual a `dist/`.

---

## File Structure

- `web/public/icon.svg` (crear) — fuente del ícono (con marco).
- `web/public/icon-maskable.svg` (crear) — variante maskable (sin marco, con zona segura).
- `web/public/favicon.svg` (crear) — favicon (copia de icon.svg).
- `web/public/pwa-192.png`, `web/public/pwa-512.png`, `web/public/pwa-512-maskable.png`, `web/public/apple-touch-icon.png` (generados, commiteados).
- `web/scripts/gen-icons.sh` (crear) — regenera los PNG desde los SVG.
- `web/package.json` (modificar) — agrega `vite-plugin-pwa`.
- `web/vite.config.ts` (modificar) — plugin `VitePWA` + manifest.
- `web/src/vite-env.d.ts` (crear) — tipos del módulo virtual.
- `web/index.html` (modificar) — meta tags de chrome móvil.
- `web/src/index.css` (modificar) — safe areas.
- `web/src/ui/UpdateToast.tsx` (crear) — aviso de actualización.
- `web/src/ui/UpdateToast.test.tsx` (crear) — test del toast.
- `web/src/routes/__root.tsx` (modificar) — montar `<UpdateToast />`.
- `web/nginx.conf` (modificar) — caché del SW/manifest.
- `scripts/smoke-r32.sh` (crear) — smoke de producción.

---

## Task 1: Íconos (SVG + script + PNGs)

**Files:**
- Create: `web/public/icon.svg`, `web/public/icon-maskable.svg`, `web/public/favicon.svg`
- Create: `web/scripts/gen-icons.sh`
- Generated: `web/public/pwa-192.png`, `web/public/pwa-512.png`, `web/public/pwa-512-maskable.png`, `web/public/apple-touch-icon.png`

- [ ] **Step 1: Crear el SVG con marco** `web/public/icon.svg`

Una "F" en bloque (tinta) sobre naranja, con marco grueso neo-brutalista. La "F" se dibuja con rectángulos (sin depender de fuentes):

```svg
<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
  <rect width="512" height="512" fill="#ff7a3d"/>
  <rect x="28" y="28" width="456" height="456" fill="none" stroke="#16130e" stroke-width="28"/>
  <!-- F en bloque -->
  <rect x="176" y="150" width="72" height="212" fill="#16130e"/>
  <rect x="176" y="150" width="196" height="64" fill="#16130e"/>
  <rect x="176" y="248" width="150" height="58" fill="#16130e"/>
</svg>
```

- [ ] **Step 2: Crear el SVG maskable** `web/public/icon-maskable.svg`

Full-bleed naranja (sin marco, para que Android pueda recortar a círculo/squircle sin comerse el borde), con la "F" más chica y centrada dentro de la zona segura (~80%):

```svg
<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
  <rect width="512" height="512" fill="#ff7a3d"/>
  <!-- F centrada, dentro de la zona segura (~64% del lienzo) -->
  <rect x="206" y="176" width="58" height="170" fill="#16130e"/>
  <rect x="206" y="176" width="156" height="52" fill="#16130e"/>
  <rect x="206" y="254" width="120" height="46" fill="#16130e"/>
</svg>
```

- [ ] **Step 3: Crear el favicon** `web/public/favicon.svg`

Idéntico a `icon.svg` (copiar el mismo contenido del Step 1).

- [ ] **Step 4: Crear el script de generación** `web/scripts/gen-icons.sh`

```bash
#!/usr/bin/env bash
# Genera los PNG de la PWA desde los SVG fuente. Re-ejecutar si cambian los SVG.
# Requiere rsvg-convert (preferido) o magick/convert como fallback.
set -euo pipefail
cd "$(dirname "$0")/../public"

render() { # <svg> <size> <out>
  local svg="$1" size="$2" out="$3"
  if command -v rsvg-convert >/dev/null; then
    rsvg-convert -w "$size" -h "$size" "$svg" -o "$out"
  elif command -v magick >/dev/null; then
    magick -background none "$svg" -resize "${size}x${size}" "$out"
  elif command -v convert >/dev/null; then
    convert -background none "$svg" -resize "${size}x${size}" "$out"
  else
    echo "FALLO: instalá rsvg-convert o imagemagick"; exit 1
  fi
}

render icon.svg          192 pwa-192.png
render icon.svg          512 pwa-512.png
render icon-maskable.svg 512 pwa-512-maskable.png
render icon.svg          180 apple-touch-icon.png
echo "Íconos generados: pwa-192.png pwa-512.png pwa-512-maskable.png apple-touch-icon.png"
```

- [ ] **Step 5: Hacer ejecutable y generar**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/web && chmod +x scripts/gen-icons.sh && ./scripts/gen-icons.sh
```
Expected: imprime "Íconos generados: ..." sin error.

- [ ] **Step 6: Verificar las dimensiones de los PNG**

Run: `cd /Users/gustavo/Desktop/focus-365/web/public && file pwa-192.png pwa-512.png pwa-512-maskable.png apple-touch-icon.png`
Expected: `pwa-192.png` → 192 x 192; `pwa-512.png` y `pwa-512-maskable.png` → 512 x 512; `apple-touch-icon.png` → 180 x 180. Todos PNG.

- [ ] **Step 7: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365 && git add web/public/icon.svg web/public/icon-maskable.svg web/public/favicon.svg web/scripts/gen-icons.sh web/public/pwa-192.png web/public/pwa-512.png web/public/pwa-512-maskable.png web/public/apple-touch-icon.png && git commit -m "feat(web): íconos de la PWA (SVG fuente + PNGs + script)"
```

---

## Task 2: vite-plugin-pwa — instalación, config, manifest, tipos, index.html

**Files:**
- Modify: `web/package.json` (vía npm)
- Modify: `web/vite.config.ts`
- Create: `web/src/vite-env.d.ts`
- Modify: `web/index.html`

- [ ] **Step 1: Instalar el plugin**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm install -D vite-plugin-pwa`
Expected: se agrega a `devDependencies` sin errores de peer-deps (es compatible con Vite 5).

- [ ] **Step 2: Configurar el plugin en `web/vite.config.ts`**

Reemplazar el contenido por (agrega el import y el plugin `VitePWA`, conservando lo existente):

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import { VitePWA } from "vite-plugin-pwa";
import path from "path";

export default defineConfig({
  plugins: [
    TanStackRouterVite(),
    react(),
    VitePWA({
      registerType: "prompt",
      includeAssets: [
        "favicon.svg",
        "apple-touch-icon.png",
        "pwa-192.png",
        "pwa-512.png",
        "pwa-512-maskable.png",
      ],
      manifest: {
        name: "Focus 365",
        short_name: "Focus 365",
        description:
          "Tu vida en orden: hábitos, metas, finanzas y entrenamiento.",
        start_url: "/",
        scope: "/",
        display: "standalone",
        orientation: "portrait",
        lang: "es",
        background_color: "#f4f0e6",
        theme_color: "#ff7a3d",
        icons: [
          { src: "/pwa-192.png", sizes: "192x192", type: "image/png" },
          { src: "/pwa-512.png", sizes: "512x512", type: "image/png" },
          {
            src: "/pwa-512-maskable.png",
            sizes: "512x512",
            type: "image/png",
            purpose: "maskable",
          },
        ],
      },
      workbox: {
        navigateFallback: "/index.html",
        globPatterns: ["**/*.{js,css,html,svg,png,woff2}"],
      },
      devOptions: { enabled: false },
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
```

- [ ] **Step 3: Tipos del módulo virtual** — crear `web/src/vite-env.d.ts`

```ts
/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/react" />
/// <reference types="vite-plugin-pwa/client" />
```

- [ ] **Step 4: Meta tags de chrome móvil en `web/index.html`**

Reemplazar el bloque `<head>` para que quede así (conservando el script de tema existente):

```html
  <head>
    <meta charset="UTF-8" />
    <meta
      name="viewport"
      content="width=device-width, initial-scale=1.0, viewport-fit=cover"
    />
    <title>Focus 365</title>
    <meta
      name="theme-color"
      content="#ff7a3d"
      media="(prefers-color-scheme: light)"
    />
    <meta
      name="theme-color"
      content="#191613"
      media="(prefers-color-scheme: dark)"
    />
    <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
    <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
    <meta name="apple-mobile-web-app-capable" content="yes" />
    <meta
      name="apple-mobile-web-app-status-bar-style"
      content="black-translucent"
    />
    <meta name="apple-mobile-web-app-title" content="Focus 365" />
    <script>
      try {
        if (localStorage.getItem("focus365-theme") === "dark") {
          document.documentElement.setAttribute("data-theme", "dark");
        }
      } catch (e) {}
    </script>
  </head>
```

- [ ] **Step 5: Build y verificar artefactos PWA**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm run build`
Expected: build exitoso (tsc + vite). Luego:
Run: `cd /Users/gustavo/Desktop/focus-365/web && ls dist/manifest.webmanifest dist/sw.js dist/pwa-192.png dist/pwa-512.png dist/pwa-512-maskable.png`
Expected: los 5 archivos existen. (Si `sw.js` tuviera otro nombre, listar `dist/*.js | grep -i sw` y ajustar; con la config de arriba el nombre por defecto es `sw.js`.)

- [ ] **Step 6: Verificar el contenido del manifest**

Run: `cd /Users/gustavo/Desktop/focus-365/web && grep -o "Focus 365" dist/manifest.webmanifest | head -1`
Expected: imprime `Focus 365`.

- [ ] **Step 7: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365 && git add web/package.json web/package-lock.json web/vite.config.ts web/src/vite-env.d.ts web/index.html && git commit -m "feat(web): configurar vite-plugin-pwa (manifest, SW, meta tags móviles)"
```

---

## Task 3: Componente `UpdateToast` (TDD) + montaje

**Files:**
- Create: `web/src/ui/UpdateToast.tsx`
- Test: `web/src/ui/UpdateToast.test.tsx`
- Modify: `web/src/routes/__root.tsx`

- [ ] **Step 1: Escribir el test que falla** — `web/src/ui/UpdateToast.test.tsx`

Mockea el módulo virtual `virtual:pwa-register/react` (no existe en el entorno de Vitest). El mock permite controlar `needRefresh`/`offlineReady` y espiar `updateServiceWorker`.

```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const updateSpy = vi.fn();
let state: {
  needRefresh: [boolean, (v: boolean) => void];
  offlineReady: [boolean, (v: boolean) => void];
  updateServiceWorker: typeof updateSpy;
};

vi.mock("virtual:pwa-register/react", () => ({
  useRegisterSW: () => state,
}));

import { UpdateToast } from "./UpdateToast";

beforeEach(() => {
  updateSpy.mockClear();
  state = {
    needRefresh: [false, vi.fn()],
    offlineReady: [false, vi.fn()],
    updateServiceWorker: updateSpy,
  };
});

describe("UpdateToast", () => {
  it("no renderiza nada sin update ni offlineReady", () => {
    const { container } = render(<UpdateToast />);
    expect(container).toBeEmptyDOMElement();
  });

  it("muestra el aviso de actualización y recarga al tocar", async () => {
    state.needRefresh = [true, vi.fn()];
    render(<UpdateToast />);
    expect(screen.getByText(/actualización/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /recargar/i }));
    expect(updateSpy).toHaveBeenCalledWith(true);
  });

  it("muestra 'lista para usar sin conexión' cuando offlineReady", () => {
    state.offlineReady = [true, vi.fn()];
    render(<UpdateToast />);
    expect(screen.getByText(/sin conexión/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Correr el test, confirmar que FALLA** (no existe `./UpdateToast`)

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test -- src/ui/UpdateToast.test.tsx`
Expected: FAIL — `Failed to resolve import "./UpdateToast"`.

- [ ] **Step 3: Implementar** `web/src/ui/UpdateToast.tsx`

```tsx
import { useRegisterSW } from "virtual:pwa-register/react";

// UpdateToast avisa cuando hay una versión nueva de la app (tocar para recargar)
// y, brevemente, cuando quedó lista para usar sin conexión. Sin nada que avisar,
// no renderiza nada.
export function UpdateToast() {
  const {
    needRefresh: [needRefresh, setNeedRefresh],
    offlineReady: [offlineReady, setOfflineReady],
    updateServiceWorker,
  } = useRegisterSW();

  if (!needRefresh && !offlineReady) return null;

  return (
    <div className="fixed inset-x-0 bottom-4 z-50 flex justify-center px-4">
      <div className="flex items-center gap-3 border-2 border-ink bg-surface px-4 py-3 shadow-brutal">
        {needRefresh ? (
          <>
            <span className="text-sm font-bold">Hay una actualización</span>
            <button
              type="button"
              onClick={() => updateServiceWorker(true)}
              className="border-2 border-ink bg-accent px-3 py-1 text-sm font-bold shadow-brutal-sm"
            >
              Recargar
            </button>
          </>
        ) : (
          <>
            <span className="text-sm font-bold">
              Lista para usar sin conexión
            </span>
            <button
              type="button"
              aria-label="Cerrar"
              onClick={() => setOfflineReady(false)}
              className="border-2 border-ink bg-surface px-3 py-1 text-sm font-bold shadow-brutal-sm"
            >
              ✕
            </button>
          </>
        )}
      </div>
    </div>
  );
}
```

Nota: `setNeedRefresh` no se usa en el flujo (el aviso desaparece al recargar); queda desestructurado por simetría. Si `noUnusedLocals` del tsconfig hace fallar el build por `setNeedRefresh` sin usar, cambiar la desestructuración a `needRefresh: [needRefresh],` (omitiendo el setter).

- [ ] **Step 4: Correr el test, confirmar PASS (3 tests)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test -- src/ui/UpdateToast.test.tsx`
Expected: 3 passed.

- [ ] **Step 5: Montar en `web/src/routes/__root.tsx`**

```tsx
import { createRootRoute, Outlet } from "@tanstack/react-router";
import { TopBar } from "@/components/TopBar";
import { UpdateToast } from "@/ui/UpdateToast";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-bg text-ink">
      <TopBar />
      <Outlet />
      <UpdateToast />
    </div>
  ),
});
```

- [ ] **Step 6: Typecheck/build (incluye el módulo virtual real)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm run build`
Expected: build limpio. Si falla por `setNeedRefresh`/`offlineReady` sin usar, aplicar la nota del Step 3 (y/o omitir setters no usados).

- [ ] **Step 7: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365 && git add web/src/ui/UpdateToast.tsx web/src/ui/UpdateToast.test.tsx web/src/routes/__root.tsx && git commit -m "feat(web): aviso de actualización de la PWA (UpdateToast)"
```

---

## Task 4: Safe areas + nginx + smoke

**Files:**
- Modify: `web/src/index.css`
- Modify: `web/nginx.conf`
- Create: `scripts/smoke-r32.sh`

- [ ] **Step 1: Safe areas en `web/src/index.css`**

Dentro de `@layer base`, agregar una regla para que en modo standalone el contenido respete el notch/áreas seguras. Agregar al final del bloque `@layer base { ... }` (después de las reglas de foco):

```css
  /* Áreas seguras en standalone (notch/barra de gestos). No afecta al navegador
     normal: env(safe-area-inset-*) vale 0 fuera de standalone. */
  body {
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
  }
```

- [ ] **Step 2: Verificar que el front compila con el CSS**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm run build`
Expected: build limpio.

- [ ] **Step 3: Caché del SW/manifest en `web/nginx.conf`**

Insertar, justo antes del bloque `location / {`:

```nginx
    # El service worker y el manifest deben revalidarse siempre, para que el
    # navegador detecte versiones nuevas (los assets con hash sí van con caché larga).
    location = /sw.js                 { add_header Cache-Control "no-cache"; }
    location = /registerSW.js         { add_header Cache-Control "no-cache"; }
    location = /manifest.webmanifest  { add_header Cache-Control "no-cache"; }

```

- [ ] **Step 4: Validar la sintaxis de nginx.conf**

Run: `cd /Users/gustavo/Desktop/focus-365/web && docker run --rm -v "$PWD/nginx.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine nginx -t 2>&1 | tail -3`
Expected: `syntax is ok` / `test is successful`. (Si Docker no está disponible localmente, omitir este paso: la sintaxis se valida en el deploy; revisar visualmente que las llaves estén balanceadas.)

- [ ] **Step 5: Escribir el smoke** — `scripts/smoke-r32.sh`

```bash
#!/usr/bin/env bash
# Smoke R32 — PWA instalable. Verifica contra producción que los artefactos PWA
# se sirven: manifest (con "Focus 365"), service worker (sin caché larga) e íconos.
# La instalación real y el standalone se prueban a mano en el celular.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"

echo "== manifest =="
MAN="$(curl -s "$BASE/manifest.webmanifest")"
echo "  body: $(printf '%s' "$MAN" | head -c 160)"
printf '%s' "$MAN" | grep -q "Focus 365" || { echo "  FALLO: el manifest no contiene 'Focus 365'"; exit 1; }
echo "  OK: manifest sirve y contiene 'Focus 365'"

echo "== service worker (status + Cache-Control) =="
SW_HEAD="$(curl -s -I "$BASE/sw.js")"
echo "$SW_HEAD" | grep -qiE "^HTTP/.* 200" || { echo "  FALLO: /sw.js no responde 200"; echo "$SW_HEAD" | head -3; exit 1; }
echo "$SW_HEAD" | grep -qi "cache-control: *no-cache" || { echo "  FALLO: /sw.js sin 'Cache-Control: no-cache'"; echo "$SW_HEAD"; exit 1; }
echo "  OK: /sw.js 200 con no-cache"

echo "== íconos =="
for icon in pwa-192.png pwa-512.png; do
  CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/$icon")"
  [ "$CODE" = "200" ] || { echo "  FALLO: /$icon -> HTTP $CODE"; exit 1; }
  echo "  OK: /$icon 200"
done

echo
echo "SMOKE R32: OK"
```

- [ ] **Step 6: Hacer ejecutable, lint y commit**

```bash
cd /Users/gustavo/Desktop/focus-365 && chmod +x scripts/smoke-r32.sh && bash -n scripts/smoke-r32.sh && git add web/src/index.css web/nginx.conf scripts/smoke-r32.sh && git commit -m "feat(web): safe areas + caché del SW en nginx + smoke (PWA)"
```

(El smoke se corre contra producción **después** del deploy manual en Coolify.)

---

## Self-Review (hecho por el autor del plan)

- **Cobertura del spec:** manifest + display standalone (T2) ✓; íconos incl. maskable + script (T1) ✓; SW con precache + navigateFallback (T2) ✓; aviso de actualización `prompt` + `UpdateToast` montado en root (T3) ✓; meta tags chrome móvil + viewport-fit (T2) ✓; safe areas (T4) ✓; nginx no-cache para SW/manifest (T4) ✓; test del UpdateToast + verificación de build (T3) ✓; smoke (T4) ✓. Limitaciones (iOS, offline solo shell, SW solo en HTTPS) documentadas en el spec.
- **Sin placeholders:** cada paso trae SVG/config/código/comando concretos.
- **Consistencia de tipos/nombres:** nombres de íconos idénticos entre `gen-icons.sh`, `includeAssets`, `manifest.icons`, `index.html` y el smoke (`pwa-192.png`, `pwa-512.png`, `pwa-512-maskable.png`, `apple-touch-icon.png`, `favicon.svg`); `useRegisterSW` consumido igual en el componente y mockeado igual en el test; `sw.js`/`manifest.webmanifest` consistentes entre la config del plugin, nginx y el smoke.
- **Riesgos conocidos y mitigados:** (a) nombre del SW (`sw.js`) — Step 5 de T2 verifica y permite ajustar; (b) `noUnusedLocals` con setters del hook — nota explícita en T3 Step 3; (c) `virtual:pwa-register/react` en Vitest — mockeado en el test; (d) nginx -t sin Docker local — Step 4 de T4 permite omitir con validación visual.
