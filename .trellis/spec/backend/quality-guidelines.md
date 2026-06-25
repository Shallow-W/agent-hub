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

### Orchestrator Prompt Agent Details

**Scope / Trigger**: Applies when changing group-chat orchestrator prompt construction or the conversation-agent query feeding it.

**Contracts**:
- `ConversationRepo.ListAgents` is the source of truth for the current group chat's available Agent list in an orchestrator prompt; it must not be replaced by a global Agent list.
- Prompt construction should keep the group Agent detail lightweight: `name`, `role`, `status`, dispatch-safe `description`, and `tags`.
- Prompt construction must not expose `cli_tool`, raw `capabilities_json`, discovered skill details, tool descriptions, or full `system_prompt` in the orchestrator Agent detail block.
- Prompt construction must not invent descriptions or tags. Missing fields should render as an explicit fallback such as `未配置`.
- Long free-form fields should be truncated before insertion so one Agent config cannot crowd out the user message or recent chat context.

**Tests Required**:
- Assert the prompt includes real Agent details from the backend query.
- Assert empty description/tag fields use fallback text rather than generated prose.
- Assert the prompt does not include `CLI工具`, raw capability/skill JSON, or management-tool instruction text.
- Assert the prompt tells the orchestrator to only dispatch to Agent names listed in the current group chat.

**Wrong vs Correct**:
```go
// Wrong: prompt layer exposes the full system prompt or tool capability JSON.
detail.Description = truncateString(ca.SystemPrompt, 300)

// Correct: prompt layer only renders a dispatch-safe description field.
detail.Description = truncateString(ca.Description, 300)
```

### Agent MCP Toolsets

**Scope / Trigger**: Applies when changing `agents.tools_config`, daemon MCP tool registration, or platform MCP endpoints.

**Signatures**:
- API: `PUT /api/agents/:id/tools-config`
- Request: `{"tools_config": string, "enable_management_tools": boolean}`
- Tool catalog API: `GET /api/tools/definitions`
- Builtin template API: `GET /api/tools/builtin-templates`
- Storage: `agents.tools_config` stores the normalized JSON string; `agents.enable_management_tools` stores whether management tools are enabled.
- Runtime read: daemon MCP resolves the current Agent through `/mcp/agents/:id` or `/mcp/agents` and must receive `tools_config`.

**Contracts**:
- `/api/tools/definitions` must expose the runtime `ToolRegistry` entries, not stale DB/catalog rows. `tool_definitions` is a sync/cache table; it is not the frontend authorization source of truth.
- `/api/tools/builtin-templates` must normalize `tool_names` against the runtime `ToolRegistry` before returning them to the frontend. Filter unknown names and map supported legacy aliases such as `list_group_agents` -> `list_conversation_agents`.
- `agents.tools_config` is the per-Agent MCP tool authorization config. The supported JSON shape is `{"toolset": string, "allowed_tools": string[]}`.
- Frontend Agent tool assignment UI must persist through `PUT /api/agents/:id/tools-config`, not only local component/store state and not the full `PUT /api/agents/:id` custom-Agent update path.
- The tools-config update endpoint only mutates `tools_config` and `enable_management_tools`; it must not change name, prompt, avatar, capabilities, Skills, status, or machine fields.
- The tools-config update endpoint applies to the current user's visible Agents, including daemon/system/custom Agents. Full Agent profile updates may remain custom-Agent-only.
- `allowed_tools` must only contain known platform tool names. Unknown names are filtered before persistence.
- Legacy non-JSON `tools_config` text may be preserved for display, but it must not grant extra MCP tools.
- Saving `enable_management_tools` must reflect the current selected tools. Removing management tools should be able to set it back to `false`.
- `/mcp/agents` and `/mcp/agents/:id` responses used by daemons must include `tools_config`; otherwise persisted assignments will not affect runtime `tools/list`.
- MCP `tools/list` must only return tools allowed for the current `agent_id`.
- MCP `tools/call` must reject unauthorized tool names before executing the tool handler.
- MCP sessions without a resolved `agent_id`, or with an unknown Agent, must fail closed and expose no tools.
- Explicit JSON `allowed_tools: []` means no tools; it must not fall back to default tools.
- Hiding tools in prompts or UI is not sufficient; runtime tool calls must enforce the same allowlist.

