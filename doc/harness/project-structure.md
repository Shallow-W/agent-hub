# 项目结构规范

## Monorepo 整体布局

```
agent-hub/
├── CLAUDE.md              # Claude Code 指令文件（入口）
├── doc/                   # 文档
│   ├── 需求文档.md
│   ├── AgentHub-_多Agent协作平台设计.pdf
│   └── harness/           # 开发规范
│       ├── git-conventions.md
│       ├── frontend-conventions.md
│       ├── backend-conventions.md
│       ├── doc-conventions.md
│       └── project-structure.md
├── src/                   # 源代码
│   ├── frontend/              # React 前端
│   │   ├── src/
│   │   │   ├── assets/        # 静态资源（图片、字体、图标）
│   │   │   ├── components/    # 通用/可复用组件（按业务域组织）
│   │   │   │   ├── common/    #   基础UI组件
│   │   │   │   ├── chat/      #   聊天相关组件
│   │   │   │   │   ├── ChatWindow.tsx
│   │   │   │   │   ├── MessageList.tsx
│   │   │   │   │   ├── MessageBubble.tsx
│   │   │   │   │   └── ChatInput.tsx
│   │   │   │   ├── sidebar/   #   侧边栏组件
│   │   │   │   │   ├── ConversationList.tsx
│   │   │   │   │   └── ConversationItem.tsx
│   │   │   │   ├── agent/     #   Agent管理组件
│   │   │   │   │   ├── AgentList.tsx
│   │   │   │   │   └── AgentCreator.tsx
│   │   │   │   └── preview/   #   产物预览组件
│   │   │   │       ├── CodeCard.tsx
│   │   │   │       ├── WebpageCard.tsx
│   │   │   │       └── FileCard.tsx
│   │   │   ├── views/         # 页面组件（与路由1:1对应）
│   │   │   │   ├── LoginView.tsx
│   │   │   │   ├── RegisterView.tsx
│   │   │   │   ├── ChatView.tsx
│   │   │   │   ├── AgentsView.tsx
│   │   │   │   ├── AgentCreateView.tsx
│   │   │   │   └── SettingsView.tsx
│   │   │   ├── layout/        # 布局组件
│   │   │   │   ├── AuthLayout.tsx
│   │   │   │   └── AppLayout.tsx
│   │   │   ├── router/        # 路由配置
│   │   │   │   └── index.tsx
│   │   │   ├── store/         # 状态管理（Zustand）
│   │   │   ├── api/           # API封装（REST + WebSocket）
│   │   │   │   ├── conversation.ts
│   │   │   │   ├── message.ts
│   │   │   │   ├── agent.ts
│   │   │   │   ├── auth.ts
│   │   │   │   └── websocket.ts
│   │   │   ├── hooks/         # 自定义Hooks
│   │   │   │   ├── useWebSocket.ts
│   │   │   │   ├── useConversation.ts
│   │   │   │   └── useAuth.ts
│   │   │   ├── types/         # TypeScript类型定义
│   │   │   │   ├── message.ts
│   │   │   │   ├── conversation.ts
│   │   │   │   ├── agent.ts
│   │   │   │   └── artifact.ts
│   │   │   ├── utils/         # 工具函数
│   │   │   ├── styles/        # 全局样式
│   │   │   ├── App.tsx
│   │   │   └── main.tsx
│   │   ├── public/
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── vite.config.ts     # 或 next.config.js
│   ├── backend/               # Go 后端
│   │   ├── cmd/
│   │   │   └── server/
│   │   │       └── main.go    # 程序入口
│   │   ├── internal/
│   │   │   ├── handler/       # HTTP/WebSocket处理器
│   │   │   │   ├── auth.go
│   │   │   │   ├── conversation.go
│   │   │   │   ├── message.go
│   │   │   │   ├── agent.go
│   │   │   │   └── websocket.go
│   │   │   ├── service/       # 业务逻辑层
│   │   │   │   ├── auth.go
│   │   │   │   ├── conversation.go
│   │   │   │   ├── orchestrator.go
│   │   │   │   └── agent.go
│   │   │   ├── repository/    # 数据访问层
│   │   │   │   ├── user.go
│   │   │   │   ├── conversation.go
│   │   │   │   └── message.go
│   │   │   ├── model/         # 数据模型
│   │   │   │   ├── user.go
│   │   │   │   ├── conversation.go
│   │   │   │   ├── message.go
│   │   │   │   └── agent.go
│   │   │   └── middleware/    # 中间件
│   │   │       ├── auth.go
│   │   │       └── cors.go
│   │   ├── pkg/               # 公共可复用包
│   │   │   └── ws/            # WebSocket工具
│   │   ├── config/
│   │   │   └── config.yaml
│   │   ├── migrations/        # 数据库迁移脚本
│   │   ├── go.mod
│   │   └── go.sum
│   └── daemon/                # 本地守护进程
│       ├── main.go            # 守护进程入口
│       ├── scanner/           # Agent发现/扫描
│       │   └── scanner.go
│       ├── adapter/           # Agent适配器
│       │   ├── adapter.go     # 统一接口定义
│       │   ├── claude.go      # Claude Code适配器
│       │   └── codex.go       # Codex适配器
│       ├── process/           # 进程管理
│       │   └── manager.go
│       └── client/            # 与后端通信
│           └── client.go
└── scripts/               # 脚本工具
    ├── dev.sh             # 开发环境启动
    └── build.sh           # 构建脚本
```

## 各模块职责

### src/frontend/
- 纯前端 React SPA
- 通过 REST API 和 WebSocket 与后端通信
- 不包含任何后端逻辑

### src/backend/
- Go HTTP/WebSocket 服务
- 所有业务逻辑在 `internal/` 下
- `cmd/server/main.go` 只做初始化和启动
- `pkg/` 放可被外部引用的公共包

### src/daemon/
- 运行在用户本地电脑上的守护进程
- 主动 WebSocket 连接 Go 后端
- 扫描本地 Agent CLI 并管理其进程
- 适配器层将不同 Agent 输出统一为结构化数据

### doc/
- 项目文档
- `harness/` 放开发规范

### scripts/
- 开发辅助脚本

## 文件命名约定

| 位置 | 规则 | 示例 |
|------|------|------|
| src/frontend/src/views/ | PascalCase.tsx (XxxView) | `ChatView.tsx` |
| src/frontend/src/components/ | PascalCase.tsx | `ChatWindow.tsx` |
| src/frontend/src/hooks/ | camelCase.ts (use前缀) | `useWebSocket.ts` |
| src/frontend/src/api/ | camelCase.ts | `conversation.ts` |
| src/frontend/src/types/ | camelCase.ts | `message.ts` |
| src/backend/internal/*/ | 小写.go | `conversation.go` |
| src/backend/migrations/ | 数字序号_描述.sql | `001_create_users.sql` |
| src/daemon/*/ | 小写.go | `adapter.go` |

## 新增文件时

- 先确认它应该放在哪个模块（src/frontend / src/backend / src/daemon）
- 再确认它属于哪一层（组件 / 服务 / 数据 / 工具）
- 如果不确定放哪，选择最接近调用方的位置
