# Plan 29 — Backups de Postgres — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Hoy la base de producción (`postgres:16-alpine`, volumen `dbdata`, dentro del
`docker-compose.coolify.yml`) **no tiene ningún backup**. Esta rebanada agrega
copias automáticas diarias, rotadas, guardadas en el VPS, más un **script y un
runbook de restauración probado**.

**Decisiones (brainstorming):**
- **Destino: local en el VPS** (volumen propio) + **bajada manual** a la compu del
  usuario cuando quiera (scp). Sin off-site automático por ahora.
- **Mecanismo: un servicio de backup en el Docker Compose** (versionado, se
  deploya con el stack), no un cron en el host.
- Retención **7 diarios / 4 semanales / 3 mensuales**; dump **diario** (~03:00).

**Fuera de alcance:** off-site automático (Backblaze B2 u otro); cifrado de los
dumps; point-in-time recovery (WAL archiving); notificaciones de fallo; UI de
backups. (Quedan como posibles mejoras futuras.)

**Limitación reconocida:** los backups viven en el **mismo VPS** que la DB →
protegen de borrados accidentales, migraciones malas y bugs de la app, pero **no**
de la pérdida total del VPS/disco. La bajada manual periódica a la compu mitiga
esto; el off-site real queda para una rebanada futura.

## 2. Componente: servicio `pgbackups` (en `docker-compose.coolify.yml`)

Se agrega un servicio con la imagen probada `prodrigestivill/postgres-backup-local`
(tag `16`, matchea Postgres 16). Reutiliza los secrets de Coolify (no duplica ni
commitea credenciales).

```yaml
  pgbackups:
    image: prodrigestivill/postgres-backup-local:16
    environment:
      POSTGRES_HOST: db
      POSTGRES_DB: ${POSTGRES_DB:-focus365}
      POSTGRES_USER: ${POSTGRES_USER:-focus}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?define POSTGRES_PASSWORD en Coolify}
      SCHEDULE: "@daily"                 # un dump por día
      BACKUP_KEEP_DAYS: "7"
      BACKUP_KEEP_WEEKS: "4"
      BACKUP_KEEP_MONTHS: "3"
      # Dumps re-aplicables sobre una DB existente (la imagen ya gzipea encima):
      POSTGRES_EXTRA_OPTS: "--clean --if-exists"
    volumes:
      - backups:/backups
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped
```

Y se agrega el volumen:
```yaml
volumes:
  dbdata:
  backups:
```

- La imagen escribe `/backups/last/<db>-latest.sql.gz` (symlink al más nuevo) y
  carpetas `daily/`, `weekly/`, `monthly/` con la rotación configurada.
- `backups` es un **volumen separado** de `dbdata`: borrar/recrear la app no lo
  toca; persiste entre deploys.
- `depends_on: db healthy` + `restart: unless-stopped` → espera a la DB y
  reintenta.

> El plan debe verificar contra la doc de la imagen los nombres exactos de las
> env vars (`SCHEDULE`, `BACKUP_KEEP_*`, `POSTGRES_EXTRA_OPTS`) y el layout de
> `/backups`, y ajustar si difieren.

## 3. Componente: `scripts/restore-db.sh`

Script para restaurar un dump dentro del contenedor `db`. Uso:
`restore-db.sh <archivo.sql.gz> [nombre_contenedor_db]`.

- Pide **confirmación** (sobrescribe datos).
- Hace `gunzip -c <archivo> | docker exec -i <db> psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"`.
  Como los dumps llevan `--clean --if-exists`, se re-aplican sobre la DB existente.
- Lee `POSTGRES_USER`/`POSTGRES_DB` de variables de entorno con defaults
  (`focus`/`focus365`); permite override.

## 4. Componente: runbook `docs/runbooks/backups-restore.md`

Documento operativo con:
- **Cómo corren** los backups (servicio `pgbackups`, horario, retención, dónde
  caen).
- **Forzar un backup ahora:** `docker exec <pgbackups_container> /backup.sh`.
- **Bajar el último dump a la compu:** localizar el volumen
  (`docker volume ls | grep backups`) y `scp` desde la ruta del host
  (`/var/lib/docker/volumes/<vol>/_data/last/focus365-latest.sql.gz`), o
  `docker cp <pgbackups_container>:/backups/last/focus365-latest.sql.gz ./`.
- **Restaurar:** con `scripts/restore-db.sh` (incluye un ejemplo y la advertencia
  de que sobrescribe).
- **Restore de prueba** (recomendado periódicamente): restaurar el último dump en
  una DB temporal y verificar.

## 5. Verificación

- **Restore probado localmente** antes de cerrar (parte del plan): generar un dump
  de la DB de desarrollo (la del `docker-compose.yml`), restaurarlo en una **DB
  temporal** y verificar que una tabla con datos vuelve. Valida el script y el
  runbook end-to-end.
- **En producción, tras el deploy:** forzar un backup manual
  (`docker exec <pgbackups> /backup.sh`) y confirmar que aparece
  `focus365-latest.sql.gz` en el volumen `backups`.

## 6. Manejo de errores / bordes

- DB no lista → el servicio espera (`depends_on` healthy) y reintenta.
- Password ausente → el compose ya falla ruidoso (`:?` en `POSTGRES_PASSWORD`).
- El volumen `backups` se conserva aunque se recree la app (volumen con nombre).
- Restore sobre una DB en uso: el `--clean --if-exists` dropea y recrea objetos;
  conviene hacerlo con la app detenida o aceptando el corte. El runbook lo aclara.

## 7. Testing

- **Local (cierre):** `scripts/restore-db.sh` restaura un dump en una DB scratch y
  se verifica una tabla/fila → el script y el procedimiento quedan validados.
- **Sin tests automatizados del cron** del contenedor (es infra); se verifica en
  producción que el dump se genera.

## 8. Criterios de aceptación

- El stack de producción incluye `pgbackups`; tras el deploy se genera un dump
  diario rotado en el volumen `backups`.
- Existe `scripts/restore-db.sh` y un runbook que documenta bajar y restaurar, con
  un restore de prueba **realmente ejecutado** durante el desarrollo.
- Sin credenciales nuevas commiteadas; reutiliza los secrets de Coolify.
- El stack sigue desplegando y la app funcionando (smoke de salud OK).
