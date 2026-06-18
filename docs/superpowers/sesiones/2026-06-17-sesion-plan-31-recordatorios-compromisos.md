# Bitácora de sesión — Rebanada 31: Recordatorios de compromisos (panel in-app)

**Fecha:** 2026-06-17
**Estado al cierre:** Mergeada a `main` y pusheada. **Smoke de producción pendiente del deploy manual.**
**Rama:** `plan-31-recordatorios` (mezclada `--no-ff` y borrada).

## Qué se entregó

Un **panel de recordatorios arriba de la home** que destaca los compromisos
**sin cumplir** cuya fecha es **hoy o anterior**. Antes los compromisos solo se
veían entrando al check-in de un día concreto; ahora aparecen al primer
pantallazo. Dos grupos: **Vencidos** (fecha < hoy, estilo de alerta) y **Hoy**.
Cada ítem tiene un check para marcarlo cumplido sin salir de la home; al marcarlo
desaparece del panel. Si no hay nada pendiente, el panel no se muestra.

In-app puro: **sin** push del navegador, email, service worker ni cron. (El push
queda como posible rebanada futura.)

## Arquitectura

- **Backend:** query `ListPendingCommitments` (`done = false AND target_date <= $2`,
  vencidos primero) → service `Pending(ctx, userID, today)` → endpoint
  `GET /commitments/pending` (`today` = `now().UTC().Truncate(24h)`). Marcar
  cumplido **reusa** el `POST /commitments/{id}/toggle` existente.
- **Frontend:** `getPendingCommitments()` en `lib/commitments.ts`; nuevo
  `ui/RemindersPanel.tsx` (query `["commitments","pending"]`, agrupa vencido/hoy
  comparando con `todayString()`, oculto si vacío, check → toggle → invalida
  `pending` + `dashboard`). Montado en `routes/index.tsx` arriba de la grilla.

## Commits

`f87b1dc` query+servicio Pending · `9cf1907` endpoint /pending · `68b5372` panel +
lib + home · `556b3ed` etiqueta "Hoy" · `1c38ae3` link sin parámetro de fecha ·
`ce6406a` test de desaparición · merge + smoke.

## Decisiones / hallazgos

- **Link al check-in sin `date`:** el spec proponía `/check-in?date=…`, pero la
  página de check-in está construida enteramente alrededor de "hoy"/"mañana" y no
  lee un parámetro de fecha. Soportar fechas arbitrarias era scope creep fuera de
  esta rebanada. La acción principal (marcar cumplido) ya está inline en el panel,
  así que el texto enlaza a `/check-in` a secas.
- **Sin manejo de error en el toggle:** se calca el `toggleMutation` ya existente
  en `check-in.tsx`, que es fire-and-forget con solo `onSuccess` invalidando. Añadir
  UI de error bespoke sería inconsistente con el patrón del repo.
- **`checked={false}` (fire-and-forget):** el ítem desaparece tras el toggle
  (invalidación → refetch), así que nunca hace falta el estado "marcado". El test
  reforzado verifica justamente esa desaparición.
- **Fechas:** backend agrupa por medianoche UTC; el front re-agrupa con
  `todayString()` local. Cerca de medianoche y lejos de UTC un ítem podría caer en
  otro grupo, pero siempre se muestra (consistente con el manejo de fechas del
  resto de la app). No bloqueante.
- **Review final: READY TO MERGE** — contrato backend↔front alineado, sin debug ni
  scope creep, tests y build verdes.

## Verificación al cierre

- Backend: `go test -p 1 ./internal/commitments/` verde (incluye `TestPending` +
  `TestPendingEndpoint`); build + vet limpios.
- Frontend: `RemindersPanel.test.tsx` 3/3; `npm run build` (tsc) limpio.
- **Smoke producción:** pendiente del deploy manual. `scripts/smoke-r31.sh` crea un
  compromiso vencido (ayer) y otro de hoy vía el `POST /checkins` (que guarda los
  compromisos para `date + 1`), verifica que `pending` trae ambos (vencido primero),
  marca el de hoy y verifica que sale de `pending` y el vencido permanece. Usa `jq`.

## Backlog restante

Push del navegador (PWA) como evolución de los recordatorios; backups off-site (B2)
— mejoras futuras.
