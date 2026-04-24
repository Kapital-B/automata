# svc/ — Go API

HTTP service per [docs/specs/initial.md](../docs/specs/initial.md): **app auth** (email/password, Microsoft, Google), **mailbox** connect (Microsoft Graph), SQLite storage, JWT API access.

## Requirements

- Go **1.22+**
- Microsoft Entra app: **mail** redirect `MS_REDIRECT_URI` (e.g. `http://localhost:8080/api/accounts/callback`) with delegated `Mail.Read`, `Mail.Send`, `User.Read`, `offline_access`.
- Same app (or another) **sign-in**: add redirect `MS_AUTH_REDIRECT_URI` (default `http://localhost:8080/api/auth/microsoft/callback`) with delegated `openid`, `offline_access`, `email`, `profile` (v2 endpoint).
- **Google** (optional): OAuth Web client with redirect `GOOGLE_REDIRECT_URI` (default `APP_PUBLIC_URL/api/auth/google/callback`).

## Run

```bash
cd svc
cp .env.example .env
# edit .env — MS_*, ENCRYPTION_KEY (32 chars), JWT_SECRET (≥32 chars)

set -a && source .env && set +a
go run ./cmd/server
```

- **Health:** `GET http://localhost:8080/api/health`
- **Start OAuth:** `POST /api/accounts` with body `{"provider":"m365","ms_account_kind":"work"}` or `"personal"`
- Open **`authorization_url`** in a browser: use the URL from **parsed** JSON (e.g. `jq -r .authorization_url`) so query parameters stay intact. Pasting from raw JSON that contains `\u0026` instead of `&` can break the request and trigger Azure errors such as **AADSTS900144** (missing `scope`).
- Microsoft redirects to `GET /api/accounts/callback?code=...&state=...`, then **302** to `{DASHBOARD_BASE_URL}{OAUTH_SUCCESS_PATH}?account_id=...`
- **Sync inbox:** `POST /api/accounts/{id}/sync`
- **List messages:** `GET /api/messages?account_id={uuid}`

## Auth

| Method | Endpoint |
| ------ | -------- |
| Register | `POST /api/auth/register` `{"email","password"}` → `user_id`, `access_token`, `refresh_token` |
| Login | `POST /api/auth/login` `{"email","password"}` → `access_token`, `refresh_token` |
| Refresh | `POST /api/auth/refresh` `{"refresh_token":"..."}` → new `access_token` + `refresh_token` (rotation) |
| Microsoft login | `GET /api/auth/microsoft` → `authorization_url`; callback redirects to `DASHBOARD_BASE_URL` + `AUTH_SUCCESS_PATH#access_token=...&refresh_token=...` (URL **fragment**, not query—keeps tokens out of server logs/Referer) |
| Google login | `GET /api/auth/google` (if configured) → same fragment redirect for `/api/auth/google/callback` |
| Current user | `GET /api/me` with `Authorization: Bearer <access_token>` |

**Linking:** Microsoft/Google sign-in uses the IdP **email** claim. If a user already exists with that email (e.g. registered with password), the external identity is **attached** to the same `users` row (`user_identities`).

## Dev fallback user

If a request has **no** `Authorization: Bearer` header, middleware treats the caller as **`AUTH_DEFAULT_USER_ID`** (default matches the seeded `dev@localhost` user from migration `002_users_auth.sql`). Use a real Bearer token in staging/production.

## Mail (unchanged flow, now per logged-in user)

Send `Authorization: Bearer <jwt>` (or omit in dev for default user).

- `POST /api/accounts` — start mailbox OAuth (state ties connection to JWT user).
- `GET /api/accounts/callback` — Microsoft returns here; redirects to `OAUTH_SUCCESS_PATH?account_id=...`
- `GET /api/accounts`, `POST .../sync`, `GET /api/messages`, etc. — scoped to the authenticated user.

Open **`authorization_url`** from parsed JSON (`jq -r .authorization_url`) so `&` in query strings is not broken.

## Layout

Hexagonal structure under `internal/`: `domain/`, `application/`, `adapters/` (see spec §2.4).

## Tests

```bash
go test ./...
```
