# Backups de Postgres — Plan de implementación (Rebanada 29)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Backups automáticos diarios y rotados de la Postgres de producción (servicio en el Docker Compose) + un script de restore probado y un runbook.

**Architecture:** Un servicio `pgbackups` (imagen `prodrigestivill/postgres-backup-local:16`) en `docker-compose.coolify.yml` corre `pg_dump` diario, comprime y rota, guardando en un volumen `backups` del VPS. Un `scripts/restore-db.sh` restaura un dump dentro del contenedor de la DB (vía `docker exec ... psql`, socket local = trust, sin password). Un runbook documenta bajar y restaurar. El restore se valida localmente contra el contenedor de dev.

**Tech Stack:** Docker Compose, Postgres 16, bash. Sin código de la app.

**Contexto del repo (leer antes de empezar):**
- `docker-compose.coolify.yml` (producción): servicio `db` = `postgres:16-alpine`, volumen `dbdata`, usa los secrets de Coolify (`POSTGRES_USER`/`POSTGRES_DB`/`POSTGRES_PASSWORD`, con `${POSTGRES_PASSWORD:?...}`). Hoy define `volumes: { dbdata }`.
- DB local de desarrollo (para probar el restore): contenedor **`focus-365-db-1`** (`postgres:16-alpine`, host port 5544), user `focus`, db `focus365`, password `changeme`. La imagen oficial de postgres usa `local all all trust` en `pg_hba.conf` → **`docker exec <contenedor> psql -U focus` funciona sin password** (socket unix). Lo mismo aplicará en producción para el restore.
- Imagen de backup `prodrigestivill/postgres-backup-local` (estándar): env `POSTGRES_HOST/DB/USER/PASSWORD`, `SCHEDULE` (cron o `@daily`), `BACKUP_KEEP_DAYS/WEEKS/MONTHS`, `POSTGRES_EXTRA_OPTS`; escribe dumps `*.sql.gz` (SQL plano gzip) en `/backups/{last,daily,weekly,monthly}/`; el symlink al más nuevo es `/backups/last/<DB>-latest.sql.gz`; un backup manual se fuerza con `docker exec <contenedor> /backup.sh`. (Verificá estos nombres contra la doc de la imagen y ajustá si difieren.)
- `docker` está disponible localmente.

---

## Estructura de archivos

- Crear `scripts/restore-db.sh` — restaura un dump `.sql.gz` en el contenedor de la DB.
- Modificar `docker-compose.coolify.yml` — servicio `pgbackups` + volumen `backups`.
- Crear `docs/runbooks/backups-restore.md` — runbook operativo.

---

## Task 1: `scripts/restore-db.sh` + verificación local del restore

**Files:**
- Create: `scripts/restore-db.sh`

- [ ] **Step 1: Escribir el script**

Crear `scripts/restore-db.sh`:

```bash
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
```

Hacelo ejecutable:
```bash
chmod +x scripts/restore-db.sh
```

- [ ] **Step 2: Verificar el restore end-to-end (local, contra `focus-365-db-1`)**

Esta es la "prueba" del slice: armar una base con datos, dumpearla como lo hace
el backup (`--clean --if-exists` + gzip), restaurarla con el script en otra base y
verificar que los datos vuelven. Corré (el contenedor `focus-365-db-1` debe estar
arriba — es el de dev en 5544):

```bash
C=focus-365-db-1
# 1) base origen con una tabla y un dato
docker exec "$C" psql -U focus -d postgres -c "DROP DATABASE IF EXISTS restore_src;" -c "CREATE DATABASE restore_src;"
docker exec "$C" psql -U focus -d restore_src -c "CREATE TABLE t (id int); INSERT INTO t VALUES (42);"
# 2) dump como el backup (SQL plano, --clean --if-exists, gzip)
docker exec "$C" pg_dump -U focus --clean --if-exists restore_src | gzip > /tmp/restore_src.sql.gz
# 3) base destino vacía
docker exec "$C" psql -U focus -d postgres -c "DROP DATABASE IF EXISTS restore_dst;" -c "CREATE DATABASE restore_dst;"
# 4) restaurar con el script (FORCE para no pedir confirmación)
POSTGRES_USER=focus POSTGRES_DB=restore_dst FORCE=1 bash scripts/restore-db.sh /tmp/restore_src.sql.gz "$C"
# 5) verificar
echo -n "valor restaurado: "; docker exec "$C" psql -U focus -d restore_dst -tAc "SELECT id FROM t;"
# limpieza
docker exec "$C" psql -U focus -d postgres -c "DROP DATABASE restore_src;" -c "DROP DATABASE restore_dst;"
rm -f /tmp/restore_src.sql.gz
```
Expected: el paso 4 imprime "✓ Restauración completada en 'restore_dst'." y el
paso 5 imprime `valor restaurado: 42`. Si no da 42, parar y revisar el script.

- [ ] **Step 3: Commit**

