# API 参考

Base URL: `http://localhost:8080`

认证方式：所有 `/api/*` 端点需在请求头携带 JWT Bearer Token：
```
Authorization: Bearer <token>
```

---

## 鉴权

### POST /api/auth/register

注册新用户。

**请求体**
```json
{
  "username": "string (3-32字符)",
  "password": "string (8-64字符)"
}
```

**成功响应** `200 OK`
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": "uuid",
    "username": "alice",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**错误响应**
- `400 Bad Request` — 参数校验失败
- `409 Conflict` — 用户名已存在

---

### POST /api/auth/login

用户登录。

**请求体**
```json
{
  "username": "string",
  "password": "string"
}
```

**成功响应** `200 OK`
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": "uuid",
    "username": "alice",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**错误响应**
- `401 Unauthorized` — 用户名或密码错误

---

## 对话

### GET /api/conversations

获取当前用户的对话列表（按最后消息时间倒序）。

**成功响应** `200 OK`
```json
[
  {
    "id": "uuid",
    "type": "direct | group",
    "title": "项目讨论",
    "pinned": false,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T12:00:00Z",
    "last_message": {
      "id": "uuid",
      "content": "最新消息内容",
      "role": "user | assistant | system",
      "created_at": "2024-01-01T12:00:00Z"
    }
  }
]
```

---

### POST /api/conversations

创建新对话。

**请求体**
```json
{
  "type": "direct | group",
  "title": "对话标题"
}
```

