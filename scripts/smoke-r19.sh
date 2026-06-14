#!/usr/bin/env bash
# Smoke R19 — metas alineadas a las 4D. Verifica en producción que:
#  1) crear meta con dimensión vieja ("general") -> 400 (CHECK/validador)
#  2) crear meta con dimensión 4D ("financiera") -> 201 y aparece en GET /goals
#  3) crear meta con otra 4D ("fisica") -> 201
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r19-$TS@focus365.test"
PASS="Smoke-r19-$TS!"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

echo "== register =="
REG="$(curl -s -c "$JAR" -X POST "$API/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R19\"}")"
TOKEN="$(printf '%s' "$REG" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token en register: $REG"; exit 1; }
echo "  register -> token OK"

mk_goal () { # $1=dimension -> prints HTTP code
  curl -s -X POST "$API/goals" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"title\":\"smoke $1 $TS\",\"dimension\":\"$1\"}" \
    -o /dev/null -w "%{http_code}"
}

echo "== 1) dimensión vieja 'general' (espera 400) =="
C1="$(mk_goal general)"; echo "  general -> $C1"
[ "$C1" = "400" ] || { echo "  FALLO: se esperaba 400 (¿deploy viejo?)"; exit 1; }

echo "== 2) dimensión 4D 'financiera' (espera 201) =="
C2="$(mk_goal financiera)"; echo "  financiera -> $C2"
[ "$C2" = "201" ] || { echo "  FALLO: se esperaba 201"; exit 1; }

echo "== 3) dimensión 4D 'fisica' (espera 201) =="
C3="$(mk_goal fisica)"; echo "  fisica -> $C3"
[ "$C3" = "201" ] || { echo "  FALLO: se esperaba 201"; exit 1; }

echo "== 4) GET /goals contiene 'financiera' =="
BODY="$(curl -s -H "Authorization: Bearer $TOKEN" "$API/goals")"
echo "$BODY" | grep -q '"dimension":"financiera"' \
  && echo "  OK: meta financiera presente" \
  || { echo "  FALLO: no aparece la meta financiera"; echo "$BODY" | head -c 300; exit 1; }

echo
echo "SMOKE R19: 4/4 OK"