**Validation & Error Matrix**:
- Empty `tools_config` -> persist normalized no-tools config `{"toolset":"none","allowed_tools":[]}`.
- Unknown tool names -> filter before persistence.
- Stale DB/catalog tool definition not registered in `ToolRegistry` -> omit from `/api/tools/definitions`.
- Builtin template contains stale/unknown tool names -> return only canonical registered names in `tool_names`.
- Invalid JSON object shape that cannot be normalized -> `ErrAgentInvalidInput`.
- Missing `agent_id` or `user_id` -> `ErrAgentInvalidInput`.
- Agent not visible to the current user -> `ErrAgentNotFound`.
- Repository failure -> wrap with `%w` and return handler 500.

**Good/Base/Bad Cases**:
- Good: User selects `list_tasks` for a daemon-added Codex Agent, saves, reloads, and the Agent still has `allowed_tools:["list_tasks"]`.
- Good: Frontend can only select tool names returned by `ToolRegistry`; saving `list_conversation_agents` persists and reloads as the same canonical name.
- Base: User selects no tools; backend persists explicit empty allowlist and daemon exposes no platform tools.
- Bad: Frontend checkbox state updates but no backend API writes `agents.tools_config`.
- Bad: Saving tools through the full `PUT /api/agents/:id` path fails for non-custom/system Agents.
- Bad: `/api/tools/definitions` exposes stale `list_group_agents`; backend filters it during persistence, so the UI shows "saved" and then clears the selection.

**Tests Required**:
- Backend service test for `tools_config` normalization, unknown tool filtering, and tools-config persistence for visible daemon/system/custom Agents.
- Backend handler test that `/api/tools/definitions` is sourced from `ToolRegistry` and that builtin templates filter/alias stale names.
- Frontend E2E for selecting an Agent tool, saving, switching tabs, refreshing, and seeing the same canonical tool still selected.
- Daemon MCP test for filtered `tools/list` and unauthorized `tools/call`.
- End-to-end daemon MCP test where one Agent's config allows tool A and denies tool B.

**Wrong vs Correct**:
```go
// Wrong: UI tool catalog comes from persisted seed rows that may be stale.
definitions, _ := repo.List(ctx)

// Correct: UI tool catalog comes from the same runtime registry used for validation.
definitions := handler.definitionsFromRegistry()
```

```go
// Wrong: all MCP tools are always exposed.
server := mcp.NewServer("agenthub", "0.1.0", mcp.AllTools(), handler, logger)

// Correct: server list/call is constrained by the current Agent's tool config.
server := mcp.NewServer("agenthub", "0.1.0", mcp.AllTools(), handler, logger).WithAllowedTools(allowed)
```

```typescript
// Wrong: frontend appears saved, but non-custom Agents never persist in backend.
await updateAgent(agent.id, fullAgentPayload)

// Correct: tool assignment uses the dedicated persistence endpoint.
await updateAgentToolsConfig(agent.id, toolsConfig, hasManagementTools)
```

### Agent Platform Skills

**Scope / Trigger**: Applies when changing `agents.custom_skills`, Agent dispatch context construction, daemon prompt splitting, or the Agent Skills UI.

**Signatures**:
- API: `GET/POST/PUT/DELETE /api/platform-skills`
- API: `POST /api/platform-skills/import-defaults`
- API: `PUT /api/agents/:id/custom-skills`
- Request: `{"custom_skills": string}` where the string is a JSON array.
- Skill item fields: `name`, `category`, `description`, `trigger`, `detail`.
- Runtime injection entry point: `OrchestratorService.InjectAgentConfig(agent, contextStr, userID, taskText)`.

