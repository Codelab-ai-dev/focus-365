# Plan 2 — Check-in diario · Implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir el primer módulo de dominio de Focus 365 — el check-in diario (ánimo/energía/disciplina 1-10 + nota), con upsert por día, historial de solo lectura y una página React dedicada en `/check-in`.

**Architecture:** Se extrae un paquete HTTP compartido `api/internal/httpx` desde `auth` (DRY), se añade la tabla `check_ins` (migración goose + sqlc), un paquete de dominio `api/internal/checkin` (service + handlers chi protegidos por `RequireAuth`, scoped por `user_id` del contexto), y en el frontend una lib `checkins.ts` sobre `apiFetch` más la ruta `/check-in` con TanStack Query (3 sliders + nota + historial).

**Tech Stack:** Go 1.23 (chi v5, pgx/v5, sqlc v1.31.1, goose, go-playground/validator/v10 v10.22.1, google/uuid). React 18.3 + Vite 5.4 + TanStack Router/Query + Vitest. PostgreSQL 16. Docker.

**Spec:** `docs/superpowers/specs/2026-06-10-plan-2-checkin-design.md`

---

## Restricciones críticas del entorno (leer antes de empezar)

- **Toolchain Go pinneado:** TODOS los comandos `go` deben correr con `GOTOOLCHAIN=local`. El proyecto está en `go 1.23` para igualar `golang:1.23-alpine`. `validator` está pinneado a `v10.22.1` (no actualizar). Ejemplo: `GOTOOLCHAIN=local go test ./...`.
- **DB de test:** export `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`. Los tests de DB hacen `t.Skip` si no está. El stack docker debe estar arriba (`db` en puerto host 5544).
- **Docker en este Mac:** los credential helpers no están en el PATH por defecto. Para comandos `docker`, prepende: `export PATH="$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"`. No pipear a `tail`/`sed` tras manipular el PATH; usar flags nativos (`docker logs --tail N`, `curl -o /tmp/x -w '%{http_code}'`).
- **Puertos host:** db 5544, api 8088, web 5174. La SPA habla con la API **mismo origen** vía el proxy nginx (`/api/` → `api:8080`).
- **El shell resetea el cwd** a la carpeta del vault Obsidian en cada llamada Bash. Hacer `cd /Users/gustavo/Desktop/focus-365` al inicio de cada comando.
- **sqlc** se ejecuta vía binario de Homebrew: `sqlc generate` desde `api/`.
- Mensajes de commit y comentarios de código en **español**.

---

## Estructura de archivos

**Backend (crear):**
- `api/internal/httpx/httpx.go` — helpers HTTP compartidos.
- `api/internal/httpx/httpx_test.go` — tests de `ValidationMessage`.
- `api/db/migrations/0002_check_ins.sql` — tabla `check_ins`.
- `api/db/queries/check_ins.sql` — queries sqlc.
- `api/internal/store/check_ins.sql.go` — **generado** por sqlc.
- `api/internal/store/check_ins_test.go` — test de la capa store.
- `api/internal/checkin/service.go` — lógica de dominio.
- `api/internal/checkin/service_test.go` — test del service.
- `api/internal/checkin/handler.go` — handlers chi + `Routes`.
- `api/internal/checkin/handler_test.go` — test HTTP con auth.

**Backend (modificar):**
- `api/sqlc.yaml` — añadir override `date` → `time.Time`.
- `api/internal/auth/handler.go` — refactor para usar `httpx` (sin cambiar comportamiento).
- `api/internal/store/models.go` — **regenerado** por sqlc (añade struct `CheckIn`).
- `api/internal/server/server.go` — montar `/checkins` con `RequireAuth`.

**Frontend (crear):**
- `web/src/lib/checkins.ts` — tipos + funciones API.
- `web/src/lib/checkins.test.ts` — tests de la lib.
- `web/src/routes/check-in.tsx` — página del check-in.
- `web/src/routes/check-in.test.tsx` — test de la página.

**Frontend (modificar):**
- `web/src/routes/index.tsx` — enlace a `/check-in`.
- `web/src/routeTree.gen.ts` — **regenerado** por el plugin de TanStack Router al hacer build.

---

## Task 1: Paquete `httpx` compartido + refactor de `auth`

Extrae los helpers HTTP de `auth/handler.go` a un paquete reusable `httpx` y enseña a `fieldLabel` las etiquetas de los campos de check-in. `auth` pasa a usar `httpx` sin cambiar su comportamiento (sus tests siguen verdes).

**Files:**
- Create: `api/internal/httpx/httpx.go`
- Create: `api/internal/httpx/httpx_test.go`
- Modify: `api/internal/auth/handler.go`

- [ ] **Step 1: Escribir el test de `httpx` (falla por no existir el paquete)**

Crear `api/internal/httpx/httpx_test.go`:

```go
package httpx

import "testing"

type sample struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=6"`
	Mood     int    `validate:"required,min=1,max=10"`
}

