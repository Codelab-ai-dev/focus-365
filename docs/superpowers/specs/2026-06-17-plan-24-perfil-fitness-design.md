# Plan 24 — Perfil de fitness (entrenamiento, slice A) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 0. Contexto: descomposición del módulo de entrenamiento

La expansión del módulo de entrenamiento se descompone en sub-proyectos
independientes, cada uno con su ciclo spec → plan → implementación:

- **A — Perfil de fitness** (este spec): datos del usuario (edad, peso, objetivo,
  equipo, etc.). Base que alimenta a la IA.
- **B — Agente de sugerencias:** el agente propone rutinas/ejercicios según el
  perfil + historial. Depende de A.
- **C — Notas por ejercicio + ajustes del agente:** anotar tras cada serie; la IA
  ajusta. Depende de A y B.
- **D — Evolución / progreso:** gráficos de avance. Independiente.

Este spec cubre **solo A**.

## 1. Visión y alcance

En `/entrenamiento`, un botón **"Mi perfil"** abre el `Modal` (componente
reutilizable de la R22) con el formulario del perfil de entrenamiento. Es un
**único perfil por usuario** que se guarda/actualiza (upsert). **Todos los campos
son opcionales** — el usuario llena lo que quiera.

**Decisiones (brainstorming):**
- Campos: fecha de nacimiento (la edad se calcula), peso, altura, sexo, objetivo,
  equipo (multi), lugar, nivel, frecuencia semanal, limitaciones (texto libre).
- La **edad se guarda como fecha de nacimiento** (no como número).
- UI en **Modal** abierto desde `/entrenamiento`.

**Fuera de alcance (slices B/C/D):** que la IA consuma el perfil; sugerencias de
rutina/ejercicios; notas por ejercicio; visualización de progreso. Este slice es
solo el CRUD del perfil.

## 2. Modelo de datos (migración `0020_fitness_profiles.sql`)

```sql
-- +goose Up
CREATE TABLE fitness_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    birthdate    DATE,
    sex          TEXT,            -- masculino | femenino | otro
    height_cm    INT,
    weight_grams INT,             -- consistente con workout_sets (80.5 kg = 80500)
    objective    TEXT,            -- perder_grasa | hipertrofia | fuerza | resistencia | salud
    location     TEXT,            -- casa | gym | ambos
    level        TEXT,            -- principiante | intermedio | avanzado
    weekly_days  INT,             -- 1..7
    equipment    TEXT[] NOT NULL DEFAULT '{}',
    limitations  TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE fitness_profiles;
```

- **1:1 con el usuario:** `user_id` es PK. Borrar el usuario borra su perfil.
- Columnas escalares **nullable** (campo opcional = NULL); `equipment` y
  `limitations` con default no-NULL para simplificar el manejo.
- `birthdate`/`weight_grams`/`height_cm` se generan como `time.Time`/`*int32`
  según el override de sqlc; los enteros opcionales serán punteros.

## 3. Backend

### Queries (`api/db/queries/fitness_profiles.sql`)
- `GetFitnessProfile(user_id)` → la fila o `ErrNoRows` (perfil no creado).
- `UpsertFitnessProfile(...)` → `INSERT ... ON CONFLICT (user_id) DO UPDATE SET ...`
  con todas las columnas + `updated_at = now()`, `RETURNING *`.

### Servicio y rutas (paquete `training`)
- El servicio gana `Profile(ctx, userID)` (devuelve `*Profile` o `nil` si no
  existe) y `SaveProfile(ctx, userID, in ProfileInput) (*Profile, error)`.
- Rutas nuevas en `training.Routes` (ya bajo `RequireAuth`):
  - `GET /training/profile` → `200` con el perfil, o `200` con `null` si no existe.
  - `PUT /training/profile` → `200` con el perfil guardado.
