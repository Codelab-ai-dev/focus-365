# Plan 3 — Finanzas · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Añadir el módulo de finanzas personales de Focus 365 (transacciones scoped por usuario, superávit por ciclo de pago, página `/finanzas` e importador CLI desde `money.quhou123.com`).

**Architecture:** Espejo del módulo `checkin` (Plan 2): migración goose → queries sqlc → paquete de dominio `finance` (lógica pura del ciclo + servicio + handlers chi protegidos con `RequireAuth`) → `lib/finances.ts` + página React con TanStack Query. El **ciclo de pago** se calcula en Go (funciones puras testeables) y se persiste en la columna `cycle` de cada transacción. Un comando `cmd/import` trae el histórico externo y lo inserta idempotentemente por `external_id`.

**Tech Stack:** Go 1.23 (chi v5, jackc/pgx/v5, sqlc v1.31.1, goose, go-playground/validator/v10, google/uuid), React 18 + Vite + TanStack Router/Query + Tailwind + Vitest, PostgreSQL 16.

**Base branch:** Este plan depende del paquete `httpx` y de los patrones introducidos en **Plan 2 (Check-in)**, que aún no está mergeado. Crear la rama de trabajo `feat/plan-3-finanzas` **a partir de `feat/plan-2-checkin`** (no de `master`).

**Convenciones del proyecto (IMPORTANTES):**
- Todos los comandos `go` se ejecutan con `GOTOOLCHAIN=local` y desde el directorio `api/`. **Nunca** editar `go.mod`/`go.sum` a mano.
- Tests de BD requieren `TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable` (Postgres dockerizado en el puerto host 5544). Sin esa variable, los tests de store/handler hacen `t.Skip`.
- Comentarios de código y mensajes de commit **en español**.
- Para Docker, anteponer los binarios del sistema y **anexar** los helpers: `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"`.

---

### Task 1: Migración `transactions`

**Files:**
- Create: `api/db/migrations/0003_transactions.sql`

- [ ] **Step 1: Escribir la migración**

`api/db/migrations/0003_transactions.sql`:

```sql
-- +goose Up
CREATE TABLE transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL CHECK (type IN ('income','expense','transfer')),
    amount      BIGINT NOT NULL CHECK (amount >= 0),  -- centavos (MXN)
    occurred_on DATE NOT NULL,
    cycle       DATE NOT NULL,            -- primer día del mes financiero
    category    TEXT NOT NULL DEFAULT '',
    remark      TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','import')),
    external_id TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_transactions_user_cycle
    ON transactions (user_id, cycle DESC, occurred_on DESC);
CREATE UNIQUE INDEX uq_transactions_user_external
    ON transactions (user_id, external_id) WHERE external_id IS NOT NULL;

-- +goose Down
DROP TABLE transactions;
```

- [ ] **Step 2: Aplicar la migración contra la BD de test**

Run (desde `api/`):
```bash
cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```
Expected: log `migrations applied` (o que la versión avanza a 3).

- [ ] **Step 3: Verificar el esquema**

Run:
```bash
PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" docker compose exec -T db psql -U focus -d focus365 -c "\d transactions"
```
Expected: tabla `transactions` con las columnas y los dos índices (`idx_transactions_user_cycle`, `uq_transactions_user_external`).

- [ ] **Step 4: Commit**

```bash
git add api/db/migrations/0003_transactions.sql
git commit -m "feat(finanzas): migración de la tabla transactions"
```

---

### Task 2: Queries sqlc de `transactions`

**Files:**
- Create: `api/db/queries/transactions.sql`
- Generate (sqlc): `api/internal/store/transactions.sql.go`, `api/internal/store/models.go` (modifica)

- [ ] **Step 1: Escribir las queries**

`api/db/queries/transactions.sql`:

```sql
-- name: CreateTransaction :one
INSERT INTO transactions (user_id, type, amount, occurred_on, cycle, category, remark)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListTransactionsByCycle :many
SELECT * FROM transactions
WHERE user_id = $1 AND cycle = $2
ORDER BY occurred_on DESC, created_at DESC;

-- name: DeleteTransaction :execrows
DELETE FROM transactions
WHERE id = $1 AND user_id = $2;

-- name: SummarizeCycle :one
SELECT
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::bigint  AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::bigint AS expense
FROM transactions
WHERE user_id = $1 AND cycle = $2;

-- name: SummarizeCycles :many
SELECT
    cycle,
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::bigint  AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::bigint AS expense
FROM transactions
WHERE user_id = $1
GROUP BY cycle
ORDER BY cycle DESC;

-- name: UpsertImportedTransaction :one
INSERT INTO transactions (user_id, type, amount, occurred_on, cycle, category, remark, source, external_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'import', $8)
ON CONFLICT (user_id, external_id) WHERE external_id IS NOT NULL
DO UPDATE SET
    type = EXCLUDED.type,
    amount = EXCLUDED.amount,
    occurred_on = EXCLUDED.occurred_on,
    cycle = EXCLUDED.cycle,
    category = EXCLUDED.category,
    remark = EXCLUDED.remark,
    updated_at = now()
RETURNING *;
```

- [ ] **Step 2: Generar el código sqlc**

Run (desde `api/`):
```bash
cd api && sqlc generate
```
Expected: sin errores; se crea `internal/store/transactions.sql.go` y se añade el tipo `Transaction` a `internal/store/models.go`.

- [ ] **Step 3: Verificar que compila y que las firmas son las esperadas**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go build ./...
```
Expected: compila. Las identidades generadas que usarán las tareas siguientes:
- `store.Transaction{ID, UserID, Type, Amount int64, OccurredOn, Cycle time.Time, Category, Remark, Source string, ExternalID *string, CreatedAt, UpdatedAt time.Time}`
- `q.CreateTransaction(ctx, store.CreateTransactionParams{UserID, Type, Amount, OccurredOn, Cycle, Category, Remark}) (store.Transaction, error)`
- `q.ListTransactionsByCycle(ctx, store.ListTransactionsByCycleParams{UserID, Cycle}) ([]store.Transaction, error)`
- `q.DeleteTransaction(ctx, store.DeleteTransactionParams{ID, UserID}) (int64, error)`
- `q.SummarizeCycle(ctx, store.SummarizeCycleParams{UserID, Cycle}) (store.SummarizeCycleRow{Income, Expense int64}, error)`
- `q.SummarizeCycles(ctx, userID uuid.UUID) ([]store.SummarizeCyclesRow{Cycle time.Time, Income, Expense int64}, error)`
- `q.UpsertImportedTransaction(ctx, store.UpsertImportedTransactionParams{UserID, Type, Amount, OccurredOn, Cycle, Category, Remark, ExternalID *string}) (store.Transaction, error)`

- [ ] **Step 4: Commit**

```bash
git add api/db/queries/transactions.sql api/internal/store/transactions.sql.go api/internal/store/models.go
git commit -m "feat(finanzas): queries sqlc de transactions"
```

---

### Task 3: Lógica del ciclo de pago (`finance/cycle.go`)

**Files:**
- Create: `api/internal/finance/cycle.go`
- Test: `api/internal/finance/cycle_test.go`

- [ ] **Step 1: Escribir el test que falla**

`api/internal/finance/cycle_test.go`:

```go
package finance

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestPaydayAjustaFinDeSemana(t *testing.T) {
	// 31 may 2026 cae domingo → retrocede al viernes 29.
	if got := payday(2026, time.May); !got.Equal(d(2026, time.May, 29)) {
		t.Errorf("payday(may) = %v, want 2026-05-29", got)
	}
	// 28 feb 2026 cae sábado → retrocede al viernes 27.
	if got := payday(2026, time.February); !got.Equal(d(2026, time.February, 27)) {
		t.Errorf("payday(feb) = %v, want 2026-02-27", got)
	}
	// 30 jun 2026 cae martes → sin ajuste.
	if got := payday(2026, time.June); !got.Equal(d(2026, time.June, 30)) {
		t.Errorf("payday(jun) = %v, want 2026-06-30", got)
	}
}

