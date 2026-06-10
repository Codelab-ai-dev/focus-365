# Plan 1 — Cimientos + Auth · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Conectar la API Go a Postgres y entregar autenticación completa (registro, login, refresh, ruta protegida) con un frontend React temado "Warm Discipline" y rutas protegidas.

**Architecture:** Backend Go con chi: capa de config (env), pool pgx, queries tipadas con sqlc, dominio de auth (bcrypt + JWT) y middleware que inyecta `user_id` en el contexto. Toda query se hará scoping por usuario. Frontend React SPA con TanStack Router + Query: cliente de API con access token en memoria y refresh por cookie httpOnly, páginas de login/registro y guard de rutas.

**Tech Stack:** Go 1.23 (chi, jackc/pgx/v5, golang-jwt/jwt/v5, x/crypto/bcrypt, pressly/goose/v3, go-playground/validator/v10, google/uuid) · sqlc · Postgres 16 · React 18 + Vite + TanStack Router/Query + Tailwind · Vitest + Testing Library.

---

## File Structure

**Backend (`api/`):**
- `internal/config/config.go` — carga y valida variables de entorno. Responsabilidad única: configuración.
- `internal/db/db.go` — crea `*pgxpool.Pool`. Conexión.
- `internal/db/migrate.go` — corre migraciones goose (usado por el comando `migrate` y por los tests).
- `cmd/migrate/main.go` — binario que aplica migraciones.
- `db/queries/users.sql` — queries sqlc de usuarios.
- `sqlc.yaml` — config de generación.
- `internal/store/*` — código generado por sqlc (no se edita a mano).
- `internal/auth/password.go` + `password_test.go` — hash/verify bcrypt.
- `internal/auth/token.go` + `token_test.go` — emisión/validación JWT (access + refresh).
- `internal/auth/service.go` — lógica Register/Login (usa store, password, token).
- `internal/auth/handler.go` + `handler_test.go` — handlers HTTP + montaje de rutas.
- `internal/auth/middleware.go` + `middleware_test.go` — guard que inyecta `user_id`.
- `internal/auth/context.go` — helpers de contexto (`UserIDFromContext`).
- `internal/server/server.go` (modificar) — recibe dependencias y monta `/api/v1/auth`.
- `cmd/server/main.go` (modificar) — arma config + db + server.
- `internal/testutil/db.go` — helper de test DB (migra + trunca).

**Frontend (`web/`):**
- `vitest.config.ts`, `src/setupTests.ts` — testing.
- `tailwind.config.js` (modificar) + `src/index.css` (modificar) — tema Warm Discipline.
- `src/lib/api.ts` + `src/lib/api.test.ts` — cliente fetch con token + refresh.
- `src/lib/auth.tsx` — contexto de auth (estado de sesión).
- `src/routes/` — `__root.tsx`, `login.tsx`, `register.tsx`, `index.tsx` (dashboard placeholder protegido).
- `src/router.tsx` — instancia del router.
- `src/main.tsx` (modificar) — RouterProvider + QueryClient + AuthProvider.
- `src/App.tsx` — **eliminar** (reemplazado por rutas).

---

## Task 1: Config loader

**Files:**
- Create: `api/internal/config/config.go`
- Test: `api/internal/config/config_test.go`

- [ ] **Step 1: Add Go dependencies**

Run from `api/`:
```bash
cd api
go get github.com/jackc/pgx/v5@v5.7.1
go get github.com/golang-jwt/jwt/v5@v5.2.1
go get golang.org/x/crypto@v0.31.0
go get github.com/pressly/goose/v3@v3.22.1
go get github.com/go-playground/validator/v10@v10.23.0
go get github.com/google/uuid@v1.6.0
```
Expected: `go.mod` updated, no errors.

- [ ] **Step 2: Write the failing test**

`api/internal/config/config_test.go`:
```go
package config

import (
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "x")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadDefaultsAndValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("API_PORT", "")
	t.Setenv("CORS_ORIGIN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("default port = %q, want 8080", cfg.Port)
	}
	if cfg.CORSOrigin != "http://localhost:5173" {
		t.Errorf("default CORS = %q", cfg.CORSOrigin)
	}
	if cfg.DatabaseURL != "postgres://localhost/focus" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd api && go test ./internal/config/`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 4: Write minimal implementation**

