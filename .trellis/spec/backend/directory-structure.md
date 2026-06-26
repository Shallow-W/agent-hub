# Directory Structure

> How backend code is organized in this project.

---

## Overview

The backend follows a layered architecture with explicit dependency
direction. Inner layers (domain) have zero infrastructure dependencies;
outer layers (infrastructure) implement interfaces defined in `port/`.
This keeps services testable and lets us swap implementations
(postgres ↔ fake, ws hub ↔ in-memory) without touching business logic.

---

## Directory Layout

```
src/backend/
├── cmd/
│   └── server/                 # Entry point, wire assembly only
├── internal/
│   ├── domain/                 # Pure domain: constants, value objects
│   ├── model/                  # DB row structs (GORM / sqlx tag friendly)
│   ├── port/                   # Interface contracts between app and infra
│   ├── repository/             # Postgres implementations of repo interfaces
│   ├── service/                # Application services (business orchestration)
│   ├── handler/                # HTTP / WS entry points (thin)
│   ├── router/                 # Centralized HTTP route registration (Deps struct)
│   ├── middleware/              # Auth, logging, response shaping
│   ├── catalog/                # Unified abstraction over directory vertical slices
│   ├── infrastructure/
│   │   └── ws/                 # Adapters: EventBroadcaster over *pkgws.Hub
│   ├── docextract/             # File content extractors
│   └── ...                     # Other domain-specific helpers
├── pkg/
│   └── ws/                     # WebSocket hub (cross-project reusable)
├── migrations/                 # Numbered SQL files (001_..., 002_...)
└── config/                     # Config loading
```

---

## Layer Dependency Rule

Dependency direction is strictly inward:

```
handler → service → port ← infrastructure
                  ↓
                 domain (zero deps)
                 model (only stdlib + domain)
```

- `domain/` must not import `model/`, `repository/`, `pkg/`, or any
  third-party driver.
- `model/` may import `domain/` (for typed methods on row structs) but
  nothing else internal.
- `service/` depends on `port/` interfaces, never on concrete
  infrastructure types.
- `infrastructure/` imports `port/` (to implement interfaces) and
  external drivers (`pkg/ws`, etc.).

---

## Module Organization

| Layer | What lives here | What doesn't |
|-------|-----------------|--------------|
| `domain/` | Role constants, value objects, pure validation | DB access, side effects |
| `model/` | Row structs, struct methods that read domain | SQL, HTTP |
| `port/` | Interface definitions only | Implementations |
| `repository/` | SQL queries, row scans, canonical interfaces (`types.go`) | Business logic, HTTP handlers |
| `service/` | Use cases, orchestration, error sentinels | HTTP, SQL strings |
| `handler/` | Request parsing, error→HTTP mapping | Business decisions |
| `router/` | HTTP route registration only (single `Setup` entry point) | Handler construction, business logic |
| `catalog/` | Unified CRUD over `platform_skill` / `tool_definition` / `agent_prompt_template` / `user_template` (DomainSpec + Registry + AdapterStore) | Direct per-domain CRUD code duplication |
| `infrastructure/` | Adapters for external systems | Business logic |

---

## Naming Conventions

- **Files**: `snake_case.go`. One type per file when the type is large
  (e.g. `role_service.go`); small helpers can share a file.
- **Canonical repo interfaces**: defined once in
  `internal/repository/types.go` (`MessageStore`, `ConvStore`,
  `AgentStore`, `OrchTaskStoreCanon`, `DeploymentStore`,
  `ArtifactStore`, `KnowledgeStore`). Each one is satisfied by the
  corresponding `*XxxRepo` struct. New services should accept these
  canonical interfaces, not define new subset interfaces.
- **Narrow service-local interfaces**: legacy services (message,
  deployment, artifact, orchestrator) still define subset interfaces
  (`MsgRepo`, `ConvRepoForMsg`, `DeployArtifactRepo`, etc.) next to the
  consuming service. These are marked `// Deprecated: migrate to
  repository.XxxStore`. Do not introduce new ones — use the canonical
  interfaces from `repository/types.go`.
- **Interfaces in `port/`**: named by capability (`EventBroadcaster`,
  `DaemonDispatcher`, `TokenIssuerPort`), not by implementation.
- **Error sentinels**: `Err<Subject><Problem>` (`ErrConvOrchConflict`,
  `ErrConvInvalidRole`). Defined in `service/` next to the consuming
  service, not in `model/` or `domain/`.
- **WS event types**: string constants live in the infrastructure
  adapter that emits them (`EventTypeRoleChanged` in
  `infrastructure/ws/event_broadcaster.go`), not scattered as bare
  strings.

---

## Examples

- `internal/domain/role.go` — `Role` type + constants; zero deps.
- `internal/model/conversation.go` — `ConversationAgents` slice with
  `FindByAgentID` / `Orchestrator` / `Workers` domain methods. Imports
  only `time` and `domain`.
- `internal/port/events.go` — `EventBroadcaster` interface; the only
  thing service layer sees.
- `internal/port/daemon.go` — `DaemonDispatcher` interface (5 methods:
  IsConnected / RegisterTaskPromise / SendToMachine / AwaitTaskResult /
  RemoveTaskPromise) abstracting `*pkgws.DaemonHub`. Compile-time
  assertion `var _ DaemonDispatcher = (*ws.DaemonHub)(nil)` guards
  signature drift.
- `internal/port/token.go` — `TokenIssuerPort` interface (1 method:
  IssueAgentToken) abstracting `*service.TokenIssuer`. Same compile-time
  assertion pattern in `service/token_issuer.go`.
