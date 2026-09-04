# Show the available project commands.
default:
    @just --list

# Prepare local configuration and install locked frontend dependencies.
setup:
    #!/usr/bin/env bash
    set -euo pipefail

    if [[ ! -e backend/.env ]]; then
      cp backend/.env.example backend/.env
      echo "Created backend/.env"
    else
      echo "Keeping existing backend/.env"
    fi

    if [[ ! -e frontend/.env ]]; then
      cp frontend/.env.example frontend/.env
      echo "Created frontend/.env"
    else
      echo "Keeping existing frontend/.env"
    fi

    pnpm --dir frontend install --frozen-lockfile
    echo "Setup complete. Review backend/.env before starting the services."

# Start the backend and frontend together.
[parallel]
dev: backend frontend

# Start the Go API.
[working-directory('backend')]
backend:
    #!/usr/bin/env bash
    set -euo pipefail

    if [[ ! -f .env ]]; then
      echo "backend/.env is missing; run 'just setup' first." >&2
      exit 1
    fi

    exec go run ./cmd/fpan

# Start the React Router development server.
[working-directory('frontend')]
frontend:
    #!/usr/bin/env bash
    set -euo pipefail

    if [[ ! -d node_modules ]]; then
      echo "frontend dependencies are missing; run 'just setup' first." >&2
      exit 1
    fi

    exec pnpm dev

# Serve the OpenAPI documentation locally with Scalar.
[working-directory('frontend')]
api-docs:
    #!/usr/bin/env bash
    set -euo pipefail

    if [[ ! -d node_modules ]]; then
      echo "frontend dependencies are missing; run 'just setup' first." >&2
      exit 1
    fi

    exec pnpm api:docs

# Run every configured backend and frontend check.
[parallel]
check: check-backend check-frontend

# Run backend tests and static checks.
[working-directory('backend')]
check-backend:
    go test ./...
    go vet ./...
    golangci-lint run

# Run frontend formatting, lint, type, and build checks.
[working-directory('frontend')]
check-frontend:
    pnpm check