`api/internal/config/config.go`:
```go
package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	CORSOrigin  string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        os.Getenv("API_PORT"),
		CORSOrigin:  os.Getenv("CORS_ORIGIN"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "http://localhost:5173"
	}
	return cfg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd api && go test ./internal/config/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/go.mod api/go.sum api/internal/config/
git commit -m "feat(api): config loader con validación de env"
```

---

## Task 2: DB connection + migration runner

**Files:**
- Create: `api/internal/db/db.go`
- Create: `api/internal/db/migrate.go`
- Create: `api/cmd/migrate/main.go`

> No test unitario aquí (es I/O puro de infraestructura); se valida vía el helper de test en Task 3 y el smoke de Task 14.

- [ ] **Step 1: Write the pool constructor**

`api/internal/db/db.go`:
```go
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
```

- [ ] **Step 2: Write the migration runner**

`api/internal/db/migrate.go`:
```go
package db

import (
	"database/sql"

	"github.com/pressly/goose/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// RunMigrations aplica todas las migraciones "up" del directorio dado.
func RunMigrations(databaseURL, dir string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(sqlDB, dir)
}
```

- [ ] **Step 3: Write the migrate command**

`api/cmd/migrate/main.go`:
```go
package main

import (
	"log"
	"os"

	"github.com/focus365/api/internal/db"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if err := db.RunMigrations(url, "db/migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Println("migrations applied")
}
```

- [ ] **Step 4: Verify it builds**

Run: `cd api && go build ./...`
Expected: build OK, no errors.

- [ ] **Step 5: Commit**

```bash
git add api/internal/db/ api/cmd/migrate/ api/go.mod api/go.sum
git commit -m "feat(api): pool pgx y runner de migraciones goose"
```

---

## Task 3: sqlc store de usuarios

**Files:**
- Create: `api/sqlc.yaml`
- Create: `api/db/queries/users.sql`
- Create: `api/internal/store/*` (generado)
- Create: `api/internal/testutil/db.go`
- Test: `api/internal/store/users_test.go`

- [ ] **Step 1: Write sqlc config**

`api/sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "db/migrations"
    queries: "db/queries"
    gen:
      go:
        package: "store"
        out: "internal/store"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_types: true
```

- [ ] **Step 2: Write the user queries**

`api/db/queries/users.sql`:
```sql
-- name: CreateUser :one
INSERT INTO users (email, password_hash, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;
```

- [ ] **Step 3: Write the test DB helper**

`api/internal/testutil/db.go`:
```go
package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/focus365/api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDB devuelve un pool contra TEST_DATABASE_URL, corre migraciones y
// trunca las tablas. Hace t.Skip si la variable no está definida.
func NewDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida; se omite test de base de datos")
	}
	if err := db.RunMigrations(url, "../../db/migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.NewPool(context.Background(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE users CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}
```

- [ ] **Step 4: Write the failing test**

`api/internal/store/users_test.go`:
```go
package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestCreateAndGetUser(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	created, err := q.CreateUser(ctx, store.CreateUserParams{
		Email:        "a@b.com",
		PasswordHash: "hash",
		Name:         "Ana",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Email != "a@b.com" {
		t.Errorf("email = %q", created.Email)
	}

	byEmail, err := q.GetUserByEmail(ctx, "a@b.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != created.ID {
		t.Errorf("id mismatch")
	}

	byID, err := q.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID.Name != "Ana" {
		t.Errorf("name = %q", byID.Name)
	}
}
```

- [ ] **Step 5: Run to verify it fails**

Run: `cd api && go test ./internal/store/`
Expected: FAIL — el paquete `store` no existe / no compila.

- [ ] **Step 6: Generate the store**

Run (instala sqlc si hace falta):
```bash
cd api
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate
go mod tidy
```
Expected: crea `internal/store/db.go`, `internal/store/models.go`, `internal/store/users.sql.go`.

- [ ] **Step 7: Start a local Postgres for tests**

Run (desde la raíz del repo):
```bash
docker compose up -d db
export TEST_DATABASE_URL="postgres://focus:focus@localhost:5432/focus?sslmode=disable"
```
> Nota: las credenciales/puerto deben coincidir con `docker-compose.yml`. Ajusta si difieren.

- [ ] **Step 8: Run to verify it passes**

