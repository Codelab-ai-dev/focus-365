# Plan 7 — Dashboard (Centro de mando) — Diseño

**Fecha:** 2026-06-11
**Rebanada:** 7 de 8 del roadmap (`docs/superpowers/specs/2026-06-09-focus-365-design.md` §7).
**Dimensión:** Dashboard que consume datos reales de las 5 dimensiones.

## Objetivo

Un "centro de mando" en `/` que reúne de un vistazo el estado del día: racha,
superávit del ciclo, ánimo/energía, check-in, entreno de hoy y metas activas.
Más una **barra superior persistente** para navegar entre módulos. Los datos
llegan en un único snapshot agregado por el backend, dejando además servido el
contexto que la rebanada 8 (asistente IA) necesita.

## Decisiones de diseño (locked)

- **Agregación:** endpoint backend `GET /api/v1/dashboard` que arma un snapshot
  único componiendo los 5 servicios existentes (Enfoque B). No hay tablas ni
  queries sqlc nuevas.
- **Navegación:** top bar persistente (app shell) + dashboard de tarjetas
  (Enfoque A, alineado con §6 del diseño macro).
- **Layout:** "Destacados + grid" (Layout B) — banda de IA full-width, saludo,
  2 tarjetas grandes (Racha + Superávit) y debajo 4 tarjetas chicas.
- **Tarjetas:** solo resumen + link a su módulo (read-only, Enfoque A). Sin
  acciones rápidas (YAGNI).
- **Banda IA:** placeholder estático ("Tu insight del día llega pronto"); se
  activa en la rebanada 8.

## Sección 1 — Backend: endpoint agregador

Nuevo paquete `api/internal/dashboard` (`types.go`, `service.go`, `handler.go`
+ tests). Sin migración ni queries sqlc: **compone los 5 servicios existentes**
en la capa de servicio, reusando la lógica de dominio (rachas, overdue,
superávit, ciclo de pago).

### Endpoint

`GET /api/v1/dashboard?today=YYYY-MM-DD`

- Bajo `RequireAuth`, montado en `/api/v1/dashboard`, scopeado por `user_id`.
- `today` opcional (zona del cliente); fallback a UTC midnight (mismo patrón
  `parseTodayParam` de metas/hábitos).
- Devuelve `200` con el snapshot. `401` sin token (middleware). `500` si
  cualquier sub-llamada falla (snapshot todo-o-nada; datos parciales engañan).

### Servicio

`dashboard.Service` recibe punteros a los servicios existentes, inyectados en
`server.go`:

```go
type Service struct {
    checkins  *checkin.Service
    finance   *finance.Service
    training  *training.Service
    habits    *habits.Service
    goals     *goals.Service
}

func NewService(c *checkin.Service, f *finance.Service, t *training.Service,
    h *habits.Service, g *goals.Service) *Service
```

Método `Snapshot(ctx, userID uuid.UUID, today time.Time) (*Snapshot, error)`
llama a cada servicio y arma la vista:

- `habits.List(ctx, userID, false, today)` → racha.
- `finance.Summary(ctx, userID, finance.Cycle(today), today)` → superávit del
  ciclo actual (`finance.Cycle` deriva el ciclo de pago vigente a partir de
  `today`; el servicio no lo deriva solo).
- `checkins.Today(ctx, userID, today)` → check-in del día (nil si no hay).
- `training.ListWorkouts(ctx, userID, &today, &today)` → entreno de hoy.
- `goals.List(ctx, userID, "active", today)` → metas activas.

Si cualquiera devuelve error, `Snapshot` propaga el error → 500 en el handler.

### Vista JSON (`Snapshot`)

```json
{
  "streak":   { "best_current": 12, "done_today": 2, "total": 4 },
  "finance":  { "cycle": "2026-06", "net": 320000, "status": "verde" },
  "checkin":  { "present": true, "mood": 8, "energy": 6, "discipline": 9 },
  "training": { "trained_today": true, "type": "Fuerza" },
  "goals":    { "active": 3, "avg_progress": 40, "overdue": 1 },
  "dimensions_active": 4
}
```