func TestCycle(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"justo el payday va al mes siguiente", d(2026, time.May, 29), d(2026, time.June, 1)},
		{"un día antes del payday queda en el mes", d(2026, time.May, 28), d(2026, time.May, 1)},
		{"mediados de mes queda en el mes", d(2026, time.June, 10), d(2026, time.June, 1)},
		{"el payday de junio va a julio", d(2026, time.June, 30), d(2026, time.July, 1)},
		{"cruce de año diciembre→enero", d(2026, time.December, 31), d(2027, time.January, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Cycle(c.in); !got.Equal(c.want) {
				t.Errorf("Cycle(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParseFormatCycle(t *testing.T) {
	c, err := ParseCycle("2026-06")
	if err != nil {
		t.Fatalf("ParseCycle: %v", err)
	}
	if !c.Equal(d(2026, time.June, 1)) {
		t.Errorf("ParseCycle = %v, want 2026-06-01", c)
	}
	if got := FormatCycle(d(2026, time.June, 1)); got != "2026-06" {
		t.Errorf("FormatCycle = %q, want 2026-06", got)
	}
}
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/finance/ -run 'TestPayday|TestCycle|TestParseFormatCycle' -v
```
Expected: FALLA con error de compilación (`undefined: payday`, `Cycle`, `ParseCycle`, `FormatCycle`).

- [ ] **Step 3: Implementar `cycle.go`**

`api/internal/finance/cycle.go`:

```go
// Package finance implementa el dominio de finanzas: transacciones scoped por
// usuario y la lógica del ciclo de pago (mes financiero que arranca el día de
// pago, el último día hábil del mes).
package finance

import "time"

// dateLayout es el formato de fecha que viaja por la API (YYYY-MM-DD).
const dateLayout = "2006-01-02"

// monthLayout es el formato de un ciclo (YYYY-MM).
const monthLayout = "2006-01"

// payday devuelve el último día hábil del mes dado. Si el último día del mes
// cae en sábado retrocede un día (al viernes); si cae en domingo, dos.
// Solo ajusta fines de semana; no contempla feriados.
func payday(year int, month time.Month) time.Time {
	// Primer día del mes siguiente menos un día = último día del mes.
	last := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1)
	switch last.Weekday() {
	case time.Saturday:
		last = last.AddDate(0, 0, -1)
	case time.Sunday:
		last = last.AddDate(0, 0, -2)
	}
	return last
}

// financialMonth devuelve (año, mes) del mes financiero al que pertenece date:
// si date es en o después del día de pago de su mes, pertenece al mes siguiente.
func financialMonth(date time.Time) (int, time.Month) {
	y, m := date.Year(), date.Month()
	if !date.Before(payday(y, m)) { // date >= payday
		next := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		return next.Year(), next.Month()
	}
	return y, m
}

// Cycle devuelve el primer día del mes financiero de date (la columna cycle).
func Cycle(date time.Time) time.Time {
	y, m := financialMonth(date)
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// ParseCycle interpreta un ciclo "YYYY-MM" como el primer día de ese mes.
func ParseCycle(s string) (time.Time, error) {
	return time.Parse(monthLayout, s)
}

// FormatCycle serializa un ciclo como "YYYY-MM".
func FormatCycle(t time.Time) string {
	return t.Format(monthLayout)
}
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/finance/ -run 'TestPayday|TestCycle|TestParseFormatCycle' -v
```
Expected: PASA (todos los sub-tests OK).

- [ ] **Step 5: Commit**

```bash
git add api/internal/finance/cycle.go api/internal/finance/cycle_test.go
git commit -m "feat(finanzas): lógica pura del ciclo de pago"
```

---

### Task 4: Servicio de dominio (`finance/service.go`)

**Files:**
- Create: `api/internal/finance/service.go`
- Test: `api/internal/finance/service_test.go`

- [ ] **Step 1: Escribir el test que falla**

`api/internal/finance/service_test.go`:

```go
package finance_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestServiceFinanzas(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := finance.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "f@b.com", PasswordHash: "h", Name: "Fin"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	now := d(2026, time.June, 15) // ciclo actual = junio (payday jun = 30)
	junio := finance.Cycle(now)

	// Crea ingreso, gasto y transferencia en el ciclo de junio.
	in1, err := svc.Create(ctx, user.ID, finance.Input{Type: "income", Amount: 500000, OccurredOn: d(2026, time.June, 10), Category: "sueldo"})
	if err != nil {
		t.Fatalf("Create income: %v", err)
	}
	if in1.Cycle != "2026-06" {
		t.Errorf("cycle = %q, want 2026-06", in1.Cycle)
	}
	if _, err := svc.Create(ctx, user.ID, finance.Input{Type: "expense", Amount: 200000, OccurredOn: d(2026, time.June, 12), Category: "renta"}); err != nil {
		t.Fatalf("Create expense: %v", err)
	}
	transfer, err := svc.Create(ctx, user.ID, finance.Input{Type: "transfer", Amount: 100000, OccurredOn: d(2026, time.June, 11)})
	if err != nil {
		t.Fatalf("Create transfer: %v", err)
	}

	// ListByCycle: 3 transacciones, descendente por occurred_on.
	list, err := svc.ListByCycle(ctx, user.ID, junio)
	if err != nil {
		t.Fatalf("ListByCycle: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	if list[0].OccurredOn != "2026-06-12" {
		t.Errorf("orden incorrecto: list[0] = %q, want 2026-06-12", list[0].OccurredOn)
	}

	// Summary del ciclo actual: net excluye la transferencia, status pendiente.
	sum, err := svc.Summary(ctx, user.ID, junio, now)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.Income != 500000 || sum.Expense != 200000 || sum.Net != 300000 {
		t.Errorf("summary = %+v, want income 500000 expense 200000 net 300000", sum)
	}
	if sum.Status != "pendiente" {
		t.Errorf("status = %q, want pendiente", sum.Status)
	}

	// Un gasto en un ciclo pasado (abril) deja ese ciclo cerrado en rojo.
	if _, err := svc.Create(ctx, user.ID, finance.Input{Type: "expense", Amount: 999999, OccurredOn: d(2026, time.April, 10)}); err != nil {
		t.Fatalf("Create abril: %v", err)
	}
	abril := finance.Cycle(d(2026, time.April, 10))
	sumAbr, err := svc.Summary(ctx, user.ID, abril, now)
	if err != nil {
		t.Fatalf("Summary abril: %v", err)
	}
	if sumAbr.Net != -999999 || sumAbr.Status != "rojo" {
		t.Errorf("summary abril = %+v, want net -999999 status rojo", sumAbr)
	}

	// Cycles: historial descendente con su status.
	cycles, err := svc.Cycles(ctx, user.ID, now)
	if err != nil {
		t.Fatalf("Cycles: %v", err)
	}
	if len(cycles) != 2 {
		t.Fatalf("len(cycles) = %d, want 2", len(cycles))
	}
	if cycles[0].Cycle != "2026-06" || cycles[0].Status != "pendiente" {
		t.Errorf("cycles[0] = %+v, want 2026-06 pendiente", cycles[0])
	}
	if cycles[1].Cycle != "2026-04" || cycles[1].Status != "rojo" {
		t.Errorf("cycles[1] = %+v, want 2026-04 rojo", cycles[1])
	}

	// Delete: borra la transferencia; borrar de nuevo no afecta filas.
	ok, err := svc.Delete(ctx, user.ID, mustUUID(t, transfer.ID))
	if err != nil || !ok {
		t.Fatalf("Delete: ok=%v err=%v", ok, err)
	}
	ok2, err := svc.Delete(ctx, user.ID, mustUUID(t, transfer.ID))
	if err != nil {
		t.Fatalf("Delete repetido: %v", err)
	}
	if ok2 {
		t.Errorf("segundo Delete devolvió true; debería ser false")
	}

	// Aislamiento: otro usuario no puede borrar la transacción de Fin.
	other, _ := q.CreateUser(ctx, store.CreateUserParams{Email: "o@b.com", PasswordHash: "h", Name: "Otro"})
	okOther, err := svc.Delete(ctx, other.ID, mustUUID(t, in1.ID))
	if err != nil {
		t.Fatalf("Delete ajeno: %v", err)
	}
	if okOther {
		t.Errorf("otro usuario borró una transacción ajena")
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", s, err)
	}
	return id
}
```

Nota: añadir el import `"github.com/google/uuid"` al bloque de imports del test.

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/finance/ -run TestServiceFinanzas -v
```
Expected: FALLA con error de compilación (`undefined: finance.NewService`, `finance.Input`, etc.).

- [ ] **Step 3: Implementar `service.go`**

`api/internal/finance/service.go`:

```go
package finance

import (
	"context"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// Input son los datos de dominio para crear una transacción manual.
type Input struct {
	Type       string
	Amount     int64 // centavos
	OccurredOn time.Time
	Category   string
	Remark     string
}

// Transaction es la vista de dominio que se serializa a JSON. occurred_on va
// como YYYY-MM-DD y cycle como YYYY-MM.
type Transaction struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Amount     int64     `json:"amount"`
	OccurredOn string    `json:"occurred_on"`
	Cycle      string    `json:"cycle"`
	Category   string    `json:"category"`
	Remark     string    `json:"remark"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CycleSummary resume un ciclo: totales en centavos, net (ingresos − gastos,
// sin transferencias) y status (pendiente | verde | rojo).
type CycleSummary struct {
	Cycle   string `json:"cycle"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
	Net     int64  `json:"net"`
	Status  string `json:"status"`
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in Input) (*Transaction, error) {
	row, err := s.q.CreateTransaction(ctx, store.CreateTransactionParams{
		UserID:     userID,
		Type:       in.Type,
		Amount:     in.Amount,
		OccurredOn: in.OccurredOn,
		Cycle:      Cycle(in.OccurredOn),
		Category:   in.Category,
		Remark:     in.Remark,
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

func (s *Service) ListByCycle(ctx context.Context, userID uuid.UUID, cycle time.Time) ([]Transaction, error) {
	rows, err := s.q.ListTransactionsByCycle(ctx, store.ListTransactionsByCycleParams{UserID: userID, Cycle: cycle})
	if err != nil {
		return nil, err
	}
	out := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		out = append(out, toView(row))
	}
	return out, nil
}

// Delete borra la transacción si pertenece al usuario; devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteTransaction(ctx, store.DeleteTransactionParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) Summary(ctx context.Context, userID uuid.UUID, cycle, now time.Time) (*CycleSummary, error) {
	row, err := s.q.SummarizeCycle(ctx, store.SummarizeCycleParams{UserID: userID, Cycle: cycle})
	if err != nil {
		return nil, err
	}
	sum := CycleSummary{
		Cycle:   FormatCycle(cycle),
		Income:  row.Income,
		Expense: row.Expense,
		Net:     row.Income - row.Expense,
	}
	sum.Status = statusFor(cycle, Cycle(now), sum.Net)
	return &sum, nil
}

func (s *Service) Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]CycleSummary, error) {
	rows, err := s.q.SummarizeCycles(ctx, userID)
	if err != nil {
		return nil, err
	}
	current := Cycle(now)
	out := make([]CycleSummary, 0, len(rows))
	for _, row := range rows {
		net := row.Income - row.Expense
		out = append(out, CycleSummary{
			Cycle:   FormatCycle(row.Cycle),
			Income:  row.Income,
			Expense: row.Expense,
			Net:     net,
			Status:  statusFor(row.Cycle, current, net),
		})
	}
	return out, nil
}

// statusFor decide el estado de un ciclo: si es el actual o futuro está
// "pendiente"; si ya cerró, "verde" cuando hubo superávit y "rojo" si no.
func statusFor(cycle, current time.Time, net int64) string {
	if !cycle.Before(current) { // cycle >= current → en curso
		return "pendiente"
	}
	if net >= 0 {
		return "verde"
	}
	return "rojo"
}

func toView(row store.Transaction) Transaction {
	return Transaction{
		ID:         row.ID.String(),
		Type:       row.Type,
		Amount:     row.Amount,
		OccurredOn: row.OccurredOn.Format(dateLayout),
		Cycle:      FormatCycle(row.Cycle),
		Category:   row.Category,
		Remark:     row.Remark,
		Source:     row.Source,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `api/`, con `TEST_DATABASE_URL` definida):
```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/finance/ -run TestServiceFinanzas -v
```
Expected: PASA.

- [ ] **Step 5: Commit**

```bash
git add api/internal/finance/service.go api/internal/finance/service_test.go
git commit -m "feat(finanzas): servicio de dominio (crear, listar, borrar, resumen, ciclos)"
```

---

### Task 5: Etiquetas de campo de finanzas en `httpx`

**Files:**
- Modify: `api/internal/httpx/httpx.go` (función `fieldLabel`)
- Test: `api/internal/httpx/httpx_test.go`

- [ ] **Step 1: Escribir el test que falla**

Añadir al final de `api/internal/httpx/httpx_test.go`:

```go
type txTipo struct {
	Type string `validate:"required,oneof=income expense transfer"`
}

type txMonto struct {
	Amount int64 `validate:"required,min=1"`
}

func TestValidationMessageFinanzas(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"tipo inválido", txTipo{Type: "bogus"}, "El tipo no es válido"},
		{"monto faltante", txMonto{Amount: 0}, "Falta el monto"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validate.Struct(c.in)
			if err == nil {
				t.Fatal("se esperaba un error de validación")
			}
			if got := ValidationMessage(err); got != c.want {
				t.Errorf("ValidationMessage = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/httpx/ -run TestValidationMessageFinanzas -v
```
Expected: FALLA en "monto faltante" (devuelve `"Falta Amount"` en vez de `"Falta el monto"`) — la etiqueta del campo aún no existe.

- [ ] **Step 3: Añadir las etiquetas de finanzas**

En `api/internal/httpx/httpx.go`, dentro del `switch field` de `fieldLabel`, añadir estos casos antes de `default:`:

```go
	case "Type":
		return "el tipo"
	case "Amount":
		return "el monto"
	case "OccurredOn":
		return "la fecha"
	case "Category":
		return "la categoría"
	case "Remark":
		return "la nota"
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/httpx/ -v
```
Expected: PASA (todos los tests de httpx, incluido el nuevo).

- [ ] **Step 5: Commit**

```bash
git add api/internal/httpx/httpx.go api/internal/httpx/httpx_test.go
git commit -m "feat(finanzas): etiquetas de validación para los campos de transacción"
```

---

### Task 6: Handlers HTTP (`finance/handler.go`)

**Files:**
- Create: `api/internal/finance/handler.go`
- Test: `api/internal/finance/handler_test.go`

- [ ] **Step 1: Escribir el test que falla**

`api/internal/finance/handler_test.go`:

```go
package finance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type env struct {
	h    http.Handler
	auth *auth.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/finances", finance.Routes(finance.NewService(q)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}

func (e *env) token(t *testing.T, email string) string {
	t.Helper()
	user, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(user.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return access
}

func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateListSummary(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "income", "amount": 500000, "occurred_on": "2026-06-10", "category": "sueldo",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var tx map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tx)
	if tx["cycle"] != "2026-06" {
		t.Errorf("cycle = %v, want 2026-06", tx["cycle"])
	}

	_ = do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "expense", "amount": 200000, "occurred_on": "2026-06-12",
	})

	// List del ciclo actual (today en junio).
	recL := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	// Summary del ciclo actual.
	recS := do(t, e.h, http.MethodGet, "/finances/summary?today=2026-06-15", tok, nil)
	var sum map[string]any
	_ = json.Unmarshal(recS.Body.Bytes(), &sum)
	if sum["net"].(float64) != 300000 {
		t.Errorf("net = %v, want 300000", sum["net"])
	}
	if sum["status"] != "pendiente" {
		t.Errorf("status = %v, want pendiente", sum["status"])
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "del@b.com")
	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "expense", "amount": 1000, "occurred_on": "2026-06-10",
	})
	var tx map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tx)
	id := tx["id"].(string)

	recD := do(t, e.h, http.MethodDelete, "/finances/transactions/"+id, tok, nil)
	if recD.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", recD.Code)
	}
	recD2 := do(t, e.h, http.MethodDelete, "/finances/transactions/"+id, tok, nil)
	if recD2.Code != http.StatusNotFound {
		t.Errorf("segundo DELETE code = %d, want 404", recD2.Code)
	}
}

func TestValidationTipo(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "v@b.com")
	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "bogus", "amount": 1000, "occurred_on": "2026-06-10",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "El tipo no es válido" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	recA := do(t, e.h, http.MethodPost, "/finances/transactions", tokA, map[string]any{
		"type": "income", "amount": 9999, "occurred_on": "2026-06-10",
	})
	var txA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &txA)
	idA := txA["id"].(string)

	// B no ve las transacciones de A.
	recL := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("user B ve %d transacciones de A; debería ver 0", len(list))
	}

	// B no puede borrar la transacción de A.
	recD := do(t, e.h, http.MethodDelete, "/finances/transactions/"+idA, tokB, nil)
	if recD.Code != http.StatusNotFound {
		t.Errorf("B borró la tx de A: code = %d, want 404", recD.Code)
	}
}
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/finance/ -run 'TestCreateListSummary|TestDelete|TestValidationTipo|TestRequiresAuth|TestUserIsolation' -v
```
Expected: FALLA con error de compilación (`undefined: finance.Routes`).

- [ ] **Step 3: Implementar `handler.go`**

`api/internal/finance/handler.go`:

```go
package finance

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createReq struct {
	Type       string `json:"type" validate:"required,oneof=income expense transfer"`
	Amount     int64  `json:"amount" validate:"required,min=1"`
	OccurredOn string `json:"occurred_on" validate:"required"`
	Category   string `json:"category"`
	Remark     string `json:"remark"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/transactions", handleCreate(svc))
	r.Get("/transactions", handleList(svc))
	r.Delete("/transactions/{id}", handleDelete(svc))
	r.Get("/summary", handleSummary(svc))
	r.Get("/cycles", handleCycles(svc))
	return r
}

// nowFrom toma la fecha local del cliente (?today=YYYY-MM-DD) como "hoy"; si no
// viene o es inválida, usa la hora del servidor en UTC.
func nowFrom(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	return time.Now().UTC()
}

// cycleFrom resuelve el ciclo objetivo: el parámetro ?cycle=YYYY-MM o, si falta,
// el ciclo actual derivado de "hoy". Devuelve false si el formato es inválido.
func cycleFrom(r *http.Request, now time.Time) (time.Time, bool) {
	if c := r.URL.Query().Get("cycle"); c != "" {
		parsed, err := ParseCycle(c)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	}
	return Cycle(now), true
}

func handleCreate(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req createReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		occurred, err := time.Parse(dateLayout, req.OccurredOn)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		tx, err := svc.Create(r.Context(), userID, Input{
			Type: req.Type, Amount: req.Amount, OccurredOn: occurred, Category: req.Category, Remark: req.Remark,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, tx)
	}
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		cycle, ok := cycleFrom(r, nowFrom(r))
		if !ok {
			httpx.WriteErr(w, http.StatusBadRequest, "el ciclo no tiene un formato válido (YYYY-MM)")
			return
		}
		list, err := svc.ListByCycle(r.Context(), userID, cycle)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleDelete(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		deleted, err := svc.Delete(r.Context(), userID, id)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if !deleted {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSummary(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		now := nowFrom(r)
		cycle, ok := cycleFrom(r, now)
		if !ok {
			httpx.WriteErr(w, http.StatusBadRequest, "el ciclo no tiene un formato válido (YYYY-MM)")
			return
		}
		sum, err := svc.Summary(r.Context(), userID, cycle, now)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, sum)
	}
}

func handleCycles(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		cycles, err := svc.Cycles(r.Context(), userID, nowFrom(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, cycles)
	}
}
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `api/`, con `TEST_DATABASE_URL`):
```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/finance/ -v
```
Expected: PASA (todos los tests del paquete finance).

- [ ] **Step 5: Commit**

```bash
git add api/internal/finance/handler.go api/internal/finance/handler_test.go
git commit -m "feat(finanzas): handlers HTTP protegidos de transacciones"
```

---

### Task 7: Montar `/finances` en el servidor

**Files:**
- Modify: `api/internal/server/server.go`

- [ ] **Step 1: Importar el paquete finance**

En `api/internal/server/server.go`, añadir el import (ordenado alfabéticamente, después de `checkin`):

```go
	"github.com/focus365/api/internal/finance"
```

- [ ] **Step 2: Instanciar el servicio**

Después de `checkinSvc := checkin.NewService(q)` añadir:

```go
	financeSvc := finance.NewService(q)
```

- [ ] **Step 3: Montar la ruta protegida**

Dentro del `r.Group` que ya usa `auth.RequireAuth(tm)`, junto al mount de `/checkins`, añadir:

```go
			r.Mount("/finances", finance.Routes(financeSvc))
```

El bloque de rutas queda así:

```go
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health)
		r.Mount("/auth", auth.Routes(authSvc))
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(tm))
			r.Mount("/checkins", checkin.Routes(checkinSvc))
			r.Mount("/finances", finance.Routes(financeSvc))
		})
	})