func TestValidationMessage(t *testing.T) {
	cases := []struct {
		name string
		in   sample
		want string
	}{
		{"email inválido", sample{Email: "bad", Password: "abcdef", Mood: 5}, "El email no tiene un formato válido"},
		{"password corta", sample{Email: "a@b.com", Password: "123", Mood: 5}, "La contraseña debe tener al menos 6 caracteres"},
		{"mood fuera de rango", sample{Email: "a@b.com", Password: "abcdef", Mood: 11}, "El ánimo debe ser como máximo 10"},
		{"mood faltante", sample{Email: "a@b.com", Password: "abcdef", Mood: 0}, "Falta el ánimo"},
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

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go test ./internal/httpx/`
Expected: FAIL — el paquete `httpx` aún no compila (`undefined: validate`, `undefined: ValidationMessage`).

- [ ] **Step 3: Crear el paquete `httpx`**

Crear `api/internal/httpx/httpx.go`:

```go
// Package httpx reúne helpers HTTP compartidos por los módulos de dominio:
// escritura de JSON/errores y decodificación + validación de requests con
// mensajes claros en español.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// WriteJSON serializa v como JSON con el status dado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr responde con el formato estándar {"error": "..."}.
func WriteErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeAndValidate decodifica el body JSON en dst y lo valida. Si algo falla
// responde 400 con un mensaje claro y devuelve false.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		WriteErr(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	if err := validate.Struct(dst); err != nil {
		WriteErr(w, http.StatusBadRequest, ValidationMessage(err))
		return false
	}
	return true
}

// ValidationMessage traduce el primer error de validación a un mensaje claro
// en español, indicando qué campo falló y por qué.
func ValidationMessage(err error) string {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) || len(verrs) == 0 {
		return "datos inválidos"
	}
	fe := verrs[0]
	label := fieldLabel(fe.Field())
	switch fe.Tag() {
	case "required":
		return "Falta " + label
	case "email":
		return "El email no tiene un formato válido"
	case "min":
		if isNumeric(fe.Kind()) {
			return capitalize(label) + " debe ser al menos " + fe.Param()
		}
		return capitalize(label) + " debe tener al menos " + fe.Param() + " caracteres"
	case "max":
		if isNumeric(fe.Kind()) {
			return capitalize(label) + " debe ser como máximo " + fe.Param()
		}
		return capitalize(label) + " debe tener como máximo " + fe.Param() + " caracteres"
	default:
		return capitalize(label) + " no es válido"
	}
}

