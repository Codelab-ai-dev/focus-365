# Focus 365

Sistema personal 360° para gobernar las 4 dimensiones: metas, finanzas, entrenamiento, mente/disciplina y mentalidad. Con asistente de IA (Groq) que analiza tus datos y te acompaña.

> Evolución del vault de Obsidian "Capitanes de Dios" a un sistema real, escalable y propio.

## Stack

- **Frontend:** React + Vite + TanStack Router + TanStack Query + Tailwind + shadcn/ui (SPA estática)
- **Backend:** Go — chi · pgx · sqlc · goose · JWT + bcrypt · validator
- **DB:** Postgres
- **IA:** Groq (API compatible OpenAI), key solo en backend
- **Deploy:** Coolify (todo dockerizado)

## Estructura

```
focus-365/
├── api/            # Go API
│   ├── cmd/server/ # entrypoint
│   ├── internal/   # dominio por módulo (handlers, auth, ...)
│   └── db/
│       ├── migrations/ # goose
│       └── queries/    # sqlc (.sql) → Go type-safe
├── web/            # React + Vite + TanStack
└── docker-compose.yml
```

## Desarrollo local

```bash
# Todo junto (API + web + Postgres)
docker compose up --build

# o por separado:
cd api && go run ./cmd/server      # API en :8080
cd web && npm install && npm run dev  # web en :5173
```

- API: http://localhost:8080/api/v1/health
- Web: http://localhost:5173

## Tests

```bash
# Backend (Go). Requiere Postgres de pruebas en :5544 (docker compose up db).
cd api && make check        # vet + test
cd api && make test         # solo tests

# Frontend
cd web && npm test
```

> Los tests del API usan `-p 1` (un paquete a la vez): comparten una sola DB de
> pruebas y cada paquete trunca `users`, así que correrlos en paralelo se pisan
> entre sí. El `Makefile` ya lo aplica.

## Módulos (v1)

Check-in diario (4D) · Finanzas (superávit por ciclo de pago) · Entrenamiento (rutina + log + progresiones) · Mente/Disciplina (retos + rachas) · Metas · Dashboard · Asistente IA.
