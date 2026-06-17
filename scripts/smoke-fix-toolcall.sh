#!/usr/bin/env bash
# Smoke del fix de tool-calls-como-texto. Verifica en producción que:
#  - el endpoint de streaming responde y cierra con un evento done
#  - GARANTÍA DURA: no se filtra la etiqueta cruda "<function" en el stream
#  - INFORMATIVO: si el modelo propuso la acción, aparece kind "checkin"
# El path exacto del bug es no determinístico (depende del modelo); el núcleo
# está cubierto por los tests unitarios. Esto es una verificación de sanidad.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-fix-$TS@focus365.test"
PASS="Smoke-fix-$TS!"

jqget() { printf '%s' "$1" | sed -n "s/.*\"$2\":\"\([^\"]*\)\".*/\1/p"; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke Fix\"}")"
TOKEN="$(jqget "$REG" access_token)"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
echo "  register -> token OK"

echo "== chat/stream: pedir registrar check-in (ánimo 9 / energía 9) =="
OUT="$(curl -sN "$API/ai/chat/stream" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"message":"registrá mi check-in de hoy con ánimo 9 y energía 9"}')"

# El stream debe cerrar con un evento done.
printf '%s' "$OUT" | grep -q 'event: done' \
  || { echo "  FALLO: el stream no cerró con done. Salida:"; printf '%s\n' "$OUT" | head -c 600; exit 1; }
echo "  stream cerró con done OK"

# GARANTÍA DURA: ninguna etiqueta cruda <function en el stream.
if printf '%s' "$OUT" | grep -q '<function'; then
  echo "  FALLO: se filtró la etiqueta cruda <function:"; printf '%s\n' "$OUT" | head -c 600; exit 1
fi
echo "  sin etiqueta <function cruda OK"

# INFORMATIVO: ¿el modelo propuso la acción de check-in?
if printf '%s' "$OUT" | grep -q '"kind":"checkin"'; then
  echo "  acción de check-in propuesta (tarjeta confirmable) OK"
else
  echo "  nota: el modelo respondió en texto sin proponer la acción esta vez (no determinístico) — sin etiqueta filtrada, que es lo que importa."
fi

echo
echo "SMOKE FIX: OK"