**Contracts**:
- `agents.capabilities_json` stores daemon-scanned native skills and may include local `source_path`; it is read-only user-facing discovery data.
- `platform_skills` stores the user's editable platform Skill library. It is independent from daemon-scanned native Skills.
- `agents.custom_skills` stores the platform Skills assigned to one Agent as dispatch/runtime snapshots. It must not be overwritten by daemon scans.
- Default platform Skill templates must include a stable `category` and use one consistent `detail` structure: `适用场景`, `输入要求`, `工作流程`, `输出格式`, and `质量检查`.
- Importing default platform Skills must be idempotent: existing same-name Skills are skipped instead of duplicated or overwritten, and the response should return the current default Skills available for assignment.
- Custom Skill persistence must keep only platform-safe fields: `name`, `category`, `description`, `trigger`, and `detail`. It must drop `source_path`, `auto`, and other local scan metadata.
- `name` is required; duplicate names collapse to the first valid item.
- `category`, `description`, `trigger`, and `detail` must be trimmed and length-limited before persistence and prompt injection.
- Agent dispatch prompts include a `[平台 Skills]` section with a compact Skill index for the current Agent.
- Skill `detail` is not injected into every prompt. It stays server-side and is progressively loaded through the read-only `get_agent_skill` MCP tool when the Agent needs the full instructions.
- `get_agent_skill` must be authorized by the current Agent's `tools_config` allowlist and must only return Skills from that same Agent's `custom_skills`.
- Orchestrator group Agent detail prompts must not expose raw `custom_skills` detail. They should continue using dispatch-safe description/tags only.
- Daemon prompt splitting must move `[平台 Skills]` into the system prompt area where the target CLI supports it; CLIs without a system prompt flag should receive it before the user prompt.

**Validation & Error Matrix**:
- Empty `custom_skills` -> saved as empty string, no Skill context injected.
- Invalid JSON -> `ErrAgentInvalidInput`.
- Non-array JSON -> `ErrAgentInvalidInput`.
- Empty Skill names -> skipped.
- Attempt to update another user's or non-custom Agent Skills -> `ErrAgentNotFound`.

**Good/Base/Bad Cases**:
- Good: Agent A has `custom_skills` with `trigger: "review, bug"`; prompt includes the Skill index and tells the Agent to call `get_agent_skill` for the detail when needed.
- Base: `get_agent_skill` is not authorized; prompt still includes the compact Skill index and the Agent works from that summary only.
- Bad: Prompt injects every Skill detail on every request, causing context bloat.
- Bad: Saving platform Skills preserves `source_path` from daemon-scanned native Skills.

**Tests Required**:
- Service test for custom Skill normalization, unsafe field filtering, and `trigger`/`detail` preservation.
- Service test for progressive prompt loading: index always present, lookup tool instruction present, raw detail omitted.
- Daemon MCP test for `get_agent_skill` scoping and authorization.
- Dispatch context test asserting `InjectAgentConfig` preserves existing blackboard/group context after the Skill section.
- Browser E2E verifying UI round-trip and API persistence for `tools_config` plus structured platform Skills.

**Wrong vs Correct**:
```go
// Wrong: platform Skills are only treated as display tags.
out = append(out, DiscoveredSkill{Name: name, Description: description})

// Correct: platform Skills keep trigger/detail for progressive loading,
// while local scan metadata stays out of persistence.
out = append(out, DiscoveredSkill{
    Name: name,
    Description: description,
    Trigger: trigger,
    Detail: detail,
})
```

### Conversation Context Blackboard

**Scope / Trigger**: Applies when changing message pin APIs, group-chat prompt context, or Agent dispatch context construction.

**Contracts**:
- `message_pins` is the source of truth for user-pinned long-term conversation context. Prompt code must query persisted pins rather than reconstructing from frontend state.
- `conversation_blackboards.manual_context` is the source of truth for user-authored long-term context. Agents must not write it until an explicit product phase adds that ability.
- Message history responses expose `pinned` so the chat UI can display and toggle pin state without a separate round-trip for each message.
- Agent prompts must include a `{会话上下文黑板}` block with `{用户 Pin 上下文}` and `{用户手写上下文}` for orchestrator dispatch, worker dispatch, direct Agent dispatch, and Agent one-on-one chats.
- Pin content must be length-limited and normalized before prompt insertion so a single pinned message cannot crowd out the current user request.
- Manual context must be length-limited before persistence and before prompt insertion so it cannot crowd out the current user request.

**Tests Required**:
- Assert orchestrator prompt construction includes the blackboard section.
- Assert `BuildConversationBlackboardContext` includes persisted pinned messages, user-authored context, and normalizes multi-line pin content.
- Run backend service/handler/repository tests and frontend build after changing the pin API or message shape.