Run: `cd api && go test ./internal/store/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add api/sqlc.yaml api/db/queries/ api/internal/store/ api/internal/testutil/ api/go.mod api/go.sum
git commit -m "feat(api): store de usuarios con sqlc + helper de test DB"
```

---

## Task 4: Password hashing (bcrypt)

**Files:**
- Create: `api/internal/auth/password.go`
- Test: `api/internal/auth/password_test.go`

- [ ] **Step 1: Write the failing test**

`api/internal/auth/password_test.go`:
```go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "s3cret!" {
		t.Fatal("hash igual al texto plano")
	}
	if !VerifyPassword(hash, "s3cret!") {
		t.Error("contraseña correcta debió verificar")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("contraseña incorrecta no debió verificar")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/auth/`
Expected: FAIL — `undefined: HashPassword`.

- [ ] **Step 3: Write minimal implementation**

`api/internal/auth/password.go`:
```go
package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/auth/ -run TestHashAndVerifyPassword`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/auth/password.go api/internal/auth/password_test.go
git commit -m "feat(api): hashing de contraseñas con bcrypt"
```

---

## Task 5: JWT tokens (access + refresh)

**Files:**
- Create: `api/internal/auth/token.go`
- Test: `api/internal/auth/token_test.go`

- [ ] **Step 1: Write the failing test**

`api/internal/auth/token_test.go`:
```go
package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestIssueAndParseAccess(t *testing.T) {
	tm := NewTokenManager("test-secret")
	id := uuid.New()

	tok, err := tm.IssueAccess(id)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := tm.ParseAccess(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}

func TestParseAccessRejectsWrongSecret(t *testing.T) {
	tok, _ := NewTokenManager("a").IssueAccess(uuid.New())
	if _, err := NewTokenManager("b").ParseAccess(tok); err == nil {
		t.Error("token firmado con otro secreto debió fallar")
	}
}

func TestRefreshRoundTrip(t *testing.T) {
	tm := NewTokenManager("test-secret")
	id := uuid.New()
	tok, err := tm.IssueRefresh(id)
	if err != nil {
		t.Fatalf("issue refresh: %v", err)
	}
	got, err := tm.ParseRefresh(tok)
	if err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/auth/ -run Token`
Expected: FAIL — `undefined: NewTokenManager`.

- [ ] **Step 3: Write minimal implementation**

`api/internal/auth/token.go`:
```go
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
)

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{secret: []byte(secret)}
}

func (tm *TokenManager) issue(userID uuid.UUID, ttl time.Duration, kind string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"typ": kind,
		"exp": time.Now().Add(ttl).Unix(),
		"iat": time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(tm.secret)
}

func (tm *TokenManager) IssueAccess(id uuid.UUID) (string, error) {
	return tm.issue(id, accessTTL, "access")
}

func (tm *TokenManager) IssueRefresh(id uuid.UUID) (string, error) {
	return tm.issue(id, refreshTTL, "refresh")
}

func (tm *TokenManager) parse(tokenStr, kind string) (uuid.UUID, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de firma inesperado")
		}
		return tm.secret, nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, errors.New("token inválido")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != kind {
		return uuid.Nil, errors.New("tipo de token inválido")
	}
	sub, _ := claims["sub"].(string)
	return uuid.Parse(sub)
}

func (tm *TokenManager) ParseAccess(tokenStr string) (uuid.UUID, error) {
	return tm.parse(tokenStr, "access")
}

func (tm *TokenManager) ParseRefresh(tokenStr string) (uuid.UUID, error) {
	return tm.parse(tokenStr, "refresh")
}

func AccessTTL() time.Duration  { return accessTTL }
func RefreshTTL() time.Duration { return refreshTTL }
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/auth/ -run Token`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/auth/token.go api/internal/auth/token_test.go
git commit -m "feat(api): JWT access y refresh con TokenManager"
```

---

## Task 6: Auth service (Register/Login)

**Files:**
- Create: `api/internal/auth/service.go`
- Test: `api/internal/auth/service_test.go`

- [ ] **Step 1: Write the failing test**