**成功响应** `201 Created`
```json
{
  "id": "uuid",
  "type": "direct",
  "title": "对话标题",
  "pinned": false,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

**错误响应**
- `400 Bad Request` — 参数校验失败

---

### DELETE /api/conversations/:id

删除对话（软删除）。

**成功响应** `204 No Content`

**错误响应**
- `404 Not Found` — 对话不存在或无权限

---

### PUT /api/conversations/:id/pin

设置对话置顶状态。

**请求体**
```json
{
  "pinned": true
}
```

**成功响应** `200 OK`
```json
{
  "id": "uuid",
  "pinned": true
}
```

**错误响应**
- `404 Not Found` — 对话不存在或无权限

---

## 消息

### POST /api/conversations/:id/messages

发送消息。

**请求体**
```json
{
  "content": "消息内容",
  "role": "user"
}
```

**成功响应** `201 Created`
```json
{
  "id": "uuid",
  "conversation_id": "uuid",
  "content": "消息内容",
  "role": "user",
  "pinned": false,
  "created_at": "2024-01-01T00:00:00Z"
}
```

**错误响应**
- `400 Bad Request` — 参数校验失败
- `404 Not Found` — 对话不存在或无权限

---

### GET /api/conversations/:id/messages

获取消息历史（游标分页）。

**查询参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| `before` | string | 游标：返回 ID 小于此值的消息 |
| `limit` | int | 每页条数，默认 50，最大 100 |

**成功响应** `200 OK`
```json
[
  {
    "id": "uuid",
    "conversation_id": "uuid",
    "content": "消息内容",
    "role": "user | assistant | system",
    "pinned": false,
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

**错误响应**
- `404 Not Found` — 对话不存在或无权限

---

### POST /api/conversations/:id/messages/:messageId/pin

将消息 Pin 到当前会话的共享上下文黑板，后续 Agent 调用会收到该内容。

**成功响应** `200 OK`
```json
{
  "id": "uuid",
  "conversation_id": "uuid",
  "message_id": "uuid",
  "created_by": "uuid",
  "created_at": "2024-01-01T00:00:00Z"
}
```

**错误响应**
- `403 Forbidden` — 无权操作此对话
- `404 Not Found` — 对话或消息不存在

---

### DELETE /api/conversations/:id/messages/:messageId/pin

取消消息 Pin。

**成功响应** `200 OK`
```json
null
```

**错误响应**
- `403 Forbidden` — 无权操作此对话
- `404 Not Found` — 对话或消息不存在

---

### GET /api/conversations/:id/pinned-context

获取当前会话共享上下文黑板中的用户 Pin 上下文。

**成功响应** `200 OK`
```json
[
  {
    "id": "uuid",
    "conversation_id": "uuid",
    "message_id": "uuid",
    "role": "user",
    "content": "关键上下文",
    "sender_id": "uuid",
    "username": "alice",
    "message_created_at": "2024-01-01T00:00:00Z",
    "pinned_by": "uuid",
    "pinned_by_name": "alice",
    "pinned_at": "2024-01-01T00:00:00Z"
  }
]
```

**错误响应**
- `403 Forbidden` — 无权操作此对话
- `404 Not Found` — 对话不存在

---

### GET /api/conversations/:id/blackboard

获取当前会话上下文黑板中的用户手写上下文。适用于群聊、普通单聊和 Agent 单聊。

**成功响应** `200 OK`
```json
{
  "conversation_id": "uuid",
  "manual_context": "用户手写的长期上下文",
  "updated_by": "uuid",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

**错误响应**
- `403 Forbidden` — 无权操作此对话
- `404 Not Found` — 对话不存在

---

### PUT /api/conversations/:id/blackboard

保存当前会话上下文黑板中的用户手写上下文。该内容会在后续 Agent 调用中随 `{会话上下文黑板}` 注入。

**请求体**
```json
{
  "manual_context": "用户手写的长期上下文"
}
```

**成功响应** `200 OK`
```json
{
  "conversation_id": "uuid",
  "manual_context": "用户手写的长期上下文",
  "updated_by": "uuid",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

**错误响应**
- `403 Forbidden` — 无权操作此对话
- `404 Not Found` — 对话不存在
- `413 Request Entity Too Large` — 手写上下文超过长度限制

---

## WebSocket

### WS /ws?token=jwt

建立 WebSocket 连接，实时接收消息推送。

**连接方式**：在 URL 中传递 JWT Token 进行认证。

**服务端推送消息类型**

`message.new` — 新消息
```json
{
  "type": "message.new",
  "data": {
    "id": "uuid",
    "conversation_id": "uuid",
    "content": "消息内容",
    "role": "assistant",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

`message.stream` — 流式消息片段（Agent 逐 token 输出）
```json
{
  "type": "message.stream",
  "data": {
    "message_id": "uuid",
    "conversation_id": "uuid",
    "chunk": "部分内容",
    "done": false
  }
}
```

`conversation.updated` — 对话信息变更
```json
{
  "type": "conversation.updated",
  "data": {
    "id": "uuid",
    "title": "新标题",
    "pinned": true
  }
}
```

**客户端发送消息类型**

`ping` — 心跳保活
```json
{ "type": "ping" }
```

---

## Agent 管理

### GET /api/agents

获取当前用户可用的 Agent 列表。

**成功响应** `200 OK`
```json
[
  {
    "id": "uuid",
    "name": "代码助手",
    "type": "custom",
    "cli_tool": "codex",
    "system_prompt": "你是一个资深工程师",
    "tools_config": "{\"toolset\":\"tasks\",\"allowed_tools\":[\"list_tasks\"]}",
    "custom_skills": "[{\"name\":\"代码审查\",\"description\":\"检查 bug 和测试缺口\",\"trigger\":\"review, bug\",\"detail\":\"按清单检查权限、边界和测试。\"}]",
    "tags": "[\"coding\"]",
    "status": "online"
  }
]
```

### POST /api/daemon/agent-candidates/:id/add

把当前电脑扫描到的候选底座添加为用户自建 Agent。`cli_tool` 必须与候选底座匹配，避免候选列表刷新后误建到错误 CLI。

**请求体**
```json
{
  "name": "代码助手",
  "cli_tool": "codex",
  "system_prompt": "你是一个资深工程师",
  "tools_config": "{\"toolset\":\"tasks\",\"allowed_tools\":[\"list_group_agents\",\"get_messages\",\"list_tasks\"]}",
  "custom_skills": "[{\"name\":\"代码审查\",\"description\":\"检查 bug 和测试缺口\",\"trigger\":\"review, bug\",\"detail\":\"按清单检查权限、边界和测试。\"}]"
}
```

**工具集配置**

`tools_config` 是字符串形式的 JSON，当前支持：

```json
{
  "toolset": "none | basic | tasks | orchestrator | agent_builder | 空字符串",
  "allowed_tools": ["list_tasks"]
}
```

- `allowed_tools` 保存前会过滤未知工具名。
- `{"toolset":"none","allowed_tools":[]}` 表示该 Agent 不授予平台 MCP 工具。
- 省略或传空 `tools_config` 会保存为无工具配置，不会回退默认工具。
- 无法解析的旧文本配置仅保留展示，不授予 MCP 工具。

**成功响应** `201 Created`
```json
{
  "id": "uuid",
  "name": "代码助手",
  "type": "custom",
  "cli_tool": "codex",
  "system_prompt": "你是一个资深工程师",
  "tools_config": "{\"toolset\":\"tasks\",\"allowed_tools\":[\"list_group_agents\",\"get_messages\",\"list_tasks\"]}",
  "custom_skills": "[{\"name\":\"代码审查\",\"description\":\"检查 bug 和测试缺口\",\"trigger\":\"review, bug\",\"detail\":\"按清单检查权限、边界和测试。\"}]"
}
```

**错误响应**
- `400 Bad Request` — 参数校验失败
- `404 Not Found` — 候选底座不存在、无权限，或 `cli_tool` 与候选底座不匹配

### PUT /api/agents/:id/custom-skills

更新 Agent 的平台 Skills。该字段用于用户配置的 Agent 能力索引和渐进式加载内容，不会被 daemon 底座扫描覆盖。
仅允许更新当前用户拥有的自建 Agent；保存前会校验为 JSON 数组并只保留 `name`、`description`、`trigger`、`detail` 字段，过滤 `source_path` 等本机扫描字段。

**请求体**
```json
{
  "custom_skills": "[{\"name\":\"代码审查\",\"description\":\"检查 bug 和测试缺口\",\"trigger\":\"review, bug\",\"detail\":\"按清单检查权限、边界和测试。\"}]"
}
```

**成功响应** `200 OK`
```json
{
  "id": "uuid",
  "custom_skills": "[{\"name\":\"代码审查\",\"description\":\"检查 bug 和测试缺口\",\"trigger\":\"review, bug\",\"detail\":\"按清单检查权限、边界和测试。\"}]"
}
```

---

## 错误码

所有错误响应遵循统一格式：
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "人类可读的错误描述"
  }
}
```

| HTTP 状态码 | 错误码 | 说明 |
|-------------|--------|------|
| 400 | VALIDATION_ERROR | 请求参数校验失败 |
| 401 | UNAUTHORIZED | 未认证或 Token 无效 |
| 403 | FORBIDDEN | 无权限访问该资源 |
| 404 | NOT_FOUND | 资源不存在 |
| 409 | CONFLICT | 资源冲突（如用户名重复） |
| 500 | INTERNAL_ERROR | 服务端内部错误 |
