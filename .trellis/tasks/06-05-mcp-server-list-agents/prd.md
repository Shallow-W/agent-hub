# MCP Server 认证重构 + list_agents 瘦身

## Goal

重构 MCP Server 的认证机制：后端新增 `/mcp/` 路由组用 daemon token 认证替代 JWT；同时修复 `list_agents` 返回 8.7MB 数据和 `delete_task` 返回 null 的问题。

## Requirements

* 后端新增 `/mcp/` 路由组，使用 daemon token（`config.yaml` 中 `daemon.token`）做认证，不依赖用户 JWT
* MCP server 的 `APIClient` 用 daemon token 作为 Bearer token 调用 `/mcp/` 路由而非 `/api/` 路由
* 工具参数保持不变，conversation_id 仍然在调用时传入（不绑定到 env var）
* `list_agents` 裁剪返回字段，去掉 capabilities_json / system_prompt / tools_config 等大字段
* `delete_task` 返回确认信息而非 null

## Acceptance Criteria

* [ ] MCP 用 daemon token 可正常调用所有工具
* [ ] `list_agents` 响应体 < 10KB
* [ ] `delete_task` 返回有意义的确认信息（如 `{"deleted": true, "id": "..."}`)
* [ ] 现有 12 个 MCP 工具全部功能正常
* [ ] build 通过，本地端到端测试通过

## Definition of Done

* `go build` / `go vet` 通过
* 本地 MCP 端到端测试通过（initialize → tools/list → tools/call）

## Out of Scope

* 细粒度权限控制（群内成员区分）
* per-conversation API key 生成
* MCP server 环境变量重命名（保持 AGENTHUB_DAEMON_TOKEN 不变，语义改为 daemon 共享 token）

## Technical Approach

### 后端改动

1. 新增 `MCPAuth` 中间件：校验 Bearer token == config daemon.token
2. 新增 `/mcp/` 路由组，挂载 MCPAuth 中间件
3. `/mcp/` 路由复用现有 handler，但注入系统上下文（无 user_id 依赖）
4. `/mcp/tasks` 相关 handler 不依赖 `c.Get("user_id")`
5. `/mcp/agents` 返回瘦身后的 agent 列表（去掉大字段）
6. `/mcp/tasks/:id` DELETE 返回确认信息

### Daemon 改动

1. `APIClient` 的 baseURL path 从 `/api/` 改为 `/mcp/`
2. 保持 `AGENTHUB_DAEMON_TOKEN` 环境变量，语义明确为 daemon 共享 token

## Technical Notes

* 相关文件：
  - `src/daemon/mcp/handlers.go` — APIClient 路径 + HandleAllTools
  - `src/backend/cmd/server/main.go` — 路由注册
  - `src/backend/internal/middleware/auth.go` — 参考 JWT 中间件写 MCPAuth
  - `src/backend/internal/handler/task.go` — task handler 需兼容无 user_id 场景
  - `src/backend/internal/handler/agent.go` — list_agents 需新增瘦身版本
* 后端 `authenticateMachine()` 已支持 daemon token（返回 nil = 系统模式）