- **Validación** (en el handler, solo de los campos presentes; todos opcionales):
  - `sex` ∈ {masculino, femenino, otro}; `objective` ∈ {perder_grasa,
    hipertrofia, fuerza, resistencia, salud}; `location` ∈ {casa, gym, ambos};
    `level` ∈ {principiante, intermedio, avanzado}; cada item de `equipment` ∈
    {peso_corporal, mancuernas, barra, banco, bandas, kettlebell, dominadas, gym}.
  - `weekly_days` ∈ 1..7; `height_cm` > 0; `weight_grams` > 0; `birthdate`
    formato `YYYY-MM-DD`.
  - Cualquier inválido → `400`. Campo ausente/`null` → se guarda NULL.
- **Conversión de unidades:** el front manda **kg** y **cm**; el backend recibe
  esas unidades en el body (peso en kg con decimales, altura en cm) y guarda
  `weight_grams = round(kg*1000)` y `height_cm`. La vista devuelve `weight_kg`
  (float) y `height_cm` para que el front no tenga que convertir.

### Vista (JSON)
```
ProfileView {
  birthdate: string|null (YYYY-MM-DD),
  sex: string|null, height_cm: number|null, weight_kg: number|null,
  objective: string|null, location: string|null, level: string|null,
  weekly_days: number|null, equipment: string[], limitations: string,
  updated_at: string
}
```

## 4. Frontend

- **`web/src/lib/fitnessProfile.ts`:** `type FitnessProfile` (espejo de
  `ProfileView`) + `getProfile(): Promise<FitnessProfile | null>` +
  `saveProfile(input): Promise<FitnessProfile>`.
- **`web/src/routes/entrenamiento.tsx`:** botón **"Mi perfil"** arriba (junto al
  header). Abre un `ProfileModal` (en `entrenamiento.tsx`) con `Modal`:
  - Query `["fitness-profile"]` con `getProfile`; precarga el form al abrir.
  - Form: `<input type="date">` (nacimiento), selects de sexo/objetivo/lugar/
    nivel, `number` de altura (cm) / peso (kg, step 0.1) / frecuencia (1–7),
    checklist de equipo (multi), textarea de limitaciones. Guardar → `saveProfile`
    + invalidar `["fitness-profile"]`; cerrar al éxito.
  - Etiquetas legibles para los enums (p. ej. `perder_grasa` → "Perder grasa").

## 5. Manejo de errores

- Enum inválido / `weekly_days` fuera de rango / altura o peso ≤ 0 / fecha mal
  formada → `400` con mensaje claro; el form muestra el error y no cierra.
- Perfil inexistente → `GET` devuelve `null`; el form arranca en blanco.
- Ownership: cada usuario solo ve/edita su perfil (`user_id` del token; PK).

## 6. Testing

- **Backend (store):** upsert inserta y luego **reemplaza** (sigue habiendo una
  sola fila por usuario); `Get` inexistente → `ErrNoRows`; `equipment` array
  ida y vuelta.
- **Backend (handler):** `PUT` válido → 200 con los valores (kg↔grams ok); enum
  inválido → 400; `weekly_days=8` → 400; peso negativo → 400; `GET` sin perfil →
  200 `null`; `GET` tras `PUT` → el perfil; dos usuarios no se pisan.
- **Frontend:** abrir el modal con un perfil mockeado precarga los campos;
  guardar hace `PUT` con kg/cm; perfil vacío → form en blanco; un enum se
  muestra con su etiqueta.
- **E2E producción:** guardar un perfil (con equipo y limitaciones), volver a
  leerlo, modificarlo (verificar que sigue habiendo un solo perfil).

## 7. Criterios de aceptación

- Desde `/entrenamiento` se abre "Mi perfil", se completan los campos (todos
  opcionales) y se guardan; al reabrir, están precargados.
- Un solo perfil por usuario (upsert); valores inválidos → 400.
- El perfil es estrictamente del usuario.
- Suites en verde; smoke de producción OK.
