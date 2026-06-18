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