### Message History and Delivery State

**Scope / Trigger**: Applies when changing message history APIs, Redis message state, offline/unread behavior, websocket post-persist flows, or Orchestrator async message delivery.

**Signatures**:
- API: `GET /api/conversations/:id/messages?before=&limit=` returns DB-backed message history.
- API: `GET /api/conversations/:id/messages/unread?limit=` may consume transient offline delivery state before falling back to DB `last_read_at`.
- Service: `MessageDeliveryState` and `OrchDeliveryState` expose offline queue and unread-count operations only.
- Redis keys: `offline:{userID}:{conversationID}` and `unread:{userID}:{conversationID}`.

**Contracts**:
- PostgreSQL `messages` plus attachment/artifact tables are the source of truth for message history.
- Message history reads must call the message repository (`ListByConversation`) and must not return Redis `msgs:*` or any other hot-cache snapshot.
- Redis may store transient delivery state only: offline queues and unread counters. It must not be required to reconstruct the conversation after refresh.
- `postPersist` and Orchestrator `postPersistAsync` must push to WebSocket, then record offline queue/unread state for non-sender members.
- Assistant and Orchestrator async messages are already persisted before delivery state is recorded; browser refresh recovery must come from DB history.
- Pin/unpin/recall should update the DB and push events as needed; they must not rely on Redis history-cache invalidation.

**Validation & Error Matrix**:
- Missing conversation or no membership -> existing message service not-found/permission errors.
- Redis unavailable -> history still works from DB; offline/unread delivery degrades but persisted messages remain recoverable.
- Offline queue malformed payload -> skip bad item and continue with valid messages.

**Good/Base/Bad Cases**:
- Good: Hard-refreshing a chat calls DB-backed history and matches `/api/conversations` latest-message summary.
- Base: Offline users receive queued messages through `messages/unread`; if the queue is empty, DB `last_read_at` fallback still works.
- Bad: `GET /messages` returns Redis `msgs:<conversationID>` and shows a stale 50-message snapshot while the sidebar shows fresh DB data.
- Bad: Code adds `InvalidateCache` calls after pin/unpin instead of keeping history DB-backed.

**Tests Required**:
- Service-test `GetHistory` with delivery state injected and assert repository messages are returned.
- Run backend service/repository tests after changing Redis delivery state.
- Run frontend build after changing message API response shape or client merge behavior.

**Wrong vs Correct**:
```go
// Wrong: history reads from transient Redis state.
cached, _ := delivery.GetCachedMessages(ctx, convID, limit)
if len(cached) > 0 { return cached, nil }

// Correct: history always reads source-of-truth rows.
messages, err := msgRepo.ListByConversation(ctx, convID, before, limit)
```

### Agent Prompt Templates

**Scope / Trigger**: Applies when changing Agent creation flows, editable system prompt templates, or template CRUD APIs.

**Signatures**:
- DB: `agent_prompt_templates(id, user_id, name, category, description, system_prompt, created_at, updated_at)`.
- API: `GET/POST/PUT/DELETE /api/agent-prompt-templates`.
- API: `POST /api/agent-prompt-templates/import-defaults`.
- Frontend: `AgentCreateModal` consumes `AgentPromptTemplateField` for the system prompt field.

**Contracts**:
- Prompt templates are user-scoped server data, not frontend-only constants.
- `(user_id, name)` is unique; importing defaults is idempotent and skips existing names.
- `name` is required. `category` defaults to `通用`. `description` and `system_prompt` are trimmed and length-limited.
- Selecting a template in Agent creation copies `system_prompt` into the draft Agent request; the Agent stores the resulting prompt snapshot, not a live template reference.
- Template CRUD must not require a connected daemon. It is account configuration and should work before/after machine connection.

**Validation & Error Matrix**:
- Missing user ID or empty name -> `ErrAgentPromptTemplateInvalid` / 400.
- Duplicate template name for the same user -> `ErrAgentPromptTemplateDuplicate` / 409.
- Update/delete another user's or missing template -> `ErrAgentPromptTemplateNotFound` / 404.
- Database failure -> wrapped with `%w` and handled as 500.

