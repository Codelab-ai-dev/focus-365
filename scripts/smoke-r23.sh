#!/usr/bin/env bash
# Smoke R23 — notas de avance por meta. Verifica en producción que:
#  1) crear una meta y agregarle 2 notas con fechas distintas
#  2) GET notas -> 2, ordenadas por fecha desc
#  3) body vacío -> 400
#  4) borrar una nota -> 204
#  5) crear nota en una meta inexistente (uuid random) -> 404
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r23-$TS@focus365.test"
PASS="Smoke-r23-$TS!"

jqget() { printf '%s' "$1" | sed -n "s/.*\"$2\":\"\([^\"]*\)\".*/\1/p"; }

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R23\"}")"
TOKEN="$(jqget "$REG" access_token)"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  register -> token OK"

echo "== 1) crear meta + 2 notas =="
G="$(curl -s "${AUTH[@]}" -X POST "$API/goals?today=$(date +%F)" -H 'Content-Type: application/json' \
  -d '{"title":"Correr 10k","dimension":"fisica"}')"
GID="$(jqget "$G" id)"
[ -n "$GID" ] || { echo "  FALLO: no se creó la meta (¿deploy viejo?): $G"; exit 1; }
echo "  meta $GID"
for D in 2026-06-10 2026-06-15; do
  C="$(curl -s "${AUTH[@]}" -X POST "$API/goals/$GID/notes" -H 'Content-Type: application/json' \
    -d "{\"note_date\":\"$D\",\"body\":\"avance $D\"}" -o /dev/null -w '%{http_code}')"
  [ "$C" = "201" ] || { echo "  FALLO: crear nota $D -> $C"; exit 1; }
done
echo "  2 notas creadas OK"

echo "== 2) GET notas -> 2, orden por fecha desc =="
N="$(curl -s "${AUTH[@]}" "$API/goals/$GID/notes")"
COUNT="$(printf '%s' "$N" | grep -o '"note_date"' | wc -l | tr -d ' ')"
[ "$COUNT" = "2" ] || { echo "  FALLO: esperaba 2 notas, hay $COUNT: $N"; exit 1; }
# la primera nota debe ser la más reciente (2026-06-15)
FIRST="$(printf '%s' "$N" | sed -n 's/.*"note_date":"\([^"]*\)".*/\1/p' | head -1)"
[ "$FIRST" = "2026-06-15" ] || { echo "  FALLO: orden incorrecto, primera = $FIRST (want 2026-06-15)"; exit 1; }
echo "  2 notas, orden desc OK"

echo "== 3) body vacío -> 400 =="
C3="$(curl -s "${AUTH[@]}" -X POST "$API/goals/$GID/notes" -H 'Content-Type: application/json' \
  -d '{"note_date":"2026-06-17","body":"   "}' -o /dev/null -w '%{http_code}')"
[ "$C3" = "400" ] || { echo "  FALLO: body vacío -> $C3, want 400"; exit 1; }
echo "  body vacío -> 400 OK"

echo "== 4) borrar una nota -> 204 =="
NID="$(printf '%s' "$N" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)"
C4="$(curl -s "${AUTH[@]}" -X DELETE "$API/goals/$GID/notes/$NID" -o /dev/null -w '%{http_code}')"
[ "$C4" = "204" ] || { echo "  FALLO: DELETE nota -> $C4, want 204"; exit 1; }
echo "  delete -> 204 OK"

echo "== 5) nota en meta inexistente -> 404 =="
FAKE="00000000-0000-0000-0000-000000000000"
C5="$(curl -s "${AUTH[@]}" -X POST "$API/goals/$FAKE/notes" -H 'Content-Type: application/json' \
  -d '{"note_date":"2026-06-17","body":"x"}' -o /dev/null -w '%{http_code}')"
[ "$C5" = "404" ] || { echo "  FALLO: meta inexistente -> $C5, want 404"; exit 1; }
echo "  meta inexistente -> 404 OK"

echo
echo "SMOKE R23: 5/5 OK"