func fieldLabel(field string) string {
	switch field {
	case "Email":
		return "el email"
	case "Password":
		return "la contraseña"
	case "Name":
		return "el nombre"
	case "Date":
		return "la fecha"
	case "Mood":
		return "el ánimo"
	case "Energy":
		return "la energía"
	case "Discipline":
		return "la disciplina"
	default:
		return field
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Correr el test de `httpx` (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go test ./internal/httpx/`
Expected: PASS (4 subtests ok).

- [ ] **Step 5: Refactorizar `auth/handler.go` para usar `httpx`**

Reemplazar el archivo completo `api/internal/auth/handler.go` por (se borran los helpers locales y se usan los de `httpx`; el comportamiento es idéntico):

```go
package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/focus365/api/internal/httpx"
	"github.com/focus365/api/internal/store"
	"github.com/go-chi/chi/v5"
)

type registerReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
	Name     string `json:"name" validate:"required"`
}

type loginReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type authResp struct {
	AccessToken string   `json:"access_token"`
	User        userView `json:"user"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/register", handleRegister(svc))
	r.Post("/login", handleLogin(svc))
	r.Post("/refresh", handleRefresh(svc))
	r.With(RequireAuth(svc.Tokens())).Get("/me", handleMe(svc))
	return r
}

func handleRegister(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Register(r.Context(), req.Email, req.Password, req.Name)
		if err != nil {
			if errors.Is(err, ErrEmailTaken) {
				httpx.WriteErr(w, http.StatusConflict, err.Error())
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		respondWithTokens(w, svc, user, http.StatusCreated)
	}
}

func handleLogin(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleRefresh(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "sin refresh token")
			return
		}
		id, err := svc.Tokens().ParseRefresh(cookie.Value)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "refresh inválido")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "usuario no encontrado")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleMe(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "usuario no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toView(user))
	}
}

func respondWithTokens(w http.ResponseWriter, svc *Service, user store.User, status int) {
	access, refresh, err := svc.IssueTokens(user.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, "error emitiendo tokens")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refresh,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(RefreshTTL()),
	})
	httpx.WriteJSON(w, status, authResp{AccessToken: access, User: toView(user)})
}

func toView(u store.User) userView {
	return userView{ID: u.ID.String(), Email: u.Email, Name: u.Name}
}
```

- [ ] **Step 6: Compilar y correr los tests de `auth` + `httpx` (verde, sin cambio de comportamiento)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/auth/ ./internal/httpx/`
Expected: PASS — `auth` mantiene `TestRegisterValidationMessages` verde (mismos mensajes), `httpx` verde.

- [ ] **Step 7: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add api/internal/httpx/ api/internal/auth/handler.go
git commit -m "$(cat <<'EOF'
refactor(api): extrae helpers HTTP a paquete httpx compartido

Mueve WriteJSON/WriteErr/DecodeAndValidate/ValidationMessage desde auth
a internal/httpx para reusarlos en checkin y futuros módulos (DRY).
fieldLabel aprende las etiquetas de los campos de check-in.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Tabla `check_ins` (migración + sqlc + capa store)

Añade la migración goose, las queries sqlc, regenera el store y prueba la capa store contra Postgres real.

**Files:**
- Create: `api/db/migrations/0002_check_ins.sql`
- Create: `api/db/queries/check_ins.sql`
- Create: `api/internal/store/check_ins_test.go`
- Modify: `api/sqlc.yaml`
- Generated: `api/internal/store/check_ins.sql.go`, `api/internal/store/models.go`

- [ ] **Step 1: Escribir el test de la capa store (falla por símbolos inexistentes)**

Crear `api/internal/store/check_ins_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestUpsertGetListCheckIns(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "c@b.com", PasswordHash: "h", Name: "Caro",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	d10 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	d11 := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	// Insert inicial.
	ci, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d10, Mood: 7, Energy: 6, Discipline: 8, Note: "buen día",
	})
	if err != nil {
		t.Fatalf("UpsertCheckIn insert: %v", err)
	}
	if ci.Mood != 7 || ci.Note != "buen día" {
		t.Errorf("valores insertados incorrectos: %+v", ci)
	}

	// Upsert el mismo día actualiza (no duplica): mismo ID, valores nuevos.
	ci2, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d10, Mood: 3, Energy: 4, Discipline: 5, Note: "regular",
	})
	if err != nil {
		t.Fatalf("UpsertCheckIn update: %v", err)
	}
	if ci2.ID != ci.ID {
		t.Errorf("el upsert debería actualizar la misma fila, IDs: %s vs %s", ci2.ID, ci.ID)
	}
	if ci2.Mood != 3 {
		t.Errorf("mood actualizado = %d, want 3", ci2.Mood)
	}

	// Otro día → fila distinta.
	if _, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d11, Mood: 9, Energy: 9, Discipline: 9, Note: "",
	}); err != nil {
		t.Fatalf("UpsertCheckIn d11: %v", err)
	}

	// GetCheckInByDate.
	got, err := q.GetCheckInByDate(ctx, store.GetCheckInByDateParams{UserID: user.ID, Date: d10})
	if err != nil {
		t.Fatalf("GetCheckInByDate: %v", err)
	}
	if got.Mood != 3 {
		t.Errorf("get mood = %d, want 3", got.Mood)
	}

	// ListCheckIns: descendente por fecha, respeta limit.
	list, err := q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: user.ID, Limit: 30})
	if err != nil {
		t.Fatalf("ListCheckIns: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if !list[0].Date.After(list[1].Date) {
		t.Errorf("la lista no está descendente: %v, %v", list[0].Date, list[1].Date)
	}

	limited, err := q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: user.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ListCheckIns limit: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("limit=1 devolvió %d filas", len(limited))
	}
}
```

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/store/`
Expected: FAIL — no compila (`store.UpsertCheckInParams` y `store.CheckIn` no existen aún).

- [ ] **Step 3: Crear la migración 0002**

Crear `api/db/migrations/0002_check_ins.sql`:

```sql
-- +goose Up
CREATE TABLE check_ins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    mood        INT  NOT NULL CHECK (mood       BETWEEN 1 AND 10),
    energy      INT  NOT NULL CHECK (energy     BETWEEN 1 AND 10),
    discipline  INT  NOT NULL CHECK (discipline BETWEEN 1 AND 10),
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, date)
);
CREATE INDEX idx_check_ins_user_date ON check_ins (user_id, date DESC);

-- +goose Down
DROP TABLE check_ins;
```

- [ ] **Step 4: Añadir el override de `date` en `sqlc.yaml`**

En `api/sqlc.yaml`, dentro de `overrides:`, añadir al final (después del override de `timestamptz`):

```yaml
          - db_type: "date"
            go_type: "time.Time"
```

El bloque `overrides` debe quedar así:

```yaml
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "date"
            go_type: "time.Time"
```

- [ ] **Step 5: Crear las queries sqlc**

Crear `api/db/queries/check_ins.sql`:

```sql
-- name: UpsertCheckIn :one
INSERT INTO check_ins (user_id, date, mood, energy, discipline, note)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    discipline = EXCLUDED.discipline,
    note = EXCLUDED.note,
    updated_at = now()
RETURNING *;

-- name: GetCheckInByDate :one
SELECT * FROM check_ins
WHERE user_id = $1 AND date = $2;

-- name: ListCheckIns :many
SELECT * FROM check_ins
WHERE user_id = $1
ORDER BY date DESC
LIMIT $2;
```

- [ ] **Step 6: Regenerar el store con sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Expected: sin output (éxito). Se crea `internal/store/check_ins.sql.go` y se actualiza `internal/store/models.go` con `type CheckIn struct { ID uuid.UUID; UserID uuid.UUID; Date time.Time; Mood int32; Energy int32; Discipline int32; Note string; CreatedAt time.Time; UpdatedAt time.Time }`.