- `internal/repository/types.go` — canonical repo interfaces
  (`MessageStore`, `ConvStore`, ...) satisfied by the concrete
  `*XxxRepo` structs. New code references these.
- `internal/router/router.go` — single `Setup(r, Deps{...})` entry
  point; `Deps` struct aggregates every handler so `main.go` stays
  thin.
- `internal/catalog/` — unified catalog abstraction (B1, pilot):
  - `types.go` — `Domain`, `Scope`, `Item`, `CreateInput`, `UpdateInput`,
    `ListQuery`, unified error sentinels (`ErrNotFound` / `ErrInvalid` /
    `ErrDuplicate` / `ErrUnknownDomain` / `ErrReadOnly`).
  - `store.go` — `Store` interface (List/GetByID/Create/Update/Delete).
  - `registry.go` — `DomainSpec` + `Registry`. Adding a new catalog domain
    means appending a `DomainSpec` in `domains.go`; the catalog core
    (`service.go`, `handler.go`) never switches on a specific Domain.
  - `adapter.go` — `AdapterStore` proxies to the four existing repos
    (no DB changes) and converts `model.*` ↔ `Item`.
  - `service.go` — `Service` (normalize, error mapping, ImportDefaults).
  - `handler.go` — unified REST handler mounted at `/api/catalog/:domain`.
  - `domains.go` — `DefaultRegistry()` registers all 4 known domains.
  - `seeders_data.go` — catalog-local copy of default-value data (the
    legacy `service.Default*` functions are kept for the legacy handlers
    until B2/B3/B4 land).
  - To add a new catalog domain: append a `DomainSpec` to
    `DefaultRegistry()` in `domains.go`, and (if the adapter doesn't
    already cover it) extend `AdapterStore` with the appropriate
    model↔Item converters. You should NOT need to modify `service.go`,
    `handler.go`, or `store.go`.
  - Transition path to a unified physical table: `Store` is the only
    storage contract `Service` depends on, so a future `SQLCatalogStore`
    (single table, one row per catalog Item) can replace `AdapterStore`
    without touching `service.go` / `handler.go` / `domains.go`. The
    four legacy repos + their handlers stay during the transition (B2 /
    B3 / B4 migrate them one at a time, then B-later deletes the
    legacy code and the duplicated `seeders_data.go`).
- `internal/service/orchestrator.go` — `OrchestratorDeps` struct +
  `NewOrchestratorServiceWithDeps` constructor; legacy setters kept
  but tagged `// Deprecated`.
- `internal/service/dispatcher_router.go` + `dispatcher.go` + `agent_queue.go`
  — P5b split of dispatch responsibilities: `Router` (@mention → target),
  `Dispatcher` (wraps `dispatchAndWait`), `AgentQueue` (same-agent
  serialization). See `dispatcher.md` for the full contract.
- `internal/service/role_service.go` — `RoleService` depends on
  `RoleConvRepo` (narrow) + `port.EventBroadcaster`.
- `internal/infrastructure/ws/event_broadcaster.go` — concrete adapter
  implementing `port.EventBroadcaster` via `*pkgws.Hub`.

---

## Common Mistakes

- **God-service with N setters**: don't accumulate dependencies via
  `SetX` methods. If a service genuinely needs many collaborators,
  bundle them in a `XxxDeps` struct and inject via a single
  `NewXxxServiceWithDeps(deps)` constructor (see OrchestratorService).
  Legacy setters may be kept temporarily but must be marked
  `// Deprecated: use NewXxxServiceWithDeps`.
- **Reaching across layers**: a service that imports `pkg/ws` directly
  violates the dependency rule. Introduce a `port/` interface and an
  `infrastructure/` adapter.
- **HTTP logic inside `pkg/` or `repository/`**: `pkg/ws/hub.go` must
  only do WebSocket transport (rooms, broadcast, connection
  management). `repository/*.go` must only do I/O (SQL, disk). HTTP
  handlers belong in `internal/handler/`. The gin import is forbidden
  in `pkg/` and `repository/`.
- **Routes registered in `cmd/server/main.go`**: all HTTP route
  registrations go through `internal/router/router.go`'s
  `Setup(r, Deps{...})`. `main.go` keeps only the handful of routes
  that depend on concrete infra types (`/health`, `/health/ready`,
  `/ws`) plus SPA fallback.

---

## Wiring Patterns

### Service with many collaborators — `Deps` struct

When a service has more than ~3 collaborators, prefer a single `Deps`
struct + constructor over positional args or chained setters:

```go
type OrchestratorDeps struct {
    ConvRepo     repository.ConvStore
    AgentRepo    repository.AgentStore
    MsgRepo      repository.MessageStore
    // ... rest of deps
    Delivery     OrchDeliveryState // optional — nil-safe at call sites
}

func NewOrchestratorServiceWithDeps(deps OrchestratorDeps) *OrchestratorService {
    return &OrchestratorService{ /* assign every field */ }
}
```

- Optional deps (caches, hubs that may be nil at boot) must be
  nil-checked at every call site. See `OrchestratorService.postPersistAsync`
  for the canonical `if s.delivery == nil { return }` guard.
- Legacy `SetX` methods on the same service must be kept (for tests /
  external callers) but tagged `// Deprecated`.

### HTTP route registration — `router.Setup`

`cmd/server/main.go` constructs all handlers, packages them into
`router.Deps{...}`, and calls `router.Setup(r, deps)`. The router file
is the single source of truth for paths, methods, and middleware
binding. Don't add ad-hoc `r.GET(...)` calls in `main.go` for routes
that don't need concrete DB/Redis types.