`api/internal/auth/service_test.go`:
```go
package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func newService(t *testing.T) *auth.Service {
	pool := testutil.NewDB(t)
	return auth.NewService(store.New(pool), auth.NewTokenManager("test-secret"))
}

func TestRegisterThenLogin(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, "user@focus.com", "p4ssword", "Gus")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Email != "user@focus.com" {
		t.Errorf("email = %q", user.Email)
	}

	logged, err := svc.Login(ctx, "user@focus.com", "p4ssword")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if logged.ID != user.ID {
		t.Errorf("login devolvió otro usuario")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "dup@focus.com", "p4ssword", "A")
	if _, err := svc.Register(ctx, "dup@focus.com", "p4ssword", "B"); err == nil {
		t.Error("email duplicado debió fallar")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "x@focus.com", "right", "X")
	if _, err := svc.Login(ctx, "x@focus.com", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/auth/ -run "Register|Login"`
Expected: FAIL — `undefined: auth.NewService`.

- [ ] **Step 3: Write minimal implementation**

`api/internal/auth/service.go`:
```go
package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrEmailTaken         = errors.New("el email ya está registrado")
)

type Service struct {
	q      *store.Queries
	tokens *TokenManager
}

func NewService(q *store.Queries, tm *TokenManager) *Service {
	return &Service{q: q, tokens: tm}
}

func (s *Service) Register(ctx context.Context, email, password, name string) (store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := s.q.GetUserByEmail(ctx, email); err == nil {
		return store.User{}, ErrEmailTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	return s.q.CreateUser(ctx, store.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		Name:         name,
	})
}

func (s *Service) Login(ctx context.Context, email, password string) (store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return store.User{}, ErrInvalidCredentials
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return store.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) IssueTokens(id uuid.UUID) (access, refresh string, err error) {
	access, err = s.tokens.IssueAccess(id)
	if err != nil {
		return "", "", err
	}
	refresh, err = s.tokens.IssueRefresh(id)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (s *Service) Tokens() *TokenManager { return s.tokens }
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/auth/ -run "Register|Login"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/auth/service.go api/internal/auth/service_test.go
git commit -m "feat(api): servicio de auth (register/login/issue tokens)"
```

---

## Task 7: Auth context helpers + middleware

**Files:**
- Create: `api/internal/auth/context.go`
- Create: `api/internal/auth/middleware.go`
- Test: `api/internal/auth/middleware_test.go`

- [ ] **Step 1: Write the failing test**

`api/internal/auth/middleware_test.go`:
```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/google/uuid"
)

func TestRequireAuthAllowsValidToken(t *testing.T) {
	tm := auth.NewTokenManager("secret")
	id := uuid.New()
	tok, _ := tm.IssueAccess(id)

	var gotID uuid.UUID
	h := auth.RequireAuth(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, _ = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if gotID != id {
		t.Errorf("user id no inyectado")
	}
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	tm := auth.NewTokenManager("secret")
	h := auth.RequireAuth(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/auth/ -run RequireAuth`
Expected: FAIL — `undefined: auth.RequireAuth`.

- [ ] **Step 3: Write the context helpers**

`api/internal/auth/context.go`:
```go
package auth

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

func withUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}
```

- [ ] **Step 4: Write the middleware**