```

- [ ] **Step 4: Verificar build + suite backend completa**

Run (desde `api/`, con `TEST_DATABASE_URL`):
```bash
cd api && GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./...
```
Expected: vet limpio; todos los tests en verde.

- [ ] **Step 5: Commit**

```bash
git add api/internal/server/server.go
git commit -m "feat(finanzas): monta /api/v1/finances bajo RequireAuth"
```

---

### Task 8: Importador (`finance/importer.go` + `cmd/import`)

**Files:**
- Create: `api/internal/finance/importer.go`
- Create: `api/cmd/import/main.go`
- Test: `api/internal/finance/importer_test.go`

> **Nota sobre el mapeo de campos:** la forma exacta de la respuesta de
> `getTransactions` (servicio `money.quhou123.com`) aún no se conoce. El struct
> `externalResponse` refleja una forma **asumida**; cuando Gustavo aporte una
> muestra real solo habrá que ajustar sus tags JSON y, si hace falta, la
> derivación de `Type`/`Amount`/`OccurredOn`. La parte de inserción idempotente
> (`Service.Import`) es estable y queda cubierta por test.

- [ ] **Step 1: Escribir el test de idempotencia que falla**

`api/internal/finance/importer_test.go`:

```go
package finance_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestImportEsIdempotente(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := finance.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "imp@b.com", PasswordHash: "h", Name: "Imp"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	day := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC) // ciclo junio
	txs := []finance.ExternalTx{
		{ExternalID: "ext-1", Type: "expense", Amount: 12345, OccurredOn: day, Category: "luz"},
		{ExternalID: "ext-2", Type: "income", Amount: 500000, OccurredOn: day, Category: "sueldo"},
	}
	n, err := svc.Import(ctx, user.ID, txs)
	if err != nil || n != 2 {
		t.Fatalf("Import: n=%d err=%v", n, err)
	}

	// Reimportar ext-1 con monto distinto: actualiza, no duplica.
	n2, err := svc.Import(ctx, user.ID, []finance.ExternalTx{
		{ExternalID: "ext-1", Type: "expense", Amount: 999, OccurredOn: day, Category: "luz"},
	})
	if err != nil || n2 != 1 {
		t.Fatalf("Reimport: n=%d err=%v", n2, err)
	}

	junio := finance.Cycle(day)
	list, err := svc.ListByCycle(ctx, user.ID, junio)
	if err != nil {
		t.Fatalf("ListByCycle: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2 (sin duplicar)", len(list))
	}
	var updated bool
	for _, tx := range list {
		if tx.Amount == 999 {
			updated = true
		}
	}
	if !updated {
		t.Errorf("ext-1 no se actualizó al monto 999")
	}
}
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go test ./internal/finance/ -run TestImportEsIdempotente -v
```
Expected: FALLA con error de compilación (`undefined: finance.ExternalTx`, `svc.Import`).

- [ ] **Step 3: Implementar `importer.go`**

`api/internal/finance/importer.go`:

```go
package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// ExternalTx es la representación normalizada de una transacción traída del
// servicio externo, lista para insertarse.
type ExternalTx struct {
	ExternalID string
	Type       string // income | expense | transfer
	Amount     int64  // centavos
	OccurredOn time.Time
	Category   string
	Remark     string
}

