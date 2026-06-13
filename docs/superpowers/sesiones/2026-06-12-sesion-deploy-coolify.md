# Bitácora de sesión — Deploy a producción (Hostinger VPS + Coolify)

**Fecha:** 2026-06-12
**Estado al cierre:** En producción, smoke E2E 8/8.
**URL:** https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io (dominio sslip.io generado por Coolify, HTTPS vía Let's Encrypt/Traefik)
**Repo:** https://github.com/Codelab-ai-dev/focus-365 (rama `main`; nota: quedó público — verificado sin secretos en el historial; existe también un espejo inicial en `Gustavo-84/focus-365` privado, borrable)

## Qué se hizo

1. **Preparación de producción** (`8509de7`, `5e8b72a`):
   - `docker-compose.coolify.yml`: sin puertos de host (Traefik enruta el
     dominio a `web:80`; nginx proxea `/api` → `api:8080` internamente, mismo
     origen), DB sin exposición, secretos exigidos por variable (`:?`),
     `restart: unless-stopped`, `COOKIE_SECURE=true`.
   - **Cookies `Secure`**: nuevo flag `COOKIE_SECURE` (config + `auth.Routes`,
     con tests) — la cookie de refresh exige HTTPS en producción.
2. **Repo a GitHub**: primero `Gustavo-84/focus-365` (privado), luego el
   definitivo `Codelab-ai-dev/focus-365` (org del usuario, rama `main`).
   Auditoría de secretos antes del push: `.env` nunca commiteado, limpio.
3. **Coolify** (panel, hecho por el usuario con guía): GitHub App en la org →
   recurso Docker Compose apuntando a `/docker-compose.coolify.yml` →
   variables (`POSTGRES_PASSWORD`, `JWT_SECRET`, `GROQ_API_KEY`,
   `CORS_ORIGIN`) → dominio https en el servicio `web` → deploy.

## Bug de producción encontrado y arreglado (`b633377`)

**Síntoma:** `POST /auth/register` → 500. **Diagnóstico:** la DB conectaba
(el api hace ping al arrancar y estaba vivo) pero estaba **sin esquema**: las
migraciones vivían en `cmd/migrate`, que ningún paso del deploy ejecutaba. En
local nunca se notó porque la suite de tests (`testutil.NewDB`) migra la misma
DB de docker — las bitácoras anteriores decían «el api aplica migraciones al
arrancar» y era falso.

**Fix:** `cmd/server/main.go` llama a `db.RunMigrations` al arrancar (goose,
idempotente) y el Dockerfile copia `db/migrations` a la imagen. Verificado en
local (`goose: current version: 9`) y en producción tras redeploy.

## Verificación al cierre (smoke `/tmp/smoke_prod.sh` contra producción)

**8/8:** registro (DB migrada) → cookie refresh Secure presente → refresh 200
(sesión sobrevive recargas) → dashboard 200 → acción propuesta vía Groq real →
confirmar deja `done` → check-in escrito (mood 8) → streaming de texto vivo.
(Una corrida tuvo el flake conocido del modelo en el check de texto; re-corrida
limpia.)

## Pendientes / notas operativas

- Secretos de producción viven SOLO en el panel de Coolify.
- Auto-deploy por push activo vía GitHub App (push a `main` → redeploy).
- Si se compra dominio propio: cambiar el dominio del servicio `web` en
  Coolify + `CORS_ORIGIN`, cero cambios de código.
- Posible mejora: backups automáticos del volumen `dbdata` (Coolify tiene
  scheduled backups para Postgres si se migra la DB a un recurso own-database).
- El repo público ↔ privado queda a decisión del usuario.
