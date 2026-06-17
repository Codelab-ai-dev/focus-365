#!/usr/bin/env bash
# Smoke R25 — entrenador IA. Verifica en producción que:
#  1) GET /training/suggestion sin sugerencia -> null
#  2) POST con {"focus":"pierna"} -> 200 con content no vacío (Groq real)
#  3) GET de nuevo -> la misma sugerencia (persistida)
#  4) POST con focus de 201 chars -> 400
# Nota: Groq real puede tardar varios segundos.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r25-$TS@focus365.test"
PASS="Smoke-r25-$TS!"

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R25\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) GET sin sugerencia -> null =="
G="$(curl -s "${AUTH[@]}" "$API/training/suggestion")"
[ "$(printf '%s' "$G" | tr -d '[:space:]')" = "null" ] || { echo "  FALLO: GET vacío = $G (¿deploy viejo?)"; exit 1; }
echo "  null OK"

echo "== 2) POST genera (Groq real, puede tardar) =="
P="$(curl -s -m 60 "${AUTH[@]}" -X POST "$API/training/suggestion" -H 'Content-Type: application/json' -d '{"focus":"pierna"}')"
CONTENT="$(printf '%s' "$P" | grep -o '"content":"[^"]*"' | head -1)"
[ -n "$CONTENT" ] && [ "$CONTENT" != '"content":""' ] \
  || { echo "  FALLO: POST no devolvió content (¿sin clave Groq / 503?): $P"; exit 1; }
echo "  sugerencia generada OK"

echo "== 3) GET de nuevo -> persistida =="
G2="$(curl -s "${AUTH[@]}" "$API/training/suggestion")"
printf '%s' "$G2" | grep -q '"content":"' \
  || { echo "  FALLO: GET tras POST no tiene content: $G2"; exit 1; }
echo "  persistida OK"

echo "== 4) focus de 201 chars -> 400 =="
LONG="$(printf 'a%.0s' $(seq 1 201))"
C4="$(curl -s "${AUTH[@]}" -X POST "$API/training/suggestion" -H 'Content-Type: application/json' \
  -d "{\"focus\":\"$LONG\"}" -o /dev/null -w '%{http_code}')"
[ "$C4" = "400" ] || { echo "  FALLO: focus largo -> $C4, want 400"; exit 1; }
echo "  400 OK"

echo
echo "SMOKE R25: 4/4 OK"
