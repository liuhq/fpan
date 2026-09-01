# Fpan

Fpan is a small self-hosted file service with a Go API and a SolidJS frontend. The backend stores metadata in PostgreSQL and file contents in a local blob directory.

## Development environment

Enter the pinned Go, Node.js, and pnpm environment from the repository root:

```bash
nix develop
```

Create a PostgreSQL database, copy the backend environment template, and fill in the OIDC client values:

```bash
cp backend/.env.example backend/.env
cd backend
go run ./cmd/fpan
```

The API listens on `http://localhost:6313` by default. Liveness and readiness checks are available at `/healthz` and `/readyz`.

## Frontend handoff

The frontend is intentionally not scaffolded yet. When creating the top-level `frontend/` SolidJS application:

- Use relative `/api/v1` URLs in the browser.
- Configure Vite to proxy `/api` to `http://localhost:6313`.
- Register `http://localhost:5173/api/v1/auth/callback` with the local OIDC provider and use the same value for `FPAN_OIDC_REDIRECT_URL`.
- Redirect a JSON `401` response to `/api/v1/auth/login`; the session itself is stored in an HttpOnly cookie.
- Generate request and response types from [`api/openapi.yaml`](api/openapi.yaml), for example with `openapi-typescript`, and use `openapi-fetch` for the typed client.

Keeping API calls on the Vite origin lets the browser use the session cookie without enabling backend CORS. The production static-file embedding step should be added after the frontend produces a stable `dist/` directory.

## Validation

Backend checks:

```bash
cd backend
go test ./...
go vet ./...
golangci-lint run
```

The PostgreSQL integration tests are optional. They are skipped unless
`FPAN_TEST_DATABASE_URL` is set, so the regular test suite does not require a
database. The configured role needs `CONNECT` and `CREATE` privileges on the
database; each test creates and removes an isolated random schema without
touching existing application tables:

```bash
cd backend
FPAN_TEST_DATABASE_URL='postgres://horin@localhost:5432/horin?sslmode=disable' \
  go test ./internal/database -run Postgres -count=1
```

If the variable is set, connection, permission, migration, and cleanup errors
fail the test instead of being treated as a skip.

After the frontend is created, its expected checks are:

```bash
cd frontend
pnpm build
pnpm lint
pnpm format
pnpm test
```

The HTTP contract is defined by [`api/openapi.yaml`](api/openapi.yaml). Update it together with any API behavior change.
