# Repository Guidelines

## Project Structure & Module Organization

The Go backend lives in `backend/`. Its executable entry point is `backend/cmd/fpan`, while `backend/internal/` contains authentication, configuration, database, and model packages. Keep package-specific tests beside their implementation. `backend/frontend/` is currently a placeholder Go package for a future production frontend handoff; built assets are not embedded yet.

The React 19, React Router 8, and TypeScript application belongs in the top-level `frontend/` directory and runs in SPA mode. Treat `api/openapi.yaml` as the contract for HTTP endpoints and update it with API behavior changes. The root `flake.nix` defines the supported development toolchain.

## Build, Test, and Development Commands

Run `nix develop` from the repository root to enter the pinned Go, Node.js, and pnpm development environment.

- `cd backend && go run ./cmd/fpan` starts the API server.
- `cd backend && go test ./...` runs all Go tests.
- `cd backend && go vet ./...` performs standard Go static checks.
- `cd backend && golangci-lint run` runs the configured Go linters.
- `cd frontend && pnpm dev` starts the React Router development server.
- `cd frontend && pnpm build` creates the React Router production output.
- `cd frontend && pnpm start` serves the production build.
- `cd frontend && pnpm typecheck` generates route types and runs TypeScript checking.
- `cd frontend && pnpm lint` and `pnpm format` run Oxlint and Oxfmt.
- `cd frontend && pnpm check` runs all configured frontend checks and the production build.

## Coding Style & Naming Conventions

Format Go code with `gofmt`; use tabs, lowercase package names, PascalCase exported identifiers, and concise camelCase local names. Let Oxfmt and Oxlint define TypeScript formatting and lint policy. Name React components in PascalCase and ordinary modules with descriptive lowercase names. Keep API naming consistent with `api/openapi.yaml`.

## Testing Guidelines

Name Go test files `*_test.go` and test functions `TestXxx`. Prefer table-driven tests for multiple cases and add regression coverage for bug fixes. No frontend test runner or test script is configured yet; until one is added, validate frontend changes with `pnpm check`.

## Commit & Pull Request Guidelines

Use focused Conventional Commit subjects consistent with history, such as `feat(database): add entry lookup` or `chore(dev): update tooling`. Pull requests should explain the change, link applicable issues, and list validation performed. Include screenshots for visible UI changes and update the OpenAPI document for API changes.

## Security & Configuration

Runtime configuration uses `FPAN_`-prefixed environment variables. Keep local values in an untracked `.env`; never commit credentials, OIDC secrets, database URLs containing passwords, or generated storage data.
