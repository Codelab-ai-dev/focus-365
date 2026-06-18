# Bitácora de sesión — Rebanada 29: backups de Postgres

**Fecha:** 2026-06-17
**Estado al cierre:** Mergeada a `main` y pusheada. **Verificación en producción pendiente del deploy manual** (forzar un backup y confirmar el dump).
**Rama:** `plan-29-backups-postgres` (mezclada `--no-ff` y borrada).

## Qué se entregó

La Postgres de producción no tenía **ningún** backup. Ahora:

- **Servicio `pgbackups`** en `docker-compose.coolify.yml` (imagen
  `prodrigestivill/postgres-backup-local:16`): `pg_dump` **diario**, comprimido y
  **rotado** (7 diarios / 4 semanales / 3 mensuales), en un **volumen `backups`**
  del VPS (separado de `dbdata`). Reutiliza los secrets de Coolify.
- **`scripts/restore-db.sh`:** restaura un dump `.sql.gz` dentro del contenedor de
  la DB (`gunzip | docker exec psql`, socket local = trust). Con confirmación
  (escribir "restaurar"), `ON_ERROR_STOP`, y `FORCE=1` para automatizar.
- **Runbook `docs/runbooks/backups-restore.md`:** cómo corren los backups, forzar
  uno, bajar el último a la compu (`scp`/`docker cp`), restaurar, y un restore de
  prueba periódico.

## Commits

`cbeb5ad` restore-db.sh · `b2fb721` servicio pgbackups + runbook · merge.

## Decisiones / hallazgos

- **Destino local + bajada manual** (elección del usuario): cero costo, cero
  cuenta externa. Limitación reconocida: los dumps viven en el mismo VPS → no
  protegen de la pérdida total del VPS; la bajada periódica a la compu mitiga.
  Off-site (B2) queda como mejora futura.
- **Servicio en el compose** (no cron en el host): versionado, se deploya con el
  stack. Coolify built-in no aplica (la DB es un servicio del compose, no un
  recurso "Database" de Coolify).
- **Restore probado de verdad:** el `restore-db.sh` se verificó end-to-end contra
  el contenedor de dev (`focus-365-db-1`): crear datos → `pg_dump --clean
  --if-exists | gzip` → restaurar con el script → `SELECT` devolvió `42`. Un
  backup que no sabés restaurar no sirve; este sí.
- **`docker exec psql` sin password:** la imagen oficial de postgres usa
  `local all all trust` en `pg_hba.conf`, así que el restore vía `docker exec` no
  necesita el password (socket unix). El servicio de backup sí lo necesita (conecta
  por TCP a `db`) y lo toma del secret de Coolify.
- **Validación del compose:** `POSTGRES_PASSWORD=dummy docker compose -f
  docker-compose.coolify.yml config` → OK (servicio y volumen parsean).
- **Review final (propia, ops):** APPROVED — secrets reutilizados (cero
  credenciales nuevas), volumen separado, script seguro, runbook completo.

## Verificación al cierre

- Local: restore end-to-end OK (devolvió `42`); `docker compose config` válido.
- **Producción:** pendiente del deploy manual. Tras deployar: `docker ps | grep
  pgbackups`, forzar `docker exec <pgbackups> /backup.sh`, y confirmar el dump en
  `/backups/last/`.

## Backlog restante

Backups off-site automáticos (B2) — mejora futura; OCR de PDFs escaneados;
recordatorios/notificaciones de compromisos.