`api/internal/auth/middleware.go`:
```go
package auth

import (
	"net/http"
	"strings"
)

func RequireAuth(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"no autorizado"}`, http.StatusUnauthorized)
				return
			}
			id, err := tm.ParseAccess(parts[1])
			if err != nil {
				http.Error(w, `{"error":"no autorizado"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), id)))
		})
	}
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd api && go test ./internal/auth/ -run RequireAuth`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/auth/context.go api/internal/auth/middleware.go api/internal/auth/middleware_test.go
git commit -m "feat(api): middleware RequireAuth e inyección de user_id"
```

---

## Task 8: Auth HTTP handlers + routes

**Files:**
- Create: `api/internal/auth/handler.go`
- Test: `api/internal/auth/handler_test.go`

- [ ] **Step 1: Write the failing test**

`api/internal/auth/handler_test.go`:
```go
package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func newHandler(t *testing.T) http.Handler {
	pool := testutil.NewDB(t)
	svc := auth.NewService(store.New(pool), auth.NewTokenManager("secret"))
	return auth.Routes(svc)
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterEndpoint(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/register", map[string]string{
		"email": "new@focus.com", "password": "p4ssword", "name": "Gus",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["access_token"] == nil {
		t.Error("falta access_token en la respuesta")
	}
}

func TestLoginEndpoint(t *testing.T) {
	h := newHandler(t)
	_ = postJSON(t, h, "/register", map[string]string{
		"email": "log@focus.com", "password": "p4ssword", "name": "G",
	})
	rec := postJSON(t, h, "/login", map[string]string{
		"email": "log@focus.com", "password": "p4ssword",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLoginBadCredentials(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/login", map[string]string{
		"email": "nope@focus.com", "password": "x",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/register", map[string]string{
		"email": "not-an-email", "password": "123", "name": "",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/auth/ -run "Endpoint|Validation|BadCredentials"`
Expected: FAIL — `undefined: auth.Routes`.

- [ ] **Step 3: Write minimal implementation**

`api/internal/auth/handler.go`:
```go
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

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
		if !decodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Register(r.Context(), req.Email, req.Password, req.Name)
		if err != nil {
			if errors.Is(err, ErrEmailTaken) {
				writeErr(w, http.StatusConflict, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		respondWithTokens(w, svc, user, http.StatusCreated)
	}
}

func handleLogin(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if !decodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleRefresh(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "sin refresh token")
			return
		}
		id, err := svc.Tokens().ParseRefresh(cookie.Value)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "refresh inválido")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "usuario no encontrado")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleMe(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromContext(r.Context())
		if !ok {
			writeErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "usuario no encontrado")
			return
		}
		writeJSON(w, http.StatusOK, toView(user))
	}
}

func respondWithTokens(w http.ResponseWriter, svc *Service, user store.User, status int) {
	access, refresh, err := svc.IssueTokens(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error emitiendo tokens")
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
	writeJSON(w, status, authResp{AccessToken: access, User: toView(user)})
}

func toView(u store.User) userView {
	return userView{ID: u.ID.String(), Email: u.Email, Name: u.Name}
}

func decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	if err := validate.Struct(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "datos inválidos")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

> Nota: `handler.go` accede a `svc.q` (campo no exportado) — está bien porque viven en el mismo paquete `auth`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/auth/`
Expected: PASS (todos los tests del paquete auth).

- [ ] **Step 5: Commit**

```bash
git add api/internal/auth/handler.go api/internal/auth/handler_test.go
git commit -m "feat(api): handlers de auth (register/login/refresh/me)"
```

---

## Task 9: Wire server + main

**Files:**
- Modify: `api/internal/server/server.go`
- Modify: `api/cmd/server/main.go`

- [ ] **Step 1: Update server.go to accept dependencies**

Reemplaza el contenido de `api/internal/server/server.go` por:
```go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Pool       *pgxpool.Pool
	JWTSecret  string
	CORSOrigin string
}

func New(d Deps) http.Handler {
	q := store.New(d.Pool)
	tm := auth.NewTokenManager(d.JWTSecret)
	authSvc := auth.NewService(q, tm)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors(d.CORSOrigin))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health)
		r.Mount("/auth", auth.Routes(authSvc))
	})

	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "focus-365-api",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cors(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2: Update main.go**

Reemplaza el contenido de `api/cmd/server/main.go` por:
```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/focus365/api/internal/config"
	"github.com/focus365/api/internal/db"
	"github.com/focus365/api/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	h := server.New(server.Deps{
		Pool:       pool,
		JWTSecret:  cfg.JWTSecret,
		CORSOrigin: cfg.CORSOrigin,
	})

	log.Printf("Focus 365 API escuchando en :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, h); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 3: Verify it builds and unit tests pass**

Run: `cd api && go build ./... && go test ./...`
Expected: build OK; tests PASS (los de DB se omiten si `TEST_DATABASE_URL` no está, pasan si está).

- [ ] **Step 4: Commit**

```bash
git add api/internal/server/server.go api/cmd/server/main.go
git commit -m "feat(api): cableado de db + auth en server y main"
```

---

## Task 10: Frontend testing setup

**Files:**
- Modify: `web/package.json`
- Create: `web/vitest.config.ts`
- Create: `web/src/setupTests.ts`

- [ ] **Step 1: Install dev deps**

Run from `web/`:
```bash
cd web
npm install -D vitest@^2.1.0 jsdom@^25.0.0 @testing-library/react@^16.0.0 @testing-library/jest-dom@^6.5.0 @testing-library/user-event@^14.5.0
```

- [ ] **Step 2: Add test script**

En `web/package.json`, dentro de `"scripts"`, agrega:
```json
    "test": "vitest run",
    "test:watch": "vitest"
```

- [ ] **Step 3: Write vitest config**

`web/vitest.config.ts`:
```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/setupTests.ts"],
  },
});
```

- [ ] **Step 4: Write setup file**

`web/src/setupTests.ts`:
```ts
import "@testing-library/jest-dom/vitest";
```

- [ ] **Step 5: Verify the runner works**

Run: `cd web && npx vitest run`
Expected: "No test files found" (exit 0 o mensaje sin fallos). Si exige al menos un test, continúa con Task 11.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/setupTests.ts
git commit -m "chore(web): configura Vitest + Testing Library"
```