Verificar: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go build ./...`
Expected: compila sin errores.

- [ ] **Step 7: Aplicar la migración a la DB de test y correr el test del store (verde)**

La migración se aplica automáticamente desde `testutil.NewDB` (corre todas las migraciones de `db/migrations`). Correr:

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/store/ -run TestUpsertGetListCheckIns -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add api/db/migrations/0002_check_ins.sql api/db/queries/check_ins.sql api/sqlc.yaml api/internal/store/
git commit -m "$(cat <<'EOF'
feat(api): tabla check_ins con upsert por día (migración + sqlc)

Migración 0002 con UNIQUE(user_id, date) e índice (user_id, date DESC).
Queries UpsertCheckIn (ON CONFLICT), GetCheckInByDate, ListCheckIns.
Override de date→time.Time en sqlc para evitar pgtype.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Service de dominio `checkin`

Capa que traduce entre tipos del dominio (con `date` como string `YYYY-MM-DD`) y el store, y aísla por `user_id`.

**Files:**
- Create: `api/internal/checkin/service.go`
- Create: `api/internal/checkin/service_test.go`

- [ ] **Step 1: Escribir el test del service (falla por no existir el paquete)**

Crear `api/internal/checkin/service_test.go`:

```go
package checkin_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestServiceUpsertTodayList(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := checkin.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "s@b.com", PasswordHash: "h", Name: "Sol"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	date := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	// Today sin check-in → nil, sin error.
	none, err := svc.Today(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Today vacío: %v", err)
	}
	if none != nil {
		t.Errorf("Today debería ser nil cuando no hay check-in, got %+v", none)
	}

	// Upsert y verificar formato de fecha.
	ci, err := svc.Upsert(ctx, user.ID, checkin.Input{
		Date: date, Mood: 7, Energy: 6, Discipline: 8, Note: "ok",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if ci.Date != "2026-06-10" {
		t.Errorf("Date = %q, want 2026-06-10", ci.Date)
	}
	if ci.Mood != 7 {
		t.Errorf("Mood = %d, want 7", ci.Mood)
	}

	// Today ahora devuelve el check-in.
	got, err := svc.Today(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if got == nil || got.Note != "ok" {
		t.Errorf("Today = %+v, want note=ok", got)
	}

	// List devuelve el historial.
	list, err := svc.List(ctx, user.ID, 30)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(list) = %d, want 1", len(list))
	}
}
```

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/checkin/`
Expected: FAIL — el paquete `checkin` no existe (`undefined: checkin.NewService`).

- [ ] **Step 3: Implementar el service**

Crear `api/internal/checkin/service.go`:

```go
// Package checkin implementa el dominio del check-in diario: upsert por día,
// consulta del día e historial, siempre scoped por user_id.
package checkin

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// dateLayout es el formato de fecha que viaja por la API (YYYY-MM-DD).
const dateLayout = "2006-01-02"

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// Input son los datos de dominio para crear/actualizar un check-in.
type Input struct {
	Date       time.Time
	Mood       int
	Energy     int
	Discipline int
	Note       string
}

// CheckIn es la vista de dominio que se serializa a JSON. Date va como string
// YYYY-MM-DD para evitar supuestos de timezone.
type CheckIn struct {
	ID         string    `json:"id"`
	Date       string    `json:"date"`
	Mood       int       `json:"mood"`
	Energy     int       `json:"energy"`
	Discipline int       `json:"discipline"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Service) Upsert(ctx context.Context, userID uuid.UUID, in Input) (*CheckIn, error) {
	row, err := s.q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID:     userID,
		Date:       in.Date,
		Mood:       int32(in.Mood),
		Energy:     int32(in.Energy),
		Discipline: int32(in.Discipline),
		Note:       in.Note,
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// Today devuelve el check-in del día o (nil, nil) si no existe.
func (s *Service) Today(ctx context.Context, userID uuid.UUID, date time.Time) (*CheckIn, error) {
	row, err := s.q.GetCheckInByDate(ctx, store.GetCheckInByDateParams{UserID: userID, Date: date})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, limit int) ([]CheckIn, error) {
	rows, err := s.q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: userID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]CheckIn, 0, len(rows))
	for _, row := range rows {
		out = append(out, toView(row))
	}
	return out, nil
}

