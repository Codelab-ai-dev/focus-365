# Bitácora de sesión — Rebanada 11: Acciones de la IA (tool-use con confirmación)

**Fecha:** 2026-06-11
**Estado al cierre:** Completada y mergeada a `master`.
**Rama de trabajo:** `plan-11-acciones-ia` (mezclada con `--no-ff` y borrada).
**Merge commit:** `9861c08`

## Qué se entregó

El chat de `/asistente` deja de ser read-only: «registra mi check-in: ánimo 8,
energía 6, disciplina 9» produce una **tarjeta de acción** en el chat con
Confirmar/Cancelar. **Nada se escribe sin confirmar.** Cuatro acciones:
check-in del día, movimiento financiero (income/expense en centavos), marcar
hábito hecho y progreso de meta. La propuesta se persiste como mensaje de
`ai_messages` (columnas nuevas `action_kind/payload/status`, migración 0009) y
sobrevive recargas; la transición `proposed→done/cancelled` es atómica vía
`UPDATE ... WHERE action_status='proposed'` (doble confirm → 409). El contexto
del chat ahora incluye hábitos y metas con IDs para que el modelo los
referencie. Sin evento SSE nuevo: el `done` carga el mensaje con `id` y
`action` opcional.

- **Spec:** `docs/superpowers/specs/2026-06-11-plan-11-acciones-ia-design.md`
- **Plan:** `docs/superpowers/plans/2026-06-11-plan-11-acciones-ia.md`

## Commits (en orden)

| Commit | Tarea |
|--------|-------|
| `198237a` | Migración 0009 + queries + `CreatePairWithAction`/`GetMessageForAction`/`SetActionStatus` |
| `cb8a54c` | Contexto del chat con hábitos y metas (IDs) |
| `b396f42` | `ChatStream` con tools y reensamblado de tool calls fragmentados |
| `ac0e9f9` | `actions.go` (kinds, validación, tools) + `SendStream` propone + `Message` con `id`/`action` |
| `1ac4991` | `actionExecutor` + `ConfirmAction`/`CancelAction` |
| `127cc1e` | Endpoints `POST /ai/actions/{id}/confirm\|cancel` (404/409/400/401) |
| `37c6533` | Lib frontend: tipo `Action`, `confirmAction`/`cancelAction` |
| `ffe56d7` | Tarjeta de acción en `/asistente` |
| `5723e33` | Nits de la review final (ver abajo) |
| `9861c08` | Merge a master |

## Decisiones / desviaciones de la sesión

- **Ejecución:** subagent-driven (un implementador Sonnet por tarea, Tasks
  1–8; verificación de diffs por el controlador). Review final holística con
  Opus: `APPROVED_WITH_NITS`; los 4 hallazgos se resolvieron en `5723e33`:
  1. *First-wins en tool calls múltiples:* la acumulación ignoraba el `index`
     del chunk; dos tool calls en un turno se mezclaban y daban 503. Ahora solo
     se acumula `index == 0` (una acción por turno, como dice el spec).
  2. *Pending por tarjeta:* `actionMutation.isPending` era global y
     deshabilitaba los botones de todas las tarjetas; ahora compara
     `variables?.id` con el id del mensaje.
  3. *Test del 400 en HTTP:* confirmar una acción cuyo dominio la rechaza
     (hábito inexistente) → 400 y la acción sigue `proposed`.
  4. *Cross-user 404:* ya estaba cubierto en `TestActionErrors` (verificado).
- **Seguridad validada por la review:** ownership en todas las capas (SQL
  `WHERE user_id` + servicios de dominio), payload re-validado y
  **re-serializado** al proponer y re-validado al ejecutar
  (`DisallowUnknownFields` + rangos/UUID/enum), sin inyección posible, el
  `uuid.MustParse` del ejecutor no puede entrar en pánico.
- El plan asumía la ruta `/check-ins/today`; la real es `/checkins/today`
  (ajustado en el smoke).

## Verificación al cierre

- Backend: `go vet` + `go test -p 1 ./...` (11 paquetes) en verde — ~30 tests
  nuevos entre store, cliente, payloads, ejecutor, servicio y handler.
- Frontend: **87/87** Vitest (18 archivos) + build estricto.
- Smoke E2E `/tmp/smoke_actions.sh` vía nginx con clave Groq real: **8/8**
  (registro → el modelo propone la acción → confirm la deja `done` → el
  check-in queda escrito con mood 8 → doble confirm 409 → sin token 401 → el
  chat normal sigue streameando). Re-corrido tras el merge: 8/8.
- Nota de flakiness esperable: un check del smoke depende del modelo (que
  decida responder texto y no una acción); una corrida intermedia falló ese
  check y al investigar resultó transitorio — re-correr con otro usuario lo
  resuelve, como ya anticipaba el plan.
- DB en migración versión **9**. Docker reconstruido con el merge. Árbol limpio.

## Notas de entorno (para retomar)

Sin cambios (ver bitácoras 9 y 10). El smoke usa `/checkins/today?date=` para
verificar la escritura.

## Fuera de alcance (posibles rebanadas futuras)

Deshacer acciones ejecutadas, múltiples acciones por turno, crear/borrar
hábitos o metas, acciones de entrenamiento, editar la propuesta antes de
confirmar, múltiples hilos, búsqueda en historial, adjuntos.
