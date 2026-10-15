# Support Desk

An IT support application with a React client, a Go/Gin REST API, and PostgreSQL persistence. Users submit tickets and discuss them with operators. Operators work a shared queue, assign tickets, change statuses, and manage categories.

## Run with Docker Compose

Requires Docker Engine with Compose. Containers use Go 1.27.1, Node 24, and PostgreSQL 18. No host language toolchain is needed.

```sh
cp .env.example .env
# Edit POSTGRES_PASSWORD and OPERATOR_INVITE_CODE; use URL-safe random values.
docker compose up --build -d
```

Open http://localhost:8080. Compose waits for PostgreSQL, applies Goose migrations in a separate process, seeds four categories, then starts the application. Register a User without an invitation, or choose Operator and enter your configured invitation code. There are no built-in accounts. An empty `OPERATOR_INVITE_CODE` disables operator registration.

```sh
docker compose logs -f app
docker compose run --rm app migrate
docker compose run --rm app seed
docker compose run --rm app cleanup-sessions
docker compose down
```

`down` preserves the database volume; `down -v` permanently removes it. The image embeds both frontend assets and migrations, runs as non-root, and does not need Node at runtime. PostgreSQL is exposed only on localhost for local development. For HTTPS deployment, put a TLS proxy in front and set `SECURE_COOKIE=true`.

## Development

For the Kubernetes, see [k8s/README.md](k8s/README.md)

Install Go 1.27+, Node 24+, and PostgreSQL 18 (or start only `db` through Compose). The database URL must point to a real database; there is no in-memory alternative.

```sh
cp .env.example .env
# Edit the placeholders, including DATABASE_URL for your local database.
set -a; . ./.env; set +a
cd backend
go run ./cmd/support migrate
go run ./cmd/support seed
go run ./cmd/support serve
# Another terminal:
cd frontend && npm ci && npm run dev
```

Vite serves http://localhost:5173 and proxies `/api` to http://localhost:8080. `API_PROXY_TARGET` overrides the proxy destination. Run `make build` at the repository root to build the frontend, copy its output into `backend/internal/staticui/dist`, and embed it in `bin/support` with the `production` build tag. Run `./bin/support serve` to serve that build. Without the production tag, the backend serves a development instruction page; Go tests and API development work from a clean source checkout without Node or compiled assets. Production uses only relative API URLs on the same origin.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | PostgreSQL DSN (URL-encode credentials when necessary) |
| `PORT` | `8080` | HTTP port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `DB_POOL_SIZE` | `20` | Maximum open and idle database connections per instance |
| `QUERY_TIMEOUT` | `5s` | Request/database context deadline |
| `SHUTDOWN_TIMEOUT` | `15s` | Graceful shutdown/admin-process deadline |
| `SESSION_DURATION` | `24h` | Session lifetime |
| `SECURE_COOKIE` | `false` | Require HTTPS for session cookies |
| `OPERATOR_INVITE_CODE` | empty | Private operator invitation; empty disables operator registration |
| `VERSION` | `local` | Compose image tag and build release metadata |

Compose configures `DATABASE_URL` using its database variables and always listens on container port 8080. `PORT` chooses the published host port. Do not commit `.env`. Passwords use bcrypt cost 12 with a 12-character minimum and 72-byte maximum. The server stores only SHA-256 hashes of random 256-bit session tokens. Cookies are HttpOnly and SameSite=Lax. Go cross-origin protection rejects unsafe cross-origin requests including login and registration. Roles are fixed at registration. The invite code is never sent to the frontend as configuration or stored in the database.

## API and permissions

The complete contract, request/response schemas, errors, and examples are in [docs/openapi.json](docs/openapi.json). All domain routes start with `/api/v1`. Authentication uses the `support_session` cookie. `/healthz` is independent of PostgreSQL; `/readyz` checks the database and shutdown state.

Users see only their own tickets and can edit/delete them while open. Operators see all tickets and may edit/delete any ticket. Named actions govern status and assignment; arbitrary PATCH changes to those fields are rejected. Only the requester can confirm or reject the outcome of their resolved ticket. Both roles can create tickets. Comments can be edited by their author; operators can delete any comment. Closed conversations are read-only for both roles until reopened. Another user's ticket always returns 404. Category management and the operator list require operator permissions (403). Referenced categories return 409 on deletion; first recategorize their tickets.

The operator queue offers **Take ticket** directly on available rows. Claiming atomically assigns the operator and starts work. Ticket details show the conversation beside a Next step panel instead of status/assignee dropdowns. **Reassign…** opens a searchable operator picker with **Assign to me** and **Return to queue** shortcuts. Returning to the queue clears assignment and sets `open`.

**Resolve…** requires a resolution message and marks an assigned, in-progress ticket `resolved`. Its requester can select **Issue fixed** to close it, or **Still need help** to return it to `in_progress` with a required explanation. Assignment is preserved. **More actions → Reopen…** lets operators reopen a resolved/closed ticket with a reason; assigned tickets return to `in_progress`, unassigned tickets to `open`. Editing/deletion are also under More actions. Resolution, feedback, confirmation, and reopening messages use the existing conversation and commit atomically with the transition. Concurrent claims or competing outcome decisions return 409 and refresh the visible state. Existing data requires no migration; previously assigned/open tickets offer **Start work** without changing their operator.

