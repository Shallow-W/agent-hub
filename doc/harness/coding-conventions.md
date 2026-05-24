# 代码规范

## 通用规则

- **注释语言**：中文（说明"为什么"，而非"做了什么"）
- **命名语言**：英文（变量、函数、类、文件名）
- **缩进**：前端2空格，后端Tab（Go标准）
- **编码**：UTF-8
- **换行**：LF（非CRLF），通过 `.editorconfig` 和 `.gitattributes` 保证

---

## 前端规范（React + TypeScript）

### 文件命名

| 类型 | 命名格式 | 示例 |
|------|----------|------|
| 组件文件 | PascalCase | `ChatWindow.tsx` |
| 工具函数 | camelCase | `formatMessage.ts` |
| 类型定义 | PascalCase | `Message.ts` |
| 样式文件 | 与组件同名 | `ChatWindow.module.css` |
| 测试文件 | 组件名.test | `ChatWindow.test.tsx` |

### 组件规范

```tsx
// 优先使用函数式组件 + Hooks
const ChatWindow: React.FC<ChatWindowProps> = ({ conversationId }) => {
  // 1. Hooks（useState, useEffect, 自定义Hooks）
  // 2. 事件处理函数
  // 3. 渲染逻辑

  return (
    <div className={styles.container}>
      {/* JSX */}
    </div>
  );
};
```

- 组件用 `React.FC<Props>` 类型
- Props 接口定义在组件文件内，命名 `{ComponentName}Props`
- 单个组件文件不超过 300 行，超过则拆分子组件
- 提取自定义 Hook 复用有状态逻辑

### TypeScript 规范

- 严格模式开启（`strict: true`）
- 禁止使用 `any`，用 `unknown` 替代或定义具体类型
- 接口（interface）优先，类型别名（type）用于联合类型/工具类型
- 枚举使用 `const enum` 或字符串字面量联合类型

### 状态管理

- 组件内状态：`useState` / `useReducer`
- 跨组件共享：通过 Context 或轻量状态库
- 服务端状态（对话、消息）：通过 API 层获取和缓存

### 样式方案

- 使用 CSS Modules（`*.module.css`）
- 类名使用 camelCase
- 避免内联样式

---

## 后端规范（Go）

### 项目结构

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

### 命名规范

| 类型 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `handler`, `service` |
| 导出函数/方法 | PascalCase | `CreateConversation` |
| 未导出函数/方法 | camelCase | `parseArtifact` |
| 接口 | 以 `-er` 结尾或 PascalCase | `Adapter`, `MessageStreamer` |
| 常量 | PascalCase（导出）/ camelCase（未导出） | `MaxRetryCount` |
| 错误变量 | `Err` 前缀 | `ErrAgentNotFound` |

### 错误处理

```go
// 使用 fmt.Errorf 包装上下文
if err != nil {
    return fmt.Errorf("create conversation: %w", err)
}

// 自定义错误类型用于业务判断
var ErrAgentNotFound = errors.New("agent not found")

// 在handler层统一处理错误响应
func (h *Handler) handleError(c echo.Context, err error) {
    // 统一错误响应格式
}
```

- 禁止忽略错误（`_ = doSomething()`），除非明确无需处理
- 错误消息包含上下文（哪个操作失败）
- 使用 `%w` 包装原始错误以保留堆栈

### 接口定义

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

### 日志规范

```go
// 结构化日志
logger.Info("conversation created",
    "conversationId", conv.ID,
    "userId", userID,
)
```

- 使用结构化日志（slog 或 zerolog）
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
