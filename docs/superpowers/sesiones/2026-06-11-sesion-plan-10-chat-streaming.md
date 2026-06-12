# Bitácora de sesión — Rebanada 10: streaming de tokens en el chat IA

**Fecha:** 2026-06-11
**Estado al cierre:** Completada y mergeada a `master`.
**Rama de trabajo:** `plan-10-chat-streaming` (mezclada con `--no-ff` y borrada).
**Merge commit:** `998a888`

## Qué se entregó

La respuesta del asistente en `/asistente` aparece **palabra por palabra** vía
SSE. Endpoint nuevo `POST /ai/chat/stream` (eventos `delta`/`done`/`error`);
el `POST /ai/chat` bloqueante queda intacto. Semántica «persistir solo ante
éxito» conservada: un corte a medias emite `event: error`, no persiste nada y
el frontend descarta el parcial. Errores previos al primer delta son HTTP
normales (400/401/503). `X-Accel-Buffering: no` evita el buffering de nginx
sin tocar `nginx.conf`.

- **Spec:** `docs/superpowers/specs/2026-06-11-plan-10-chat-streaming-design.md`
- **Plan:** `docs/superpowers/plans/2026-06-11-plan-10-chat-streaming.md`

## Commits (en orden)

| Commit | Tarea |
|--------|-------|
| `98f1744` | `GroqClient.ChatStream` (parser SSE de Groq + cliente HTTP de 60s) |
| `5d76884` | `ChatService.SendStream` + interfaz `chatStreamer` + wiring (constructor a 5 args) |
| `d54b46a` | Endpoint SSE `POST /ai/chat/stream` + helper `decodeChatMessage` compartido |
| `afdee8e` | Lib frontend `sendMessageStream` (fetch + ReadableStream, buffer de reensamblado) |
| `5b36e36` | Página `/asistente` con burbuja en vivo y descarte del parcial ante error |
| `6c2aca9` | Nits de la review final (cancel del reader + comentario respuesta vacía) |
| `998a888` | Merge a master |

## Decisiones / desviaciones de la sesión

- **Ejecución:** subagent-driven development (un implementador Sonnet por
  tarea, Tasks 1–5; verifiqué los diffs yo mismo en vez de dos reviewers por
  tarea, igual que en la rebanada 9). Review final holística con Opus:
  veredicto `APPROVED_WITH_NITS` (ambos nits aplicados en `6c2aca9`).
- **Desviación aceptada del plan (Task 5):** el `onSuccess` de la página usa
  `qc.setQueryData` (append optimista del par persistido) en vez de
  `await qc.invalidateQueries`. El plan tenía un fallo real: con el mock del
  GET devolviendo `[]`, invalidar hacía desaparecer la burbuja (y en
  producción era un round-trip extra). El cache se sincroniza con el server en
  el siguiente mount. La review final lo verificó sin bugs de orden.
- **Hallazgo de la review (no-bug):** no hay carrera en el flag `started` del
  handler — `ChatStream` lee el body y llama `onDelta` en la misma goroutine
  del request.

## Verificación al cierre

- Backend: `go vet` + `go test -p 1 ./...` (11 paquetes) en verde, incluidos
  3 tests nuevos de cliente, 3 de servicio y 5 del handler SSE.
- Frontend: **81/81** Vitest (18 archivos) + `npm run build` estricto.
- Smoke E2E `/tmp/smoke_stream.sh` a través del **proxy nginx** (puerto 5174)
  con la clave Groq real: **7/7** (registro → deltas → done → sin error → par
  persistido → vacío 400 → sin token 401). Re-corrido tras el merge.
- Entrega incremental verificada midiendo timestamps por evento: el primer
  delta llega antes del `done`. Nota: Groq genera ~400+ tok/s, así que en
  respuestas cortas el stream completo dura <300ms — el efecto visual es
  sutil, pero el mecanismo funciona y escala a respuestas largas.
- Docker reconstruido con el código mergeado. Árbol git limpio.

## Notas de entorno (para retomar)

Sin cambios respecto a la rebanada 9: `GOTOOLCHAIN=local` desde `api/`,
`TEST_DATABASE_URL` con puerto 5544, docker con PATH extendido y
`dangerouslyDisableSandbox`, `USERID` en scripts zsh. La sesión también
arregló (antes de esta rebanada) el bug de sesión-al-recargar: bootstrap de
`AuthProvider` vía `POST /auth/refresh` + endpoint nuevo `POST /auth/logout`
(merge `f1233e4`).

## Fuera de alcance (posibles rebanadas futuras)

Acciones/tool-use de la IA, múltiples hilos, búsqueda en el historial,
adjuntos, reanudación de un stream cortado. Mejora opcional: cota dura de
bytes previa al rune-count en la validación del chat.