func toView(row store.CheckIn) CheckIn {
	return CheckIn{
		ID:         row.ID.String(),
		Date:       row.Date.Format(dateLayout),
		Mood:       int(row.Mood),
		Energy:     int(row.Energy),
		Discipline: int(row.Discipline),
		Note:       row.Note,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
```

- [ ] **Step 4: Correr el test del service (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/checkin/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add api/internal/checkin/service.go api/internal/checkin/service_test.go
git commit -m "$(cat <<'EOF'
feat(api): service de dominio checkin (upsert/today/list)

Traduce entre tipos de dominio (date como YYYY-MM-DD) y el store.
Today devuelve nil sin error cuando no hay check-in del día.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Handlers HTTP + `Routes` de `checkin`

Endpoints chi que usan `httpx` para validar/responder y `auth.UserIDFromContext` para el `user_id`. `Routes` NO envuelve con `RequireAuth` (eso lo hace `server.go`), pero el test sí monta el middleware para probar 401/aislamiento.

**Files:**
- Create: `api/internal/checkin/handler.go`
- Create: `api/internal/checkin/handler_test.go`

- [ ] **Step 1: Escribir el test de los handlers (falla por no existir `Routes`)**

Crear `api/internal/checkin/handler_test.go`:

```go
package checkin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
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
		r.Mount("/checkins", checkin.Routes(checkin.NewService(q)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}

// token registra un usuario y devuelve su access token.
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

func TestUpsertCreatesAndUpdates(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	body := map[string]any{"date": "2026-06-10", "mood": 7, "energy": 6, "discipline": 8, "note": "buen día"}
	rec := do(t, e.h, http.MethodPost, "/checkins", tok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var ci map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ci)
	if ci["mood"].(float64) != 7 {
		t.Errorf("mood = %v, want 7", ci["mood"])
	}

	// Segundo POST mismo día → actualiza (mismo id).
	firstID := ci["id"]
	body["mood"] = 3
	rec2 := do(t, e.h, http.MethodPost, "/checkins", tok, body)
	var ci2 map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &ci2)
	if ci2["id"] != firstID {
		t.Errorf("el upsert creó una fila nueva: %v vs %v", ci2["id"], firstID)
	}
	if ci2["mood"].(float64) != 3 {
		t.Errorf("mood actualizado = %v, want 3", ci2["mood"])
	}
}

func TestTodayAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "b@b.com")

	// Sin check-in → today devuelve null.
	rec := do(t, e.h, http.MethodGet, "/checkins/today?date=2026-06-10", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET today code = %d", rec.Code)
	}
	if got := bytes.TrimSpace(rec.Body.Bytes()); string(got) != "null" {
		t.Errorf("today vacío = %q, want null", got)
	}

	// Creamos uno y lo recuperamos.
	_ = do(t, e.h, http.MethodPost, "/checkins", tok, map[string]any{
		"date": "2026-06-10", "mood": 5, "energy": 5, "discipline": 5, "note": "",
	})
	rec2 := do(t, e.h, http.MethodGet, "/checkins/today?date=2026-06-10", tok, nil)
	var ci map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &ci)
	if ci["date"] != "2026-06-10" {
		t.Errorf("today date = %v", ci["date"])
	}

	// List.
	rec3 := do(t, e.h, http.MethodGet, "/checkins", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec3.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("len(list) = %d, want 1", len(list))
	}
}

func TestValidationOutOfRange(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "c@b.com")
	rec := do(t, e.h, http.MethodPost, "/checkins", tok, map[string]any{
		"date": "2026-06-10", "mood": 11, "energy": 5, "discipline": 5, "note": "",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "El ánimo debe ser como máximo 10" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/checkins", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	_ = do(t, e.h, http.MethodPost, "/checkins", tokA, map[string]any{
		"date": "2026-06-10", "mood": 9, "energy": 9, "discipline": 9, "note": "de A",
	})

	rec := do(t, e.h, http.MethodGet, "/checkins", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("user B ve %d check-ins de A; debería ver 0", len(list))
	}
}
```

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/checkin/`
Expected: FAIL — `undefined: checkin.Routes`.

- [ ] **Step 3: Implementar los handlers**

Crear `api/internal/checkin/handler.go` (nota: `dateLayout` ya está declarado en `service.go`, mismo paquete — NO redeclararlo):

```go
package checkin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const (
	defaultLimit = 30
	maxLimit     = 100
)

type upsertReq struct {
	Date       string `json:"date" validate:"required"`
	Mood       int    `json:"mood" validate:"required,min=1,max=10"`
	Energy     int    `json:"energy" validate:"required,min=1,max=10"`
	Discipline int    `json:"discipline" validate:"required,min=1,max=10"`
	Note       string `json:"note"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/", handleUpsert(svc))
	r.Get("/today", handleToday(svc))
	r.Get("/", handleList(svc))
	return r
}

func handleUpsert(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req upsertReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		date, err := time.Parse(dateLayout, req.Date)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		ci, err := svc.Upsert(r.Context(), userID, Input{
			Date: date, Mood: req.Mood, Energy: req.Energy, Discipline: req.Discipline, Note: req.Note,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ci)
	}
}

func handleToday(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			httpx.WriteErr(w, http.StatusBadRequest, "Falta la fecha")
			return
		}
		date, err := time.Parse(dateLayout, dateStr)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		ci, err := svc.Today(r.Context(), userID, date)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// ci puede ser nil → se serializa como null (200).
		httpx.WriteJSON(w, http.StatusOK, ci)
	}
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		limit := defaultLimit
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		list, err := svc.List(r.Context(), userID, limit)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}
```

- [ ] **Step 4: Correr todos los tests de `checkin` (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/checkin/ -v`
Expected: PASS — Upsert crea/actualiza, today/list, validación 400 con mensaje, 401 sin token, aislamiento por usuario.

