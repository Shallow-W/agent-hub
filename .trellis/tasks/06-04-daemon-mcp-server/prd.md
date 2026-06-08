# PRD: Daemon 内置 MCP Server

## 背景

AgentHub 作为多 Agent 协作平台，需要让不同类型的 agent（Claude Code、Codex、OpenCode 等）能够操作平台功能（发送消息、创建会话、管理上下文等）。目前各 agent 的 tool 调用协议不同，逐个适配成本高。

## 目标

在现有 daemon 进程中内置一个 MCP Server（stdio 模式），将平台管理能力统一封装为 MCP tool，所有支持 MCP 协议的 agent 只需配置一条 stdio 指令即可使用。

## 设计方案

### 架构

```
┌─────────────────────────────────────────────────┐
│  Daemon 进程                                      │
│                                                    │
│  ┌──────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │ Scanner  │  │ Adapter 层   │  │ MCP Server  │ │
│  │ (现有)    │  │ (现有)       │  │ (新增)       │ │
│  └──────────┘  └──────────────┘  └──────┬──────┘ │
│                                         │ stdio   │
└─────────────────────────────────────────┼─────────┘
                                          │
                    ┌─────────────────────┼──────────────────┐
                    │                     │                    │
              Claude Code             Codex               OpenCode
```

### 启动方式

daemon 新增 `--mcp` 子命令模式：
- `agent-hub daemon`：正常模式（扫描 + 注册 + 进程管理）
- `agent-hub daemon --mcp`：仅启动 MCP Server（供 agent 配置使用）

两种模式共享同一份 HTTP client 代码（调用后端 API）。

### Tool 划分

#### 第一期（核心工具）

| Tool | 描述 | 对应后端 API |
|------|------|-------------|
| `list_conversations` | 获取用户的会话列表 | GET /api/conversations |
| `get_conversation` | 获取单个会话详情（含最近消息） | GET /api/conversations/:id |
| `send_message` | 在指定会话中发送消息 | POST /api/conversations/:id/messages |
| `create_conversation` | 创建新会话（单聊/群聊） | POST /api/conversations |
| `list_messages` | 分页获取会话消息历史 | GET /api/conversations/:id/messages |
| `list_agents` | 获取可用 agent 列表 | GET /api/agents |
| `search_messages` | 按关键词搜索消息 | GET /api/messages/search |

#### 第二期（扩展工具）

| Tool | 描述 |
|------|------|
| `pin_message` | Pin 消息到上下文 |
| `get_pinned_context` | 获取当前 pinned 上下文 |
| `dispatch_task` | 向指定 agent 分派任务 |
| `upload_artifact` | 上传产物文件 |
| `list_artifacts` | 获取会话产物列表 |

### 技术选型

- **MCP SDK**：使用 Go 官方 MCP SDK (`github.com/modelcontextprotocol/go-sdk`) 或手写 JSON-RPC 2.0 处理（协议本身很薄）
- **通信方式**：stdio（标准输入输出），agent 通过 `command` 字段配置启动
- **认证**：daemon MCP 模式启动时通过环境变量 `AGENTHUB_DAEMON_TOKEN` 或 flag 传入 token，用于调用后端 API
- **与后端通信**：复用现有 HTTP API，daemon 作为 API client（不直接连数据库）

### 配置示例

Claude Code 的 `claude_desktop_config.json`：
```json
{
  "mcpServers": {
    "agenthub": {
      "command": "agent-hub",
      "args": ["daemon", "--mcp"],
      "env": {
        "AGENTHUB_SERVER_URL": "http://localhost:8080",
        "AGENTHUB_DAEMON_TOKEN": "<token>"
      }
    }
  }
}
```

### 文件结构

```
src/daemon/
├── mcp/
│   ├── server.go          # MCP Server 主入口（JSON-RPC 2.0 + tool 注册）
│   ├── tools.go           # tool 定义和 schema
│   ├── handlers.go        # tool 执行逻辑
│   └── server_test.go     # 单元测试
├── adapter/               # 现有，不变
├── client/                # 现有，扩展 HTTP API 调用能力
├── scanner/               # 现有，不变
└── main.go                # 新增 --mcp 分支
```

## 验收标准

1. `agent-hub daemon --mcp` 启动后通过 stdio 响应 MCP `initialize` 和 `tools/list` 请求
2. 第一期 7 个 tool 全部可用，调用后端 API 返回正确结果
3. Claude Code 连接后能自动发现 tool 并调用
4. 单元测试覆盖 tool handler 逻辑（mock 后端 API）
5. 认证失败（token 无效）时返回明确错误信息，不 panic

## 不做的事

- 不做 MCP Resource / Prompt 能力，只做 Tool
- 不做 SSE/Streamable HTTP 传输模式，只做 stdio
- 不修改 daemon 正常模式的现有行为
- 不直接连数据库，全部通过 HTTP API
