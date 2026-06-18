# Bitácora de sesión — Rebanada 28: evolución/progreso (entrenamiento slice D)

**Fecha:** 2026-06-17
**Estado al cierre:** Completada, mergeada a `main` y pusheada. **Cierra la expansión de entrenamiento (A→D).** Verificación en producción: visual tras el deploy (slice solo-frontend).
**Rama:** `plan-28-progreso-entrenamiento` (mezclada `--no-ff` y borrada).

## Contexto

Última pieza de la expansión de entrenamiento. Sub-proyectos completos:
A perfil (R24), B sugerencias (R25), C1 notas por serie (R26), C2 ajustes del
agente (R27), **D evolución/progreso** (este).

## Qué se entregó (D)

Una sección **"Progreso"** en `/entrenamiento` (debajo del historial) con cuatro
vistas, calculadas **en el frontend** desde el historial de entrenos (sin
backend, sin endpoints, sin dependencias nuevas):

- **Volumen por semana** — barras de Σ reps×peso (kg) por semana.
- **Frecuencia por semana** — barras de sesiones por semana.
- **Progresión por ejercicio** — selector + barras del peso máximo por sesión.
- **Records (PRs)** — lista del mejor peso por ejercicio.

## Arquitectura

- **`lib/trainingProgress.ts`** (lógica pura, el núcleo): `weeklyVolume`,
  `weeklyFrequency`, `exerciseNames`, `exerciseProgression`, `personalRecords`.
  Agrupa por semana (lunes vía `mondayOf`), parsea fechas en local (sin UTC),
  calcula máximos. Reutiliza `gramsToKg`.
- **`ui/BarChart.tsx`** (nuevo, reutilizable): barras SVG a mano (relleno
  `accent`, borde `ink`), alto ∝ máximo de la serie, `role="img"`+aria, estado
  "sin datos". Sin librería de gráficos.
- **`entrenamiento.tsx`:** sección "Progreso" que reutiliza la query del
  historial (`historyQuery`), arma las series con `useMemo` y renderiza los
  charts + el `<select>` de ejercicio + la lista de PRs. Estado vacío.

## Commits

`0f72179` agregaciones puras · `5880704` BarChart · `f798167` sección Progreso ·
merge.

## Decisiones / hallazgos

- **Frontend-only:** todo se computa de los datos que ya carga la página; cero
  backend, cero endpoints, cero dependencias. La lógica pesada vive en funciones
  puras muy testeables (TDD).
- **SVG a mano** en vez de una lib de gráficos — encaja con el design system
  neo-brutalista y evita una dependencia. Los colores `fill-accent`/`stroke-ink`
  resuelven contra las CSS vars del tema.
- **Colisión de test:** la nueva lista de PRs (`Sentadilla / 80 kg`) chocaba con
  un matcher del test de Historial (`startsWith("Sentadilla")`); se ajustó el
  matcher viejo para exigir también "reps" (solo lo tiene el item del historial),
  sin debilitar la cobertura.
- **Review final (Opus): APPROVED.** Verificó a mano las cinco agregaciones
  (incluyendo el límite de semana en DST y el manejo de nulls). Nits cosméticos:
  `ChartPoint` duplicado en lib y BarChart (estructuralmente idéntico),
  `BarChart` de volumen sin `unit`.
- **Lección de proceso:** 6.ª rebanada con las tasks de lib/ui corriendo
  `npm run build` — sin filtrar typecheck.

## Verificación al cierre

- Frontend: **168/168** + build OK. (Backend sin cambios.)
- Verificación en producción: visual tras el deploy manual (ver la sección
  "Progreso" con los gráficos y el selector en `/entrenamiento`).

## Estado del módulo de entrenamiento

**Completo (A→D):** perfil de fitness, agente de sugerencias, notas por serie,
ajustes del agente, y evolución/progreso. Todo lo pedido para entrenamiento está
entregado.

## Backlog restante (general)

Backups de Postgres en el VPS; OCR de PDFs escaneados; recordatorios/
notificaciones de compromisos.
