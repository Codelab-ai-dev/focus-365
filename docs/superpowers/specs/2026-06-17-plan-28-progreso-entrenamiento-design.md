# Plan 28 — Evolución / progreso (entrenamiento, slice D) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 0. Contexto: descomposición del módulo de entrenamiento

Última pieza de la expansión de entrenamiento. Sub-proyectos: A perfil (R24),
B sugerencias (R25), C1 notas por serie (R26), C2 ajustes del agente (R27),
**D — evolución/progreso** (este spec). Independiente: solo lee datos existentes,
sin IA y sin backend nuevo.

## 1. Visión y alcance

Una sección **"Progreso"** en `/entrenamiento` (debajo del historial) con cuatro
vistas, **calculadas en el frontend** a partir de los entrenos ya registrados
(reutiliza la query del historial; **sin endpoint nuevo**):

- **Volumen por semana** — barras del volumen total (Σ reps×peso, en kg) por
  semana.
- **Frecuencia por semana** — barras de la cantidad de sesiones por semana.
- **Progresión por ejercicio** — un selector de ejercicio + barras del **peso
  máximo por sesión** de ese ejercicio en el tiempo.
- **Records (PRs)** — lista de cada ejercicio con su mejor peso.

Gráficos **SVG hechos a mano** (neo-brutalista: borde de tinta, barras `accent`),
**sin librería nueva**.

**Decisiones (brainstorming):**
- Las cuatro métricas.
- **Cómputo en el frontend** (no backend de agregación).
- **SVG a mano** (no chart lib).
- Sección dentro de `/entrenamiento` (no página aparte).

**Fuera de alcance:** endpoint de agregación; rangos de fecha configurables;
exportar; gráficos de línea/avanzados (se usan barras simples).

## 2. Arquitectura

Frontend-only. No toca el backend.

### `web/src/lib/trainingProgress.ts` (lógica pura, el grueso testeable)
Funciones puras sobre la lista de `Workout` (la que ya devuelve `listWorkouts`,
con `date: YYYY-MM-DD` y `sets[]` con `exercise`, `reps`, `weight_grams`, `note`):

- `weeklyVolume(workouts, weeks = 12): { label: string; value: number }[]`
  agrupa por semana (lunes de cada semana como clave/etiqueta) y suma
  `reps × (weight_grams/1000)` de todas las series; devuelve las últimas `weeks`
  semanas en orden cronológico (las semanas sin entrenos cuentan 0 dentro del
  rango cubierto).
- `weeklyFrequency(workouts, weeks = 12): { label; value }[]` — cantidad de
  sesiones por semana (mismas semanas que volumen).
- `exerciseNames(workouts): string[]` — ejercicios distintos (orden alfabético),
  para poblar el selector.
- `exerciseProgression(workouts, exercise, sessions = 12): { label; value }[]` —
  por cada sesión (fecha) que incluyó ese ejercicio, el **peso máximo** (kg)
  registrado; últimas `sessions` en orden cronológico.
- `personalRecords(workouts): { exercise: string; weightKg: number }[]` — el
  mejor peso por ejercicio (orden por peso desc); ejercicios sin peso registrado
  se omiten.

Helpers internos: `mondayOf(date)` (lunes de la semana de una fecha local),
parseo de `YYYY-MM-DD` en local (sin corrimiento UTC), `gramsToKg` (ya existe en
`lib/training`).

### `web/src/ui/BarChart.tsx` (nuevo, reutilizable)
- Props: `{ data: { label: string; value: number }[]; unit?: string; className? }`.
- Dibuja un **SVG** con una barra por dato (alto proporcional al máximo del
  set), borde de tinta y relleno `accent`; etiqueta corta debajo de cada barra y
  el valor (redondeado) arriba o en `title`. Lista vacía → un texto "sin datos".
- Estilo del design system: `border-ink`, `bg-accent`, `text-muted`. Accesible:
  `role="img"` con `aria-label` que resume la serie.

### `web/src/routes/entrenamiento.tsx`
- Una sección **"Progreso"** después del historial. Reutiliza
  `historyQuery.data` (la lista de workouts ya cargada por la página).
- Arma las series con `trainingProgress` y renderiza:
  - `BarChart` de volumen semanal y de frecuencia semanal.
  - Un `<select>` poblado con `exerciseNames` (estado `progressExercise`) + un
    `BarChart` de `exerciseProgression`.
  - La lista de PRs (`personalRecords`).
- Estado vacío (sin entrenos) → "Registrá entrenos para ver tu progreso".

## 3. Detalles

- Ventana: **12 semanas** para volumen/frecuencia; **12 sesiones** para la
  progresión del ejercicio elegido; PRs sobre todo el historial.
- Pesos en kg vía `gramsToKg`. Series con pocos datos se muestran igual.
- El selector de progresión arranca en el primer ejercicio (alfabético) si hay;
  si el usuario no tiene ejercicios, la sub-sección de progresión no se muestra.

## 4. Manejo de errores / bordes

- Sin workouts → estado vacío en toda la sección. Ejercicio sin peso → progresión
  vacía. Semanas sin entrenos dentro del rango → barra en 0. Series con un solo
  dato → una barra. Todo client-side: sin errores de red nuevos.

## 5. Testing

- **`lib/trainingProgress.ts` (unit, el núcleo):** `weeklyVolume` agrupa y suma
  por semana correctamente (dos sesiones en la misma semana se suman; sesiones de
  semanas distintas en barras distintas); `weeklyFrequency` cuenta sesiones por
  semana; `exerciseProgression` toma el **máximo** por sesión del ejercicio y solo
  de ese ejercicio; `personalRecords` da el mejor peso por ejercicio y omite los
  sin peso; casos vacíos → arrays vacíos.
- **`ui/BarChart.tsx`:** renderiza una barra (`<rect>`) por dato; lista vacía →
  el texto "sin datos"; el `aria-label` describe la serie.
- **`entrenamiento.tsx`:** con workouts mockeados, la sección "Progreso" muestra
  los gráficos; cambiar el `<select>` cambia la serie de progresión; los PRs se
  listan.
- **E2E producción:** visual (frontend-only, sin smoke con curl): registrar un
  par de entrenos y ver los gráficos poblarse.

## 6. Criterios de aceptación

- En `/entrenamiento` hay una sección "Progreso" con volumen y frecuencia
  semanales, progresión de peso por ejercicio (con selector) y PRs, calculados de
  los entrenos registrados.
- Funciona sin entrenos (estado vacío) y con pocos datos.
- Sin dependencias nuevas; sin cambios de backend.
- Suite web en verde + build; verificación visual en producción OK.
