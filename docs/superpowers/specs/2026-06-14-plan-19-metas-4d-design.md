# Plan 19 — Alinear las dimensiones de Metas a las 4D — Diseño

**Fecha:** 2026-06-14
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Las dimensiones de **Metas** pasan del vocabulario genérico actual
(`checkin/finanzas/entrenamiento/mente/general`) a las **4 dimensiones de
Capitanes de Dios** (`espiritual, emocional, fisica, financiera`) — las mismas
que el check-in. Cierra la incoherencia: toda la app habla un solo lenguaje de
dimensiones.

**Decisiones (brainstorming):**
- **Solo las 4D**, sin «general». El modelo de Capitanes es estrictamente 4D;
  toda meta pertenece a una.
- **Mapeo de las metas existentes:** `finanzas→financiera`,
  `entrenamiento→fisica`, `mente→emocional`, `checkin→emocional`,
  `general→espiritual`.

**Fuera de alcance:** el `countActive` del dashboard (cuenta las 5 áreas de la
app —racha, finanzas, check-in, entreno, metas—, no la dimensión de la meta);
cualquier otra feature.

## 2. Modelo de datos (migración `0016_metas_4d.sql`)

`goals.dimension` es hoy TEXT libre (sin CHECK; la enum la valida solo el
handler). La migración mapea los datos y endurece la columna:

```sql
-- +goose Up
UPDATE goals SET dimension = CASE dimension
    WHEN 'finanzas'      THEN 'financiera'
    WHEN 'entrenamiento' THEN 'fisica'
    WHEN 'mente'         THEN 'emocional'
    WHEN 'checkin'       THEN 'emocional'
    WHEN 'general'       THEN 'espiritual'
    ELSE dimension
END;
ALTER TABLE goals ADD CONSTRAINT goals_dimension_valid
    CHECK (dimension IN ('espiritual','emocional','fisica','financiera'));

-- +goose Down
ALTER TABLE goals DROP CONSTRAINT goals_dimension_valid;
-- (No se revierte el mapeo de datos; los valores quedan en las 4D.)
```

El `UPDATE` corre **antes** del `ADD CONSTRAINT`, así toda fila preexistente ya
está en las 4D cuando se agrega el CHECK. Cualquier valor fuera de las 4D
(improbable) caería en el `ELSE` y violaría el CHECK → la migración fallaría
ruidosamente (correcto: avisa de datos inesperados).

## 3. Backend — validación y IA

- **`goals/handler.go`:** los dos tags `oneof` (crear `createReq.Dimension` y
  patch `patchReq.Dimension`) → `oneof=espiritual emocional fisica financiera`.
- **`ai/actions.go`:**
  - `goalDimensions` map → `{espiritual, emocional, fisica, financiera}`.
  - El `enum` del JSON Schema del tool `crear_meta` → las 4D.
  - La `Description` del tool y la línea del prompt (`chatprompt.go` si aplica)
    instruyen: elegir siempre la dimensión más cercana de las cuatro
    (dinero/ahorro→financiera, cuerpo/ejercicio/energía→fisica,
    emociones/autocontrol/mente→emocional, identidad/propósito/conexión/
    visión→espiritual). Sin «general».

## 4. Frontend — `/metas`

- `DIMENSIONS` pasa a las 4D. Para mostrarlas bonitas se agrega un mapa
  etiqueta→valor: `{Espiritual: "espiritual", Emocional: "emocional", Física:
  "fisica", Financiera: "financiera"}` (label con acento, valor almacenado en
  minúscula sin acento — consistente con el check-in `dim_fisica`).
- El `<select>` usa las 4 opciones con su etiqueta visible; el default del
  formulario pasa de `"general"` a `"espiritual"`.
- El chip de cada meta muestra la **etiqueta** capitalizada (mapeo
  valor→etiqueta), no el valor crudo.

## 5. Manejo de errores

- Crear/editar meta con dimensión inválida (incluida `general` vieja) → `400`
  (validador), como hoy con cualquier oneof.
- Compat: no hay propuestas `crear_meta` pendientes en producción con `general`;
  las acciones de meta ya confirmadas no se re-validan.

## 6. Testing

- **Migración:** sembrar metas con cada dimensión vieja, migrar, verificar el
  mapeo (finanzas→financiera, etc.); el CHECK rechaza un INSERT con `general`.
- **Handler de metas:** crear con una 4D → 200; con `general` → 400; patch con
  4D → 200.
- **IA:** `parseActionPayload("meta_nueva")` acepta las 4D y rechaza `general`;
  el `enum` del tool tiene las 4D.
- **Frontend:** el selector muestra las 4 etiquetas; guardar manda el valor en
  minúscula correcto; el chip muestra la etiqueta capitalizada.
- **E2E producción:** crear una meta con dimensión «financiera» → aparece en
  `/goals` con esa dimensión; las metas viejas del usuario quedan en una 4D.

## 7. Criterios de aceptación

- Metas, check-in e IA hablan el mismo lenguaje de 4 dimensiones.
- Las metas existentes quedan mapeadas a las 4D; no hay datos fuera de las
  cuatro (CHECK en la DB).
- Crear meta por formulario y por chat usa solo las 4D.
- Suites en verde; smoke de producción OK.