**Good/Base/Bad Cases**:
- Good: User imports defaults, selects "代码实现 Agent", edits the prompt, and creates a machine candidate Agent with that edited prompt.
- Base: User deletes or renames a template after creating an Agent; existing Agents keep their stored prompt snapshot.
- Bad: Prompt templates are hard-coded only in the frontend, so another browser or server cannot list/edit them.
- Bad: Agent rows store a template ID and silently change behavior when the template is later edited.

**Tests Required**:
- Service tests for normalization, duplicate/default import behavior, update-not-found, and delete-not-found.
- Frontend build after changing template request/response types or creation modal wiring.
- Browser smoke test for opening Agent creation and loading `/api/agent-prompt-templates`.

**Wrong vs Correct**:
```go
// Wrong: candidate Agent creation depends on a mutable template row.
agent.SystemPromptTemplateID = req.TemplateID

// Correct: template selection copies the prompt into the Agent draft.
agent.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
```

### Uploaded File Storage URLs

**Scope / Trigger**: Applies when changing chat attachment upload, knowledge-base file upload, static uploaded-file serving, or the `upload` config.

**Signatures**:
- Config: `upload.dir`, `upload.public_base_url`, `upload.max_image_mb`, `upload.max_pdf_mb`.
- API: `POST /api/upload` returns `file_path`, `thumbnail_path`, `url`, and `thumbnail_url`.
- API: message payloads expose attachment `url` and `thumbnail_url` as computed JSON fields.
- API: knowledge file payloads expose `url` as a computed JSON field.
- Storage DB: `message_attachments.file_path`, `message_attachments.thumbnail_path`, and `knowledge_files.file_path` store relative paths only.

**Contracts**:
- Binary file contents live under `upload.dir`; the database stores metadata and relative storage paths, not file bytes.
- `upload.public_base_url` is optional. Empty means return relative API URLs such as `/api/uploads/originals/a.png`; non-empty means prefix that API path with the configured origin.
- `FileURLBuilder` is the single place that converts storage paths to API/public URLs. Do not hand-roll public URL concatenation in handlers, repositories, or frontend-only logic.
- Message-service responses must enrich attachment URLs after database history/search reads and after offline queue reads so changing `upload.public_base_url` does not require rewriting persisted messages.
- Static uploaded-file serving remains authenticated by `/api/uploads/*filepath` and must reject path traversal before joining with `upload.dir`.
- Knowledge-base file content remains permission-checked by `GET /api/knowledge-bases/:id/files/:fileId/content`; the `url` field points to that checked endpoint.

**Validation & Error Matrix**:
- Empty upload -> `ErrUploadEmpty` / 400.
- Unsupported extension or detected MIME -> `ErrUploadTypeInvalid` / 400.
- File over configured size limit -> `ErrUploadTooBig` / 413.
- Uploaded-file path containing `..` -> 403.
- Missing physical file -> 404 from the serving endpoint.
- Knowledge file without permission -> `ErrKBNoPermission` / 403.

**Good/Base/Bad Cases**:
- Good: Production sets `upload.dir: "/root/agenthub-data/uploads"` and `upload.public_base_url: "https://agenthub.example.com"`; responses carry absolute URLs while DB paths remain relative.
- Base: Local dev leaves `upload.public_base_url` empty; frontend uses relative `/api/...` URLs.
- Bad: Code stores `http://server-ip/...` in `message_attachments.file_path`, making server migration require database rewrites.
- Bad: Offline queued messages return stale or empty URLs because enrichment only happens in repository reads.

**Tests Required**:
- Unit-test `FileURLBuilder` for relative and absolute public URL modes.
- Service-test message history/offline enrichment for `url` and `thumbnail_url`.
- Run backend service/handler/repository tests after changing upload or static serving.
- Run frontend type-check/build after adding attachment or knowledge-file response fields.

**Wrong vs Correct**:
```go
// Wrong: public URL is persisted or concatenated ad hoc.
attachment.FilePath = "http://111.228.35.61:8080/api/uploads/originals/a.png"

// Correct: persist a relative path and compute the URL at the service boundary.
attachment.FilePath = "uploads/originals/a.png"
attachment.URL = fileURLs.UploadURL(attachment.FilePath)
```

### Daemon CLI One-Shot Context

