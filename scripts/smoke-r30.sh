#!/usr/bin/env bash
# Smoke R30 — OCR de PDFs escaneados. Sube un PDF escaneado (con imagen embebida,
# sin capa de texto) al importador y verifica que el nuevo camino a visión está
# vivo: la respuesta NO debe ser el rechazo previo "súbelo como foto" (eso probaría
# que el PDF llegó a la visión, no que se cortó antes). El fixture es una imagen de
# ruido (no un comprobante real), así que la visión puede devolver 0 movimientos
# (422 "no pude leer movimientos") o algún movimiento (200) — ambos OK; lo que NO
# debe pasar es el rechazo "súbelo como foto" ni un 503.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r30-$TS@focus365.test"
PASS="Smoke-r30-$TS!"
FIXTURE="$(dirname "$0")/fixtures/scanned-sample.pdf"

[ -f "$FIXTURE" ] || { echo "  FALLO: falta el fixture $FIXTURE"; exit 1; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R30\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
echo "  register -> token OK"

echo "== subir PDF escaneado a /ai/import (Groq real, puede tardar) =="
RESP="$(curl -s -m 90 -w $'\n%{http_code}' -H "Authorization: Bearer $TOKEN" \
  -F "file=@$FIXTURE;type=application/pdf" "$API/ai/import")"
CODE="$(printf '%s' "$RESP" | tail -1)"
BODY="$(printf '%s' "$RESP" | sed '$d')"
echo "  HTTP $CODE"
echo "  body: $(printf '%s' "$BODY" | head -c 200)"

# Garantía dura: el PDF escaneado NO debe caer en el rechazo previo.
if printf '%s' "$BODY" | grep -q "súbelo como foto"; then
  echo "  FALLO: el PDF escaneado fue rechazado con 'súbelo como foto' (¿deploy viejo?)"; exit 1
fi
# Tampoco debe ser 503 (IA no disponible) ni 5xx.
case "$CODE" in
  503) echo "  FALLO: 503 — el entrenador/visión no está disponible (¿sin clave Groq?)"; exit 1;;
  5*)  echo "  FALLO: error de servidor $CODE"; exit 1;;
esac
# 200 (extrajo movimientos) o 422 (imagen de ruido, 0 movimientos) son ambos OK:
# en los dos casos el PDF LLEGÓ a la visión (no se cortó en el ruteo).
case "$CODE" in
  200|422) echo "  OK: el PDF escaneado llegó a la visión (HTTP $CODE, sin 'súbelo como foto')";;
  *) echo "  FALLO: código inesperado $CODE"; exit 1;;
esac

echo
echo "SMOKE R30: OK"