Tipos en `api/internal/dashboard/types.go`:

- `StreakView { BestCurrent int32; DoneToday int; Total int }` —
  `best_current` = mayor `current_streak` entre hábitos activos; `done_today` =
  cuántos activos están marcados hoy; `total` = nº de hábitos activos.
- `FinanceView { Cycle string; Net int64; Status string }` — directo del
  `CycleSummary`. `net` en centavos.
- `CheckinView { Present bool; Mood, Energy, Discipline int32 }` — el campo
  `checkin` serializa **`null`** cuando no hay check-in hoy (`*CheckinView`).
- `TrainingView { TrainedToday bool; Type string }` — `type` vacío si no entrenó
  hoy. Si hay varios workouts hoy, toma el primero que devuelve `ListWorkouts`.
- `GoalsView { Active int; AvgProgress int; Overdue int }` — `avg_progress` =
  promedio entero de `progress` de las activas (0 si no hay); `overdue` =
  cuántas activas tienen `overdue=true`.
- `Snapshot { Streak StreakView; Finance FinanceView; Checkin *CheckinView;
  Training TrainingView; Goals GoalsView; DimensionsActive int }`.

### `dimensions_active` (calculado)

Cuántas de las 5 dimensiones tienen algo que mostrar hoy. Cuenta `true` de:

- **checkin:** hay check-in hoy (`Checkin != nil`).
- **habits:** al menos 1 hábito activo (`Total > 0`).
- **finance:** el ciclo tiene movimiento (`Status != "pendiente"`).
- **training:** entrenó hoy (`TrainedToday`).
- **goals:** al menos 1 meta activa (`Active > 0`).

Helper puro `countActive(s *Snapshot) int` para testear sin DB.

### Montaje

En `server.go`, dentro del grupo `RequireAuth`:

```go
dashboardSvc := dashboard.NewService(checkinSvc, financeSvc, trainingSvc, habitsSvc, goalsSvc)
r.Mount("/dashboard", dashboard.Routes(dashboardSvc))
```

## Sección 2 — Frontend: app shell + ruta del dashboard

### `web/src/components/TopBar.tsx` (nuevo)

Barra superior persistente. Marca "Focus 365", links a Inicio · Check-in ·
Finanzas · Entreno · Disciplina · Metas, y botón "Salir" (`logout`). Sólo se
renderiza cuando hay usuario (`useAuth`); en `/login` y `/register`, `user` es
null → no se muestra. Es **aditivo**: las páginas de módulo no cambian (su
"Volver" interno queda redundante pero no se toca). Usa `<Link>` de TanStack y
resalta el activo con `useRouterState` (clase ámbar en el link actual).

### `__root.tsx`

Renderiza `<TopBar />` antes del `<Outlet />`, dentro del contenedor existente.

### `web/src/lib/dashboard.ts` (nuevo)

- Tipos `Snapshot`, `StreakView`, `FinanceView`, `CheckinView`, `TrainingView`,
  `GoalsView` (espejo del JSON; `checkin: CheckinView | null`).
- `todayString(date?)` (mismo patrón que las otras libs).
- `getDashboard(): Promise<Snapshot>` → `apiFetch('/api/v1/dashboard?today=...')`.

### `web/src/routes/index.tsx`

El home pasa de lista de links a **dashboard** (Layout B). Una sola query
TanStack `['dashboard', todayString()]` → `getDashboard()`. Un único estado de
carga ("Cargando tu día…") y un único error con reintento para toda la vista.
El header propio del home desaparece (la TopBar lleva marca + Salir).

`routeTree.gen.ts` se regenera con `npx vite build` antes de `npm run build`.

## Sección 3 — Tarjetas (Layout B)

