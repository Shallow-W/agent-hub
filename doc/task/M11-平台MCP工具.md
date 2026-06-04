# M11 平台 MCP 工具

## 目标

本机 daemon 暴露一个 `agenthub-platform` MCP server，让支持 MCP 的 Agent（Claude Code / Codex 等）通过标准 MCP 工具直接操作 AgentHub 平台（发消息、查会话、建群等），替代现有 prompt 内注入 curl 说明的方式。

## 背景

- 现状：平台操作能力通过 [agent_tools.go](../../src/backend/internal/service/agent_tools.go) 以 markdown + curl 形式注入 Agent prompt，明确要求"不要用 MCP"。
- 底层范式已具备：`agent_management` scope 的 JWT（[agent.go](../../src/backend/internal/service/agent.go) `GenerateAgentToken`，5 分钟有效）+ 现成 REST 端点 + `middleware.Auth` 仅校验 `user_id`（scoped token 可直接调用）。
- 本任务把这套 curl 范式升级为正规 MCP server，后端几乎不改。

## 子任务

### M11-1 后端：机器 key 换 scoped JWT（已完成）

- 新增 `GET /daemon/agent-token`，以 per-machine api-key 鉴权，签发该机器所属用户的 `agent_management` JWT。
- 复用 `AgentService.GenerateAgentToken`，不接受全局 daemon token（无用户归属）。

### M11-2 daemon：`--mcp` 模式 + 手写 stdio JSON-RPC server（已完成）

- `agent-hub daemon --server-url <url> --api-key <key> --mcp` 进入纯 MCP stdio 模式。
- 零依赖手写 JSON-RPC（换行分隔）：`initialize` / `notifications/initialized` / `ping` / `tools/list` / `tools/call`。
- 日志全部走 stderr，stdout 只承载协议报文。
- 启动用机器 key 换 JWT 并缓存到临近过期；REST 调用遇 401 刷新一次重试。

### M11-3 首批 5 个工具（已完成）

| 工具 | REST |
|------|------|
| `list_conversations` | `GET /api/conversations` |
| `get_messages` | `GET /api/conversations/:id/messages?limit=` |
| `send_message` | `POST /api/conversations/:id/messages` |
| `create_group` | `POST /api/groups` |
| `list_agents` | `GET /api/agents` |

### M11-4 派发任务自动注入（claude 已完成）

- daemon 派发 claude 任务时（[agenthub-daemon.js](../../src/daemon-npm/bin/agenthub-daemon.js) `commandForTask`）自动追加 `--mcp-config <inline JSON>` + `--allowedTools mcp__agenthub-platform`，把本 daemon 以 `--mcp` 作为 stdio MCP server 挂上，聊天任务原生可调平台工具，无需手动 `claude mcp add`。
- 凭证复用轮询 daemon 自身的 machine key（`daemonConn`）。
- OpenClaw / Codex 的 `agent`/`exec` 子命令无按次注入能力，需走各自全局 MCP 配置（如 `openclaw mcp add`），不在自动注入范围内。

## 验收标准

- [x] 机器 key 可换取带 `user_id` 的 scoped JWT
- [x] MCP 握手、`tools/list`、`tools/call` 全流程可用
- [x] 5 个工具均真实命中后端，且携带换取的 JWT
- [x] 派发 claude 任务时自动注入平台 MCP（命令参数验证通过）
- [ ] 在真实 Claude Code 聊天任务中由 Agent 实际调用端到端验证

## 依赖

- M4（daemon + 机器接入）
- 现有 `agent_management` JWT 与平台 REST 端点

## 优先级

P1
