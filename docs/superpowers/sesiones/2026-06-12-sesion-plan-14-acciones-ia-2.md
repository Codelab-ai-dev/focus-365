# Bitácora de sesión — Rebanada 14: Acciones de la IA parte 2

**Fecha:** 2026-06-12
**Estado al cierre:** Completada, mergeada a `main` y **verificada en producción**.
**Rama:** `plan-14-acciones-ia-2` (mezclada `--no-ff` y borrada). **Merge:** `1c158be`

## Qué se entregó

Tres acciones nuevas sobre el mecanismo de la R11:
- **Crear hábito** (`crear_habito` → `habits.Create`, con `target_days` opcional).
- **Crear meta** (`crear_meta` → `goals.Create`, dimension enum + deadline opcional).
- **Registrar entrenamiento completo** (`registrar_entrenamiento` →
  `training.CreateWorkout` con series `{exercise, reps?, weight_kg?}`,
  kg→gramos con `math.Round`, máx. 20 series; los ejercicios se auto-crean).

Migración 0010 amplía el CHECK de `action_kind` a 7 kinds. Tarjetas del
frontend con detalles por kind (el entreno lista series: «press banca ×8 @60kg»).

## Commits

`7abfd2b` migración 0010 · `0f7a7ad` payloads/tools/summaries · `5544681`
ejecutor 7 deps · `6edfa39` prompt + e2e · `e2567c4` tarjetas · `aadf2d9`
nit review (round) · `1c158be` merge.

## Decisiones / hallazgos

- **Review final (Opus): APPROVED_WITH_NITS** — un nit sustantivo: el código
  truncaba kg→gramos y el spec decía `round`; alineado con `math.Round` + test
  que fija el redondeo (60.5499 → 60550). La review verificó además: overflow/
  NaN inalcanzables (validación + JSON sin NaN), re-validación al ejecutar,
  enum de dimension idéntico al handler HTTP, invariancia de las 4 acciones
  R11, Down de la migración correcto, XSS-safe en tarjetas.
- **El auto-deploy no estaba activo:** el push a `main` no desplegó (el repo
  está conectado sin webhook). Producción siguió en la versión anterior — se
  detectó porque el modelo respondía «no puedo crear hábitos» (sin la tool).
  Deploy manual del usuario + instrucciones para activar el webhook/GitHub App.
  **Pendiente del usuario:** activar Auto Deploy en Coolify.

## Verificación al cierre

- Backend completo (vet + `-p 1 ./...`) y frontend **106/106** + build.
- Smoke local 8/8 (con el flake conocido del modelo en una corrida, re-corrida
  limpia).
- **Producción:** smoke R14 **4/4** (propone `habito_nuevo` → confirmar →
  hábito real «leer 30 minutos» en la DB) + smoke de regresión **8/8** (todo
  lo de R11–13 sigue vivo).

## Fuera de alcance (R15, ya acordada en brainstorming)

Multi-acción por turno (tabla propia de acciones, reensamblado por index>0) y
deshacer acciones ejecutadas (reversa por kind; el check-in necesita snapshot
previo). También pendiente del backlog: importador de gastos, hilos+búsqueda,
backups de Postgres en el VPS.