```bash
git add scripts/restore-db.sh
git commit -m "feat(ops): script de restore de Postgres (restore-db.sh)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Servicio `pgbackups` en el compose + runbook

**Files:**
- Modify: `docker-compose.coolify.yml`
- Create: `docs/runbooks/backups-restore.md`

- [ ] **Step 1: Agregar el servicio `pgbackups`**

En `docker-compose.coolify.yml`, agregar el servicio (después de `db`, junto a los
demás servicios) y declarar el volumen `backups`:

```yaml
  pgbackups:
    image: prodrigestivill/postgres-backup-local:16
    environment:
      POSTGRES_HOST: db
      POSTGRES_DB: ${POSTGRES_DB:-focus365}
      POSTGRES_USER: ${POSTGRES_USER:-focus}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?define POSTGRES_PASSWORD en Coolify}
      SCHEDULE: "@daily"
      BACKUP_KEEP_DAYS: "7"
      BACKUP_KEEP_WEEKS: "4"
      BACKUP_KEEP_MONTHS: "3"
      # Dumps re-aplicables sobre una DB existente (la imagen gzipea por su cuenta):
      POSTGRES_EXTRA_OPTS: "--clean --if-exists"
    volumes:
      - backups:/backups
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped
```

Y actualizar el bloque `volumes:` del final del archivo de:
```yaml
volumes:
  dbdata:
```
a:
```yaml
volumes:
  dbdata:
  backups:
```

- [ ] **Step 2: Validar el compose**

Run (con un password dummy, porque `POSTGRES_PASSWORD` es obligatorio):
```bash
cd /Users/gustavo/Desktop/focus-365 && POSTGRES_PASSWORD=dummy docker compose -f docker-compose.coolify.yml config >/dev/null && echo "compose OK"
```
Expected: imprime `compose OK` (YAML válido, el servicio `pgbackups` y el volumen
`backups` parsean). Si `docker compose config` se queja del nombre de alguna env
var, corregir.

- [ ] **Step 3: Escribir el runbook**

Crear `docs/runbooks/backups-restore.md`:

````markdown
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
````

- [ ] **Step 4: Commit**

```bash
git add docker-compose.coolify.yml docs/runbooks/backups-restore.md
git commit -m "feat(ops): backups diarios de Postgres (servicio pgbackups) + runbook

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Cierre — review, merge y verificación en producción

**Files:** verificación + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-29-backups-postgres-design.md`. Verificar: el servicio `pgbackups` reutiliza los secrets (no commitea credenciales), apunta a `db`, usa el volumen `backups` (separado de `dbdata`); el `restore-db.sh` es seguro (confirmación, ON_ERROR_STOP); el runbook está completo. Aplicar nits.

- [ ] **Step 2: Re-validar** `POSTGRES_PASSWORD=dummy docker compose -f docker-compose.coolify.yml config >/dev/null` (compose válido). El backend/web no cambian, pero por las dudas: `cd web && npm run build` sigue OK (no debería verse afectado; este slice no toca el código de la app — saltear si no hay cambios en web/api).

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + verificación en producción.** Tras el
  deploy, en el VPS: confirmar que el servicio `pgbackups` levantó
  (`docker ps | grep pgbackups`), forzar un backup
  (`docker exec <pgbackups> /backup.sh`) y confirmar que aparece el dump
  (`docker exec <pgbackups> ls -la /backups/last/`). (Recordatorio: si el deploy
  falla en build con el código compilando local → disco lleno del VPS →
  `docker system prune -af`.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-29-backups-postgres.md`.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 servicio `pgbackups` (imagen, env reusando secrets, schedule, retención,
  extra opts, volumen `backups`) → Task 2 Step 1. ✓
- §3 `scripts/restore-db.sh` (confirmación, docker exec psql, --clean replay) → Task 1. ✓
- §4 runbook (cómo corre, forzar, bajar, restaurar, restore de prueba) → Task 2 Step 3. ✓
- §5 verificación: restore probado **localmente** → Task 1 Step 2; en producción → Task 3 Step 4. ✓
- §6 bordes (depends_on healthy, password obligatorio, volumen aparte) → Task 2 (compose). ✓
- §7 testing (restore local + verificación prod) → Tasks 1 y 3. ✓
- §8 aceptación → Task 3. ✓

**Placeholders:** la nota «verificá los nombres de env contra la doc de la imagen»
es una validación determinista (con `docker compose config` + la doc del image);
no es un TODO de diseño. Los `<contenedor_pgbackups>`/`<VPS>` del runbook son
marcadores de comandos operativos (el usuario los completa), no placeholders del
plan.

**Consistencia:** `restore-db.sh` usa `POSTGRES_USER`/`POSTGRES_DB` (defaults
focus/focus365) y `docker exec <contenedor> psql` — coincide con el contenedor de
dev (`focus-365-db-1`, trust local) en la verificación y con el de prod en el
runbook. El servicio `pgbackups` usa los mismos nombres de secret que el `db`
existente. El volumen `backups` se declara en `volumes:` y se monta en el servicio. ✓
