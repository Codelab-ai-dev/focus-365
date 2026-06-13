# Bitácora de sesión — Rebanada 15: Multi-acción por turno + deshacer

**Fecha:** 2026-06-12 (cierre 2026-06-13)
**Estado al cierre:** Completada, mergeada a `main` y **verificada en producción**.
**Rama:** `plan-15-multiaccion-undo` (mezclada `--no-ff` y borrada). **Merge:** `a646ee1`

## Qué se entregó

Dos capacidades estructurales del asistente:
- **Multi-acción por turno:** un mensaje puede proponer hasta 5 acciones
  (tarjetas independientes); validación all-or-nothing (si una es inválida o
  hay >5, nada se persiste).
- **Deshacer:** toda tarjeta `done` gana botón Deshacer (una vez,
  `done → undone`), con reversa por kind y best-effort.

Cambio estructural: las acciones se mudaron de columnas en `ai_messages` (1:1)
a tabla propia `ai_actions` (1:N) con columna `result` JSONB para el undo,
migrando los datos existentes (migración 0011). `ChatStream` ahora devuelve
`[]ToolCall` reensambladas por index (adiós «first wins» de la R11). Se agregó
`checkin.Delete`.

## Commits

`0da3c1d` checkin.Delete · `35a1692` mudanza a ai_actions (1:N) · `fc130e7`
ChatStream []ToolCall · `e5fe2ae` multi-acción all-or-nothing · `50a865a`
result + UndoAction · `d0c38a9` endpoint /undo · `a1e2b43` frontend N tarjetas
+ Deshacer · `2374fe3` nits review · `a646ee1` merge.

## Decisiones / hallazgos

- **Review final (Opus): APPROVED_WITH_NITS**, sin items bloqueantes. Verificó
  a fondo lo delicado: orden correcto de la migración (datos antes de dropear
  columnas), atomicidad de TODA transición vía `SetActionStatusFrom ... WHERE
  status=$from` (proposed→done/cancelled, done→undone), undo usando la **fecha
  de ejecución** guardada en el result (no el `today` del undo), best-effort
  bien distinguido (dato inexistente → undone igual; error real de DB → sigue
  done), y all-or-nothing sin ruta de persistencia parcial.
- Nits aplicados (cosméticos): comentario de `ai_actions.kind` y la nota de que
  `prev_progress` de meta se lee de la lista «activa» (asunción documentada).
- **Reuso del result para la fecha:** clave para que deshacer un check-in o
  hábito días después revierta el día correcto.
- Ejecución por subagentes con dos tareas de peso (la mudanza T2 y result+undo
  T5) en modelo capaz; el resto en Sonnet. Verifiqué los diffs de T2/T5 a mano.
- **Flake de Groq en el smoke:** correr el smoke de prod 3× seguidas dio
  rate-limiting (cada corrida hace ~4 llamadas al modelo) → falsos negativos;
  una corrida limpia dio 8/8. El multi-acción depende de que el modelo proponga
  2 tool calls — local también necesitó un reintento. La lógica está cubierta
  por tests unitarios deterministas.

## Verificación al cierre

- Backend completo (vet + `-p 1 ./...`) y frontend **109/109** + build.
- Smoke local R15 **8/8** (crear hábito → turno con 2 acciones → confirmar
  check-in → deshacer → check-in borrado → doble undo 409).
- **Producción** (auto-deploy ya activo tras la R14): smoke R15 **8/8**. DB en
  migración 11 aplicada en limpio al arrancar.

## Fuera de alcance / backlog

Redo (deshacer un undo), editar propuesta antes de confirmar, acciones de
borrado propuestas por la IA. Backlog general: importador de gastos (la última
pieza del diseño original sin construir), hilos + búsqueda en el chat, backups
de Postgres en el VPS.
