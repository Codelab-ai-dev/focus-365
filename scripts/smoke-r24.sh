#!/usr/bin/env bash
# Smoke R24 — perfil de fitness. Verifica en producción que:
#  1) GET /training/profile sin perfil -> null
#  2) PUT con un perfil válido -> 200 con los valores
#  3) GET de nuevo -> el perfil (un solo registro; re-PUT lo reemplaza)
#  4) PUT con weekly_days=8 -> 400
#  5) PUT con sex inválido -> 400
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r24-$TS@focus365.test"
PASS="Smoke-r24-$TS!"

field() { printf '%s' "$1" | grep -o "\"$2\":[^,}]*" | head -1 | sed 's/.*://; s/"//g'; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R24\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) GET sin perfil -> null =="
G="$(curl -s "${AUTH[@]}" "$API/training/profile")"
[ "$(printf '%s' "$G" | tr -d '[:space:]')" = "null" ] || { echo "  FALLO: GET vacío = $G (¿deploy viejo?)"; exit 1; }
echo "  null OK"

echo "== 2) PUT perfil válido -> 200 =="
BODY='{"birthdate":"1990-05-01","sex":"masculino","height_cm":178,"weight_grams":80500,"objective":"hipertrofia","location":"casa","level":"intermedio","weekly_days":4,"equipment":["mancuernas","bandas"],"limitations":"cuido la rodilla"}'
P="$(curl -s "${AUTH[@]}" -X PUT "$API/training/profile" -H 'Content-Type: application/json' -d "$BODY")"
[ "$(field "$P" objective)" = "hipertrofia" ] || { echo "  FALLO: PUT no devolvió el perfil: $P"; exit 1; }
echo "  PUT 200 OK (objetivo hipertrofia)"

echo "== 3) GET de nuevo -> el perfil, weight_grams 80500 =="
G2="$(curl -s "${AUTH[@]}" "$API/training/profile")"
[ "$(field "$G2" weight_grams)" = "80500" ] || { echo "  FALLO: GET tras PUT = $G2"; exit 1; }
echo "  perfil persistido OK"

echo "== 4) weekly_days=8 -> 400 =="
C4="$(curl -s "${AUTH[@]}" -X PUT "$API/training/profile" -H 'Content-Type: application/json' -d '{"weekly_days":8}' -o /dev/null -w '%{http_code}')"
[ "$C4" = "400" ] || { echo "  FALLO: weekly_days=8 -> $C4, want 400"; exit 1; }
echo "  400 OK"

echo "== 5) sex inválido -> 400 =="
C5="$(curl -s "${AUTH[@]}" -X PUT "$API/training/profile" -H 'Content-Type: application/json' -d '{"sex":"x"}' -o /dev/null -w '%{http_code}')"
[ "$C5" = "400" ] || { echo "  FALLO: sex inválido -> $C5, want 400"; exit 1; }
echo "  400 OK"

echo
echo "SMOKE R24: 5/5 OK"
