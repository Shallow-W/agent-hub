# 系统架构概览

## 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        浏览器 (SPA)                         │
│          React + TypeScript + Vite + Zustand                │
└──────────┬──────────────────────────────┬───────────────────┘
           │ HTTP REST                    │ WebSocket
           ▼                              ▼
┌──────────────────────────────────────────────────────────────┐
│                    后端 API Server (Go)                       │
│              Gin + pgx/sqlx + nhooyr/websocket               │
│  ┌────────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │  Handler    │ │ Service  │ │ Repository│ │  WS Hub      │  │
│  │  (路由/校验)│ │ (业务逻辑)│ │ (数据访问)│ │ (消息分发)   │  │
│  └────────────┘ └──────────┘ └─────┬────┘ └──────┬───────┘  │
└─────────────────────────────────────┼────────────┼───────────┘
                                      │            │
                                      ▼            │
┌──────────────────────────────────────────────┐    │
│            PostgreSQL 15                     │    │
│   users / conversations / messages           │    │
└──────────────────────────────────────────────┘    │
                                                    │
┌──────────────────────────────────────────────┐    │
│            Daemon 守护进程 (Go)               │◄───┘
│  ┌─────────┐ ┌─────────┐ ┌────────────────┐  │
│  │ Scanner │ │ Adapter │ │ ProcessManager │  │
│  │ (发现)  │ │ (适配)  │ │ (进程管理)     │  │
│  └─────────┘ └─────────┘ └────────────────┘  │
│         │                                     │
│         ▼                                     │
│  ┌─────────────────────────────────┐          │
│  │  Agent CLI (Claude/Codex/...)   │          │
│  └─────────────────────────────────┘          │
└──────────────────────────────────────────────┘
```

## 数据流

用户发送消息的完整流程：

```
1. 用户在浏览器输入消息
2. 前端 POST /api/conversations/:id/messages
3. Handler 校验 JWT → 调用 Service
4. Service 写入 PostgreSQL
5. Service 通过 WS Hub 广播 message.new 事件
6. 所有已连接的客户端（含发送者）收到 WebSocket 推送
7. Daemon 监听到用户消息 → 分派给对应 Agent CLI
8. Agent 逐 token 输出 → Daemon 通过 WebSocket 推送 message.stream
9. 前端实时渲染流式内容
```

## 组件说明

### Frontend SPA
单页应用，负责对话列表、聊天窗口、消息渲染。通过 REST API 操作数据，通过 WebSocket 接收实时推送。

### Backend API Server
Go 实现的 HTTP 服务，采用分层架构（Handler → Service → Repository）。JWT 鉴权，Gin 路由，pgx 驱动 PostgreSQL。

### WebSocket Hub
内嵌于后端的 WebSocket 消息中心。管理连接池，按 conversation ID 分组广播消息。支持流式推送（Agent 逐 token 输出）。

### PostgreSQL
主数据存储，保存用户、对话、消息等核心数据。

### Daemon
本地守护进程，负责发现已安装的 Agent CLI 工具、管理 Agent 子进程生命周期、适配不同 Agent 的输出格式，并通过 WebSocket 与后端通信。

## 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 前端框架 | React 18 + TypeScript | SPA |
| 前端构建 | Vite | 快速 HMR |
| 状态管理 | Zustand | 轻量级 |
| 前端路由 | React Router v6 | |
| 前端样式 | CSS Modules | 作用域隔离 |
| 后端框架 | Go + Gin | HTTP 路由 |
| 数据库驱动 | pgx / sqlx | PostgreSQL |
| WebSocket | nhooyr/websocket | Go 标准 |
| 配置管理 | koanf | |
| 日志 | slog | Go 标准库 |
| 数据库 | PostgreSQL 15 | |
| 容器化 | Docker Compose | 本地开发 |
| 守护进程 | Go | Agent 管理 |

## 目录结构

详细目录说明见 `doc/conventions/project-structure.md`。简要概览：

```
agent-hub/
├── src/
│   ├── frontend/          # React SPA
│   ├── backend/           # Go API Server
│   └── daemon/            # Agent 守护进程
├── doc/                   # 文档
│   ├── architecture/      # 架构设计
│   ├── conventions/       # 编码规范
│   ├── design/            # 详细设计
│   ├── reference/         # API 参考
│   └── task/              # 任务详情
├── scripts/               # 开发脚本
├── bin/                   # 构建产物
└── docker-compose.yml     # 本地开发环境
```