**Scope / Trigger**: Applies when changing daemon one-shot adapters for Codex, OpenCode, OpenClaw, or any CLI that uses global/config-file MCP registration instead of Claude Code's per-command `--mcp-config`.

**Signatures**:
- Codex command: `codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --ephemeral --color never --output-last-message <file> <prompt>`.
- Codex config: `$CODEX_HOME/config.toml` section `[mcp_servers.agenthub-platform]`.
- One-shot env: `AGENTHUB_CONVERSATION_ID`, `AGENTHUB_USER_ID`, `AGENTHUB_AGENT_ID`, and `AGENTHUB_TASK_ID`.
- Daemon MCP command args: `node <agenthub-daemon.js> --server-url <url> --api-key <key> --mcp --conversation-id <id> --user-id <id> --agent-id <id> --task-id <id>`.

**Contracts**:
- Codex must default to the user's normal `CODEX_HOME` (`$CODEX_HOME` if set, otherwise `~/.codex`) so daemon scan-time login detection and task execution use the same auth store.
- `AGENTHUB_CODEX_HOME` is an explicit override for isolated Codex homes; do not silently switch to `~/.agenthub/codex`.
- Codex/OpenCode one-shot env must include `AGENTHUB_TASK_ID` when a task ID is available so MCP subprocesses can emit task-scoped cards.
- Codex per-task MCP config must pass `--task-id` to the daemon MCP subprocess, matching Claude Code's `buildPlatformMcpArgs(..., taskId)` behavior.
- Codex `agenthub-platform` MCP config must set `default_tools_approval_mode = "approve"` because one-shot automation cannot surface interactive tool approval prompts reliably.
- Codex and OpenCode remain one-shot unless their spec explicitly implements and tests `spawnPersistent`; do not route them into the Claude persistent slot by changing only dispatcher conditions.
- Tests that call Codex `commandForTask` must set `AGENTHUB_CODEX_HOME` to a temp directory to avoid mutating the developer's real `~/.codex/config.toml`.

**Validation & Error Matrix**:
- Missing daemon server URL/API key while building MCP config -> skip writing the MCP server section; command construction still succeeds.
- Missing Codex login in the effective `CODEX_HOME` -> Codex CLI fails at execution; daemon should report the CLI error instead of claiming the Agent is usable from a different home.
- Missing `task_id` -> MCP tool card emission logs `card.emit_no_task` and the tool result still returns.
- MCP tool approval prompt during one-shot -> treated as adapter misconfiguration; set `default_tools_approval_mode = "approve"`.

**Good/Base/Bad Cases**:
- Good: `codex login status` is true for `~/.codex`, daemon Codex task executes with `CODEX_HOME=~/.codex`, and platform MCP receives conversation/user/agent/task IDs.
- Base: Operator sets `AGENTHUB_CODEX_HOME=/secure/codex-home`; scan and execution must be checked against that same home.
- Bad: scan uses default `~/.codex` and execution uses empty `~/.agenthub/codex`, so the UI shows Codex online but tasks fail as not logged in.
- Bad: Codex one-shot MCP config omits `--task-id`, so deployment/card-producing tools complete without attaching cards to the task.

**Tests Required**:
- Codex spec unit test asserts `buildCommand` passes task ID to `ensureAgentHubCodexMcpConfig` and `buildAgentHubContextEnv`.
- Daemon command test asserts Codex one-shot env contains `CODEX_HOME` and all `AGENTHUB_*` context keys, including `AGENTHUB_TASK_ID`.
- Daemon MCP config test asserts `[mcp_servers.agenthub-platform]` includes `--task-id` and `default_tools_approval_mode = "approve"`.
- OpenCode command test asserts one-shot env includes `AGENTHUB_TASK_ID` when task context exists.

**Wrong vs Correct**:
```js
// Wrong: execution uses an empty isolated home even though scan saw ~/.codex login.
const codexHome = path.join(os.homedir(), '.agenthub', 'codex');

// Correct: reuse the same Codex auth/config home by default; allow explicit override.
const codexHome = process.env.AGENTHUB_CODEX_HOME || process.env.CODEX_HOME || path.join(os.homedir(), '.codex');
```

---

## Testing Requirements

(To be filled by the team)

---

## Code Review Checklist

- For branch integrations, verify that generated backend binaries are ignored or removed from the index.