// Import inserta (idempotentemente por external_id) un lote de transacciones
// externas para un usuario, calculando el ciclo de cada una. Devuelve cuántas
// procesó.
func (s *Service) Import(ctx context.Context, userID uuid.UUID, txs []ExternalTx) (int, error) {
	n := 0
	for _, tx := range txs {
		ext := tx.ExternalID
		_, err := s.q.UpsertImportedTransaction(ctx, store.UpsertImportedTransactionParams{
			UserID:     userID,
			Type:       tx.Type,
			Amount:     tx.Amount,
			OccurredOn: tx.OccurredOn,
			Cycle:      Cycle(tx.OccurredOn),
			Category:   tx.Category,
			Remark:     tx.Remark,
			ExternalID: &ext,
		})
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ImportConfig configura el cliente del servicio externo (money.quhou123.com).
type ImportConfig struct {
	BaseURL string
	Token   string
	From    string // YYYY-MM-DD
	To      string // YYYY-MM-DD
}

// externalResponse refleja la forma ASUMIDA de la respuesta de getTransactions.
// AJUSTAR cuando se disponga de una muestra real del servicio.
type externalResponse struct {
	Data []struct {
		ID       string `json:"id"`
		Kind     string `json:"type"`   // "income" | "expense" | "transfer"
		Amount   int64  `json:"amount"` // se asume en centavos
		Date     string `json:"date"`   // YYYY-MM-DD
		Category string `json:"category"`
		Remark   string `json:"remark"`
	} `json:"data"`
}

// FetchTransactions llama al servicio externo y normaliza la respuesta. El
// mapeo de campos es provisional (ver externalResponse).
func FetchTransactions(ctx context.Context, cfg ImportConfig) ([]ExternalTx, error) {
	u, err := url.Parse(cfg.BaseURL + "/getTransactions")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from", cfg.From)
	q.Set("to", cfg.To)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getTransactions devolvió %d", res.StatusCode)
	}

	var body externalResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]ExternalTx, 0, len(body.Data))
	for _, item := range body.Data {
		day, err := time.Parse(dateLayout, item.Date)
		if err != nil {
			return nil, fmt.Errorf("fecha inválida %q: %w", item.Date, err)
		}
		out = append(out, ExternalTx{
			ExternalID: item.ID,
			Type:       item.Kind,
			Amount:     item.Amount,
			OccurredOn: day,
			Category:   item.Category,
			Remark:     item.Remark,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `api/`, con `TEST_DATABASE_URL`):
```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/finance/ -run TestImportEsIdempotente -v
```
Expected: PASA.

- [ ] **Step 5: Implementar el comando CLI**

`api/cmd/import/main.go`:

```go
// Command import trae el histórico de transacciones del servicio externo
// (money.quhou123.com) y lo inserta idempotentemente para un usuario de Focus.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/focus365/api/internal/db"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
)

func main() {
	var (
		email   = flag.String("email", "", "email del usuario de Focus al que asignar las transacciones")
		baseURL = flag.String("base-url", os.Getenv("FOCUS_IMPORT_BASE_URL"), "URL base del servicio externo")
		token   = flag.String("token", os.Getenv("FOCUS_IMPORT_TOKEN"), "token del servicio externo")
		from    = flag.String("from", "", "fecha inicial YYYY-MM-DD")
		to      = flag.String("to", "", "fecha final YYYY-MM-DD")
	)
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" || *email == "" || *baseURL == "" || *token == "" || *from == "" || *to == "" {
		log.Fatal("faltan parámetros: DATABASE_URL, --email, --base-url, --token, --from, --to son obligatorios")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	q := store.New(pool)

	user, err := q.GetUserByEmail(ctx, *email)
	if err != nil {
		log.Fatalf("usuario %q no encontrado: %v", *email, err)
	}

	svc := finance.NewService(q)
	txs, err := finance.FetchTransactions(ctx, finance.ImportConfig{
		BaseURL: *baseURL, Token: *token, From: *from, To: *to,
	})
	if err != nil {
		log.Fatalf("fetch: %v", err)
	}

	started := time.Now()
	n, err := svc.Import(ctx, user.ID, txs)
	if err != nil {
		log.Fatalf("import (procesadas %d): %v", n, err)
	}
	log.Printf("importadas %d transacciones para %s en %s", n, *email, time.Since(started))
}
```

- [ ] **Step 6: Verificar build del comando + vet**

Run (desde `api/`):
```bash
cd api && GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go build ./cmd/import
```
Expected: vet limpio; el comando compila (se genera el binario `import`; eliminarlo con `rm -f import` si queda en el árbol).

- [ ] **Step 7: Commit**

```bash
git add api/internal/finance/importer.go api/internal/finance/importer_test.go api/cmd/import/main.go
git commit -m "feat(finanzas): importador idempotente desde money.quhou123.com (mapeo provisional)"
```

---

### Task 9: Lib del frontend (`lib/finances.ts`)

**Files:**
- Create: `web/src/lib/finances.ts`
- Test: `web/src/lib/finances.test.ts`

- [ ] **Step 1: Escribir el test que falla**

`web/src/lib/finances.test.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { setAccessToken } from "./api";
import {
  create,
  listByCycle,
  remove,
  summary,
  cycles,
  pesosToCents,
  centsToPesos,
} from "./finances";

describe("lib finances", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("create hace POST /finances/transactions con el body y el Bearer", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: "t1" }), { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);

    await create({
      type: "expense", amount: 12345, occurred_on: "2026-06-10",
      category: "luz", remark: "",
    });

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/finances/transactions");
    expect(opts.method).toBe("POST");
    expect((opts.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok");
    expect(JSON.parse(opts.body as string).amount).toBe(12345);
  });

  it("listByCycle pide el ciclo dado y manda today", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await listByCycle("2026-06");

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/finances/transactions?cycle=2026-06");
    expect(url).toContain("today=");
  });

  it("listByCycle sin ciclo solo manda today (ciclo actual)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await listByCycle();

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/finances/transactions?today=");
    expect(url).not.toContain("cycle=");
  });

  it("remove hace DELETE con el id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await remove("t1");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/finances/transactions/t1");
    expect(opts.method).toBe("DELETE");
  });

  it("summary y cycles pegan a sus rutas con today", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await summary("2026-06");
    await cycles();

    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/finances/summary?cycle=2026-06");
    expect(fetchMock.mock.calls[1][0]).toContain("/api/v1/finances/cycles?today=");
  });

  it("convierte pesos a centavos y viceversa", () => {
    expect(pesosToCents(123.45)).toBe(12345);
    expect(centsToPesos(12345)).toBe(123.45);
  });
});
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `web/`):
```bash
cd web && npx vitest run src/lib/finances.test.ts
```
Expected: FALLA (no existe `./finances`).

- [ ] **Step 3: Implementar `finances.ts`**

`web/src/lib/finances.ts`:

```ts
import { apiFetch } from "./api";

export type TxType = "income" | "expense" | "transfer";

export type Transaction = {
  id: string;
  type: TxType;
  amount: number; // centavos
  occurred_on: string; // YYYY-MM-DD
  cycle: string; // YYYY-MM
  category: string;
  remark: string;
  source: string;
  created_at: string;
  updated_at: string;
};

export type TransactionInput = {
  type: TxType;
  amount: number; // centavos
  occurred_on: string;
  category: string;
  remark: string;
};

export type CycleSummary = {
  cycle: string; // YYYY-MM
  income: number;
  expense: number;
  net: number;
  status: "pendiente" | "verde" | "rojo";
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function create(input: TransactionInput): Promise<Transaction> {
  return apiFetch<Transaction>("/api/v1/finances/transactions", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// listByCycle lista las transacciones de un ciclo (YYYY-MM); sin ciclo usa el
// actual (el servidor lo deriva de today).
export function listByCycle(cycle?: string): Promise<Transaction[]> {
  const params = new URLSearchParams();
  if (cycle) params.set("cycle", cycle);
  params.set("today", todayString());
  return apiFetch<Transaction[]>(`/api/v1/finances/transactions?${params.toString()}`);
}

export function remove(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/finances/transactions/${id}`, { method: "DELETE" });
}

export function summary(cycle?: string): Promise<CycleSummary> {
  const params = new URLSearchParams();
  if (cycle) params.set("cycle", cycle);
  params.set("today", todayString());
  return apiFetch<CycleSummary>(`/api/v1/finances/summary?${params.toString()}`);
}

export function cycles(): Promise<CycleSummary[]> {
  return apiFetch<CycleSummary[]>(`/api/v1/finances/cycles?today=${todayString()}`);
}

export function pesosToCents(pesos: number): number {
  return Math.round(pesos * 100);
}

export function centsToPesos(cents: number): number {
  return cents / 100;
}

// formatMXN muestra centavos como moneda mexicana ($1,234.50).
export function formatMXN(cents: number): string {
  return new Intl.NumberFormat("es-MX", {
    style: "currency",
    currency: "MXN",
  }).format(cents / 100);
}
```

- [ ] **Step 4: Ejecutar el test para verlo pasar**

Run (desde `web/`):
```bash
cd web && npx vitest run src/lib/finances.test.ts
```
Expected: PASA (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/finances.ts web/src/lib/finances.test.ts
git commit -m "feat(finanzas): lib del frontend (create, list, delete, summary, cycles)"
```

---

### Task 10: Página `/finanzas` + enlace desde el home

**Files:**
- Create: `web/src/routes/finanzas.tsx`
- Test: `web/src/routes/finanzas.test.tsx`
- Modify: `web/src/routes/index.tsx`
- Generate (TanStack Router): `web/src/routeTree.gen.ts` (se regenera con el build)

- [ ] **Step 1: Escribir el test que falla**

`web/src/routes/finanzas.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as FinanzasRoute } from "./finanzas";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(
        new Response(JSON.stringify({ id: "t9" }), { status: 201 })
      );
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/summary")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            cycle: "2026-06", income: 500000, expense: 200000,
            net: 300000, status: "pendiente",
          }),
          { status: 200 }
        )
      );
    }
    if (url.includes("/cycles")) {
      return Promise.resolve(
        new Response(
          JSON.stringify([
            { cycle: "2026-06", income: 500000, expense: 200000, net: 300000, status: "pendiente" },
          ]),
          { status: 200 }
        )
      );
    }
    // GET /transactions
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "t1", type: "expense", amount: 200000, occurred_on: "2026-06-12",
            cycle: "2026-06", category: "renta", remark: "", source: "manual",
            created_at: "", updated_at: "",
          },
        ]),
        { status: 200 }
      )
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const finanzasRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/finanzas",
    component: FinanzasRoute.options.component,
  });
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([finanzasRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/finanzas"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("FinanzasPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el resumen del ciclo y una transacción", async () => {
    renderPage();
    expect(await screen.findByText(/pendiente/i)).toBeInTheDocument();
    expect(await screen.findByText("renta")).toBeInTheDocument();
  });

  it("al guardar dispara un POST", async () => {
    renderPage();
    const monto = await screen.findByLabelText("Monto");
    await userEvent.type(monto, "150");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.some(
        ([url, opts]) =>
          url === "/api/v1/finances/transactions" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });

  it("al borrar una transacción dispara un DELETE", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Borrar renta" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const deleted = calls.some(
        ([url, opts]) =>
          url === "/api/v1/finances/transactions/t1" && opts?.method === "DELETE"
      );
      expect(deleted).toBe(true);
    });
  });
});
```

- [ ] **Step 2: Ejecutar el test para verlo fallar**

Run (desde `web/`):
```bash
cd web && npx vitest run src/routes/finanzas.test.tsx
```
Expected: FALLA (no existe `./finanzas`).

- [ ] **Step 3: Implementar la página**

`web/src/routes/finanzas.tsx`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  create,
  listByCycle,
  remove,
  summary,
  cycles,
  pesosToCents,
  formatMXN,
  todayString,
  type Transaction,
  type CycleSummary,
  type TxType,
} from "@/lib/finances";

export const Route = createFileRoute("/finanzas")({ component: FinanzasPage });

const STATUS_COLOR: Record<CycleSummary["status"], string> = {
  pendiente: "text-sand-400",
  verde: "text-streak",
  rojo: "text-red-400",
};

function FinanzasPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const summaryQuery = useQuery({
    queryKey: ["finance", "summary"],
    queryFn: () => summary(),
    enabled: !!user,
  });
  const listQuery = useQuery({
    queryKey: ["finance", "list"],
    queryFn: () => listByCycle(),
    enabled: !!user,
  });
  const cyclesQuery = useQuery({
    queryKey: ["finance", "cycles"],
    queryFn: () => cycles(),
    enabled: !!user,
  });

  const [type, setType] = useState<TxType>("expense");
  const [montoPesos, setMontoPesos] = useState("");
  const [occurredOn, setOccurredOn] = useState(todayString());
  const [category, setCategory] = useState("");
  const [remark, setRemark] = useState("");
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["finance"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      create({
        type,
        amount: pesosToCents(Number(montoPesos)),
        occurred_on: occurredOn,
        category,
        remark,
      }),
    onSuccess: () => {
      setError(null);
      setMontoPesos("");
      setCategory("");
      setRemark("");
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => remove(id),
    onSuccess: invalidate,
  });

  if (!user) return null;

  const sum = summaryQuery.data;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Finanzas</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      {sum && (
        <section className="mt-6 rounded-xl border border-ink-700 bg-ink-900 p-6">
          <div className="flex items-center justify-between">
            <span className="text-sm text-sand-400">Ciclo {sum.cycle}</span>
            <span className={`text-sm font-bold ${STATUS_COLOR[sum.status]}`}>
              {sum.status}
            </span>
          </div>
          <p className="mt-2 text-2xl font-extrabold">{formatMXN(sum.net)}</p>
          <p className="mt-1 text-xs text-sand-400">
            Ingresos {formatMXN(sum.income)} · Gastos {formatMXN(sum.expense)}
          </p>
        </section>
      )}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          createMutation.mutate();
        }}
        className="mt-6 space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Tipo</span>
          <select
            aria-label="Tipo"
            value={type}
            onChange={(e) => setType(e.target.value as TxType)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          >
            <option value="expense">Gasto</option>
            <option value="income">Ingreso</option>
            <option value="transfer">Transferencia</option>
          </select>
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Monto</span>
          <input
            type="number"
            aria-label="Monto"
            min="0"
            step="0.01"
            value={montoPesos}
            onChange={(e) => setMontoPesos(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Fecha</span>
          <input
            type="date"
            aria-label="Fecha"
            value={occurredOn}
            onChange={(e) => setOccurredOn(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Categoría</span>
          <input
            type="text"
            aria-label="Categoría"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <input
            type="text"
            aria-label="Nota"
            value={remark}
            onChange={(e) => setRemark(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Movimientos del ciclo</h2>
        {listQuery.data && listQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {listQuery.data.map((tx: Transaction) => (
              <li
                key={tx.id}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span>
                  <span className="text-sand-400">{tx.occurred_on}</span>{" "}
                  {tx.category || tx.type}
                </span>
                <span className="flex items-center gap-3">
                  <span className={tx.type === "income" ? "text-streak" : ""}>
                    {formatMXN(tx.amount)}
                  </span>
                  <button
                    type="button"
                    aria-label={`Borrar ${tx.category || tx.type}`}
                    onClick={() => deleteMutation.mutate(tx.id)}
                    className="text-xs text-sand-400 hover:text-red-400"
                  >
                    ✕
                  </button>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay movimientos.</p>
        )}
      </section>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial de ciclos</h2>
        {cyclesQuery.data && cyclesQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {cyclesQuery.data.map((c: CycleSummary) => (
              <li
                key={c.cycle}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span className="text-sand-400">{c.cycle}</span>
                <span className="flex items-center gap-3">
                  <span>{formatMXN(c.net)}</span>
                  <span className={`font-bold ${STATUS_COLOR[c.status]}`}>{c.status}</span>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay ciclos.</p>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 4: Añadir el enlace desde el home**

En `web/src/routes/index.tsx`, debajo del `<Link to="/check-in">…</Link>`, añadir:

```tsx
      <Link
        to="/finanzas"
        className="mt-4 ml-2 inline-block rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
      >
        Finanzas
      </Link>