- [ ] **Step 5: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add api/internal/checkin/handler.go api/internal/checkin/handler_test.go
git commit -m "$(cat <<'EOF'
feat(api): handlers HTTP de checkin (upsert/today/list)

Endpoints chi scoped por user_id del contexto, validación 1-10 con
mensajes claros vía httpx; today devuelve null sin check-in.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Montar `/checkins` en el servidor con `RequireAuth`

Cablea el módulo en el router bajo `/api/v1`, protegido.

**Files:**
- Modify: `api/internal/server/server.go`

- [ ] **Step 1: Editar `server.go`**

En `api/internal/server/server.go`, añadir el import de `checkin`:

```go
	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/store"
```

Crear el service junto a los demás dentro de `New`:

```go
	q := store.New(d.Pool)
	tm := auth.NewTokenManager(d.JWTSecret)
	authSvc := auth.NewService(q, tm)
	checkinSvc := checkin.NewService(q)
```

Y montar la ruta protegida dentro del bloque `r.Route("/api/v1", ...)`:

```go
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health)
		r.Mount("/auth", auth.Routes(authSvc))
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(tm))
			r.Mount("/checkins", checkin.Routes(checkinSvc))
		})
	})
```

- [ ] **Step 2: Compilar, vet y correr toda la suite backend (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./...`
Expected: PASS — todos los paquetes verdes.

- [ ] **Step 3: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add api/internal/server/server.go
git commit -m "$(cat <<'EOF'
feat(api): monta /api/v1/checkins protegido con RequireAuth

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Lib frontend `checkins.ts`

Tipos y funciones sobre `apiFetch` para los tres endpoints, más un helper de fecha local.

**Files:**
- Create: `web/src/lib/checkins.ts`
- Create: `web/src/lib/checkins.test.ts`

- [ ] **Step 1: Escribir el test de la lib (falla por no existir el módulo)**

Crear `web/src/lib/checkins.test.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { setAccessToken } from "./api";
import { getToday, list, upsert, todayString } from "./checkins";

describe("lib checkins", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("getToday llama a GET /checkins/today con la fecha y el Bearer", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response("null", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getToday("2026-06-10");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/checkins/today?date=2026-06-10");
    expect((opts.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok");
    expect(res).toBeNull();
  });

  it("list llama a GET /checkins con limit", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await list(15);

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/checkins?limit=15");
  });

  it("upsert hace POST /checkins con el body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "c1" }), { status: 200 })
    );
    vi.stubGlobal("fetch", fetchMock);

    await upsert({ date: "2026-06-10", mood: 7, energy: 6, discipline: 8, note: "ok" });

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/checkins");
    expect(opts.method).toBe("POST");
    expect(JSON.parse(opts.body as string)).toEqual({
      date: "2026-06-10", mood: 7, energy: 6, discipline: 8, note: "ok",
    });
  });

  it("todayString formatea la fecha local como YYYY-MM-DD", () => {
    const d = new Date(2026, 5, 9); // 9 de junio de 2026 (mes 0-indexado)
    expect(todayString(d)).toBe("2026-06-09");
  });
});
```

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test -- checkins`
Expected: FAIL — no existe `./checkins`.

- [ ] **Step 3: Implementar la lib**

Crear `web/src/lib/checkins.ts`:

```ts
import { apiFetch } from "./api";

export type CheckIn = {
  id: string;
  date: string;
  mood: number;
  energy: number;
  discipline: number;
  note: string;
  created_at: string;
  updated_at: string;
};

export type CheckInInput = {
  date: string;
  mood: number;
  energy: number;
  discipline: number;
  note: string;
};

// getToday devuelve el check-in del día o null si no existe.
export function getToday(date: string): Promise<CheckIn | null> {
  return apiFetch<CheckIn | null>(
    `/api/v1/checkins/today?date=${encodeURIComponent(date)}`
  );
}

export function list(limit = 30): Promise<CheckIn[]> {
  return apiFetch<CheckIn[]>(`/api/v1/checkins?limit=${limit}`);
}

export function upsert(input: CheckInInput): Promise<CheckIn> {
  return apiFetch<CheckIn>("/api/v1/checkins", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(d = new Date()): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}
```

