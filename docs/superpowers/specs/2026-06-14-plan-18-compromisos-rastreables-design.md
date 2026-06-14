# Plan 18 — Compromisos rastreables — Diseño

**Fecha:** 2026-06-14
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Los compromisos del check-in («Mañana me comprometo a:») pasan de ser texto
suelto a **rastreables**: lo que te comprometiste un día aparece al día
siguiente para marcar si lo cumpliste. Cierra el ciclo de rendición de cuentas
del check-in de Capitanes de Dios.

**Flujo:** durante el check-in del día D escribes compromisos para **mañana**
(target D+1). Al abrir el check-in del día D, arriba ves «📋 Ayer te
comprometiste a» con los compromisos cuyo target = D, cada uno con un check para
marcar cumplido al instante.

**Decisiones (brainstorming con mockup, aprobado):**
- **Dónde se marcan:** en el check-in del día (sección arriba).
- **Sin cumplir:** queda marcado **no-cumplido**, **no se arrastra** (cada
  compromiso vive en su fecha objetivo).
- **IA:** el asistente ve los compromisos recientes y su cumplimiento como
  contexto de rendición de cuentas.

**Fuera de alcance:** arrastrar pendientes a días siguientes; recordatorios/
notificaciones; señal de compromisos en el dashboard (se evalúa después).

## 2. Modelo de datos (migración `0015_commitments.sql`)

Los compromisos salen del JSONB del check-in a una **tabla propia** (cada uno
con fecha objetivo y estado):

```sql
-- +goose Up
CREATE TABLE commitments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_date DATE NOT NULL,
    text        TEXT NOT NULL,
    done        BOOLEAN NOT NULL DEFAULT false,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_commitments_user_target ON commitments (user_id, target_date);

-- Migra los compromisos existentes del JSONB del check-in: cada string del
-- array de un check-in de fecha F se vuelve un commitment con target = F+1.
INSERT INTO commitments (user_id, target_date, text, position)
SELECT c.user_id, c.date + INTERVAL '1 day',
       elem.value #>> '{}', elem.ordinality - 1
FROM check_ins c,
     jsonb_array_elements(c.commitments) WITH ORDINALITY elem(value, ordinality)
WHERE jsonb_array_length(c.commitments) > 0;

ALTER TABLE check_ins DROP COLUMN commitments;

-- +goose Down
ALTER TABLE check_ins ADD COLUMN commitments JSONB NOT NULL DEFAULT '[]';
-- (Repoblar el JSONB desde la tabla es opcional; el Down deja la columna vacía.)
DROP TABLE commitments;
```

El check-in deja de tener `commitments`; el resto de sus campos (4D, win,
avoided, mood, energy) no cambia.

**Queries (`db/queries/commitments.sql`):**
- `ReplaceCommitmentsForDate` — borra los commitments del usuario para un
  `target_date` y los re-inserta (lo usa el guardado del check-in al escribir
  «mañana»). En transacción: `DELETE WHERE user_id=$1 AND target_date=$2` +
  N `INSERT`.
- `ListCommitmentsByTarget :many` — `WHERE user_id=$1 AND target_date=$2 ORDER
  BY position`.
- `ToggleCommitment :one` — `UPDATE ... SET done = NOT done, updated_at=now()
  WHERE id=$1 AND user_id=$2 RETURNING *`.
- `ListRecentCommitments :many` — `WHERE user_id=$1 AND target_date >= $2 ORDER
  BY target_date DESC, position` (para el contexto de la IA; `$2` = hoy−7).

## 3. Backend — paquete `commitments` (nuevo)

`commitments.Service` con:
- `DueOn(userID, date) ([]Commitment, error)` — los del target = date (lo que se
  marca en el check-in de ese día).
- `ReplaceForDate(userID, target date, texts []string)` — reemplaza los
  compromisos de un día (filtra vacíos/trim, posición por orden). Lo llama el
  guardado del check-in con `target = checkin_date + 1`.
- `Toggle(userID, id) (*Commitment, error)` — invierte `done`; (nil,nil) si no
  es del usuario → 404.
- `Recent(userID, since date) ([]Commitment, error)` — para la IA.