---

## Task 11: Tema Warm Discipline

**Files:**
- Modify: `web/tailwind.config.js`
- Modify: `web/src/index.css`

> Paleta B: carbón cálido + ámbar/oro. Acentos: verde dinero, naranja racha, violeta-ámbar IA.

- [ ] **Step 1: Extend Tailwind colors**

Reemplaza `web/tailwind.config.js` por:
```js
/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: {
          950: "#13110f",
          900: "#1c1814",
          800: "#241f1a",
          700: "#2a2520",
        },
        sand: {
          100: "#f5ede0",
          400: "#a89b8c",
        },
        amber: { brand: "#e0a458" },
        money: "#5ca86b",
        streak: "#e8763e",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
};
```

- [ ] **Step 2: Update base styles**

Reemplaza `web/src/index.css` por:
```css
@import url("https://fonts.googleapis.com/css2?family=Inter:wght@400;500;700;800&display=swap");
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  html, body, #root {
    @apply h-full;
  }
  body {
    @apply bg-ink-950 text-sand-100 font-sans antialiased;
  }
}
```

- [ ] **Step 3: Verify build compiles styles**

Run: `cd web && npx tsc --noEmit`
Expected: sin errores de tipos.

- [ ] **Step 4: Commit**

```bash
git add web/tailwind.config.js web/src/index.css
git commit -m "feat(web): tema Warm Discipline (paleta + tipografía)"
```

---

## Task 12: Cliente de API + token store

**Files:**
- Create: `web/src/lib/api.ts`
- Test: `web/src/lib/api.test.ts`

- [ ] **Step 1: Write the failing test**

`web/src/lib/api.test.ts`:
```ts
import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { setAccessToken, getAccessToken, apiFetch } from "./api";

describe("token store", () => {
  beforeEach(() => setAccessToken(null));

  it("guarda y lee el access token", () => {
    expect(getAccessToken()).toBeNull();
    setAccessToken("abc");
    expect(getAccessToken()).toBe("abc");
  });
});

describe("apiFetch", () => {
  afterEach(() => vi.restoreAllMocks());

  it("agrega el header Authorization cuando hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/v1/health");

    const headers = (fetchMock.mock.calls[0][1] as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("lanza ApiError en respuesta no-ok", async () => {
    setAccessToken(null);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "boom" }), { status: 400 })
      )
    );
    await expect(apiFetch("/x")).rejects.toThrowError("boom");
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/lib/api.test.ts`
Expected: FAIL — no existe `./api`.

- [ ] **Step 3: Write minimal implementation**

`web/src/lib/api.ts`:
```ts
let accessToken: string | null = null;

export function setAccessToken(token: string | null) {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function apiFetch<T = unknown>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string>),
  };
  if (accessToken) {
    headers["Authorization"] = `Bearer ${accessToken}`;
  }

  const res = await fetch(path, { ...options, headers, credentials: "include" });

  if (!res.ok) {
    let message = `Error ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(message, res.status);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && npx vitest run src/lib/api.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/api.test.ts
git commit -m "feat(web): cliente apiFetch con token en memoria"
```

---

## Task 13: Auth context + router + páginas

**Files:**
- Create: `web/src/lib/auth.tsx`
- Create: `web/src/routes/__root.tsx`
- Create: `web/src/routes/login.tsx`
- Create: `web/src/routes/register.tsx`
- Create: `web/src/routes/index.tsx`
- Create: `web/src/router.tsx`
- Modify: `web/src/main.tsx`
- Delete: `web/src/App.tsx`
- Test: `web/src/routes/login.test.tsx`

- [ ] **Step 1: Write the auth context**

`web/src/lib/auth.tsx`:
```tsx
import { createContext, useContext, useState, ReactNode, useCallback } from "react";
import { apiFetch, setAccessToken } from "./api";

export type User = { id: string; email: string; name: string };
type AuthResp = { access_token: string; user: User };

