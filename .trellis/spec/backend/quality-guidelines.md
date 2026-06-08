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

**Contracts**:
- `agents.tools_config` is the per-Agent MCP tool authorization config. The supported JSON shape is `{"toolset": string, "allowed_tools": string[]}`.
- `allowed_tools` must only contain known platform tool names. Unknown names are filtered before persistence.
- Legacy non-JSON `tools_config` text may be preserved for display, but it must not grant extra MCP tools.
- MCP `tools/list` must only return tools allowed for the current `agent_id`.
- MCP `tools/call` must reject unauthorized tool names before executing the tool handler.
- MCP sessions without a resolved `agent_id`, or with an unknown Agent, must fail closed and expose no tools.
- Explicit JSON `allowed_tools: []` means no tools; it must not fall back to default tools.
- Hiding tools in prompts or UI is not sufficient; runtime tool calls must enforce the same allowlist.

**Tests Required**:
- Backend service test for `tools_config` normalization and unknown tool filtering.
- Daemon MCP test for filtered `tools/list` and unauthorized `tools/call`.
- End-to-end daemon MCP test where one Agent's config allows tool A and denies tool B.

**Wrong vs Correct**:
```go
// Wrong: all MCP tools are always exposed.
server := mcp.NewServer("agenthub", "0.1.0", mcp.AllTools(), handler, logger)

// Correct: server list/call is constrained by the current Agent's tool config.
server := mcp.NewServer("agenthub", "0.1.0", mcp.AllTools(), handler, logger).WithAllowedTools(allowed)
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
- Cache invalidation is required after pin/unpin because message history may be served from Redis.

**Tests Required**:
- Assert orchestrator prompt construction includes the blackboard section.
- Assert `BuildConversationBlackboardContext` includes persisted pinned messages, user-authored context, and normalizes multi-line pin content.
- Run backend service/handler/repository tests and frontend build after changing the pin API or message shape.

---

## Testing Requirements

(To be filled by the team)

---

## Code Review Checklist

- For branch integrations, verify that generated backend binaries are ignored or removed from the index.
