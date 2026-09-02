# Fpan frontend

The Fpan frontend uses React 19, React Router 8 in SPA mode (`ssr: false`), TypeScript, Tailwind CSS, and daisyUI.

## Development

Enter the repository's pinned development environment from the repository root:

```bash
nix develop
```

Install dependencies on the first run:

```bash
cd frontend
cp .env.example .env
pnpm install --frozen-lockfile
```

`FPAN_API_PROXY_TARGET` in `.env` controls the backend origin used by the development proxy and defaults to `http://127.0.0.1:6313`.

Start the development server with hot module replacement:

```bash
pnpm dev
```

The application is available at `http://localhost:5173`.

The Go API runs separately on port 6313. The Vite development server proxies `/api` requests to `FPAN_API_PROXY_TARGET`, allowing relative `/api/v1` requests and the browser login flow to use the frontend origin.

## Commands

- `pnpm dev` starts the development server.
- `pnpm build` creates the React Router production output under `build/`.
- `pnpm start` serves the generated production build.
- `pnpm typecheck` generates React Router types and runs TypeScript checking.
- `pnpm lint` checks the project with Oxlint; `pnpm lint:fix` applies safe fixes.
- `pnpm format` formats the project with Oxfmt; `pnpm format:check` only checks it.
- `pnpm check` runs formatting, lint, type checking, and the production build.

No frontend test runner is configured yet.

## API integration

[`../api/openapi.yaml`](../api/openapi.yaml) is the HTTP contract. Frontend API types are not generated from it yet. Browser requests should ultimately use relative `/api/v1` URLs so authentication can use the backend's HttpOnly session cookie through the frontend origin.

For local authentication behavior and backend setup, see the [repository README](../README.md).