type AuthState = {
  user: User | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);

  const login = useCallback(async (email: string, password: string) => {
    const resp = await apiFetch<AuthResp>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    setAccessToken(resp.access_token);
    setUser(resp.user);
  }, []);

  const register = useCallback(async (email: string, password: string, name: string) => {
    const resp = await apiFetch<AuthResp>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password, name }),
    });
    setAccessToken(resp.access_token);
    setUser(resp.user);
  }, []);

  const logout = useCallback(() => {
    setAccessToken(null);
    setUser(null);
  }, []);

  return (
    <AuthContext.Provider value={{ user, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth debe usarse dentro de AuthProvider");
  return ctx;
}
```

- [ ] **Step 2: Write the root route**

`web/src/routes/__root.tsx`:
```tsx
import { createRootRoute, Outlet } from "@tanstack/react-router";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-ink-950 text-sand-100">
      <Outlet />
    </div>
  ),
});
```

- [ ] **Step 3: Write the login page (+ register/index)**

`web/src/routes/login.tsx`:
```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/login")({ component: LoginPage });

function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await login(email, password);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al iniciar sesión");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6">
        <h1 className="text-2xl font-extrabold">Focus 365</h1>
        <p className="text-sm text-sand-400">Inicia sesión para continuar.</p>
        <input
          aria-label="Email" type="email" placeholder="Email" value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        <input
          aria-label="Contraseña" type="password" placeholder="Contraseña" value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        {error && <p className="text-sm text-streak">{error}</p>}
        <button type="submit" className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950">
          Entrar
        </button>
        <p className="text-center text-xs text-sand-400">
          ¿Sin cuenta? <Link to="/register" className="text-amber-brand">Regístrate</Link>
        </p>
      </form>
    </div>
  );
}
```

`web/src/routes/register.tsx`:
```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/register")({ component: RegisterPage });

function RegisterPage() {
  const { register } = useAuth();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await register(email, password, name);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al registrarse");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6">
        <h1 className="text-2xl font-extrabold">Crea tu cuenta</h1>
        <input aria-label="Nombre" placeholder="Nombre" value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        <input aria-label="Email" type="email" placeholder="Email" value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        <input aria-label="Contraseña" type="password" placeholder="Contraseña (mín. 6)" value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        {error && <p className="text-sm text-streak">{error}</p>}
        <button type="submit" className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950">
          Crear cuenta
        </button>
        <p className="text-center text-xs text-sand-400">
          ¿Ya tienes cuenta? <Link to="/login" className="text-amber-brand">Inicia sesión</Link>
        </p>
      </form>
    </div>
  );
}
```

`web/src/routes/index.tsx` (dashboard placeholder protegido):
```tsx
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/")({ component: HomePage });

