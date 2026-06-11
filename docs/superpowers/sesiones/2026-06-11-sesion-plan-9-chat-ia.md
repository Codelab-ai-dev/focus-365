# Bitácora de sesión — Rebanada 9: Asistente IA on-demand (chat)

**Fecha:** 2026-06-11
**Estado al cierre:** Completada y mergeada a `master`.
**Rama de trabajo:** `plan-9-chat-ia` (mezclada con `--no-ff` y borrada).
**Merge commit:** `02a820f`

## Qué se entregó

Chat conversacional **read-only** en la ruta `/asistente`: el usuario pregunta
sobre sus datos y la IA responde con su contexto real. Multi-turno, historial
persistido en la tabla nueva `ai_messages` (sobrevive a recargas). Una sola
conversación por usuario. Contexto enriquecido: snapshot del día + ciclos
financieros + check-ins recientes (14). Degradación 503 sin clave o ante fallo
de Groq (no persiste nada). Comparte el cliente Groq con el insight proactivo
(rebanada 8) sin acoplarse.

- **Spec:** `docs/superpowers/specs/2026-06-11-plan-9-chat-ia-design.md`
- **Plan:** `docs/superpowers/plans/2026-06-11-plan-9-chat-ia.md`

## Commits (en orden)

| Commit | Tarea |
|--------|-------|
| `ecf5e3b` | Tabla `ai_messages` (migración 0008 + queries + store round-trip) |
| `e91af1d` | `GroqClient.Chat` multi-turno (refactor del POST compartido en `send`) |
| `60ffacd` | Constructor de contexto (`chatcontext.go`: snapshot + ciclos + check-ins) |
| `1bec7a0` | System prompt del chat (`chatprompt.go`, español, solo-datos) |
| `5a669d8` | `ChatService` (`chat.go`, persiste solo ante éxito, cola ~10 turnos) |
| `b1301e8` | Endpoints `GET /ai/messages` + `POST /ai/chat` + wiring |
| `fe8561d` | Lib frontend (`web/src/lib/ai.ts`: `getMessages`, `sendMessage`) |
| `8cef51f` | Ruta `/asistente` + nav del TopBar + link de la banda del dashboard |
| `8b0f440` | Revisión final: persistencia atómica del par + límite por caracteres |
| `02a820f` | Merge a master |

## Decisiones / desviaciones de la sesión

- **Ejecución:** subagent-driven development. Un subagente implementador por
  tarea (Tasks 1–8); para tareas mecánicas verifiqué los diffs/firmas yo mismo
  en vez de lanzar dos subagentes de review por tarea (control de costo). Review
  final holística con un subagente Opus: veredicto `APPROVED_WITH_NITS`.
- **Aislamiento:** rama de feature en vez de worktree (docker está atado a
  `/Users/gustavo/Desktop/focus-365`, como en rebanadas previas).
- **Nits de la review final, ya resueltos en `8b0f440`:**
  - *Persistencia atómica:* nuevo `pgChatStore.CreatePair` (`chatstore.go`)
    inserta pregunta+respuesta en una transacción (mismo patrón que `training`),
    evitando mensajes de usuario huérfanos si fallara el 2.º insert. El fake
    `memStore` y el wiring (`server.go`, `handler_test.go`) se ajustaron.
  - *Límite por caracteres:* la validación del mensaje cuenta runes
    (`utf8.RuneCountInString > 2000`) en el handler, no bytes vía tag `max`, para
    que el español con acentos no se rechace antes. Test
    `TestChatValidationLengthCountsRunes` (2000 OK / 2001 → 400).

## Verificación al cierre

- Backend `make check` (vet + `go test -p 1 ./...`): verde, incluido el test nuevo.
- Frontend: 71/71 Vitest + `npm run build` (tsc estricto) sin errores.
- Smoke E2E docker `/tmp/smoke_chat.sh`: **6/6 checks OK** con la clave Groq real
  (registro → historial vacío → chat 200 con reply assistant → par persistido →
  validación vacío 400 → sin token 401).
- DB en migración versión **8**. Árbol git limpio tras el merge.

## Notas de entorno (para retomar)

- Comandos `go`/`sqlc`: `GOTOOLCHAIN=local`, desde `api/`. Go en `/usr/local/go/bin`.
- `make check` necesita `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`.
- Docker: puertos host db `5544`, api `8088`, web `5174`. Los comandos docker
  necesitan `dangerouslyDisableSandbox: true` y, en la misma línea,
  `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"`.
  El `.env` del entorno **tiene** `GROQ_API_KEY` real (el camino feliz funciona end-to-end).
- El api aplica migraciones al arrancar (`docker compose up -d --build`).
- Smoke: usar `USERID` (no `UID`, que es readonly en zsh).

## Fuera de alcance (posibles rebanadas futuras)

Acciones/tool-use de la IA, streaming de tokens, múltiples conversaciones/hilos,
búsqueda en el historial, adjuntos. Mejora opcional pendiente: cota dura de bytes
previa al rune-count si se quisiera blindar contra payloads enormes.
