# Backend Quality Guidelines

> 后端开发硬性规则与禁止模式。

---

## 硬性规则

### Context 传递
- 所有跨函数调用传递 **`context.Context` 作为第一个参数**

### 错误处理
- 错误使用 **`%w` 包装**以保留堆栈
- handler 层统一处理错误响应

### 依赖管理
- **禁止使用 `init()` 函数**或包级全局变量管理依赖
- 依赖注入在 **`cmd/server/main.go`** 中统一组装

### 接口设计
- 接口在**消费方定义**
- 保持小（**1-3 个方法**）

---

## Forbidden Patterns

- Do not commit local backend build outputs such as `src/backend/agenthub-server`, `src/backend/main`, or files under `src/backend/tmp/`.
- Do not leave merge conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`) in committed files.

---

## Required Patterns

### Daemon Liveness Timing

**Scope / Trigger**: Applies when changing daemon WebSocket heartbeat, machine liveness tracking, or agent online/offline behavior.

**Contracts**:
- Daemon registration marks the machine online immediately.
- WebSocket ping/pong or task heartbeat refreshes liveness.
- The backend offline threshold must be longer than the daemon/server ping cadence.

**Good/Base/Bad Cases**:
- Good: `machineOfflineThreshold` is comfortably greater than the 30s daemon/server ping interval.
- Base: A machine stays online between registration and the first normal ping/pong.
- Bad: An offline threshold shorter than the ping interval marks a healthy daemon offline before it can answer @mention tasks.

**Tests Required**:
- Assert the offline threshold exceeds the ping cadence.
- Assert a registered machine is not swept offline inside the first ping window.
- Assert a stale machine is swept after the threshold expires.

**Wrong vs Correct**:
```go
// Wrong: shorter than the 30s ping cadence.
machineOfflineThreshold = 15 * time.Second

// Correct: longer than the ping cadence and watchdog window.
machineOfflineThreshold = 75 * time.Second
```

### Daemon Candidate Agent Creation

**Scope / Trigger**: Applies when changing the frontend/backend flow that adds a detected daemon CLI candidate as a user Agent.

**Signatures**:
- API: `POST /api/daemon/agent-candidates/:id/add`
- Request: `{"name": string, "cli_tool": string, "system_prompt"?: string}`
- Service/repository: `AddCandidateAgent(ctx, userID, candidateID, displayName, expectedCLITool, systemPrompt)`

**Contracts**:
- `cli_tool` is required and must match the candidate row selected by `:id`.
- The repository must filter by both `candidateID` and `cli_tool` before inserting into `agents`.
- The inserted Agent inherits `cli_tool`, `machine_id`, `machine_name`, version, and capabilities from the verified candidate row.

**Validation & Error Matrix**:
- Missing `name`, `candidateID`, `userID`, or `cli_tool` -> `ErrAgentInvalidInput`.
- Candidate not owned by the user or candidate `cli_tool` does not match -> `ErrAgentNotFound`.
- Database insert/query failure -> wrap with `%w` and return a 500 from the handler.

**Good/Base/Bad Cases**:
- Good: User clicks the Claude candidate; request includes `cli_tool: "claude"` and the inserted Agent has `cli_tool = "claude"`.
- Base: Slow refresh updates candidate metadata but the same `id + cli_tool` still matches and creation succeeds.
- Bad: A stale or fast repeated click sends a candidate ID with a mismatched `cli_tool`; creation must fail instead of creating the wrong CLI Agent.

**Tests Required**:
- Service test asserts `expectedCLITool` is passed through to the repository.
- Repository or integration test asserts mismatched candidate `cli_tool` returns no Agent.
- Frontend type-check must cover the required `cli_tool` request field.

**Wrong vs Correct**:
```go
// Wrong: trusts only the candidate id.
WHERE c.id = $1 AND m.user_id = $2

