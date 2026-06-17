#!/usr/bin/env bash
# Smoke R27 — análisis/ajustes del agente. Verifica en producción que:
#  1) GET /training/adjustment sin análisis -> null
#  2) POST {"scope":"last"} -> 200 con content no vacío (Groq real)
#  3) GET de nuevo -> el análisis (scope "last")
#  4) POST {"scope":"week"} -> 200
#  5) POST {"scope":"mes"} -> 400
# Nota: Groq real puede tardar varios segundos.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r27-$TS@focus365.test"
PASS="Smoke-r27-$TS!"

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R27\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) GET sin análisis -> null =="
G="$(curl -s "${AUTH[@]}" "$API/training/adjustment")"
[ "$(printf '%s' "$G" | tr -d '[:space:]')" = "null" ] || { echo "  FALLO: GET vacío = $G (¿deploy viejo?)"; exit 1; }
echo "  null OK"

echo "== 2) POST scope=last genera (Groq real, puede tardar) =="
P="$(curl -s -m 60 "${AUTH[@]}" -X POST "$API/training/adjustment" -H 'Content-Type: application/json' -d '{"scope":"last"}')"
C="$(printf '%s' "$P" | grep -o '"content":"[^"]*"' | head -1)"
{ [ -n "$C" ] && [ "$C" != '"content":""' ]; } \
  || { echo "  FALLO: POST no devolvió content (¿sin clave Groq / 503?): $P"; exit 1; }
printf '%s' "$P" | grep -q '"scope":"last"' || { echo "  FALLO: scope no es last: $P"; exit 1; }
echo "  análisis generado (scope last) OK"

echo "== 3) GET de nuevo -> persistido =="
G2="$(curl -s "${AUTH[@]}" "$API/training/adjustment")"
printf '%s' "$G2" | grep -q '"content":"' \
  || { echo "  FALLO: GET tras POST no tiene content: $G2"; exit 1; }
echo "  persistido OK"

echo "== 4) POST scope=week -> 200 =="
C4="$(curl -s -m 60 "${AUTH[@]}" -X POST "$API/training/adjustment" -H 'Content-Type: application/json' -d '{"scope":"week"}' -o /dev/null -w '%{http_code}')"
[ "$C4" = "200" ] || { echo "  FALLO: scope=week -> $C4, want 200"; exit 1; }
echo "  200 OK"

echo "== 5) scope inválido -> 400 =="
C5="$(curl -s "${AUTH[@]}" -X POST "$API/training/adjustment" -H 'Content-Type: application/json' -d '{"scope":"mes"}' -o /dev/null -w '%{http_code}')"
[ "$C5" = "400" ] || { echo "  FALLO: scope inválido -> $C5, want 400"; exit 1; }
echo "  400 OK"

echo
echo "SMOKE R27: 5/5 OK"
