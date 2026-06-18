# Runbook — Backups y restauración de Postgres

La base de producción se respalda con el servicio **`pgbackups`** del
`docker-compose.coolify.yml` (imagen `prodrigestivill/postgres-backup-local:16`).

## Qué hace
- Corre `pg_dump` **diario** (`SCHEDULE=@daily`) y guarda dumps **SQL plano
  gzip** en el volumen `backups`.
- Retención: **7 diarios, 4 semanales, 3 mensuales** (rotación automática).
- Layout dentro del volumen: `last/`, `daily/`, `weekly/`, `monthly/`. El más
  reciente: `last/focus365-latest.sql.gz`.
- Los dumps llevan `--clean --if-exists` → se re-aplican sobre una DB existente.

> Limitación: los dumps viven en el **mismo VPS** que la DB. Protegen de borrados
> accidentales, migraciones malas y bugs, pero **no** de la pérdida total del VPS.
> Bajate el último dump a tu compu cada tanto (ver abajo).

## Forzar un backup ahora
```bash
# en el VPS: encontrá el contenedor de backups
docker ps --format '{{.Names}}' | grep pgbackups
# corré un dump inmediato
docker exec <contenedor_pgbackups> /backup.sh
```

## Bajar el último dump a tu compu
```bash
# en el VPS: nombre real del volumen (Coolify le pone prefijo)
docker volume ls | grep backups
# copialo a tu compu desde la ruta del host del volumen:
scp root@<VPS>:/var/lib/docker/volumes/<volumen>/_data/last/focus365-latest.sql.gz ./
# alternativa sin saber la ruta del volumen:
docker cp <contenedor_pgbackups>:/backups/last/focus365-latest.sql.gz ./   # (en el VPS)
```

## Restaurar
`scripts/restore-db.sh` restaura un dump dentro del contenedor de la DB.
**Sobrescribe los datos** (DROP/CREATE de objetos por `--clean --if-exists`):
conviene hacerlo con la app detenida o aceptando el corte.

```bash
# en el VPS: nombre del contenedor de la DB
docker ps --format '{{.Names}}' | grep -- -db
# restaurar (pide confirmación escribiendo 'restaurar'):
POSTGRES_USER=focus POSTGRES_DB=focus365 scripts/restore-db.sh <archivo.sql.gz> <contenedor_db>
```
Para automatizar (sin prompt): anteponé `FORCE=1`.

## Restore de prueba (recomendado periódicamente)
Restaurá el último dump en una base temporal y verificá, sin tocar producción:
```bash
C=<contenedor_db>
docker exec "$C" psql -U focus -d postgres -c "DROP DATABASE IF EXISTS restore_check;" -c "CREATE DATABASE restore_check;"
POSTGRES_USER=focus POSTGRES_DB=restore_check FORCE=1 scripts/restore-db.sh <dump.sql.gz> "$C"
docker exec "$C" psql -U focus -d restore_check -c "\dt"   # deben verse las tablas
docker exec "$C" psql -U focus -d postgres -c "DROP DATABASE restore_check;"
```