```

- [ ] **Step 5: Ejecutar el test + build (regenera routeTree)**

Run (desde `web/`):
```bash
cd web && npx vitest run src/routes/finanzas.test.tsx && npm run build
```
Expected: los 3 tests de finanzas pasan; `tsc -b && vite build` sin errores; `src/routeTree.gen.ts` ahora incluye la ruta `/finanzas`.

- [ ] **Step 6: Ejecutar toda la suite del frontend**

Run (desde `web/`):
```bash
cd web && npm test
```
Expected: todos los tests del frontend en verde.

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/finanzas.tsx web/src/routes/finanzas.test.tsx web/src/routes/index.tsx web/src/routeTree.gen.ts
git commit -m "feat(finanzas): página /finanzas con resumen, captura, movimientos e historial"
```

---

### Task 11: Smoke e2e dockerizado

**Files:**
- (sin archivos nuevos; validación de extremo a extremo)

- [ ] **Step 1: Levantar el stack y aplicar migraciones**

Run:
```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"
cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```
Expected: contenedores `db`, `api`, `web` arriba; migración a la versión 3 aplicada.

- [ ] **Step 2: Registrar un usuario y guardar el token**

Run:
```bash
curl -s -X POST http://localhost:5174/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"fin-smoke@focus.com","password":"p4ssword","name":"FinSmoke"}' \
  -o /tmp/fin-reg.json -w "register HTTP %{http_code}\n"
TOKEN=$(python3 -c "import json;print(json.load(open('/tmp/fin-reg.json'))['access_token'])")
```
Expected: `register HTTP 201` (o `409` si ya existe; en ese caso usar `/login`).

