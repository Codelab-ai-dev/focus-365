#!/usr/bin/env bash
# Smoke R21 — búsqueda en el chat. Verifica en producción que:
#  1) crear un hilo con un mensaje (devuelve thread_id) y renombrarlo "Finanzas"
#  2) GET /ai/search?q=<palabra sin acento> encuentra el mensaje (insensible a acentos)
#  3) GET /ai/search?q=finanzas encuentra el hilo por título
#  4) GET /ai/search?q=a -> 400 (término demasiado corto)
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r21-$TS@focus365.test"
PASS="Smoke-r21-$TS!"

jqget() { printf '%s' "$1" | sed -n "s/.*\"$2\":\"\([^\"]*\)\".*/\1/p"; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R21\"}")"
TOKEN="$(jqget "$REG" access_token)"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) crear hilo + mensaje, renombrar a Finanzas =="
R1="$(curl -s "${AUTH[@]}" -X POST "$API/ai/chat" -H 'Content-Type: application/json' \
  -d "{\"message\":\"este mes gasté mucho en café\"}")"
TID="$(jqget "$R1" thread_id)"
[ -n "$TID" ] || { echo "  FALLO: sin thread_id (¿deploy viejo o IA caída?): $R1"; exit 1; }
C="$(curl -s "${AUTH[@]}" -X PATCH "$API/ai/threads/$TID" -H 'Content-Type: application/json' \
  -d "{\"title\":\"Finanzas\"}" -o /dev/null -w '%{http_code}')"
[ "$C" = "200" ] || { echo "  FALLO: rename -> $C"; exit 1; }
echo "  hilo $TID renombrado OK"

echo "== 2) buscar 'gaste' (sin acento) encuentra el mensaje =="
S2="$(curl -s "${AUTH[@]}" "$API/ai/search?q=gaste")"
M="$(printf '%s' "$S2" | grep -o '"thread_id"' | wc -l | tr -d ' ')"
[ "$M" -ge "1" ] || { echo "  FALLO: 'gaste' no encontró el mensaje: $S2"; exit 1; }
echo "  mensajes encontrados: $M OK"

echo "== 3) buscar 'finanzas' encuentra el hilo por título =="
S3="$(curl -s "${AUTH[@]}" "$API/ai/search?q=finanzas")"
# el primer "title" en la sección threads debe ser Finanzas
printf '%s' "$S3" | grep -q '"title":"Finanzas"' \
  || { echo "  FALLO: 'finanzas' no encontró el hilo por título: $S3"; exit 1; }
echo "  hilo por título OK"

echo "== 4) término corto 'a' -> 400 =="
C4="$(curl -s "${AUTH[@]}" "$API/ai/search?q=a" -o /dev/null -w '%{http_code}')"
[ "$C4" = "400" ] || { echo "  FALLO: q=a -> $C4, want 400"; exit 1; }
echo "  término corto -> 400 OK"

echo
echo "SMOKE R21: 4/4 OK"