function HomePage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  if (!user) return null;

  return (
    <div className="p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Focus 365</h1>
        <button onClick={logout} className="text-sm text-sand-400">Salir</button>
      </header>
      <p className="mt-6 text-sand-400">
        Bienvenido, <span className="text-amber-brand">{user.name}</span>. El centro de mando llega en el siguiente plan.
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Write the router**

`web/src/router.tsx`:
```tsx
import { createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
```

- [ ] **Step 5: Update main.tsx and enable the router plugin**

Reemplaza `web/src/main.tsx` por:
```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { router } from "./router";
import { AuthProvider } from "./lib/auth";
import "./index.css";

const queryClient = new QueryClient();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  </React.StrictMode>
);
```

En `web/vite.config.ts`, agrega el plugin de rutas de TanStack ANTES de `react()`:
```ts
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
// ...
plugins: [TanStackRouterVite(), react()],
```

Borra el archivo `web/src/App.tsx`:
```bash
git rm web/src/App.tsx
```

- [ ] **Step 6: Generate the route tree**

Run: `cd web && npx vite build` (o `npm run dev` una vez) para que el plugin genere `src/routeTree.gen.ts`.
Expected: se crea `web/src/routeTree.gen.ts` y el build compila.

- [ ] **Step 7: Write the failing test**

`web/src/routes/login.test.tsx`:
```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import { AuthProvider } from "@/lib/auth";
import { Route as LoginRoute } from "./login";

function renderLogin() {
  const rootRoute = createRootRoute();
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: LoginRoute.options.component,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const registerRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/register",
    component: () => <div>register</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([loginRoute, homeRoute, registerRoute]),
    history: createMemoryHistory({ initialEntries: ["/login"] }),
  });
  render(
    <AuthProvider>
      {/* @ts-expect-error router de prueba */}
      <RouterProvider router={router} />
    </AuthProvider>
  );
}

describe("LoginPage", () => {
  it("muestra los campos de email y contraseña", async () => {
    renderLogin();
    expect(await screen.findByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña")).toBeInTheDocument();
  });

  it("muestra error cuando el login falla", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "credenciales inválidas" }), { status: 401 })
      )
    );
    renderLogin();
    await userEvent.type(await screen.findByLabelText("Email"), "x@y.com");
    await userEvent.type(screen.getByLabelText("Contraseña"), "bad");
    await userEvent.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByText("credenciales inválidas")).toBeInTheDocument();
    vi.restoreAllMocks();
  });
});
```

- [ ] **Step 8: Run to verify it passes**

Run: `cd web && npx vitest run src/routes/login.test.tsx`
Expected: PASS (la implementación de las páginas ya existe del paso 3).

- [ ] **Step 9: Full frontend check**

Run: `cd web && npx tsc --noEmit && npx vitest run`
Expected: sin errores de tipos; todos los tests PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/ web/vite.config.ts
git commit -m "feat(web): auth context, router y páginas login/registro"
```

---

## Task 14: Smoke end-to-end

**Files:** ninguno (validación manual).

- [ ] **Step 1: Levantar todo con Docker**

Run desde la raíz:
```bash
cp .env.example .env   # si no existe; ajusta credenciales si hace falta
docker compose up -d --build
```
Expected: contenedores `db`, `api`, `web` arriba.

- [ ] **Step 2: Aplicar migraciones**

Run:
```bash
docker compose exec api ./migrate 2>/dev/null || \
  (cd api && DATABASE_URL="postgres://focus:focus@localhost:5432/focus?sslmode=disable" go run ./cmd/migrate)
```
Expected: "migrations applied".

- [ ] **Step 3: Probar registro y login vía API**

Run:
```bash
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"gus@focus.com","password":"p4ssword","name":"Gustavo"}'
```
Expected: `HTTP/1.1 201`, body con `access_token` y `user`, y cabecera `Set-Cookie: refresh_token=...`.

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"gus@focus.com","password":"p4ssword"}'
```
Expected: `HTTP/1.1 200` con `access_token`.

- [ ] **Step 4: Probar la UI**

Abre http://localhost:5173 → debe redirigir a `/login`. Regístrate desde la UI → cae en `/` con el saludo y el tema oscuro cálido (Warm Discipline).

- [ ] **Step 5: Commit (si hubo ajustes de env/compose)**

```bash
git add -A
git commit -m "chore: validación e2e de auth (cimientos + auth completos)"
```

---

## Self-Review

**Spec coverage (sección → tarea):**
- §2 Stack/monorepo/Docker → ya en scaffold; Tasks 9, 14 lo cablean y validan.
- §2 DB pgx/sqlc/goose → Tasks 2, 3.
- §3 Modelo `users` con `user_id` scoping → tabla ya migrada; store Task 3; contexto/middleware Task 7.
- §4 Auth bcrypt + JWT access/refresh + middleware → Tasks 4, 5, 6, 7, 8.
- §6 UI estilo Warm Discipline + shell base → Tasks 11, 13 (login/registro/home; el centro de mando completo es Plan 6).
- §5 IA, §3 finanzas/check-in/hábitos/metas → **fuera de este plan** (Planes 2–7).

**Placeholder scan:** sin TBD/TODO; cada paso de código incluye el código real.

**Type consistency:** `store.New`, `store.Queries`, `store.CreateUserParams`, `store.User` (generados por sqlc) usados consistentemente; `NewTokenManager`/`IssueAccess`/`ParseAccess`/`IssueRefresh`/`ParseRefresh`, `NewService`/`Register`/`Login`/`IssueTokens`/`Tokens`, `RequireAuth`/`UserIDFromContext`, y en frontend `setAccessToken`/`getAccessToken`/`apiFetch`/`useAuth`/`AuthProvider` coinciden entre tareas.

**Nota de dependencia:** Tasks 3, 6, 8 requieren `TEST_DATABASE_URL` (Postgres local vía `docker compose up -d db`). Sin ella esos tests se omiten (no fallan).
