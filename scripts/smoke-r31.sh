#!/usr/bin/env bash
# Smoke R31 — Recordatorios de compromisos. Crea un compromiso vencido (ayer) y
# uno de hoy vía el POST de check-in (que guarda los compromisos del body para el
# día SIGUIENTE del `date` enviado), verifica que GET /commitments/pending
# devuelve ambos (vencido primero), marca el de hoy con toggle y verifica que ya
# no aparece y el vencido permanece.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r31-$TS@focus365.test"
PASS="Smoke-r31-$TS!"

command -v jq >/dev/null || { echo "  FALLO: jq no está instalado"; exit 1; }

# Fechas UTC: anteayer y ayer, para que los compromisos caigan en ayer y hoy.
ANTEAYER="$(date -u -v-2d +%Y-%m-%d 2>/dev/null || date -u -d '2 days ago' +%Y-%m-%d)"
AYER="$(date -u -v-1d +%Y-%m-%d 2>/dev/null || date -u -d '1 day ago' +%Y-%m-%d)"

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R31\"}")"
TOKEN="$(printf '%s' "$REG" | jq -r '.access_token // empty')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
echo "  register -> token OK"

auth=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

echo "== crear compromisos (check-in anteayer -> vencido; ayer -> de hoy) =="
curl -s "${auth[@]}" -X POST "$API/checkins" \
  -d "{\"date\":\"$ANTEAYER\",\"mood\":5,\"energy\":5,\"commitments\":[\"Vencido\"]}" >/dev/null
curl -s "${auth[@]}" -X POST "$API/checkins" \
  -d "{\"date\":\"$AYER\",\"mood\":5,\"energy\":5,\"commitments\":[\"De hoy\"]}" >/dev/null

echo "== GET /commitments/pending (debe traer ambos, vencido primero) =="
PEND="$(curl -s "${auth[@]}" "$API/commitments/pending")"
echo "  body: $(printf '%s' "$PEND" | head -c 300)"
N="$(printf '%s' "$PEND" | jq '.commitments | length')"
FIRST="$(printf '%s' "$PEND" | jq -r '.commitments[0].text')"
HASV="$(printf '%s' "$PEND" | jq '[.commitments[].text] | index("Vencido") != null')"
HASH="$(printf '%s' "$PEND" | jq '[.commitments[].text] | index("De hoy") != null')"
[ "$N" -ge 2 ] || { echo "  FALLO: pending trae $N (esperaba >=2)"; exit 1; }
[ "$FIRST" = "Vencido" ] || { echo "  FALLO: el primero es '$FIRST' (esperaba 'Vencido')"; exit 1; }
[ "$HASV" = "true" ] || { echo "  FALLO: falta 'Vencido'"; exit 1; }
[ "$HASH" = "true" ] || { echo "  FALLO: falta 'De hoy'"; exit 1; }
echo "  OK: pending trae vencido + hoy (vencido primero)"

echo "== marcar 'De hoy' cumplido; verificar que sale de pending =="
HOY_ID="$(printf '%s' "$PEND" | jq -r '.commitments[] | select(.text=="De hoy") | .id')"
[ -n "$HOY_ID" ] || { echo "  FALLO: no pude extraer el id de 'De hoy'"; exit 1; }
curl -s "${auth[@]}" -X POST "$API/commitments/$HOY_ID/toggle" >/dev/null
PEND2="$(curl -s "${auth[@]}" "$API/commitments/pending")"
STILLH="$(printf '%s' "$PEND2" | jq '[.commitments[].text] | index("De hoy") != null')"
STILLV="$(printf '%s' "$PEND2" | jq '[.commitments[].text] | index("Vencido") != null')"
[ "$STILLH" = "false" ] || { echo "  FALLO: 'De hoy' sigue en pending tras toggle"; exit 1; }
[ "$STILLV" = "true" ]  || { echo "  FALLO: 'Vencido' desapareció (no debía)"; exit 1; }
echo "  OK: 'De hoy' cumplido sale de pending; 'Vencido' permanece"

echo
echo "SMOKE R31: OK"