Use `POST /api/v1/tickets/{id}/actions` with `{action: "claim"}`, `{action: "resolve", message: "..."}`, `{action: "confirm"}`, `{action: "request_changes", message: "..."}`, `{action: "reopen", message: "..."}`, or `{action: "reassign", assignee_id: operatorIdOrNull}`. Ticket PATCH handles only title, description, category, and priority.

Ticket statuses are `open`, `in_progress`, `resolved`, `closed`; priorities are `low`, `medium`, `high`, `urgent`. Lists support `search`, `status`, `priority`, `category_id`, `assignment=me|unassigned`, `page`, and `page_size`. Page size defaults to 25, maximum 100. Counts and items share visibility/filter rules and a database snapshot. Comments are ordered oldest first. Ticket and comment writes lock the same parent ticket row before checking permissions/status. Ticket deletion cascades to comments.

```sh
curl -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"a-long-test-password","password_confirmation":"a-long-test-password","role":"user"}' \
  http://localhost:8080/api/v1/auth/register
curl -b cookies.txt http://localhost:8080/api/v1/categories
curl -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"title":"Printer offline","description":"Cannot print from my laptop","category_id":1,"priority":"medium"}' \
  http://localhost:8080/api/v1/tickets
```

## Architecture

```text
backend/                  Go module and server source
  cmd/support/            Server and administrative entry point
  internal/               Configuration, HTTP, services, repositories
    staticui/             Go embedding code and development fallback
  migrations/             Embedded Goose SQL migrations
frontend/                 React/TypeScript source and npm manifests
  src/                    UI features and shared components
  e2e/                    Playwright scenarios
  dist/                   Generated Vite output (ignored)
docs/                     API contract and verification record
scripts/                  Source packaging
Dockerfile, docker-compose.yaml  Root deployment configuration
Makefile                  Root build and check commands
```

Go embedding belongs to `backend/internal/staticui`. Its development build serves a small instruction page so backend tests and API development do not require a frontend build. Its production build embeds the generated React assets into the server binary. Go embed patterns cannot reference files outside their package directory, so `make build` stages Vite output there; Docker copies it directly between build stages. The frontend contains no Go files and builds independently with `npm run build`.

`backend/cmd/support` owns process startup, admin commands, and signal shutdown. `backend/internal/config` reads and validates environment settings. `backend/internal/httpapi` handles strict JSON DTOs, cookies, request IDs, logging, deadlines, and Gin routing. `backend/internal/service` enforces permissions and domain rules. `backend/internal/store` owns Bun queries, transactions, and PostgreSQL error translation. `backend/migrations` holds the embedded Goose SQL schema. `frontend/src` groups authentication, tickets, workflow controls, comments, categories, and shared components. TanStack Query caches API responses; logout/session expiry clears private cache. CSS Modules provide a responsive sidebar layout. Filters persist in URL query parameters.

The application keeps authoritative state in PostgreSQL, so identical instances can share sessions without sticky routing. The HTTP server supports HTTP/1.1. SIGTERM marks readiness unavailable and drains active requests within the configured deadline. Logs are JSON events on stdout and exclude request bodies, passwords, cookies, and invitation codes.

## Tests

```sh
(cd frontend && npm ci)
make test
make vet
# Or run Go commands inside backend/ and npm commands inside frontend/.
```

PostgreSQL tests skip unless `TEST_DATABASE_URL` is supplied. **Use a disposable test database:** these tests truncate the application tables and reset identity values.

```sh
docker run -d --name support-test-db -e POSTGRES_PASSWORD=test-password \
  -e POSTGRES_DB=support_test -p 127.0.0.1:55432:5432 postgres:18.0-alpine3.22
(cd backend && TEST_DATABASE_URL='postgres://postgres:test-password@localhost:55432/support_test?sslmode=disable' go test -race ./...)
docker rm -f support-test-db
```

Wait for `pg_isready` before running tests. Tests cover registration/invites/passwords, sessions, CSRF, CRUD, user isolation, combined filter totals, assignment, closed-ticket restrictions, foreign-key conflicts, cascade deletion, migration/seed repeatability, concurrent closure versus ticket edits and comment creation/editing/deletion, competing claims/confirmations, and rollback of messages when status writes fail.

Browser tests run against a running, seeded application and create uniquely named accounts. They exercise user submission, invited operator registration, queue claiming, reassignment, reply/resolution, requester rejection/confirmation, isolation, reloads, filters, logout, closed conversations, category CRUD, and pagination.

```sh
cd frontend
npx playwright install chromium
E2E_BASE_URL=http://localhost:8080 E2E_INVITE_CODE=your-configured-code npm run test:e2e
```

For a preinstalled Chromium, set `CHROMIUM_PATH` to its executable.

## Submission

```sh
./scripts/package.sh
```

The resulting `artifacts/support-desk-source.tar.gz` includes complete source,
lockfiles, migrations, tests, OpenAPI, Docker/Compose configuration, all Kubernetes
manifests and `deploy.sh`, the Russian [Отчёт.md](Отчёт.md), and `screencast.mp4`.
The report and recording must exist before packaging. The archive includes the
public demo Secret from `k8s/secret.yaml`; local `.env` files, volumes, dependencies,
compiled assets/binaries, IDE metadata, and the assignment materials are excluded.
Extract it and follow [k8s/README.md](k8s/README.md) to build and deploy in Minikube.

The application uses one Git repository; commit reviewed changes and tag releases when deploying. Attachments, email notifications, password recovery, SLA automation, external integrations, and monitoring are deferred.
