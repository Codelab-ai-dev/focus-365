#!/usr/bin/env bash
# restore-db.sh — restaura un dump (.sql.gz) de Postgres dentro de un contenedor.
#
# Uso:  scripts/restore-db.sh <archivo.sql.gz> [contenedor_db]
#
# - Lee el usuario y la base de POSTGRES_USER / POSTGRES_DB (defaults focus / focus365).
# - Conecta por `docker exec ... psql` (socket local = trust; no necesita password).
# - SOBRESCRIBE los datos (los dumps llevan --clean --if-exists): pide confirmación
#   escribiendo "restaurar". Poné FORCE=1 para saltear la confirmación (automatización).
set -euo pipefail

FILE="${1:-}"
CONTAINER="${2:-focus365-db}"
PG_USER="${POSTGRES_USER:-focus}"
PG_DB="${POSTGRES_DB:-focus365}"

if [ -z "$FILE" ]; then
  echo "Uso: $0 <archivo.sql.gz> [contenedor_db]" >&2
  exit 2
fi
if [ ! -f "$FILE" ]; then
  echo "No existe el archivo: $FILE" >&2
  exit 1
fi

echo "⚠️  Vas a RESTAURAR '$FILE'"
echo "    en la base '$PG_DB' del contenedor '$CONTAINER'."
echo "    Esto SOBRESCRIBE los datos actuales (DROP/CREATE de objetos)."
if [ "${FORCE:-}" != "1" ]; then
  read -r -p "Escribí 'restaurar' para confirmar: " ans
  [ "$ans" = "restaurar" ] || { echo "Cancelado."; exit 1; }
fi

gunzip -c "$FILE" | docker exec -i "$CONTAINER" psql -v ON_ERROR_STOP=1 -U "$PG_USER" -d "$PG_DB"
echo "✓ Restauración completada en '$PG_DB'."
