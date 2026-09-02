# Fpan

Fpan is a small self-hosted file service with a Go API and a React Router frontend. The backend stores metadata in PostgreSQL and file contents in a local blob directory.

## Development environment

The repository provides a pinned Go, Node.js, and pnpm environment through `nix develop`.

Create a PostgreSQL database and copy the backend environment template:

```bash
cp backend/.env.example backend/.env
```

Set `FPAN_DATABASE_URL` in `backend/.env` to a working PostgreSQL connection. For local development without an OIDC provider, use mock authentication and remove the `FPAN_OIDC_*` values:

```dotenv
FPAN_AUTH_MODE=mock
FPAN_LISTEN_ADDR=127.0.0.1:6313
```

Mock authentication is rejected unless the backend listens on `localhost`, a
`127.0.0.0/8` address, or `::1`.

Start the API in the first terminal from the repository root:

```bash
nix develop
cd backend
go run ./cmd/fpan
```

The API listens on `http://localhost:6313` by default. Liveness and readiness checks are available at `/healthz` and `/readyz`.

In a second terminal, enter the development environment and install the frontend dependencies on the first run:

```bash
nix develop
cd frontend
cp .env.example .env
pnpm install --frozen-lockfile
```

`frontend/.env` defaults to proxying API requests to `http://127.0.0.1:6313`. Change `FPAN_API_PROXY_TARGET` there if the backend uses another origin.

Then start the React Router development server in that terminal:

```bash
pnpm dev
```

The frontend is available at `http://localhost:5173`.

## Frontend development

The top-level `frontend/` application uses React 19, React Router 8 in SPA mode, TypeScript, Tailwind CSS, and daisyUI. Its available package scripts are:

```bash
pnpm dev
pnpm build
pnpm start
pnpm typecheck
pnpm lint
pnpm format
pnpm check
```

The Vite development server proxies `/api` to `FPAN_API_PROXY_TARGET`, preserving relative `/api/v1` browser URLs. Keeping API calls on the frontend origin lets the browser use the HttpOnly session cookie without enabling backend CORS. Opening `http://localhost:5173/api/v1/auth/login` completes the mock login flow.

For OIDC development, register `http://localhost:5173/api/v1/auth/callback` with the local provider and use the same value for `FPAN_OIDC_REDIRECT_URL`.

The HTTP contract is defined by [`api/openapi.yaml`](api/openapi.yaml). OpenAPI-generated frontend types and the production handoff from React Router's `build/` output to the Go application have not been implemented yet.

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

Frontend checks currently provided by `frontend/package.json` are:

```bash
cd frontend
pnpm check
```

The HTTP contract is defined by [`api/openapi.yaml`](api/openapi.yaml). Update it together with any API behavior change.
