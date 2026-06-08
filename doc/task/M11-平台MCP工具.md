# M11 平台 MCP 工具

## 目标

本机 daemon 暴露一个 `agenthub-platform` MCP server，让支持 MCP 的 Agent（Claude Code / Codex 等）通过标准 MCP 工具直接操作 AgentHub 平台（发消息、查会话、建群等），替代现有 prompt 内注入 curl 说明的方式。每个自建 Agent 通过 `agents.tools_config` 单独配置可用工具，runtime 按 `agent_id` 强制过滤 `tools/list` 与 `tools/call`。

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
| `list_conversation_agents` | `GET /api/conversations/:id/agents` |
| `list_group_agents` | `GET /api/conversations/:id/agents` |
| `get_messages` | `GET /api/conversations/:id/messages?limit=` |
| `create_group` | `POST /api/groups` |
| `get_group_info` | `GET /api/groups/:id` |
| `list_group_members` | `GET /api/groups/:id/members` |
| `list_tasks` | `GET /api/tasks` |
| `create_task` | `POST /api/tasks` |
| `update_task` | `PUT /api/tasks/:id` |
| `move_task_status` | `POST /api/tasks/:id/status` |
| `delete_task` | `DELETE /api/tasks/:id` |
| `list_agents` | `GET /api/agents` |
| `list_agent_candidates` | `GET /api/daemon/agent-candidates` |
| `list_machines` | `GET /api/daemon/machines` |

### M11-5 per-Agent 工具授权（已完成）

- `agents.tools_config` 支持 `{"toolset": string, "allowed_tools": string[]}`。
- 后端保存 Agent 配置时过滤未知工具名，避免 UI 或旧配置授予不存在的工具。
- daemon MCP server 启动时按当前 `agent_id` 查询后端 Agent 配置。
- `tools/list` 只返回该 Agent 被授权的工具；`tools/call` 在执行 handler 前拒绝未授权工具。
- 显式 `allowed_tools: []` 或 `toolset: "none"` 表示无工具；无法解析的旧文本配置不授予工具。

### M11-4 自动注入（已完成，全 CLI 覆盖）

按各 CLI 的能力分两条路径，凭证统一复用轮询 daemon 自身的 machine key：

- **Claude Code（按次注入）**：`commandForTask` 的 claude 分支自动追加
  `--mcp-config <inline JSON>` + `--allowedTools mcp__agenthub-platform`，每个聊天任务挂载本 daemon 的 `--mcp` server。
- **OpenClaw / Codex（启动时全局注入）**：`agent`/`exec` 无按次 flag，故 daemon 启动（轮询模式）时 `ensureGlobalMcpConfigs` 幂等写入各自全局 MCP 配置：
  - OpenClaw：`openclaw mcp set agenthub-platform <json>`
  - Codex：`codex mcp remove`（忽略错误）+ `codex mcp add agenthub-platform -- node <daemon> ... --mcp`
  - 仅对本机已安装的 CLI 执行，失败仅告警、不阻断轮询。
- **MCP server 命令统一用 `node`（PATH 解析）而非 `process.execPath`**，规避 `C:\Program Files\nodejs` 空格在子进程参数转义中被拆断的问题。

## 验收标准

- [x] 机器 key 可换取带 `user_id` 的 scoped JWT
- [x] MCP 握手、`tools/list`、`tools/call` 全流程可用
- [x] 5 个工具均真实命中后端，且携带换取的 JWT
- [x] 派发 claude 任务时自动注入平台 MCP（命令参数验证通过）
- [x] daemon 启动时为 OpenClaw 幂等写入全局 MCP 配置（set/show/unset 实测通过）
- [x] daemon 为 Codex 幂等写入全局 MCP 配置（用 VSCode 扩展自带 codex.exe，add/get/remove 实测通过）
- [x] 真实端到端：真 daemon(--mcp) + 真后端 + 真 machine key，`list_agents`/`list_conversations` 返回真实后端数据（换 token → 带 JWT 调 REST 全链路打通）
- [x] per-Agent 工具授权生效：无 `agent_id` 或未知 Agent 不暴露工具，显式空工具集不回退默认工具，未授权 `tools/call` 被拒绝

## 备注：Codex 可用性

- daemon `resolveCommand('codex')` 已自动定位 VSCode ChatGPT 扩展自带的 `codex.exe`（`~/.vscode/extensions/openai.chatgpt-*/bin/windows-x86_64/codex.exe`），无需 codex 在 PATH。
- codex CLI 自身的 ChatGPT 登录独立于 openclaw 的 openai-codex provider 配置；前者有效即可作为 agent 与平台 MCP 使用。

## 依赖

- M4（daemon + 机器接入）
- 现有 `agent_management` JWT 与平台 REST 端点

## 优先级

P1