- [ ] **Step 3: Crear transacciones y verificar el resumen**

Run:
```bash
curl -s -X POST http://localhost:5174/api/v1/finances/transactions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"income","amount":500000,"occurred_on":"2026-06-10","category":"sueldo"}' \
  -w "\ncreate income HTTP %{http_code}\n"
curl -s -X POST http://localhost:5174/api/v1/finances/transactions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"expense","amount":200000,"occurred_on":"2026-06-12","category":"renta"}' \
  -w "\ncreate expense HTTP %{http_code}\n"
curl -s "http://localhost:5174/api/v1/finances/summary?today=2026-06-15" \
  -H "Authorization: Bearer $TOKEN" -w "\nsummary HTTP %{http_code}\n"
```
Expected: ambos `create … HTTP 201`; el resumen muestra `"income":500000,"expense":200000,"net":300000,"status":"pendiente"` y `summary HTTP 200`.

- [ ] **Step 4: Listar, borrar y verificar 401 sin token**

Run:
```bash
curl -s "http://localhost:5174/api/v1/finances/transactions?today=2026-06-15" \
  -H "Authorization: Bearer $TOKEN" -w "\nlist HTTP %{http_code}\n"
ID=$(curl -s "http://localhost:5174/api/v1/finances/transactions?today=2026-06-15" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "import json,sys;print(json.load(sys.stdin)[0]['id'])")
curl -s -X DELETE "http://localhost:5174/api/v1/finances/transactions/$ID" \
  -H "Authorization: Bearer $TOKEN" -o /dev/null -w "delete HTTP %{http_code}\n"
curl -s "http://localhost:5174/api/v1/finances/transactions?today=2026-06-15" \
  -o /dev/null -w "sin token HTTP %{http_code}\n"
```
Expected: `list HTTP 200` (array con 2 elementos); `delete HTTP 204`; `sin token HTTP 401`.

