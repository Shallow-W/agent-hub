# Error Handling

> How errors are handled in this project.

---

## Overview

- Errors are sentinel `var Err... = errors.New(...)` values defined in
  the `service` package next to the consuming service.
- Repo layer wraps low-level errors with `fmt.Errorf("... : %w", err)`
  to preserve the original `*pgconn.PgError` for inspection.
- Service layer inspects errors via `errors.Is` (for sentinels) and
  `errors.As` (for typed PG errors) and returns its own sentinel.
- Handler layer maps service sentinels to HTTP status + numeric code.

---

## Error Sentinels

Defined in `service/`, named `Err<Subject><Problem>`:

```go
var ErrConvInvalidRole   = errors.New("invalid conversation agent role")
var ErrConvNotFound      = errors.New("conversation not found")
var ErrConvNoPerm        = errors.New("no permission to manage conversation agents")
var ErrConvOrchConflict  = errors.New("并发设置 Orchestrator 冲突，请重试")
```

A sentinel lives next to the service that produces it
(`ErrConvOrchConflict` is in `conversation.go` next to `RoleService`'s
callers). Do not centralize all sentinels in one `errors.go` — keep
them local.

---

## Error Handling Patterns

### Pattern: DB unique violation → domain conflict

```go
// service/role_service.go
if err := s.convRepo.UpdateAgentRole(ctx, convID, agentID, string(role)); err != nil {
    if role == domain.RoleOrchestrator && isUniqueViolation(err) {
        return ErrConvOrchConflict
    }
    return fmt.Errorf("update agent role: %w", err)
}
```

The `isUniqueViolation` helper is shared across services
(`service/pgerr.go`):

```go
func isUniqueViolation(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

Always scope the check tightly (e.g. `role == RoleOrchestrator`) so
genuine DB errors on other paths are not swallowed as conflicts.

### Pattern: best-effort side effects after commit

When a side effect (WS broadcast, audit log) follows a successful
commit, wrap it so failures do not invalidate the committed state:

```go
if err := s.events.BroadcastRoleChanged(ctx, memberIDs, event); err != nil {
    slog.Warn("role changed: broadcast failed",
        "conv_id", convID, "actor_id", userID, "error", err)
}
return nil  // HTTP success — role is already persisted
```

---

## API Error Responses

Every error response uses `middleware.ErrorResponse(c, status, code, msg)`:

```go
middleware.ErrorResponse(c, http.StatusBadRequest, 40030, "参数错误: "+err.Error())
```

### HTTP Code Convention

| HTTP | When | Code range |
|------|------|------------|
| 400 | Bad request (param parsing, invalid value) | 400xx |
| 401 | Unauthenticated | 401xx |
| 403 | Forbidden (permission denied) | 403xx |
| 404 | Resource not found | 404xx |
| 409 | Conflict (concurrent write, unique violation) | 409xx |
| 500 | Internal error | 500xx |

Codes are numeric, 5-digit, grouped by HTTP status family. The trailing
two digits identify the specific error within the family.

### Concrete Conversation Role Codes

| Code | HTTP | Sentinel | Trigger |
|------|------|----------|---------|
| 40015 | 400 | `ErrConvInvalidRole` | role not in `{orchestrator, worker}` |
| 40016 | 400 | (sub-codes of 40015) | (legacy / kept for compat) |
| 40315 | 403 | `ErrConvNoPerm` | non-owner / non-admin attempts role change |
| 40415 | 404 | `ErrConvNotFound` | conversation does not exist |
| 40915 | 409 | `ErrConvOrchConflict` | concurrent Orchestrator assignment, DB unique violation |

When introducing a new error path, pick the next free code within the
relevant HTTP family and document it here.

---

## Common Mistakes

- Wrapping errors with `%v` instead of `%w` — breaks `errors.As` /
  `errors.Is` chains; downstream code cannot detect the underlying PG
  error code.
- Catching unique violation without scoping the check (e.g. omitting
  `role == RoleOrchestrator`) — swallows real DB errors on other paths.
- Returning HTTP 500 for a 409-class conflict — the operator cannot
  distinguish "transient server error" from "another tab beat me to it".
- Centralizing all error sentinels in a single `errors.go` — encourages
  cross-service coupling; prefer colocated sentinels near the producer.
