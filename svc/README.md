# svc/ — Go API (Phase 0–1)

HTTP service per [docs/specs/initial.md](../docs/specs/initial.md): health, Microsoft OAuth connect, inbox sync into SQLite, account and message APIs.

## Requirements

- Go **1.22+**
- Microsoft Entra app registration: **Accounts in any organizational directory and personal Microsoft accounts**, delegated scopes `Mail.Read`, `Mail.Send`, `User.Read`, `offline_access`, redirect URI = `MS_REDIRECT_URI` (must hit this service, e.g. `http://localhost:8080/api/accounts/callback`).

## Run

```bash
cd svc
cp .env.example .env
# edit .env — set MS_* and ENCRYPTION_KEY (32 chars)

set -a && source .env && set +a
go run ./cmd/server
```

- **Health:** `GET http://localhost:8080/api/health`
- **Start OAuth:** `POST /api/accounts` with body `{"provider":"m365","ms_account_kind":"work"}` or `"personal"`
- Open **`authorization_url`** in a browser: use the URL from **parsed** JSON (e.g. `jq -r .authorization_url`) so query parameters stay intact. Pasting from raw JSON that contains `\u0026` instead of `&` can break the request and trigger Azure errors such as **AADSTS900144** (missing `scope`).
- Microsoft redirects to `GET /api/accounts/callback?code=...&state=...`, then **302** to `{DASHBOARD_BASE_URL}{OAUTH_SUCCESS_PATH}?account_id=...`
- **Sync inbox:** `POST /api/accounts/{id}/sync`
- **List messages:** `GET /api/messages?account_id={uuid}`

## Layout

Hexagonal structure under `internal/`: `domain/`, `application/`, `adapters/` (see spec §2.4).

## Tests

```bash
go test ./...
```
