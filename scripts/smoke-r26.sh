#!/usr/bin/env bash
# Smoke R26 — notas por serie. Verifica en producción que:
#  1) POST /training/workouts con una serie con note -> 201
#  2) GET /training/workouts -> la serie trae el note
#  3) POST con una serie con note de 201 chars -> 400
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r26-$TS@focus365.test"
PASS="Smoke-r26-$TS!"

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R26\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) POST workout con nota por serie -> 201 =="
BODY='{"date":"2026-06-17","type":"Pierna","sets":[{"exercise":"Sentadilla","reps":8,"weight_grams":80000,"note":"lei pesado"}]}'
C1="$(curl -s "${AUTH[@]}" -X POST "$API/training/workouts" -H 'Content-Type: application/json' -d "$BODY" -o /dev/null -w '%{http_code}')"
[ "$C1" = "201" ] || { echo "  FALLO: POST workout -> $C1 (¿deploy viejo?)"; exit 1; }
echo "  201 OK"

echo "== 2) GET workouts -> la serie trae note =="
G="$(curl -s "${AUTH[@]}" "$API/training/workouts")"
printf '%s' "$G" | grep -q '"note":"lei pesado"' \
  || { echo "  FALLO: la serie no trae la nota: $G"; exit 1; }
echo "  note presente OK"

echo "== 3) nota de serie de 201 chars -> 400 =="
LONG="$(printf 'a%.0s' $(seq 1 201))"
BODY2="{\"date\":\"2026-06-17\",\"sets\":[{\"exercise\":\"Sentadilla\",\"note\":\"$LONG\"}]}"
C3="$(curl -s "${AUTH[@]}" -X POST "$API/training/workouts" -H 'Content-Type: application/json' -d "$BODY2" -o /dev/null -w '%{http_code}')"
[ "$C3" = "400" ] || { echo "  FALLO: nota larga -> $C3, want 400"; exit 1; }
echo "  400 OK"

echo
echo "SMOKE R26: 3/3 OK"
