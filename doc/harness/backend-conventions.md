# 后端编码规范（Go）

## 通用规则

- **注释语言**：中文（说明"为什么"，而非"做了什么"）
- **命名语言**：英文（变量、函数、包名、文件名）
- **缩进**：Tab（Go标准）
- **编码**：UTF-8
- **换行**：LF（非CRLF），通过 `.editorconfig` 和 `.gitattributes` 保证

---

## 技术选型

| 层级 | 库 | 说明 |
|------|----|------|
| HTTP 框架 | `github.com/gin-gonic/gin` | 社区生态最丰富，性能优异，中间件齐全 |
| WebSocket | `nhooyr.io/websocket` | 原生支持 `context.Context`，API 简洁 |
| 数据库驱动 | `github.com/jackc/pgx/v5` + `github.com/jmoiron/sqlx` | pgx 为 PostgreSQL 最佳驱动，sqlx 提供轻量映射 |
| 数据库迁移 | `github.com/golang-migrate/migrate/v4` | 支持 CLI + Go API，迁移文件放 `src/backend/migrations/` |
| 配置管理 | `github.com/knadh/koanf/v2` | 支持 yaml + 环境变量覆盖，比 viper 轻量 |
| JWT | `github.com/golang-jwt/jwt/v5` | 用户鉴权 |
| 参数校验 | `github.com/go-playground/validator/v10` | struct tag 声明式校验 |
| 测试 | `github.com/stretchr/testify` | 断言 + mock |
| 日志 | `log/slog` | Go 1.21+ 标准库 |
| 数据库 | PostgreSQL | 关系型数据（用户、对话、消息），支持 JSONB 存储半结构化数据 |
| 缓存/消息 | `github.com/redis/go-redis/v9` | WebSocket 连接映射、消息 pub/sub（多实例广播）、在线状态、接口限流 |

---

## 项目结构

遵循 [Go Standard Project Layout](https://github.com/golang-standards/project-layout)：

```
backend/
├── cmd/           # 程序入口
│   └── server/
│       └── main.go
├── internal/      # 私有代码
│   ├── handler/   # HTTP/WebSocket 处理器
│   ├── service/   # 业务逻辑
│   ├── repository/# 数据访问层
│   ├── model/     # 数据模型
│   └── middleware/ # 中间件
├── pkg/           # 可复用公共包
├── config/        # 配置文件
└── go.mod
```

## 命名规范

| 类型 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `handler`, `service` |
| 导出函数/方法 | PascalCase | `CreateConversation` |
| 未导出函数/方法 | camelCase | `parseArtifact` |
| 接口 | 以 `-er` 结尾或 PascalCase | `Adapter`, `MessageStreamer` |
| 常量 | PascalCase（导出）/ camelCase（未导出） | `MaxRetryCount` |
| 错误变量 | `Err` 前缀 | `ErrAgentNotFound` |

## 错误处理

```go
// 使用 fmt.Errorf 包装上下文
if err != nil {
    return fmt.Errorf("create conversation: %w", err)
}

// 自定义错误类型用于业务判断
var ErrAgentNotFound = errors.New("agent not found")

// 在handler层统一处理错误响应
func (h *Handler) handleError(c *gin.Context, err error) {
    // 统一错误响应格式
}
```

- 禁止忽略错误（`_ = doSomething()`），除非明确无需处理
- 错误消息包含上下文（哪个操作失败）
- 使用 `%w` 包装原始错误以保留堆栈

## 接口定义

```go
// 在使用方定义接口，不是在实现方
// handler 包定义它需要的服务接口
type ConversationService interface {
    Create(ctx context.Context, req CreateConversationReq) (*Conversation, error)
    GetByID(ctx context.Context, id string) (*Conversation, error)
}
```

- 接口保持小（1-3个方法）
- 在消费方定义接口，不是在实现方

## 并发与 Context

- 所有跨函数调用传递 `context.Context` 作为第一个参数
- Handler 层从请求创建 context（`c.Request.Context()`），Service/Repository 层接收但不创建
- 长生命周期任务（如 WebSocket 连接）使用独立 context，通过 `context.WithCancel` 控制

```go
// WebSocket 连接使用独立 context
ctx, cancel := context.WithCancel(context.Background())
defer cancel() // 连接关闭时取消所有子操作
```

- 禁止在全局变量中存储请求级状态
- goroutine 必须有明确的退出机制（context cancel 或 done channel）

## 数据库规范

### Repository 层

```go
type MessageRepository interface {
    Create(ctx context.Context, msg *Message) error
    ListByConversation(ctx context.Context, convID string, limit, offset int) ([]*Message, error)
}
```

- Repository 只做数据读写，不包含业务逻辑
- 查询方法接受 `ctx` 和筛选参数，返回领域模型
- 批量操作使用事务，事务边界在 Service 层控制

### 迁移脚本

- 文件命名：`数字序号_描述.sql`（如 `001_create_users.sql`）
- 序号连续递增，禁止复用已用序号
- 迁移必须可回滚（提供 `DOWN` 部分）

## 依赖注入

- Handler 创建时注入 Service 接口，Service 创建时注入 Repository 接口
- 在 `cmd/server/main.go` 中统一组装依赖链

```go
// main.go 中组装
msgRepo := repository.NewMessageRepo(db)
msgSvc := service.NewMessageService(msgRepo)
msgHandler := handler.NewMessageHandler(msgSvc)
```

- 禁止使用 `init()` 函数或包级全局变量管理依赖

## 日志规范

```go
// 结构化日志
logger.Info("conversation created",
    "conversationId", conv.ID,
    "userId", userID,
)
```

- 使用结构化日志（`log/slog`，Go 1.21+ 标准库）
- 日志级别：Debug / Info / Warn / Error
- Error 级别日志必须附带错误详情

---

## API 响应格式

### REST 统一响应

```json
{
  "code": 0,
  "message": "success",
  "data": { }
}
```

```json
{
  "code": 40001,
  "message": "参数错误: conversationId 不能为空",
  "data": null
}
```

### WebSocket 消息格式

```json
{
  "type": "message.streaming",
  "data": {
    "conversationId": "xxx",
    "messageId": "xxx",
    "content": "部分内容...",
    "done": false
  }
}
```
