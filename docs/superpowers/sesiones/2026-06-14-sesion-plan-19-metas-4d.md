# Bitácora de sesión — Rebanada 19: dimensiones de Metas alineadas a las 4D

**Fecha:** 2026-06-14
**Estado al cierre:** Completada, mergeada a `main` y **verificada en producción**.
**Rama:** `plan-19-metas-4d` (mezclada `--no-ff` y borrada). **Merge:** `8993174`

## Qué se entregó

Las dimensiones de **Metas** pasan del vocabulario genérico
(`checkin/finanzas/entrenamiento/mente/general`) a las **4 dimensiones de
Capitanes de Dios** (`espiritual/emocional/fisica/financiera`) — las mismas del
check-in. Toda la app (metas, check-in, IA) habla ahora un solo lenguaje de
dimensiones.

- **Selector de `/metas`** con las 4 etiquetas capitalizadas (Espiritual,
  Emocional, Física, Financiera); valor almacenado en minúscula-sin-acento.
- El **chip** de cada meta muestra la etiqueta capitalizada (`DIM_LABEL`).
- La **IA** (`crear_meta`) solo propone una de las 4D, con pistas de
  desambiguación en la descripción del tool.
- Las **metas existentes** quedaron remapeadas y la columna endurecida con un
  CHECK en la DB.

## Arquitectura

- **Migración 0016:** `UPDATE` mapea old→new (finanzas→financiera,
  entrenamiento→fisica, mente→emocional, checkin→emocional, general→espiritual)
  **antes** del `ADD CONSTRAINT goals_dimension_valid CHECK (dimension IN 4D)`.
  El Down solo dropea la constraint (no revierte el remapeo).
- **`goals/handler.go`:** ambos `oneof` → `espiritual emocional fisica
  financiera` (crear `required`, patch `omitempty`).
- **`ai/actions.go`:** `goalDimensions` map + enum/descripción de `crear_meta`
  a las 4D.
- **`web/src/routes/metas.tsx`:** `DIMENSIONS` como `{value,label}[]`,
  `DIM_LABEL` value→label, default `espiritual`, chip con fallback `?? dimension`.

## Commits

`c70a792` migración + CHECK + validación · `8e2ee15` IA crear_meta 4D ·
`419299c` selector frontend · `96e083c` nit (literal viejo en wrapper test) ·
`8993174` merge · `7b522ce` script de smoke.

## Decisiones / hallazgos

- **`goals.dimension` era TEXT libre** (sin CHECK; migración 0006); la enum solo
  la validaba el handler. La 0016 la endurece a nivel DB.
- **Ejecución por capas (subagent-driven):** T1 store+goals, T2 ai, T3 web; cada
  task verde su paquete. El build completo se mantuvo (dimension es string, sin
  cambio de tipo).
- **Daño colateral esperado:** los tests de `dashboard` usaban dimensiones
  viejas como fixture; el CHECK de la 0016 los rompió a nivel DB → se
  actualizaron a 4D (el `countActive` del dashboard, que cuenta las 5 áreas de
  la app y no la dimensión de la meta, quedó intacto, como en el spec).
- **Review final (Opus): APPROVED_WITH_NITS.** Sin Critical ni Important. Nit 1
  (literal `"general"` en un wrapper test inocuo) aplicado; Nit 2 (no hay test
  directo del remapeo de datos, porque `testutil.NewDB` ya crea el schema con la
  constraint) se cubre con el smoke de producción.
- **Auto-deploy: NO disparó (5.ª vez:** R14, R16, R17, R18, R19). El usuario
  deployó a mano. **Pendiente real:** revisar el toggle "Auto Deploy" del
  recurso + webhook de la GitHub App para dejarlo automático.

## Verificación al cierre

- Backend 12 paquetes verde, frontend **127/127** + build.
- **Smoke producción 4/4:** crear meta con dimensión vieja `general` → **400**
  (CHECK/validador), `financiera` → **201**, `fisica` → **201**, `GET /goals`
  muestra la meta `financiera`. Migración 0016 aplicada (metas viejas remapeadas
  a las 4D).

## Backlog restante

Dejar el auto-deploy de Coolify confirmado (diagnóstico de raíz); hilos +
búsqueda en el chat; backups de Postgres en el VPS; OCR de PDFs escaneados;
recordatorios/notificaciones de compromisos.