`Commitment` vista: `{id, target_date, text, done}`.

## 4. API

- `GET /api/v1/commitments/due?date=YYYY-MM-DD` → `{commitments:[...]}` (los del
  target = date). Default date = hoy.
- `POST /api/v1/commitments/{id}/toggle` → `{commitment: ...}` (invierte done).
  404 si no es del usuario.
- **Check-in (`POST /checkins`)**: el body sigue trayendo `commitments:
  string[]`, pero ahora el handler, tras el upsert del check-in, llama
  `commitments.ReplaceForDate(userID, date+1, body.commitments)`. El response
  del check-in ya **no** incluye `commitments` (viven aparte). `GET
  /checkins/today` tampoco.

Montadas bajo `RequireAuth` en un `commitments.Routes`, montado en `/commitments`.

## 5. IA — rendición de cuentas

`chatcontext` incluye los **compromisos recientes** (target ≥ hoy−7) con su
estado: el asistente ve «2026-06-13: tender la cama ✓, pasear a Ruffo ✗». Una
interfaz estrecha nueva `commitmentLister` (porción de `commitments.Service.
Recent`); el `NewChatContextBuilder` gana esa dependencia (wiring en server.go y
handler_test). El JSON del contexto gana `"commitments": [...]`. Sin acción IA
nueva (los compromisos los marca el usuario).

## 6. Frontend

- **`web/src/lib/commitments.ts`** (nuevo): tipos + `getDue(date)`,
  `toggle(id)`. El check-in `upsert` sigue mandando `commitments: string[]`
  (ahora son los de mañana).
- **`/check-in`**: arriba, sección **«📋 Ayer te comprometiste a»** —
  `useQuery(["commitments","due", today])` con `getDue(today)`; cada compromiso
  con un checkbox (estilo neo-brutalista) que al click llama `toggle(id)`
  (optimista, invalida la query). Contador «N/M ✓». Si no hay compromisos para
  hoy, la sección no se muestra. La lista **«Mañana me comprometo a»** sigue
  igual (se guarda con el check-in → target mañana). El check-in ya no precarga
  `commitments` desde el check-in (esa lista de "mañana" arranca vacía o, si
  quieres editar los de mañana ya escritos, se cargan con `getDue(tomorrow)` —
  **decisión:** precargar «mañana» con `getDue(today+1)` para poder editarlos).
- Los tipos de check-in (`CheckIn`) pierden `commitments`.

## 7. Manejo de errores

| Caso | Respuesta | UX |
|------|-----------|-----|
| Toggle de compromiso ajeno/inexistente | 404 | el check se revierte |
| Sin token | 401 | redirige a login |
| Guardar check-in con compromisos vacíos | OK (se filtran) | — |

## 8. Testing

- **Migración:** los compromisos del JSONB de un check-in sembrado migran a la
  tabla con target = fecha+1 y posición correcta.
- **Servicio:** `ReplaceForDate` reemplaza (no duplica) y filtra vacíos;
  `DueOn` filtra por target; `Toggle` invierte y respeta ownership; `Recent`
  trae los de la ventana.
- **Handler:** `GET /due` lista; `toggle` invierte y 404 ajeno; el check-in
  POST escribe los de mañana (target+1) vía ReplaceForDate.
- **chatcontext:** incluye los compromisos recientes con su done.
- **Frontend:** la sección «ayer» aparece con los due, el checkbox togglea
  (optimista), el contador; «mañana» se guarda con el check-in; sin due no se
  muestra la sección.
- **E2E producción:** guardar check-in del día con 2 compromisos de mañana →
  al pedir `/commitments/due?date=mañana` aparecen → toggle uno → queda done.

## 9. Criterios de aceptación

- Lo que te comprometes hoy «para mañana» aparece mañana en el check-in para
  marcar; marcar es inmediato y persiste.
- Los no cumplidos quedan no-cumplidos, sin arrastrarse.
- La IA ve tus compromisos recientes y su cumplimiento.
- El check-in (4D, win, etc.) sigue funcionando; los compromisos existentes
  (pre-migración) se conservan; suites en verde; smoke de producción OK.
