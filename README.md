# AgentHub

AI Agent 聊天平台 —— 多 Agent 接入的 IM 系统。

## 技术栈

- **后端**: Go 1.22+ / Gin / PostgreSQL 15 / Redis 7 / WebSocket
- **前端**: React 18 / TypeScript / Vite / Zustand / Ant Design 5

## 快速开始

### 环境要求

- Go 1.22+
- Node.js 20+
- PostgreSQL 15+
- Redis 7+ (可选，无 Redis 时降级运行)

### 1. 启动依赖服务

```bash
docker compose up -d
```

### 2. 后端

```bash
cd src/backend
cp config/config.example.yaml config/config.yaml
# 编辑 config.yaml 填入实际数据库密码和 JWT 密钥
go run ./cmd/server/
```

### 3. 前端

```bash
cd src/frontend
npm install
npm run dev
```

访问 http://localhost:5173

### 4. Electron 桌面端

开发模式复用现有后端，并在 Electron 窗口里打开同一套 React UI：

```bash
bash scripts/desktop-dev.sh
```

生产目录包会构建 Go 后端、Vite 前端和 Electron 壳：

```bash
bash scripts/desktop-build.sh
```

桌面端通过本地 HTTP origin 保持与 Web 端一致。生产模式可由 Electron
启动打包内的 Go 后端，并通过 `AGENTHUB_CONFIG` 与
`AGENTHUB_FRONTEND_DIST` 指定配置和静态资源路径。自动拉起后端时仍沿用
现有 PostgreSQL/Redis 依赖，启动桌面包前请确保 PostgreSQL 可访问。

### Docker 部署

```bash
docker build -t agenthub .
docker run -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config/config.yaml \
  agenthub
```

## 项目结构

```
src/
├── backend/           # Go 后端
│   ├── cmd/server/    # 入口
│   ├── internal/      # 业务代码
│   │   ├── handler/   # HTTP/WebSocket 处理器
│   │   ├── service/   # 业务逻辑
│   │   ├── repository/# 数据访问
│   │   ├── middleware/ # 中间件（鉴权、限流、CORS）
│   │   └── model/     # 数据模型
│   ├── pkg/           # 公共包（WebSocket Hub、Redis）
│   ├── migrations/    # SQL 迁移
│   └── config/        # 配置
└── frontend/          # React 前端
    └── src/
        ├── api/       # HTTP/WS 客户端
        ├── components/# UI 组件
        ├── hooks/     # 自定义 hooks
        ├── store/     # Zustand 状态
        └── views/     # 页面视图
```

## API 概览

| 路径 | 说明 |
|------|------|
| `POST /api/auth/register` | 用户注册 |
| `POST /api/auth/login` | 用户登录 |
| `GET /ws?token=` | WebSocket 连接 |
| `GET /api/conversations` | 对话列表 |
| `POST /api/conversations/private` | 创建/获取私聊 |
| `GET/POST /api/conversations/:id/messages` | 消息历史/发送 |
| `GET /api/friends` | 好友列表 |
| `POST /api/groups` | 创建群聊 |
| `POST /api/upload` | 文件上传 |
| `GET /health` | 健康检查 |

## 配置

见 `src/backend/config/config.example.yaml`。