Todo se arma desde el `Snapshot`. De arriba a abajo:

1. **Banda IA (placeholder)** — full-width, fondo ámbar tenue + ✦. Texto fijo
   "Tu insight del día llega pronto". No interactivo (rebanada 8).
2. **Saludo** — "Hola, {user.name}" · fecha legible (`es-MX`) ·
   "{dimensions_active} dimensiones en marcha".
3. **Racha** (grande) — `best_current` días en naranja `streak` +
   "{done_today}/{total} hábitos hoy". Vacío (`total=0`): "Sin hábitos aún".
   → `/disciplina`.
4. **Superávit del ciclo** (grande) — `net` formateado MXN con signo + chip de
   `status` (verde / rojo / `sand` si pendiente) + ciclo legible. → `/finanzas`.
5. **Ánimo/Energía** — barras de `mood` y `energy` (1-10). `checkin` null:
   "Sin check-in hoy". → `/check-in`.
6. **Check-in de hoy** — "Hecho ✓" (si `present`) o "Pendiente" + disciplina si
   hay. → `/check-in`.
7. **Entreno de hoy** — `trained_today` ? "{type} ✓" : "Sin entreno hoy".
   → `/entrenamiento`.
8. **Metas activas** — "{active} activas · {avg_progress}% prom." y si
   `overdue>0` aviso rojo "{overdue} vencida(s)". → `/metas`.

**Estados:** loading único y error único (con reintento) para toda la vista.
Cada tarjeta resuelve su caso vacío con el texto de arriba.

**Colores:** ámbar (IA), naranja `streak` (racha), verde `money` / rojo (plata y
vencidas), `sand` secundario. Sin colores nuevos en `tailwind.config.js`.

## Sección 4 — Testing y criterios de aceptación

### Backend — unit (dominio puro)
`countActive`: 0 datos → 0; las 5 presentes → 5; subconjuntos representativos.
Mapeo del snapshot: `checkin` null cuando no hay; `training.type` vacío sin
entreno; `avg_progress` con/ sin metas.

### Backend — integración (`handler_test.go`, `testutil.NewDB`)
1. `TestEmptyDashboard` — usuario sin datos → todo en cero, `checkin` null,
   `dimensions_active=0`.
2. `TestPopulatedDashboard` — sembrar hábito marcado hoy + transacción +
   check-in + workout hoy + meta activa → verifica cada sección y
   `dimensions_active=5`.
3. `TestOverdueGoalsCounted` — meta activa con deadline pasado y `?today=`
   posterior → `goals.overdue=1`.
4. `TestRequiresAuth` — sin token → 401.
5. `TestUserIsolation` — el dashboard de B no refleja datos de A.

### Frontend — Vitest
- `dashboard.test.ts`: `getDashboard` arma URL con `?today=` y método correcto.
- `index.test.tsx`: con snapshot mock renderiza racha, superávit en MXN, saludo
  con `dimensions_active`, casos vacíos ("Sin check-in hoy"), aviso de vencidas;
  estado de carga; cada tarjeta linkea a su ruta.
- `TopBar.test.tsx`: muestra los links con usuario; nada sin usuario.

### E2E docker smoke (bash, como Plan 6)
Levantar stack, registrar usuario, sembrar datos vía endpoints (check-in,
hábito+check, transacción, workout, meta), `GET /dashboard`, verificar campos
del snapshot y aislamiento entre usuarios → `SMOKE OK`.

### Criterios de aceptación
- `GET /dashboard` agrega las 5 dimensiones, scopeado por usuario, con
  `dimensions_active` correcto.
- TopBar persistente en páginas autenticadas; oculta sin usuario.
- Dashboard Layout B con las 6 tarjetas + banda IA placeholder + saludo.
- Estados de carga/error/vacío correctos.
- `make check` verde + frontend Vitest verde + smoke `SMOKE OK`.