// Correct: also verifies the CLI tool selected by the UI.
WHERE c.id = $1 AND m.user_id = $2 AND c.cli_tool = $5
```

### Desktop SPA Runtime

**Scope / Trigger**: Applies when changing backend static serving, Electron desktop packaging, or production SPA routing.

**Signatures**:
- Environment: `AGENTHUB_CONFIG` overrides the backend config file path.
- Environment: `AGENTHUB_FRONTEND_DIST` points the backend to a built Vite `dist` directory.
- Backend helper: `registerSPARoutes(router, distDir)` registers a fallback only when `index.html` exists.

**Contracts**:
- `/api/*`, `/ws`, `/daemon/*`, and `/mcp/*` must not be served by the SPA fallback.
- Browser-history routes such as `/settings` and `/tasks` must serve `index.html`.
- Static assets must be served only when the resolved file exists inside the frontend dist directory.
- Electron production mode should pass `AGENTHUB_CONFIG` and `AGENTHUB_FRONTEND_DIST` instead of relying on process cwd.

**Validation & Error Matrix**:
- Missing `dist/index.html` -> skip SPA fallback registration.
- Missing API route -> normal 404, never `index.html`.
- Suspicious/traversal asset path -> reject or fall back without serving files outside dist.
- Missing database for packaged backend -> backend exits; Electron should log backend stdout/stderr for diagnosis.

**Good/Base/Bad Cases**:
- Good: `bin/server` started from repo root serves `/settings` from `src/frontend/dist/index.html`.
- Base: `go run ./cmd/server` from `src/backend` serves the same dist via `../../frontend/dist`.
- Bad: `/api/missing` returns the SPA HTML, hiding API routing errors.

**Tests Required**:
- `go test ./cmd/server` asserts asset serving, BrowserRouter fallback, API 404 isolation, env path loading, and dist candidate coverage.
- Desktop smoke should verify packaged resources include `resources/bin/server.exe`, `resources/frontend-dist/index.html`, and `resources/config/config.yaml`.

**Wrong vs Correct**:
```go
// Wrong: root static handler can swallow frontend history routes as 404.
router.StaticFS("/", http.Dir(distDir))

// Correct: NoRoute checks API exclusions, existing assets, then falls back to index.html.
router.NoRoute(spaFallbackHandler(distDir, indexPath))
```

### Published File Path Boundaries

**Scope / Trigger**: Applies when changing any handler that serves files from disk, including deployment previews, uploads, knowledge files, PPT previews, and SPA static assets.

**Signatures**:
- Deployment site route: `GET /api/sites/:id/*filepath`.
- Knowledge file route: `GET /api/knowledge-bases/:id/files/:fileId/content`.
- Upload/PPT routes: `GET /api/uploads/*filepath`, `GET /api/ppt-preview/*filepath`.

**Contracts**:
- A route scoped by an object ID must serve files only under that object's directory, not merely under the broader storage root.
- Paths stored in the database are not automatically trusted; handlers must re-validate them before serving or deleting files.
- Normalize URL-style paths and Windows backslash paths before checking traversal.
- Existing files outside the allowed root must return `403`, not `200`.

**Validation & Error Matrix**:
- `../` traversal or encoded traversal -> `403`.
- Absolute path or Windows drive/rooted path -> `403`.
- Valid in-root path that does not exist -> `404`.
- Valid existing file in the scoped root -> `200`.

**Good/Base/Bad Cases**:
- Good: `/api/sites/{id}/../{otherID}/secret.html` is rejected even though `{otherID}` is under the deployment base directory.
- Base: `/api/sites/{id}/index.html` serves the current deployment's index.
- Bad: checking only against `BaseDir()` lets one deployment ID read another deployment's files.

**Tests Required**:
- Handler test asserts traversal into another deployment is rejected.
- Helper or handler test asserts knowledge file paths reject `../`, absolute paths, and rooted Windows-style paths.
- Existing happy-path tests must still assert valid files are served.

**Wrong vs Correct**:
```go
// Wrong: only constrains access to the broad deployment root.
target := filepath.Join(baseDir, id, rel)
checkWithin(target, baseDir)

// Correct: constrains access to the current object's directory.
siteRoot := svc.SiteDir(id)
target := filepath.Join(siteRoot, rel)
checkWithin(target, siteRoot)
```

---

## Testing Requirements

(To be filled by the team)

---

## Code Review Checklist

- For branch integrations, verify that generated backend binaries are ignored or removed from the index.
