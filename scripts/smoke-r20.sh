#!/usr/bin/env bash
# Smoke R20 — hilos en el asistente. Verifica en producción que:
#  1) enviar sin thread_id crea un hilo (devuelve thread_id)
#  2) GET /threads lista ese hilo con preview
#  3) enviar con ese thread_id NO crea otro hilo (sigue 1)
#  4) PATCH renombra el hilo (200)
#  5) GET /threads/{id}/messages trae los 4 mensajes
#  6) DELETE borra el hilo (204) y luego GET del hilo borrado -> 404
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r20-$TS@focus365.test"
PASS="Smoke-r20-$TS!"

jqget() { printf '%s' "$1" | sed -n "s/.*\"$2\":\"\([^\"]*\)\".*/\1/p"; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R20\"}")"
TOKEN="$(jqget "$REG" access_token)"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
echo "  register -> token OK"

AUTH=(-H "Authorization: Bearer $TOKEN")

echo "== 1) enviar sin thread_id crea hilo =="
R1="$(curl -s "${AUTH[@]}" -X POST "$API/ai/chat" -H 'Content-Type: application/json' \
  -d "{\"message\":\"hola, ¿cuánto gasté este mes?\"}")"
TID="$(jqget "$R1" thread_id)"
[ -n "$TID" ] || { echo "  FALLO: no devolvió thread_id (¿deploy viejo o IA caída?): $R1"; exit 1; }
echo "  thread_id -> $TID"

echo "== 2) GET /threads lista 1 hilo =="
TL="$(curl -s "${AUTH[@]}" "$API/ai/threads")"
N="$(printf '%s' "$TL" | grep -o '"id"' | wc -l | tr -d ' ')"
[ "$N" = "1" ] || { echo "  FALLO: esperaba 1 hilo, hay $N: $TL"; exit 1; }
echo "  threads -> 1 OK"

echo "== 3) enviar con thread_id NO crea otro hilo =="
curl -s "${AUTH[@]}" -X POST "$API/ai/chat" -H 'Content-Type: application/json' \
  -d "{\"message\":\"y la semana que viene?\",\"thread_id\":\"$TID\"}" -o /dev/null
TL2="$(curl -s "${AUTH[@]}" "$API/ai/threads")"
N2="$(printf '%s' "$TL2" | grep -o '"id"' | wc -l | tr -d ' ')"
[ "$N2" = "1" ] || { echo "  FALLO: tras 2.º envío hay $N2 hilos, want 1"; exit 1; }
echo "  sigue 1 hilo OK"

echo "== 4) PATCH renombra (200) =="
C4="$(curl -s "${AUTH[@]}" -X PATCH "$API/ai/threads/$TID" -H 'Content-Type: application/json' \
  -d "{\"title\":\"Finanzas del mes\"}" -o /dev/null -w '%{http_code}')"
[ "$C4" = "200" ] || { echo "  FALLO: PATCH -> $C4"; exit 1; }
echo "  rename -> 200 OK"

echo "== 5) GET mensajes del hilo (4) =="
MSGS="$(curl -s "${AUTH[@]}" "$API/ai/threads/$TID/messages")"
M="$(printf '%s' "$MSGS" | grep -o '"role"' | wc -l | tr -d ' ')"
[ "$M" = "4" ] || { echo "  FALLO: esperaba 4 mensajes, hay $M"; exit 1; }
echo "  mensajes -> 4 OK"

echo "== 6) DELETE (204) y luego 404 =="
C6="$(curl -s "${AUTH[@]}" -X DELETE "$API/ai/threads/$TID" -o /dev/null -w '%{http_code}')"
[ "$C6" = "204" ] || { echo "  FALLO: DELETE -> $C6"; exit 1; }
C7="$(curl -s "${AUTH[@]}" "$API/ai/threads/$TID/messages" -o /dev/null -w '%{http_code}')"
[ "$C7" = "404" ] || { echo "  FALLO: GET hilo borrado -> $C7, want 404"; exit 1; }
echo "  delete -> 204, luego 404 OK"

echo
echo "SMOKE R20: 6/6 OK"