- [ ] **Step 4: Correr el test de la lib (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test -- checkins`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add web/src/lib/checkins.ts web/src/lib/checkins.test.ts
git commit -m "$(cat <<'EOF'
feat(web): lib checkins (getToday/list/upsert) sobre apiFetch

Incluye todayString para calcular la fecha local del usuario.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Página `/check-in` + enlace desde el home

Ruta protegida con 3 sliders 1-10, nota, botón Guardar (pre-rellenado con el check-in de hoy) e historial de solo lectura. Enlace desde el home.

**Files:**
- Create: `web/src/routes/check-in.tsx`
- Create: `web/src/routes/check-in.test.tsx`
- Modify: `web/src/routes/index.tsx`
- Generated: `web/src/routeTree.gen.ts`

- [ ] **Step 1: Escribir el test de la página (falla por no existir la ruta)**

Crear `web/src/routes/check-in.test.tsx`:

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

// Inyectamos un usuario autenticado falso para evitar el redirect a /login.
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as CheckInRoute } from "./check-in";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            id: "c1", date: "2026-06-10", mood: 5, energy: 5,
            discipline: 5, note: "", created_at: "", updated_at: "",
          }),
          { status: 200 }
        )
      );
    }
    if (url.includes("/today")) {
      return Promise.resolve(new Response("null", { status: 200 }));
    }
    return Promise.resolve(new Response("[]", { status: 200 }));
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const checkinRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/check-in",
    component: CheckInRoute.options.component,
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
    routeTree: rootRoute.addChildren([checkinRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/check-in"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("CheckInPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("renderiza los 3 sliders y la nota", async () => {
    renderPage();
    expect(await screen.findByLabelText("Ánimo")).toBeInTheDocument();
    expect(screen.getByLabelText("Energía")).toBeInTheDocument();
    expect(screen.getByLabelText("Disciplina")).toBeInTheDocument();
    expect(screen.getByLabelText("Nota")).toBeInTheDocument();
  });

  it("al Guardar dispara un POST", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Guardar" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.some(
        ([url, opts]) => url === "/api/v1/checkins" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });
});
```

- [ ] **Step 2: Correr el test para verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test -- check-in`
Expected: FAIL — no existe `./check-in`.

- [ ] **Step 3: Implementar la página**

Crear `web/src/routes/check-in.tsx`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getToday, list, upsert, todayString, type CheckIn } from "@/lib/checkins";

export const Route = createFileRoute("/check-in")({ component: CheckInPage });

function CheckInPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const today = todayString();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const todayQuery = useQuery({
    queryKey: ["checkin", "today", today],
    queryFn: () => getToday(today),
    enabled: !!user,
  });
  const historyQuery = useQuery({
    queryKey: ["checkin", "list"],
    queryFn: () => list(30),
    enabled: !!user,
  });

  const [mood, setMood] = useState(5);
  const [energy, setEnergy] = useState(5);
  const [discipline, setDiscipline] = useState(5);
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Pre-rellena el formulario con el check-in de hoy si existe.
  useEffect(() => {
    const ci = todayQuery.data;
    if (ci) {
      setMood(ci.mood);
      setEnergy(ci.energy);
      setDiscipline(ci.discipline);
      setNote(ci.note);
    }
  }, [todayQuery.data]);

  const mutation = useMutation({
    mutationFn: () => upsert({ date: today, mood, energy, discipline, note }),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["checkin", "today"] });
      qc.invalidateQueries({ queryKey: ["checkin", "list"] });
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Check-in de hoy</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
        className="mt-6 space-y-6 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <Slider label="Ánimo" value={mood} onChange={setMood} />
        <Slider label="Energía" value={energy} onChange={setEnergy} />
        <Slider label="Disciplina" value={discipline} onChange={setDiscipline} />

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <textarea
            aria-label="Nota"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={3}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-streak">{error}</p>}

        <button
          type="submit"
          disabled={mutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {mutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial</h2>
        {historyQuery.data && historyQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {historyQuery.data.map((ci: CheckIn) => (
              <li
                key={ci.id}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span className="text-sand-400">{ci.date}</span>
                <span>
                  Á{ci.mood} · E{ci.energy} · D{ci.discipline}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay check-ins.</p>
        )}
      </section>
    </div>
  );
}

function Slider({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
}) {
  return (
    <label className="block space-y-1">
      <span className="flex items-center justify-between text-sm">
        <span className="text-sand-400">{label}</span>
        <span className="font-bold text-amber-brand">{value}</span>
      </span>
      <input
        type="range"
        min={1}
        max={10}
        step={1}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-amber-brand"
      />
    </label>
  );
}
```

- [ ] **Step 4: Añadir el enlace desde el home**

En `web/src/routes/index.tsx`, cambiar el import de la primera línea para incluir `Link`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
```

Y reemplazar el `<p>` de bienvenida por el texto + un enlace al check-in:

```tsx
      <p className="mt-6 text-sand-400">
        Bienvenido, <span className="text-amber-brand">{user.name}</span>.
      </p>
      <Link
        to="/check-in"
        className="mt-4 inline-block rounded-lg bg-amber-brand px-4 py-2 text-sm font-bold text-ink-950"
      >
        Check-in de hoy
      </Link>
```

- [ ] **Step 5: Regenerar el route tree, typecheck y build**

El plugin de TanStack Router regenera `routeTree.gen.ts` al hacer build. Correr:

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm run build`
Expected: build OK; `git status` muestra `routeTree.gen.ts` modificado con la ruta `/check-in` registrada.

- [ ] **Step 6: Correr toda la suite frontend (verde)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test`
Expected: PASS — incluye `check-in`, `checkins`, `login`, `api`.

- [ ] **Step 7: Commit**

```bash
cd /Users/gustavo/Desktop/focus-365
git add web/src/routes/check-in.tsx web/src/routes/check-in.test.tsx web/src/routes/index.tsx web/src/routeTree.gen.ts
git commit -m "$(cat <<'EOF'
feat(web): página /check-in con sliders, nota e historial

Ruta protegida con TanStack Query: pre-rellena con el check-in de hoy,
guarda vía upsert e invalida queries; historial de solo lectura.
Enlace desde el home.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Smoke e2e contra el stack dockerizado

Verifica el flujo completo a través del proxy nginx (mismo origen): migración aplicada, upsert/today/list y aislamiento.

**Files:** ninguno (validación manual; documentar resultados).

- [ ] **Step 1: Reconstruir y levantar el stack**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365
export PATH="$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"
docker compose up -d --build
```
Expected: contenedores `db`, `api`, `web` en estado `Up`.

- [ ] **Step 2: Aplicar la migración 0002 a la DB dockerizada**

El binario `cmd/migrate` corre todas las migraciones de `db/migrations` (sin subcomando) usando `DATABASE_URL`. Debe ejecutarse desde el directorio `api/` (la ruta `db/migrations` es relativa). Correr contra la DB dockerizada:

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/api && DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go run ./cmd/migrate
```
Expected: `migrations applied` (idempotente: aplica `0002_check_ins` si falta). Verificar la tabla:
```bash
export PATH="$HOME/.docker/bin:$PATH"
docker compose exec db psql -U focus -d focus365 -c "\d check_ins"
```
Expected: la tabla `check_ins` existe con las columnas y el UNIQUE(user_id, date).

- [ ] **Step 3: Registrar un usuario y guardar token (vía proxy nginx en :5174)**

Run:
```bash
curl -s -X POST http://localhost:5174/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"smoke@focus.com","password":"p4ssword","name":"Smoke"}' \
  -o /tmp/reg.json -w 'HTTP %{http_code}\n'
```
Expected: `HTTP 201`. Extraer el token:
```bash
TOK=$(cd /Users/gustavo/Desktop/focus-365 && node -e "console.log(require('/tmp/reg.json').access_token)")
echo "token len: ${#TOK}"
```

- [ ] **Step 4: Upsert, today y list**

Run:
```bash
# POST crea
curl -s -X POST http://localhost:5174/api/v1/checkins \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"date":"2026-06-10","mood":7,"energy":6,"discipline":8,"note":"smoke"}' \
  -w '\nHTTP %{http_code}\n'

# POST mismo día actualiza (debe devolver el MISMO id, mood=3)
curl -s -X POST http://localhost:5174/api/v1/checkins \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"date":"2026-06-10","mood":3,"energy":6,"discipline":8,"note":"smoke 2"}' \
  -w '\nHTTP %{http_code}\n'

# GET today
curl -s "http://localhost:5174/api/v1/checkins/today?date=2026-06-10" \
  -H "Authorization: Bearer $TOK" -w '\nHTTP %{http_code}\n'

# GET list
curl -s "http://localhost:5174/api/v1/checkins?limit=30" \
  -H "Authorization: Bearer $TOK" -w '\nHTTP %{http_code}\n'
```
Expected: todos `HTTP 200`. El segundo POST devuelve el mismo `id` que el primero con `mood:3` (upsert, no duplica). `today` devuelve el check-in; `list` devuelve un array con un elemento.

- [ ] **Step 5: Verificar 401 sin token**

Run:
```bash
curl -s "http://localhost:5174/api/v1/checkins?limit=30" -w '\nHTTP %{http_code}\n'
```
Expected: `HTTP 401`.

- [ ] **Step 6: Verificación visual en el navegador**

Abrir `http://localhost:5174`, iniciar sesión, click en "Check-in de hoy". Mover los sliders, escribir una nota, Guardar → debe persistir. Recargar → los valores guardados aparecen pre-rellenados. El historial lista el check-in. Confirmar que volver a guardar el mismo día actualiza (no crea duplicado).

- [ ] **Step 7: Correr la suite completa final (backend + frontend)**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./...
cd /Users/gustavo/Desktop/focus-365/web && npm run build && npm test
```
Expected: todo verde.

- [ ] **Step 8: Commit final (si hubo ajustes durante el smoke)**

Si el smoke reveló algún ajuste, corregir, re-testear y commitear. Si todo pasó sin cambios, no hay nada que commitear; continuar al cierre.

---

## Criterios de aceptación (verificación final)

- `docker compose up` + migraciones: la tabla `check_ins` existe (Task 8 Step 2).
- Logueado, en `/check-in`: guardar responde 200 y persiste; recargar muestra los valores; re-guardar el mismo día **actualiza** (no duplica) — verificado en Task 4 (test) y Task 8 (e2e).
- El historial lista los check-ins recientes descendente (Task 2/3 tests + Task 8).
- Un usuario nunca ve los check-ins de otro (Task 4 `TestUserIsolation` + Task 8).
- Sin sesión, `/check-in` redirige a `/login`; la API responde 401 (página + `TestRequiresAuth` + Task 8 Step 5).
- `go build/vet/test` y `tsc/vitest` en verde (Task 5 Step 2, Task 7 Step 6, Task 8 Step 7).

## Cierre

Tras completar todas las tareas con sus reviews, usar **superpowers:finishing-a-development-branch** para decidir el destino de la rama `feat/plan-2-checkin` (merge a `master`, PR, o conservar).
