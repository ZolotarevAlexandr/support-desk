# Implement this plan?

# IT Support System — Gin, React, and User/Operator Roles

## 1. Application and stack

Build an IT support application where users submit tickets and discuss them with operators. Operators manage a shared queue, assign work, update ticket statuses, and maintain categories.

| Area                 | Technology                                         |
|----------------------|----------------------------------------------------|
| Backend              | Go 1.27 + Gin                                      |
| Database             | PostgreSQL 18                                      |
| Database access      | Bun + PostgreSQL dialect + `pgdriver`; `database/sql` pooling |
| Migrations           | Goose v3                                           |
| Frontend             | React + TypeScript + Vite                          |
| Navigation           | React Router                                       |
| API data and caching | TanStack Query                                     |
| Styling              | CSS Modules with shared design variables           |
| Testing              | Go tests, Vitest/React Testing Library, Playwright |
| Deployment           | Docker Compose                                     |

Gin provides request binding, validation, route groups, and middleware appropriate for the API. React with Vite supports the interactive ticket queue and separate role views. [Gin documentation](https://gin-gonic.com/en/docs/), [React setup guidance](https://react.dev/learn/build-a-react-app-from-scratch).

Use one repository with `backend/` for the Go module and `frontend/` for the React application, and one production application image. Go asset embedding belongs to `backend/internal/staticui`; the build stages compiled frontend assets there. Build the frontend into static assets and embed them in the Go binary. Production runs the Go application and PostgreSQL; Node is needed only for development, builds, and frontend tests.

The UI and README will be English; `../../.reports/report_week_1.md` will be Russian.

## 2. Features, registration, and permissions

### Registration and authentication

The registration page contains name, email, password, password confirmation, and a **User / Operator** selector.

- User registration requires no invitation.
- Selecting Operator reveals an invitation-code field.
- The backend validates that code against `OPERATOR_INVITE_CODE`.
- An unset invitation code disables operator registration while leaving user registration available.
- The code stays in backend configuration and is never embedded in frontend assets, persisted with accounts, or logged.
- Successful registration signs the person in.
- Provide login, logout, session restoration after page reload, and clear handling of expired sessions.
- Roles are fixed at registration; there is no role-management or account-administration screen.

Store password hashes using Go’s bcrypt implementation, with cost 12, a minimum of 12 characters, and explicit validation of bcrypt’s 72-byte maximum. [Go bcrypt documentation](https://pkg.go.dev/golang.org/x/crypto/bcrypt).

Use opaque session cookies backed by PostgreSQL:

- Generate a cryptographically random token; store only its hash in the database.
- Sessions expire after 24 hours by default.
- Cookies use `HttpOnly`, `SameSite=Lax`, and `Secure` for HTTPS deployments.
- Logout deletes the server-side session and clears the cookie.
- Apply Go’s cross-origin request protection around Gin, including authentication endpoints. [Go CSRF protection](https://pkg.go.dev/net/http#CrossOriginProtection).

### Permission model

| Action | User | Operator |
|---|---|---|
| Create a ticket | Yes, as themselves | Yes, as themselves |
| List/read tickets | Own tickets | All tickets |
| Edit ticket content | Own tickets while `open` | Any ticket |
| Delete tickets | Own tickets while `open` | Any ticket |
| Change status or assignee | No | Yes |
| Read/add comments | Own tickets | Any ticket |
| Edit comments | Own comments | Own comments |
| Delete comments | Own comments | Any comment |
| Read categories | Yes | Yes |
| Create/edit/delete categories | No | Yes |
| View operator assignment list | No | Yes |

Closed tickets have read-only comment threads. Operators can reopen them.

Authorization is enforced on the backend for every operation. Frontend visibility only reflects those permissions. Requests for another user’s ticket or comments return `404`; accessing an operator-only endpoint returns `403`.

### Tickets and comments

Tickets contain a title, description, category, priority, requester, optional assigned operator, status, and timestamps.

- Priorities: `low`, `medium`, `high`, `urgent`; default `medium`.
- Statuses: `open`, `in_progress`, `resolved`, `closed`; default `open`.
- Operators may move between any supported statuses, including reopening.
- The requester comes from the authenticated account and cannot be changed.
- Assignment accepts only an existing operator account.
- Ticket deletion permanently removes the ticket and its comments after UI confirmation.
- Categories referenced by tickets cannot be deleted; return a conflict explaining that those tickets must first be recategorized.

Comments are plain-text messages with author and creation/edit timestamps. Display them chronologically, identifying operator replies.

### Screens

- **Register / Login:** validation, conditional invitation field, submission feedback.
- **My tickets:** the user’s tickets, search, filters, pagination, and create action.
- **Operator queue:** all tickets, with shortcuts for “Assigned to me” and “Unassigned.”
- **Ticket creation:** title, description, category, and priority.
- **Ticket details:** ticket content, status/assignment controls where permitted, and conversation thread.
- **Categories:** operator management screen with create/edit/delete.
- **Application shell:** navigation, current user and role, logout.

Use a responsive sidebar layout, readable tables, priority/status badges, accessible forms, and consistent loading, empty, error, and confirmation states. Keep filters in URL query parameters so navigation and refresh preserve them.

Deferred features: attachments, email notifications, password recovery, email verification, SLA automation, audit-history screens, and external integrations.

## 3. Backend, database, and API implementation

### Structure and data flow

Organize the backend into configuration, HTTP handlers/middleware, application services, and PostgreSQL repositories. Keep authorization and ticket rules in services rather than spreading them through Gin handlers.

Use Bun models and query builders inside PostgreSQL repositories, keeping persistence models separate from HTTP request DTOs. Initialize Bun with its PostgreSQL dialect and `pgdriver` over `database/sql`; configure connection-pool limits through `database/sql` and pass request contexts to every database operation. Repositories manage query execution, transactions, and database-error translation.

Keep Goose SQL migrations as the schema source of truth, including constraints and indexes. Bun provides database access; schema changes are applied through the existing migration process.

Organize the React frontend by feature: authentication, tickets, comments, and categories. Use a shared typed API client and reusable form/table/status components.

The request flow is:

1. Gin middleware identifies the session and authenticated account.
2. The handler validates the request.
3. The service checks permissions and business rules.
4. The repository executes queries through Bun using bound values.
5. The frontend updates the relevant TanStack Query data after success.

### Database model

| Table | Main fields |
|---|---|
| `users` | ID, name, unique email, password hash, role, timestamps |
| `sessions` | Token hash, user ID, creation time, expiry |
| `categories` | ID, unique name, description, timestamps |
| `tickets` | ID, requester ID, assignee ID, category ID, title, description, priority, status, timestamps |
| `comments` | ID, ticket ID, author ID, body, timestamps |

Use generated IDs, foreign keys, UTC timestamps, and database constraints for required values and supported enums. Email and category-name uniqueness are case-insensitive.

Index requester, assignee, category, status, ticket creation order, comment order, and session expiry. Scope ticket searches and pagination totals to the authenticated user before returning results.

Build optional ticket filters incrementally with Bun query builders. Apply identical visibility and filter conditions to the result query and its pagination-count query.

Enforce permission-dependent writes atomically: for example, a user editing an open ticket must not bypass that restriction if an operator changes its status concurrently.

Use conditional updates and transactions to enforce these rules. For comment creation, editing, and deletion, lock the parent ticket row within the transaction before checking its status and permissions, and hold the lock until the mutation completes. Ticket-status updates must coordinate through the same row lock so closing a ticket cannot race past the comment checks.

Validation limits:

- Ticket title: 200 characters.
- Description: 10,000 characters.
- Comment: 5,000 characters.
- Names: 100 characters.
- Category description: 1,000 characters.
- Required text must remain nonempty after trimming.

### REST API

All domain endpoints use JSON under `/api/v1`.

| Endpoints | Purpose |
|---|---|
| `POST /auth/register` | Register a user or invited operator |
| `POST /auth/login` | Create a session |
| `POST /auth/logout` | Revoke the current session |
| `GET /auth/me` | Return the current account |
| `GET, POST /tickets` | List or create tickets |
| `GET, PATCH, DELETE /tickets/{id}` | Read, update, or delete a ticket |
| `GET, POST /tickets/{id}/comments` | List or add comments |
| `PATCH, DELETE /tickets/{id}/comments/{comment_id}` | Edit or delete a comment |
| `GET, POST /categories` | List or create categories |
| `GET, PATCH, DELETE /categories/{id}` | Read, update, or delete a category |
| `GET /operators` | Return assignment choices to operators |
| `GET /healthz`, `GET /readyz` | Liveness and database readiness |

Ticket listing supports text search, status, priority, category, assignment, and pagination. Default to 25 tickets per page, maximum 100, ordered newest first with ID as a tie-breaker. Paginate comments oldest first.

Use explicit request DTOs so clients cannot update requester IDs, account roles, or other protected fields through generic binding.

- Paginated responses: `{items, total, page, page_size}`.
- Errors: `{error: {code, message, fields?}}`.
- Standard status codes: `201`, `200`, `204`, `400`, `401`, `403`, `404`, `409`, `503`.
- Return sanitized `500` errors for unexpected failures.
- Document payloads, permissions, and examples in OpenAPI.

### Frontend integration

Use relative `/api/v1` URLs and cookie-based authentication. During development, Vite proxies API requests to Gin; production serves both from the same origin.

Restore authentication through `/auth/me`. Clear cached account and ticket data on logout or session expiry so another login cannot see the previous account’s cached information.

Serve the React entry page for application routes, while preserving proper JSON `404` responses for unknown API routes and ordinary `404` responses for missing assets.

## 4. Deployment and the 12 factors

### Build and runtime

Implement a multi-stage build:

1. Install pinned frontend dependencies with `npm ci`.
2. Type-check and build the React application.
3. Build Go with embedded frontend assets and migrations.
4. Produce a minimal non-root runtime image with release metadata.

Docker Compose starts PostgreSQL, runs a separate migration container, and starts the application after migrations succeed. PostgreSQL data uses a named volume.

Provide environment configuration for database connection, HTTP port, log level, connection-pool size, query/shutdown timeouts, session duration, secure-cookie behavior, and the operator invitation code. Include an example configuration with placeholders; exclude actual secrets.

Add one-off commands for migrations, category seeding, and expired-session cleanup. Account creation remains in the registration UI.

Use structured JSON logs to stdout, request IDs, bounded HTTP/database operations, panic recovery, and graceful `SIGTERM` shutdown. Liveness must remain independent of PostgreSQL; readiness must fail during database outages or shutdown.

### Report evidence

| Factor | Implementation |
|---|---|
| **1. Codebase** | One Git repository for backend, frontend, migrations, and deployment files. |
| **2. Dependencies** | Go manifests/checksums, npm lockfile, and pinned container images. |
| **3. Configuration** | Deployment settings and invitation code supplied through environment variables. |
| **4. Backing services** | PostgreSQL connected through configuration, including session storage. |
| **5. Build, release, run** | Build an immutable image, associate configuration and migrations with the release, then run it. |
| **6. Processes** | Tickets, accounts, comments, and sessions persist in PostgreSQL; no process-local authoritative state. |
| **7. Port binding** | Gin runs within the Go HTTP server on a configurable port. |
| **8. Concurrency** | Identical application instances share PostgreSQL; authentication requires no sticky sessions. |
| **9. Disposability** | Fast startup, readiness checks, transactional writes, and graceful shutdown. |
| **10. Dev/prod parity** | PostgreSQL throughout; the production image can also run locally through Compose. |
| **11. Logs** | JSON event streams on stdout, collected externally. |
| **12. Admin processes** | Migrations, category seeding, and session cleanup run as one-off processes from the application image. |

Retain HTTP/1.1 compatibility for later traffic-balancing assignments. Kubernetes, Prometheus, Grafana, and log aggregation are deferred.

## 5. Implementation order, testing, and delivery

### Implementation order

1. Establish the repository structure, dependency manifests, configuration, database migrations, and build pipeline.
2. Implement registration, login/logout, PostgreSQL sessions, and role middleware.
3. Implement ticket/category CRUD, assignment, filtering, and backend permission checks.
4. Implement comments and their permissions.
5. Build the React screens and connect them to the API.
6. Complete container startup, health checks, logging, administrative commands, and shutdown handling.
7. Run acceptance tests, finish documentation, and package the source archive.

### Verification

**Backend tests**

- Registration for both roles; incorrect, missing, and disabled invitation codes.
- Duplicate emails, password validation, login failures, session expiry, and logout revocation.
- Cross-origin mutation rejection.
- User isolation across listing, search totals, ticket details, comments, updates, and deletion.
- Matching visibility and filter conditions in ticket results and pagination totals, including combined filters.
- Operator permissions and rejection of assignment to a normal user.
- Ticket, category, and comment CRUD, including closed-ticket restrictions.
- PostgreSQL integration tests for concurrent ticket closure versus user edits and comment creation, editing, or deletion; verify that permission-dependent writes remain atomic.
- Referenced-category deletion conflicts and cascading ticket/comment deletion.
- Migration repeatability and category-seed repeatability.

**Frontend and browser tests**

- Registration’s operator-code field, validation, and authentication redirects.
- User and operator navigation and actions.
- Complete Playwright scenario: user registers and submits a ticket; invited operator registers, claims it, replies, and resolves it; user sees the reply and status.
- A second user cannot access the first user’s ticket.
- Logout clears private cached data.
- Filters, pagination, empty states, failed submissions, and direct page reloads work.

**Operational checks**

- Run Go tests with the race detector, `go vet`, TypeScript checking, frontend tests, and production builds.
- Verify tickets and sessions survive application restarts.
- Verify the same session works across two application instances.
- Verify bounded failures and recovery during PostgreSQL outages.
- Verify graceful shutdown and clean startup from an extracted source archive.

### Submission artifacts and assumptions

Deliver the full source archive, English README, OpenAPI specification, environment example, Docker configuration, tests, and Russian `../../.reports/report_week_1.md`. The report will explain the domain, stack, entities, roles, and evidence for each factor.

The archive will exclude secrets, database volumes, dependency directories, compiled binaries, and IDE files.

Final acceptance requires working Docker daemon access and writable Git metadata; the current session’s restrictions must be resolved before those checks can be marked complete.


## Approved workflow refinement

Ticket management uses explicit claim, reassign, resolve, requester confirm/request_changes, and operator reopen actions. Resolution, feedback, and reopening require a message; requester confirmation records a fixed confirmation message. Named actions and their messages are atomic under the parent row lock. PATCH edits ticket content only. The queue includes Take ticket; detail management shows contextual buttons, a searchable reassignment dialog, and More actions for edit/delete/reopen. The requester gains only the permission to confirm or reject their own resolved outcome. The four existing statuses and database schema remain in use.