- [ ] **Step 5: Validación visual (manual, opcional)**

Abrir `http://localhost:5174`, iniciar sesión y entrar a **Finanzas**: ver el resumen del ciclo, capturar una transacción y verla en la lista, y comprobar el historial de ciclos. (Este paso lo realiza el usuario.)

- [ ] **Step 6: Suites completas en verde**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./...
cd /Users/gustavo/Desktop/focus-365/web && npm test && npm run build
```
Expected: backend y frontend en verde; build del frontend OK.

- [ ] **Step 7: Commit (si hubo ajustes) y cierre**

Si los pasos anteriores requirieron correcciones, commitearlas. Luego, usar la skill **superpowers:finishing-a-development-branch** para decidir el cierre de la rama `feat/plan-3-finanzas`.

---

## Auto-revisión del plan

**1. Cobertura del spec:**
- §4 Modelo de datos → Task 1 (migración) + Task 2 (sqlc).
- §5 Lógica de ciclo de pago → Task 3 (`cycle.go` + test con casos de fin de semana y cruce de año).
- §6 API (POST/GET cycle/DELETE/summary/cycles, status pendiente/verde/rojo, transfers excluidas) → Task 4 (servicio) + Task 6 (handlers) + Task 7 (montaje).
- §6 mensajes de validación → Task 5 (`httpx` etiquetas).
- §7 Frontend (`lib/finances.ts`, `/finanzas`, enlace home) → Task 9 + Task 10.
- §7 Importador (`cmd/import`, mapeo provisional) → Task 8.
- §8 Flujo de datos / §9 errores → cubiertos por handlers (Task 6) y página (Task 10).
- §10 Testing (cycle puro, store, handler, lib, página, idempotencia) → Tasks 3,4,6,8,9,10.
- §11 Criterios de aceptación → Task 11 (smoke e2e).

**2. Placeholders:** el único punto abierto (mapeo de campos del importador) está implementado con una forma asumida y test sobre la parte estable (`Import`), documentado como ajustable. No hay "TBD" sin código.

**3. Consistencia de tipos:** `finance.Input`, `finance.Transaction` (cycle como `YYYY-MM`), `finance.CycleSummary`, `finance.ExternalTx` y `finance.ImportConfig` se definen una vez y se usan igual en service/handler/importer/tests. Las firmas sqlc esperadas en Task 2 coinciden con su uso en Tasks 4 y 8. En el frontend, `Transaction`/`CycleSummary`/`TxType` se definen en `lib/finances.ts` y se reusan en la página y los tests.



