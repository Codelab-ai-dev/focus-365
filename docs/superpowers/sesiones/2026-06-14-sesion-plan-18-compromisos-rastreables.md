# Bitácora de sesión — Rebanada 18: Compromisos rastreables

**Fecha:** 2026-06-14
**Estado al cierre:** Completada, mergeada a `main` y **verificada en producción**.
**Rama:** `plan-18-compromisos-rastreables` (mezclada `--no-ff` y borrada). **Merge:** `95d3b6b`

## Qué se entregó

Los compromisos del check-in («Mañana me comprometo a:») pasan de texto suelto a
**rastreables**: lo que te comprometes hoy para mañana aparece mañana en el
check-in con un check para marcar cumplido. Cierra el ciclo de rendición de
cuentas del check-in de Capitanes de Dios.

- **Sección «📋 Ayer te comprometiste a»** arriba del check-in, con checkbox por
  compromiso (toggle inmediato) y contador N/M ✓.
- Sin cumplir queda no-cumplido, **no se arrastra**.
- La **IA** ve los compromisos recientes (target ≥ hoy−7) con su cumplimiento,
  como contexto de rendición de cuentas.

## Arquitectura

- **Migración 0015:** tabla `commitments` (id, user_id, target_date DATE, text,
  done, position); **migra los compromisos del JSONB** de cada check-in a filas
  con target = fecha+1 (vía `jsonb_array_elements WITH ORDINALITY`); elimina la
  columna JSONB del check-in.
- **Paquete `commitments`:** servicio (`DueOn`, `ReplaceForDate` transaccional,
  `Toggle`, `Recent`) + rutas (`GET /due`, `POST /{id}/toggle`).
- El check-in escribe los de mañana vía una **interfaz estrecha**
  `commitmentWriter` (`ReplaceForDate(date+1)`), sin acoplar paquetes.
- `chatcontext` gana `commitmentLister`.
- Frontend: `lib/commitments.ts`, sección «ayer» con toggle optimista, precarga
  de «mañana» desde `getDue(tomorrow)`.

## Commits

`aa369e5` migración 0015 + tabla · `b0f7dba` servicio · `c179b71` rutas +
check-in escribe mañana · `8a3be23` IA ve compromisos · `ce476b9` frontend ·
`831c996` nit review (comentario no-atomicidad) · `95d3b6b` merge.

## Decisiones / hallazgos

- **sqlc genera `time.Time` para DATE** (no `pgtype.Date`) por un override en
  `sqlc.yaml`; el Task 1 lo detectó y el servicio usa `time.Time` directo.
- **Ejecución por capas:** build roto a propósito entre Tasks 1-3 (el handler
  del check-in necesita el `commitmentWriter` que se cablea en la 3); cada task
  verifica su paquete, el build completo se exige en la 3.
- **Review final (Opus): APPROVED_WITH_NITS.** Hallazgo «Important» real: el
  upsert del check-in y `ReplaceForDate` no son una transacción; si lo segundo
  falla, el check-in queda guardado sin los compromisos (500). Es **auto-sanable
  al reintentar** (upsert idempotente + delete-then-insert) → se documentó con
  un comentario, no se bloquea. La review verificó el round-trip de fechas
  (escribe target+1, lee target del día) sin off-by-one, ownership sin fuga de
  info, y la migración de datos correcta.
- **Auto-deploy:** no disparó (4.ª vez: R14, R16, R17, R18). El usuario confirmó
  que el repo está conectado con la **GitHub App**; tras el deploy manual
  funcionó. (Pendiente: verificar que el toggle de Auto Deploy de la GitHub App
  esté encendido en el recurso para que dispare solo.)

## Verificación al cierre

- Backend 12 paquetes verde, frontend **124/124** + build.
- Smoke local 4/4 y **producción 5/5:** check-in escribe los de mañana, toggle
  marca cumplido, ownership 404, el check-in ya no expone commitments, chat
  sigue streameando. Migración 0015 aplicada (compromisos viejos migrados).

## Backlog restante

Alinear las dimensiones de *Metas* a las 4D de Capitanes; hilos + búsqueda en el
chat; backups de Postgres en el VPS; OCR de PDFs escaneados; recordatorios/
notificaciones de compromisos; dejar el auto-deploy de Coolify confirmado.
