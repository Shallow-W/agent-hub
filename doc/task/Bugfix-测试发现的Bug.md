# Bug 修复清单

> 2026-05-27 API 接口测试 + 前端代码审查发现的 bug

## 严重程度说明

`P0` 数据安全/功能完全失效 | `P1` 核心功能异常 | `P2` 体验问题 | `P3` 代码质量

---

## 后端 Bug

### B01 [P0] 消息撤回无权限校验——任何人可撤回别人的消息

- **文件**: `internal/handler/message.go` — Recall 方法
- **现象**: user1 可以撤回 user2 发的消息，返回 200 成功
- **测试**:
  ```
  user2 发消息 → user1 用 DELETE /api/conversations/:id/messages/:msgId → 200 OK（消息被撤回）
  ```
- **预期**: 只有消息发送者本人可以撤回，其他人应返回 403
- **修复**: Recall handler 增加发送者校验 `message.sender_id != currentUserID → 403`

### B02 [P0] 已读标记不生效——markAsRead 后仍返回已读消息

- **文件**: `internal/handler/message.go` — MarkAsRead / Unread
- **现象**: 调用 `PUT /conversations/:id/read` 返回 200 后，`GET /conversations/:id/messages/unread` 仍返回所有消息
- **测试**:
  ```
  发2条消息 → PUT /read (200) → GET /unread → 返回全部2条（应返回0条）
  ```
- **预期**: 标记已读后，unread 不应返回这些消息
- **修复**: 检查 MarkAsRead 是否正确写入了 read_status 表；检查 Unread 查询是否正确排除已读消息

### B03 [P1] 创建私聊给非好友返回 500 而非 400/403

- **文件**: `internal/service/conversation.go` — GetOrCreatePrivate
- **现象**: `POST /api/conversations/private {"friend_id": "<非好友ID>"}` → HTTP 500 `{"code":50016,"message":"创建私聊失败"}`
- **预期**: HTTP 400 或 403，提示"对方不是你的好友"

### B04 [P1] 创建私聊给自己返回 500

- **文件**: `internal/service/conversation.go` — GetOrCreatePrivate
- **现象**: `POST /api/conversations/private {"friend_id": "<自己ID>"}` → HTTP 500
- **预期**: HTTP 400，提示"不能和自己创建私聊"

### B05 [P1] reply_to 指向不存在的消息返回 500

- **文件**: `internal/service/message.go` — Send
- **现象**: `POST /conversations/:id/messages {"content":"...","reply_to":"<不存在的ID>"}` → HTTP 500 `{"code":50020}`
- **预期**: HTTP 400 或 404，提示"被回复的消息不存在"

### B06 [P2] 附件数据未正确保存

- **文件**: `internal/service/message.go` — Send (attachments 处理)
- **现象**: 发送带 attachments 的消息，返回的 attachment 字段全空（id、mime_type、file_path、created_at 均为零值）
- **测试**:
  ```json
  POST {"content":"带附件","attachments":[{"file_name":"test.txt","file_url":"/uploads/test.txt","file_size":17,"file_type":"text/plain"}]}
  → 返回 attachment: {"id":"","message_id":"...","file_name":"test.txt","mime_type":"","file_size":17,"file_path":"","created_at":"0001-01-01T00:00:00Z"}
  ```
- **预期**: attachment 应正确保存所有字段

### B07 [P2] 通过 conversations 端点创建的群聊无成员记录

- **文件**: `internal/service/conversation.go` — Create
- **现象**: `POST /api/conversations {"type":"group","title":"测试"}` 创建的群聊在 conversation_members 表中没有记录，导致后续重命名等需要成员校验的操作返回 403
- **预期**: 创建群聊时应同时写入创建者的 owner 成员记录
- **注**: 通过 `POST /api/groups` 创建的群聊没有此问题

### B08 [P3] 消息无长度限制

- **现象**: 10000 字符的消息直接被接受并存入数据库
- **建议**: 在 SendMessageRequest 的 Content 字段添加 `validate:max=5000` 或类似限制

### B09 [P3] 用户搜索返回自己

- **文件**: `internal/repository/user.go` 或 handler
- **现象**: `GET /api/users/search?q=test` 返回结果中包含当前登录用户自己
- **建议**: 排除当前用户（`WHERE id != currentUserId`）

---

## 前端 Bug

### B10 [P0] togglePin API 方法不匹配——Pin 功能完全失效

- **文件**: `src/api/conversation.ts:29` + `src/store/conversationStore.ts`
- **现象**: 前端发送 `PUT /api/conversations/:id/pin` + body `{pinned: boolean}`
- **后端**: 路由定义为 `POST /:id/pin`，handler 不读取 body（直接 toggle）
- **结果**: HTTP 404（PUT 路由不存在），Pin 功能完全不可用
- **修复**: 前端改为 `post<void>('/api/conversations/${id}/pin')`，不发 body

### B11 [P0] API 返回 data:null 导致前端崩溃

- **文件**: `src/api/client.ts:63`
- **现象**: 后端返回 `{"code":0,"data":null}` 时，`return json.data as T` 将 null 强制转换为 T
- **后果**: store 中 `sortConversations(null)` → `[...null]` → `TypeError: null is not iterable`
- **修复**: client.ts 对 null data 做防御处理，或各 store 添加 `?? []` 兜底

### B12 [P1] 回复功能数据未发送——UI 存在但无效

- **文件**: `src/store/messageStore.ts:108`, `src/components/chat/ChatInput.tsx:126`
- **现象**: ChatWindow 设置 replyTo 状态，ChatInput 显示回复栏 UI，但 sendMessage 不传 reply_to 字段
- **结果**: 用户点击回复、看到 UI、发送消息，但回复上下文丢失
- **修复**: sendMessage 增加 replyTo 参数，通过 API 传递 `reply_to`

### B13 [P1] 消息撤回功能前端完全缺失

- **文件**: `src/api/message.ts`（缺失函数）, `src/components/chat/MessageBubble.tsx`（缺失 UI）
- **现象**: 后端有完整的 recall 端点 `DELETE /:id/messages/:messageId`，前端无 API 函数、无 UI 按钮、无 WS 处理
- **修复**: 添加 `recallMessage(convId, msgId)` API 函数 + MessageBubble 撤回按钮（带时间限制）

### B14 [P1] JWT 过期后会话永久损坏

- **文件**: `src/api/client.ts:54`
- **现象**: JWT 过期后所有 API 返回 401，但 client.ts 无拦截器处理 401，不会自动跳转登录页
- **结果**: 用户看到空白/错误的 UI，无任何反馈，需手动刷新
- **修复**: 在 request 函数中检测 401，调用 `clearToken()` 并跳转 `/login`

### B15 [P1] typingUsers 显示 UUID 而非用户名

- **文件**: `src/components/chat/ChatWindow.tsx:92-94`
- **现象**: 输入指示器显示 `550e8400-e29b-41d4-a716-446655440000 正在输入...`（原始 UUID）
- **预期**: 显示用户名如 `testuser2 正在输入...`
- **修复**: 维护 userId→username 映射，或在 typing 事件中携带 username

### B16 [P2] retryOptimistic 丢弃附件

- **文件**: `src/store/messageStore.ts:206`
- **现象**: 重试失败消息时只传 content，不传 attachments，原始附件丢失
- **修复**: OptimisticMessage 增加 attachments 字段，重试时一并传递

### B17 [P2] deleteConversation 无错误处理

- **文件**: `src/store/conversationStore.ts:51-61`
- **现象**: 调用 API 无 try/catch，失败时乐观更新已执行（对话从列表消失），但服务端未删除
- **修复**: API 成功后再更新 state，失败时 toast 提示

### B18 [P2] fetchMessages 竞态条件

- **文件**: `src/store/messageStore.ts:58-81`
- **现象**: 快速切换对话时，多个 fetchMessages 并发执行，loading 是单布尔值，后完成的响应可能覆盖正确数据
- **修复**: 使用按对话的 loading 状态或 AbortController

### B19 [P3] ProtectedRoute 不响应式

- **文件**: `src/router/index.tsx:14`
- **现象**: 从 localStorage 读取 token 而非订阅 Zustand store，token 清除后路由守卫不重新评估
- **修复**: 使用 `useAuthStore(s => s.isAuthenticated)` 替代 `localStorage.getItem`

### B20 [P3] 重复 token 管理

- **文件**: `src/api/client.ts:3`, `src/store/authStore.ts:18`
- **现象**: 两个地方都读写 `agenthub_token`，职责不清
- **修复**: 统一到 client.ts 管理，store 不重复存 token

---

## 后端 Bug（第二轮深度测试）

### B21 [P1] 无效 UUID 路径参数返回 500

- **文件**: `internal/handler/conversation.go`, `internal/handler/group.go`
- **现象**: 所有使用 `/:id` 的接口，传入非 UUID 字符串（如 "not-a-uuid"）时返回 500
- **示例**: `DELETE /api/conversations/not-a-uuid → 500`, `GET /api/groups/not-a-uuid/members → 500`
- **预期**: HTTP 400 + "无效 ID 格式"

### B22 [P1] 消息撤回后 Redis 缓存未失效

- **文件**: `internal/service/message.go` RecallMessage, `internal/repository/redis_msg.go`
- **现象**: 撤回消息后，`GET /messages?limit=N`（走缓存）仍返回原始内容且 `deleted_at=null`；仅用 `before` 参数（绕缓存查 DB）才能看到正确状态
- **修复**: 撤回后清除对应 Redis 缓存 key

### B23 [P2] reply_to_message 字段始终为 null

- **文件**: `internal/repository/message.go` Create 方法
- **现象**: 发送带 `reply_to` 的消息，返回的 message 中 `reply_to` ID 有值但 `reply_to_message` 始终 null；历史查询中也缺失
- **根因**: Create 方法用 RETURNING 扫描，不调用 fillReplyTo；缓存路径的消息也不经过 fillReplyTo

### B24 [P2] 对话列表缺少 members_count 字段

- **文件**: `internal/repository/conversation.go` ListByUserID
- **现象**: 群聊对话不返回 `members_count`，前端无法在列表中显示群成员数
- **修复**: 从 conversation_members 表 COUNT 查询

### B25 [P2] 私聊对话缺少 peer_id 字段

- **文件**: `internal/repository/conversation.go` ListByUserID
- **现象**: 私聊返回了 `peer_name` 但缺少 `peer_id`，前端无法跳转用户详情
- **修复**: 查询时 JOIN 获取对方用户 ID

### B26 [P2] 私聊 user_id 始终是创建者 ID

- **文件**: `internal/repository/conversation.go` ListByUserID
- **现象**: 用户 B 查询对话列表，私聊的 `user_id` 是创建者（A）的 ID，非当前用户 ID
- **预期**: 应明确字段语义（如改为 `creator_id`）或返回当前用户视角的角色信息

### B27 [P2] 消息内容和群名存储原始 HTML/JS 未转义

- **文件**: `internal/handler/message.go` Send, `internal/handler/group.go` CreateGroup
- **现象**: `<script>alert(1)</script>` 原样存入数据库并返回
- **注**: React 默认转义可防护，但 API 层应作为最后防线
- **修复**: 对用户输入做 HTML 实体转义

### B28 [P3] 好友请求发给不存在用户返回 500

- **文件**: `internal/handler/friend.go` SendRequest
- **现象**: `POST /api/friends/request {"friend_id":"<不存在的UUID>"}` → HTTP 500
- **预期**: HTTP 404 + "用户不存在"

### B29 [P3] 消息历史 limit 负数未处理

- **文件**: `internal/handler/message.go` History
- **现象**: `GET /messages?limit=-1` 返回全部消息，缓存路径无限制
- **预期**: limit <= 0 时使用默认值 50

### B30 [P3] 不存在的群聊 UUID 返回 403 而非 404

- **文件**: `internal/service/group.go` GetGroupInfo
- **现象**: `GET /api/groups/<不存在UUID>` → 403 "用户不是群成员"
- **预期**: 先检查群是否存在，不存在返回 404；非成员才返回 403

### B31 [P3] leave_room 后用户仍通过成员推送收到消息

- **文件**: `internal/pkg/ws/hub.go` handlePersistedMsg
- **现象**: 用户 B 发 leave_room 后，A 发消息，B 仍收到推送（PushToConversation 按成员列表推，绕过房间级别）
- **注**: 可能是设计意图（确保消息不丢失），但需明确 leave_room 语义

---

## 前端 Bug（第二轮确认）

### B32 [P1] 群成员数量硬编码为 9

- **文件**: `src/components/chat/ChatWindow.tsx:118`
- **现象**: `{isGroup && <span>9</span>}` — 成员数永远显示 9
- **修复**: 从 GroupMemberPanel 数据或 GET /api/groups/:id 获取真实成员数

---

## 前端缺失功能清单

### P0 — 核心功能缺失

| ID | 功能 | 后端状态 | 说明 |
|----|------|----------|------|
| MISS-001 | 群聊重命名 UI | 已有 API (PUT /:id) | ChatWindow 设置面板中应提供修改群名入口 |
| MISS-002 | 个人资料编辑/展示 | 需新增 API | 无法修改密码、头像、昵称；设置按钮为空壳 |
| MISS-003 | 设置页面实现 | 大部分前端本地 | 暗色模式不持久、无通知/语言/安全设置 |

### P1 — 重要功能缺失

| ID | 功能 | 后端状态 | 说明 |
|----|------|----------|------|
| MISS-004 | 群成员角色管理 UI | 部分已有 | 无法提升/降级 admin |
| MISS-005 | 好友删除 | 需新增 API | 无法解除好友关系 |
| MISS-006 | /api/users/search 对接 | 已有 API | 后端已实现但前端完全未对接 |
| MISS-007 | 归档对话列表/查看 | 需新增 API | 有归档入口但无法查看归档内容，归档=永久丢失 |
| MISS-008 | GetGroupInfo 对接 | 已有 API | 群详情页未实现 |
| MISS-009 | 消息转发 | 需新增 API | IM 标准功能 |
| MISS-010 | @提及功能 | 需新增 API | 群聊核心功能 |
| MISS-011 | 消息已读回执展示 | 部分已有 | 发送者无法知道消息是否被对方看到 |

### P2 — 次要功能缺失

| ID | 功能 | 后端状态 | 说明 |
|----|------|----------|------|
| MISS-012 | 群公告/群描述 | 需新增字段 | 锦上添花 |
| MISS-013 | 群头像设置 | 需新增字段 | 视觉个性化 |
| MISS-014 | 转让群主 | 需新增 API | 错误提示已引用但未实现 |
| MISS-015 | 好友备注名 | 需新增字段 | 好友多时难以区分 |
| MISS-016 | 好友分组/黑名单 | 部分准备 | model 有 blocked 状态但无 API |
| MISS-017 | 好友申请撤回 | 需新增 API | 低频功能 |
| MISS-018 | 对话分组/标签 | 需新增模型 | 大量对话时才需要 |
| MISS-019 | 全局消息搜索 | 需新增 API | 跨对话搜索 |
| MISS-020 | 声音/震动通知设置 | 纯前端 | 提示音不可配置 |
| MISS-021 | 浏览器推送通知 | 纯前端 | Web Notification API |
| MISS-022 | 对话列表搜索/过滤 | 已有参数 | ConversationList 无搜索框 |
| MISS-023 | 群组搜索 | 纯前端 | 群聊列表无过滤 |
| MISS-024 | 语言设置(i18n) | 纯前端 | 文本硬编码中文 |

---

## 后端 API vs 前端对接状态

| 后端 API | 前端状态 | 备注 |
|----------|----------|------|
| POST /api/auth/register | 已对接 | OK |
| POST /api/auth/login | 已对接 | OK |
| POST /api/conversations | 已对接 | OK |
| POST /api/conversations/private | 已对接 | OK |
| GET /api/conversations | 已对接 | OK |
| PUT /api/conversations/:id | **未对接** | MISS-001 |
| DELETE /api/conversations/:id | 已对接 | OK |
| POST /:id/archive | 部分对接 | MISS-007 |
| POST /:id/pin | **Bug** | PUT vs POST, B10 |
| POST /:id/messages | 已对接 | OK |
| GET /:id/messages | 已对接 | OK |
| PUT /:id/read | 已对接 | OK |
| GET /:id/messages/unread | 已对接 | OK |
| DELETE /:id/messages/:messageId | 已对接 | OK |
| GET /api/friends | 已对接 | OK |
| GET /api/friends/pending | 已对接 | OK |
| POST /api/friends/request | 已对接 | OK |
| POST /api/friends/:id/accept | 已对接 | OK |
| POST /api/friends/:id/reject | 已对接 | OK |
| GET /api/friends/search | 已对接 | OK |
| GET /api/users/search | **未对接** | MISS-006 |
| POST /api/groups | 已对接 | OK |
| GET /api/groups/:id | **未对接** | MISS-008 |
| POST /api/groups/:id/members | 已对接 | 仅添加 |
| DELETE /api/groups/:id/members/:userId | 已对接 | OK |
| GET /api/groups/:id/members | 已对接 | OK |
| POST /api/groups/:id/leave | 已对接 | OK |
| POST /api/upload | 已对接 | OK |
| GET /ws | 已对接 | OK |

---

## 安全问题（第三轮安全测试）

### SEC-01 [HIGH] 用户名缺少白名单校验

- **文件**: `internal/service/auth.go`, `internal/handler/auth.go`
- **现象**: 注册时用户名可以是空格、`<script>alert(1)</script>`、SQL 关键字等，全部 201 成功
- **风险**: 存储型 XSS（用户名渲染时）、数据污染
- **修复**: 添加正则白名单 `^[a-zA-Z0-9_一-龥]{2,20}$`

### SEC-02 [MEDIUM] 消息 content 纯空格被接受

- **文件**: `internal/handler/message.go`
- **现象**: `POST {"content":"   "}` → 201 成功
- **修复**: handler 或 service 层添加 `strings.TrimSpace` 后长度校验

### SEC-03 [MEDIUM] 群名纯空格通过 binding 校验

- **文件**: `internal/handler/group.go`
- **现象**: `POST {"name":"   "}` → 201 成功，群名为纯空格
- **修复**: binding min=1 只检查 rune 数，需 TrimSpace 后再校验

### SEC-04 [LOW] 上传文件名 XSS 字符未净化

- **文件**: `internal/service/upload.go`
- **现象**: `filename=test<svg onload=alert(1)>.png` 原样存入并返回
- **风险**: 前端渲染 file_name 未转义时可触发 XSS
- **修复**: 上传时净化文件名中的 `<>"'&` 字符

### SEC-05 [LOW] limit 参数无上界/无正数校验

- **文件**: 多个 handler
- **现象**: `limit=999999999` 返回全部数据，`limit=-1` 也返回数据
- **修复**: 添加上限校验如 `Math.min(limit, 200)`

### SEC-06 [LOW] 用户搜索接口缺少独立限流

- **文件**: `cmd/server/main.go`
- **现象**: RateLimit(100, 200) 对搜索接口无细粒度限制，可枚举所有用户名
- **修复**: 搜索接口添加更严格的独立限流（如 10次/分钟）

---

## 前端 UI/CSS 问题（第三轮深度审查）

### UI-01 [P0] WebSocket 主动断开后仍自动重连

- **文件**: `src/api/websocket.ts:49-56, 80-83`
- **现象**: `disconnect()` 调用 `clearRetryTimer()` → `ws.close()` → onclose 异步触发 `scheduleReconnect()` 创建新定时器。clearRetryTimer 已执行过，无法清除新定时器
- **影响**: 登出后 WebSocket 仍在后台重连，收到不属于自己的消息，内存泄漏
- **修复**: 添加 `intentionalClose` 标志位，disconnect 时设为 true，onclose 中检查此标志

### UI-02 [P1] 已登录用户访问 /login 不会被重定向

- **文件**: `src/router/index.tsx:21-24`
- **现象**: /login 和 /register 路由没有反向保护，已登录用户可以直接访问登录页
- **修复**: 添加 GuestRoute 组件，已登录时重定向到 /

### UI-03 [P1] 展开输入框按钮无实际功能

- **文件**: `src/components/chat/ChatInput.tsx:194-199`
- **现象**: UpOutlined 按钮渲染了但没有绑定 onClick
- **修复**: 实现展开逻辑或移除按钮

### UI-04 [P1] 搜索结果高亮效果不可见

- **文件**: `src/components/chat/ChatWindow.tsx:194-198`
- **现象**: transition 设为 2s，但 50ms 后就清除背景色，高亮几乎不可见
- **修复**: setTimeout 延迟改为 2000ms 或更合理的值

### UI-05 [P2] 无对话级别 URL

- **文件**: `src/router/index.tsx`, `src/layout/AppLayout.tsx`
- **现象**: 切换对话只改变 Zustand state，不改变 URL。浏览器后退直接退出应用
- **影响**: 无法通过 URL 分享对话；刷新后 activeConversationId 丢失
- **修复**: 引入 `/chat/:id` 路由

### UI-06 [P2] 页面刷新后恢复到空状态

- **文件**: `src/hooks/useConversation.ts:14-16`
- **现象**: 刷新后 Zustand state 重置，conversations 为空，activeConversationId 为 null
- **修复**: 将 activeConversationId 持久化到 URL 或 sessionStorage

### UI-07 [P2] 多处硬编码颜色不跟随主题

- **文件**: `ChatWindow.module.css:6` (#fff), `MessageBubble.module.css` 多处, `AppLayout.module.css:71`
- **现象**: 大量硬编码颜色值，暗色主题切换时需要大量覆盖
- **修复**: 统一使用 CSS 变量

### UI-08 [P2] AuthLayout 固定宽度窄屏溢出

- **文件**: `src/layout/AuthLayout.module.css:36`
- **现象**: .card 固定 width: 420px，无 max-width: 100%
- **修复**: 添加响应式保护

### UI-09 [P2] 多处内联 style 无法被暗色主题覆盖

- **文件**: AppLayout.tsx:187, ChatWindow.tsx:224, MessageList.tsx:83-92, FriendList.tsx:70-74 等
- **现象**: 多处 `style={{ ... }}` 内联样式，优先级高于 CSS
- **修复**: 改用 CSS Module class

### UI-10 [P2] WebSocket 重连无 jitter 可能导致惊群

- **文件**: `src/api/websocket.ts:90-95`
- **现象**: 纯指数退避无随机抖动，服务器重启后所有客户端同时重连
- **修复**: 添加随机 jitter

### UI-11 [P2] 断线期间发送队列不持久化

- **文件**: `src/api/websocket.ts:43-46`
- **现象**: 断线时消息缓存在内存 queue 中，用户关闭页面后消息丢失
- **修复**: 使用 localStorage 持久化队列

### UI-12 [P3] 无键盘 focus-visible 样式

- **文件**: ConversationItem.module.css, SettingsPanel.module.css
- **现象**: role="button" 元素无 :focus-visible 样式
- **修复**: 添加 focus-visible 轮廓

### UI-13 [P3] GroupMemberPanel 缺少 aria-label

- **文件**: `src/components/groups/GroupMemberPanel.tsx:144, 241`
- **现象**: Drawer 组件缺少 aria-label
- **修复**: 添加描述性 aria-label

### UI-14 [P3] "文件" 和 "停止任务" 按钮无功能

- **文件**: `src/components/chat/ChatWindow.tsx:126-128, 143`
- **现象**: FolderOpenOutlined 和 StopOutlined 按钮 onClick 为空
- **修复**: 实现功能或移除按钮

### UI-15 [P3] friendStore accept/reject 缺少 loading 状态

- **文件**: `src/store/friendStore.ts:72-96`
- **现象**: acceptRequest/rejectRequest 不设 loading，用户可能重复点击
- **修复**: 添加 loading 状态

### UI-16 [P3] hasMore 翻页边界判断不准

- **文件**: `src/store/messageStore.ts:70-78`
- **现象**: 返回恰好 PAGE_SIZE 条时 hasMore=true，但实际已无更多
- **修复**: 改为 `list.length > 0` 时加载更多

---

## 修复优先级建议

1. **立即修复** (P0): B01 撤回权限、B02 已读标记、B10 togglePin、B11 null 崩溃、UI-01 WebSocket 重连、CODE-01 goroutine 泄漏
2. **本轮修复** (P1): B03-05 错误码、B12 回复功能、B13 撤回 UI、B14 401处理、B15 typing 显示、B21 无效UUID、B22 Redis缓存、B32 成员数硬编码、SEC-01 用户名校验、UI-02~04、CODE-02~06、BUILD-07 内联样式
3. **下轮修复** (P2+P3): B06-09, B16-20, B23-31, SEC-02~06, UI-05~16, CODE-07~22, BUILD-01~06/08
4. **功能补全** (MISS): MISS-001~003 → MISS-004~011 → MISS-012~024

---

## 后端代码质量问题（第四轮深度代码审查）

### CODE-01 [P0] rate limiter goroutine 永不退出

- **文件**: `internal/middleware/ratelimit.go:23-34`
- **问题**: `go func()` 清理循环仅 `time.Sleep(time.Minute)`，无 context 取消或退出 channel。服务器关闭后 goroutine 永久运行。多次调用 `RateLimit()` 会启动多个永不终止的清理协程
- **影响**: goroutine 泄漏，进程生命周期内无法 GC
- **修复**: 接受 `context.Context`，使用 `time.NewTicker` + `<-ctx.Done()` 退出

### CODE-02 [P1] Hub.clients 值并发竞争

- **文件**: `pkg/ws/hub.go:239-241, 256-258, 377-388`
- **问题**: `Hub.clients` 是 `sync.Map`，值为 `*[]*Client` 切片。`IsOnline()` 和 `shutdown()` 是公共方法，直接在调用 goroutine 上运行，可能并发读写与 bus goroutine 正在修改的切片
- **修复**: 保护每个用户客户端列表用单独 mutex，或让 IsOnline/shutdown 也通过 bus 执行

### CODE-03 [P1] WebSocket 连接 ctx 未绑定 hub 生命周期

- **文件**: `internal/handler/websocket.go:71`
- **问题**: `ctx, cancel := context.WithCancel(context.Background())` 创建完全独立的上下文。hub 关闭时不会取消各连接的 ctx
- **修复**: 从 hub ctx 派生：`ctx, cancel := context.WithCancel(hubCtx)`

### CODE-04 [P1] Hub bus channel 发送可无限阻塞 handler

- **文件**: `pkg/ws/hub.go:444, 449, 455, 464, 485, 493, 503, 511, 578`
- **问题**: 所有公共方法 `h.bus <- BusMessage{...}` 为阻塞发送。bus 容量 256。满载时 HTTP handler goroutine 阻塞
- **修复**: 非关键方法用 `select/default` 丢弃消息并 warn

### CODE-05 [P1] config.yaml 缺 upload 和 redis.db 字段

- **文件**: `cmd/server/main.go:55-59`, `config/config.yaml`
- **问题**: Config struct 有 Upload 块，但 config.yaml 无 upload 部分。Redis 缺 db 字段。静默使用默认值
- **修复**: 补充 config.yaml 和 config.example.yaml

### CODE-06 [P1] Client.LastActive 无同步并发读写

- **文件**: `internal/handler/websocket.go:96`, `pkg/ws/hub.go:66`
- **问题**: `Client.LastActive` 是普通 `time.Time`，在 readLoop 中写入无同步机制。`lastPong` 用了 `atomic.Int64` 但 LastActive 没有
- **修复**: 改为 `atomic.Int64` 与 lastPong 一致

### CODE-07 [P2] createDatabase 数据库名通过 Sprintf 拼接

- **文件**: `cmd/server/main.go:315`
- **问题**: `fmt.Sprintf("CREATE DATABASE %s", cfg.Database.DBName)` 直接拼接。虽来自配置非用户输入，但若配置被篡改可执行任意 SQL
- **修复**: 验证数据库名 `^[a-zA-Z_][a-zA-Z0-9_]*$`

### CODE-08 [P2] 多语句迁移无事务包裹

- **文件**: `cmd/server/main.go:354`
- **问题**: `db.Exec(sql)` 直接执行。多语句迁移部分失败时已执行部分保留，重试可能不幂等
- **修复**: 每个迁移包装在 `BEGIN`/`COMMIT` 中

### CODE-09 [P2] WS chat handler 忽略 SendMessage 错误

- **文件**: `internal/handler/websocket.go:162`
- **问题**: `_, _ = h.msgSender.SendMessage(ctx, ...)` 丢弃错误和返回值。消息静默丢失
- **修复**: 记录错误并向客户端发送 WS 错误消息

### CODE-10 [P2] WS readLoop JSON 解组错误被吞噬

- **文件**: `internal/handler/websocket.go:112-114, 129-131, 141-143`
- **问题**: `_ = json.Unmarshal(raw, &payload)` 失败后 payload 为零值，客户端无反馈
- **修复**: 添加 warn 日志，可选向客户端发送格式错误提示

### CODE-11 [P2] ListMemberIDs 不包含会话所有者

- **文件**: `internal/repository/conversation.go:177-187`
- **问题**: 只查 conversation_members 表，私聊会话所有者可能不在成员表中，导致推送通知遗漏
- **修复**: SQL 加 `UNION SELECT user_id FROM conversations WHERE id = $1`

### CODE-12 [P2] Create 方法独立查询用户名(N+1)

- **文件**: `internal/repository/message.go:72-79`
- **问题**: Create 后单独 `SELECT username FROM users WHERE id = $1`，与 GetByID 的 JOIN 模式不一致
- **修复**: 在 RETURNING 子句中用子查询获取 username

### CODE-13 [P2] 静态文件服务缺少路径边界检查

- **文件**: `cmd/server/main.go:159-164`
- **问题**: `filepath.Join` 虽有清理但无明确边界验证
- **修复**: 验证 `strings.HasPrefix(absPath, absUploadDir)`

### CODE-14 [P2] postPersist 异步推送无重试

- **文件**: `internal/service/message.go:155-157`
- **问题**: `go s.postPersist(...)` 为 fire-and-forget，推送失败只 slog.Warn，无重试
- **修复**: 关键操作至少重试一次

### CODE-15 [P2] config.example 缺字段

- **文件**: `config/config.example.yaml`
- **修复**: 补充 upload 和 redis.db 部分

### CODE-16 [P2] 无单用户 WebSocket 连接数限制

- **文件**: `internal/handler/websocket.go:43-82`
- **问题**: 无并发连接数限制，恶意用户可开数千连接 DoS
- **修复**: 注册前检查用户已有连接数，设上限（如 5）

### CODE-17 [P2] Hub Register/Unregister 异步竞态

- **文件**: `pkg/ws/hub.go:442-449`
- **问题**: Register 入队后客户端指针在 bus 处理前就可能收到 JoinRoom/SendToRoom
- **修复**: 文档化生命周期保证，考虑为 Register/Unregister 添加同步确认

### CODE-18 [P3] Client.enqueue 背压时阻塞 dispatch

- **文件**: `pkg/ws/hub.go:106`
- **问题**: 背压场景第二次 `c.sendCh <- data` 无 select/default，可能阻塞整个 dispatch 循环
- **修复**: 用 select/default 丢弃消息并 warn

### CODE-19 [P3] 迁移 006 缺少 DOWN 部分

- **文件**: `migrations/006_add_indexes.sql`
- **修复**: 添加 `---- DOWN` 部分含 `DROP INDEX IF EXISTS`

### CODE-20 [P3] group handler 错误码 40300 被多个错误复用

- **文件**: `internal/handler/group.go:38, 56, 62, 66, 73, 108, 134, 156`
- **修复**: 为不同失败原因分配唯一错误码

### CODE-21 [P3] Redis 客户端未在 shutdown 时 Close

- **文件**: `cmd/server/main.go:105-116`
- **问题**: 有 `defer db.Close()` 但无 `rdb.Close()`
- **修复**: 添加 `defer rdb.Close()`

### CODE-22 [P3] RecallMessage 将 DB 错误误报为"消息不存在"

- **文件**: `internal/service/message.go:333-336`
- **问题**: `GetByID` 返回任何错误都映射为 `ErrMsgNotFound`，掩盖数据库故障
- **修复**: 区分 `sql.ErrNoRows` 和其他数据库错误

---

## 前端构建/规范问题（第四轮）

### BUILD-01 [P3] 缺少 ESLint 配置

- 项目无 .eslintrc 或 eslint.config.js，package.json 无 eslint 依赖

### BUILD-02 [P2] vendor-antd chunk 超 500 kB

- **文件**: dist/assets/vendor-antd-*.js (843 kB, gzip 274 kB)
- **修复**: 拆分 antd 组件为动态 import，或调高 chunkSizeWarningLimit

### BUILD-03 [P3] 多个依赖有 Major 版本更新

- react 18→19, react-router-dom 6→7, vite 6→8 等
- 建议: 稳定阶段统一评估升级

### BUILD-04 [P3] screenshot.mjs 幽灵 playwright 依赖

- 文件引用 playwright 但不在 package.json 中

### BUILD-05 [P3] 无 .env 环境文件

- 代理目标地址硬编码 localhost:8080

### BUILD-06 [P3] useWebSocket.ts TODO 残留

- 第 141 行 `// TODO: 全局错误提示`

### BUILD-07 [P1] 大量内联 style 违反编码规范

- **文件**: 9 个组件文件共 40+ 处 `style={{...}}`
- **违反**: AGENTS.md 规则 9 "样式使用 CSS Modules，禁止内联样式"
- **修复**: 全部迁移到对应 .module.css 文件

### BUILD-08 [P2] messageStore.ts 超 300 行限制

- 311 行，超出 AGENTS.md 规定的 300 行上限
- **修复**: 拆分 streaming 相关逻辑到独立 store

---

## 文档准确性（第六轮审计）

### DOC-01 [P1] API 文档覆盖率仅 26%

- 31 个端点仅 8 个有文档，74% 端点无文档覆盖
- **修复**: 补全所有端点文档，包括请求/响应示例

### DOC-02 [P1] 错误响应格式文档与代码不一致

- **文档**: `{"error": {"code": "string", "message": "string"}}`
- **代码**: `{"code": 50016, "message": "创建私聊失败"}`（扁平结构，code 为数字）
- **修复**: 统一文档与实际响应格式

### DOC-03 [P1] WebSocket 消息类型命名文档与代码不匹配

- **文档**: `message.new`, `message.stream`
- **代码**: `message.complete`, `message.streaming`
- 前端 typing 消息: `typing_start` vs 后端 `user.typing`
- **修复**: 统一消息类型命名，更新文档或代码

### DOC-04 [P2] Conversation type 文档与代码不一致

- **文档**: `"direct"` 表示私聊
- **代码**: `"single"` 表示私聊
- **修复**: 统一为一种命名

### DOC-05 [P2] username 校验规则三处冲突

- binding tag: `min=3,max=20`
- regex: `^[a-zA-Z0-9_]{2,20}$`（最小2）
- service 层: `len < 2 || len > 20`
- 三处规则不统一（min=2 vs 3）
- **修复**: 统一为一处校验

### DOC-06 [P2] ErrGroupNotFound 可能未定义

- group handler 使用 `ErrGroupNotFound` 但错误包中可能未声明
- **修复**: 确认错误声明存在，不存在则添加

### DOC-07 [P2] model.ConversationMember.JoinedAt 类型不当

- Go 中类型为 `string`，应为 `time.Time`
- **修复**: 修改模型类型并确保 JSON 序列化正确

### DOC-08 [P2] model.ConversationMember 缺少 last_read_at 字段

- 前端使用 `last_read_at` 但模型未定义
- **修复**: 添加字段或确认前端适配

### DOC-09 [P3] service/message.go 超 300 行限制

- 402 行，远超 AGENTS.md 规定的 300 行上限
- **修复**: 拆分为 message.go + message_stream.go

### DOC-10 [P3] Commit message 格式违反 50 字符限制

- 多个 commit 超过 50 字符标题长度
- **修复**: 后续 commit 遵守约定

---

## 前端性能问题（第六轮审计）

### PERF-01 [P1] messages store 无限增长

- **文件**: `src/frontend/src/store/messageStore.ts`
- **现象**: 所有对话消息累积在内存，永不清理，切换对话不释放旧消息
- **修复**: 实现 LRU 淘汰策略或限制每个对话最多缓存 N 条

### PERF-02 [P1] AppLayout 订阅整个 unreadCounts 对象

- **文件**: `src/frontend/src/layout/AppLayout.tsx`
- **现象**: `useConversationStore(s => s.unreadCounts)` 订阅全量对象，任何对话未读数变化都触发重渲染
- **修复**: 使用 selector 只取需要的值，或用 `shallow` 比较

### PERF-03 [P1] ChatWindow 订阅全部 typingUsers

- **文件**: `src/frontend/src/components/chat/ChatWindow.tsx`
- **现象**: 订阅整个 `typingUsers` map，任何对话的 typing 变化都触发重渲染
- **修复**: 只订阅当前对话的 typing 用户

### PERF-04 [P2] MessageBubble 未使用 React.memo

- **现象**: 父组件重渲染时所有消息气泡都重新渲染
- **修复**: 包裹 `React.memo`，用 `content` + `status` 做浅比较

### PERF-05 [P2] smooth scroll 在流式消息时引起抖动

- **文件**: `src/frontend/src/components/chat/MessageList.tsx`
- **现象**: streaming 时每收到新 chunk 都 smooth scroll，导致视口持续跳动
- **修复**: streaming 期间改用 `instant` scroll，结束后再 smooth

### PERF-06 [P2] 对话切换重复 API 请求

- **现象**: 切换对话时 fetchMessages 可能触发多次（竞态），且缓存命中仍发起请求
- **修复**: 增加请求去重逻辑，缓存有效时跳过请求

### PERF-07 [P2] transition:all 导致布局抖动

- **文件**: 多处 CSS
- **现象**: `transition: all 0.3s` 在 width/height 变化时触发昂贵的 layout 重计算
- **修复**: 改为具体属性 `transition: background-color 0.3s, opacity 0.3s`

### PERF-08 [P2] renderMarkdown 无缓存

- **文件**: `src/frontend/src/components/chat/MessageBubble.tsx`
- **现象**: 每次 render 都重新解析 markdown，长消息性能差
- **修复**: 使用 `useMemo` 缓存，依赖 `content` 变化

### PERF-09 [P3] conversationStore 订阅粒度过粗

- **现象**: 组件订阅 `conversations` 数组，任何对话属性变化都触发所有订阅者重渲染
- **修复**: 使用细粒度 selector 或引入 `useShallow`

### PERF-10 [P3] friendStore 同理，订阅全量 friends 数组

- **修复**: 细粒度 selector

### PERF-11 [P3] 未使用 React.lazy 懒加载

- **现象**: 所有页面组件同步加载，首屏包含未使用代码
- **修复**: 非首屏路由用 `React.lazy` + `Suspense`

### PERF-12 [P3] WebSocket 消息处理未做 batching

- **现象**: 高频消息每条都触发 setState，导致连续重渲染
- **修复**: 使用 `unstable_batchedUpdates` 或 requestAnimationFrame 合并更新

---

## 第七轮深度审计（后端+迁移+前端状态）

### B33 [P1] WebSocket 发消息静默丢弃 DB 持久化错误

- **文件**: `internal/handler/websocket.go:165`
- **现象**: `_, _ = h.msgSender.SendMessage(...)` 丢弃所有返回值。消息持久化失败时，用户看到乐观 UI 但消息从未存储，其他客户端看不到，历史记录中没有
- **修复**: 检查 error 并通过 WebSocket 发送错误反馈给客户端

### B34 [P2] 负 offset 值直接传 SQL 无校验

- **文件**: `internal/handler/conversation.go:87-88`
- **现象**: `strconv.Atoi` 错误被忽略，`?offset=-50` 直接传入 SQL `OFFSET -50`，PostgreSQL 返回错误导致 500
- **修复**: 增加负数和零值校验，offset < 0 返回 400

### B35 [P1] conversation 服务创建群聊非原子

- **文件**: `internal/service/conversation.go:68-82`
- **现象**: `Create()` + `AddMember()` 是两个独立操作，若 `AddMember` 失败则尝试补偿 `Delete()`，但补偿也可能失败，留下无成员的孤立群聊
- **修复**: 使用数据库事务包裹整个创建流程

### B36 [P2] GetUnreadMessages 降级查询返回全部消息

- **文件**: `internal/service/message.go:294`
- **现象**: Redis 无缓存时，fallback 传 `nil` 作为 `afterTime`，返回对话全部消息而非仅未读
- **修复**: 从 DB 查询 `last_read_at` 作为 afterTime 传入

### B37 [P2] 删除私聊后用户仍可发消息

- **文件**: `internal/service/message.go:93-105`
- **现象**: `checkMembership` 先检查 `conv.UserID == userID`，即使用户已从 `conversation_members` 中删除（删除对话操作），仍可绕过发送消息
- **修复**: 仅通过 `conversation_members` 表判断成员身份

### B38 [P2] 并发 401 导致重复 token 清除+重定向

- **文件**: `src/frontend/src/api/client.ts:68-71`
- **现象**: 多个并发请求同时收到 401 时，各自独立调用 `clearToken()` + `window.location.href = '/login'`
- **修复**: 引入模块级 `isRedirecting` 标志

### B39 [P1] 归档对话错误触发 delete API

- **文件**: `src/frontend/src/components/sidebar/ConversationList.tsx:63-66`
- **现象**: archive handler 调用 `convApi.archiveConversation()` 后又调用 `remove()` 即 `deleteConversation()`，后者额外发起 DELETE 请求，导致归档 = 删除
- **修复**: archive 后仅更新本地状态，不调用 delete

### B40 [P2] upload.ts JSON 解析无 try/catch

- **文件**: `src/frontend/src/api/upload.ts:16`
- **现象**: 直接 `await res.json()` 无错误处理，非 JSON 响应抛出未处理 SyntaxError
- **修复**: 包裹 try/catch 转为 ApiError

### SEC-07 [P2] WebSocket JWT 通过 query string 被日志明文记录

- **文件**: `internal/handler/websocket.go:45`, `internal/middleware/logger.go:22`
- **现象**: `?token=xxx` 出现在请求路径中，RequestLogger 打印完整路径含 JWT
- **修复**: Logger 对 token 参数脱敏，或 WebSocket 端点跳过完整路径日志

### SEC-08 [P2] SendFriendRequest 的 friend_id 未校验 UUID

- **文件**: `internal/handler/friend.go:24`
- **现象**: `SendFriendRequestBody.FriendID string` 无 `binding:"uuid"` 标签，任意字符串直达 DB 查询
- **修复**: 添加 `binding:"required,uuid"` 标签

### SEC-09 [P2] CreateGroup member_ids 未逐个校验 UUID

- **文件**: `internal/handler/group.go:24-26`
- **现象**: `MemberIDs []string` 仅 `max=100`，未校验每个元素为 UUID，导致 FK 约束错误返回 500
- **修复**: 添加 `dive,uuid` 校验标签

### CODE-23 [P2] RemoveMember 错误未包装为 sentinel

- **文件**: `internal/repository/group.go:80-93`
- **现象**: 返回 `fmt.Errorf("member not found")` 而非 `ErrNotMember`，handler 无法匹配，降级 500
- **修复**: 使用 `errors.Is` 兼容的 sentinel error

### CODE-24 [P2] schema_migrations 表创建错误被忽略

- **文件**: `cmd/server/main.go:324`
- **现象**: `_, _ = db.Exec(CREATE TABLE...)` 忽略错误，若创建失败所有迁移被跳过且无报错
- **修复**: 检查错误并 fatal

### CODE-25 [P2] postPersist goroutine 与 Hub shutdown 竞态

- **文件**: `internal/service/message.go:160`
- **现象**: postPersist 使用 `context.Background()` 不感知 shutdown，可能在 Hub 关闭后向已关闭 channel 发送
- **修复**: 传入可取消的 context 或检查 Hub 状态

### DB-08 [P1] CASCADE 删除用户时销毁群聊

- **文件**: `migrations/002_create_conversations.sql:4`
- **现象**: `conversations.user_id REFERENCES users(id) ON DELETE CASCADE`，删除用户会级联删除该用户创建的所有群聊，影响其他所有成员
- **修复**: 改为 `ON DELETE SET NULL` 并允许 `user_id` 为空

### DB-09 [P1] 可空 DB 列映射为非指针 Go 类型

- **文件**: `internal/model/attachment.go:13-15`
- **现象**: `thumbnail_path VARCHAR(1000)` 可空但 Go 模型为 `string`；`width/height INTEGER` 可空但 Go 为 `int`。StructScan 时遇到 NULL 将 panic
- **修复**: 改为 `*string` 和 `*int`

### DB-10 [P2] ANY($1)+[]string 在 sqlx 下可能运行时失败

- **文件**: `internal/repository/message.go:310`
- **现象**: `r.db.QueryxContext(ctx, query, replyIDs)` 传 Go `[]string` 给 PostgreSQL UUID 数组，需 pq.Array() 或 pgx 支持
- **修复**: 使用 `pq.Array(replyIDs)` 包装

### DB-11 [P2] GroupRepo.AddMember 缺 ON CONFLICT 幂等

- **文件**: `internal/repository/group.go:68-77`
- **现象**: 裸 INSERT 无 ON CONFLICT，重复添加同一成员返回唯一约束错误
- **修复**: 添加 `ON CONFLICT (conversation_id, user_id) DO NOTHING`

### FE-01 [P1] useWebSocket 中 currentUserId 闭包过期

- **文件**: `src/frontend/src/hooks/useWebSocket.ts:52,114`
- **现象**: `currentUserId` 在 useEffect 外通过 selector 读取，但不在依赖数组中。用户信息变更后，typing 判断使用过期 userId，可能导致自己显示为"他人正在输入"
- **修复**: 将 currentUserId 加入依赖数组或使用 ref

### FE-02 [P1] archive handler 错误调用 delete

- 同 B39

### FE-03 [P2] upload.ts 绕过共享 request()函数

- **文件**: `src/frontend/src/api/upload.ts:3-21`
- **现象**: 自建 fetch 逻辑，缺少 5xx 重试、网络错误 toast、JSON 解析安全网
- **修复**: 尽可能复用 client.ts 的基础设施

### FE-04 [P2] conversationStore togglePin/createConversation 无错误处理

- **文件**: `src/frontend/src/store/conversationStore.ts:42-49,68-77`
- **现象**: 两个方法无 try/catch，API 失败时 unhandled promise rejection，无用户反馈
- **修复**: 添加 try/catch + antd message.error

### FE-05 [P2] retryOptimistic 中 attachments 类型强转

- **文件**: `src/frontend/src/store/messageStore.ts:227`
- **现象**: `optMsg.attachments as AttachmentPayload[]` 但 `MessageAttachment` 和 `AttachmentPayload` 字段不同，可能导致请求失败
- **修复**: 转换为正确的 AttachmentPayload 结构

### FE-06 [P2] useConversation 每次挂载触发重复 API 调用

- **文件**: `src/frontend/src/hooks/useConversation.ts:14-16`
- **现象**: ConversationList 和 AppLayout 都调用 useConversation，页面加载时 2 个并发 GET /api/conversations
- **修复**: 添加"已请求"标志或请求去重

### FE-07 [P2] 无 AbortController 取消机制

- **文件**: `src/frontend/src/api/client.ts`
- **现象**: 所有 fetch 请求无 signal，组件卸载后请求继续执行并更新状态
- **修复**: request 函数接受 AbortSignal 参数

---

## 第十轮：后端 E2E 流程追踪 + 前端 UX 审计

### B44 [P1] 好友请求只单向检查，允许双向重复请求

- **文件**: `src/backend/internal/repository/friend.go` — GetFriendship
- **现象**: GetFriendship 仅查询 `(user_id=A, friend_id=B)`，不检查 `(user_id=B, friend_id=A)`。用户 B 可在收到 A 的请求后，再向 A 发送好友请求，产生两条 friend 记录
- **复现**: A→B 发请求（status=pending），B→A 再发请求成功（第二条 pending 记录）
- **修复**: GetFriendship 改为 `WHERE (user_id=$1 AND friend_id=$2) OR (user_id=$2 AND friend_id=$1)`

### B45 [P1] 归档私聊后对方 GetOrCreatePrivateChat 创建重复对话

- **文件**: `src/backend/internal/repository/conversation.go` — FindPrivateChat
- **现象**: FindPrivateChat 带 `archived_at IS NULL` 过滤，用户 A 归档私聊后，B 调用 GetOrCreatePrivateChat 找不到原对话，创建新的。同一对用户出现两条 single 类型对话
- **修复**: FindPrivateChat 仅用 user_id 过滤，归档用 conversation_members 维护而非对话级别

### B46 [P1] ListMemberIDs UNION 返回重复 userID，群主收到重复消息

- **文件**: `src/backend/internal/repository/conversation.go` — ListMemberIDs
- **现象**: 查询用 `SELECT user_id FROM conversations WHERE … UNION SELECT user_id FROM conversation_members WHERE …`，群主同时出现在两表中，UNION 去重了但实际推送时 Hub 按 memberIDs 循环发送，若调用方展开为 slice 则重复
- **修复**: 改用 `UNION` 后 `SELECT DISTINCT` 或统一为单查询

### B47 [P1] 私聊非创建者无法归档（只检查 conv.UserID）

- **文件**: `src/backend/internal/service/conversation.go` — Archive
- **现象**: 归档权限仅检查 `conv.UserID == userID`，私聊对方无法归档自己的副本
- **修复**: single 类型应检查 conversation_members 而非 conv.UserID

### B48 [P1] WS join_room 权限检查与 REST 不一致

- **文件**: `src/backend/internal/handler/websocket.go` vs `service/conversation.go`
- **现象**: WS join_room 用 `GroupRepo.IsMember` 仅查 conversation_members 表，REST checkMembership 有 fallback 查 conv.UserID。对 single 类型对话，WS join 可能拒绝合法用户
- **修复**: WS 复用 service 层 checkMembership

### B49 [P1] 群创建 owner INSERT 无 ON CONFLICT 非幂等

- **文件**: `src/backend/internal/repository/group.go` — CreateGroup
- **现象**: 群主 INSERT conversation_members 无 ON CONFLICT，重试/并发时主键冲突返回 500
- **修复**: 添加 `ON CONFLICT (conversation_id, user_id) DO NOTHING`

### B50 [P2] username 校验 max 冲突：binding 50 vs regex 20

- **文件**: `src/backend/internal/dto/request.go` + `service/user.go`
- **现象**: Gin binding tag `max=50`，但 regex 校验 `^.{2,20}$`。21-50 字符用户名通过 binding 但被 regex 拒绝，错误消息不明确
- **修复**: 统一为同一上限值

### B51 [P2] typing 通知广播包含发送者自己

- **文件**: `src/backend/internal/handler/websocket.go` — typing handler
- **现象**: 广播 typing 事件给所有在线成员包括发送者自己，浪费流量
- **修复**: 推送时排除 sender connection

### B52 [P2] RecallMessage 对无 sender_id 的群聊历史消息推断错误

- **文件**: `src/backend/internal/service/message.go` — RecallMessage
- **现象**: 群聊历史消息 sender_id 可能为空（系统消息等），撤回权限检查时 fallback 到 conv.UserID（群创建者），普通成员撤回自己的消息反而失败
- **修复**: sender_id 为空时直接拒绝撤回（非正常路径），不要 fallback

---

## 第十轮：前端 UX 审计发现

### UX-01 [P1] 删除对话无二次确认弹窗

- **文件**: `src/frontend/src/components/sidebar/ConversationList.tsx`
- **现象**: 点击删除直接调用 deleteConversation API，无确认弹窗。误触即不可恢复
- **修复**: 添加 Ant Design `Modal.confirm` 二次确认

### UX-02 [P1] 创建群组后群不自动激活

- **文件**: `src/frontend/src/layout/AppLayout.tsx` — handleGroupCreate
- **现象**: handleGroupCreate 调用 createConversation 但不调用 setActive，用户需手动在列表中找新建的群
- **修复**: createConversation 成功后调用 setActive(conv.id)

### UX-03 [P1] 切换对话无"新消息"指示器

- **现象**: 用户在 A 对话中时，B 对话收到新消息仅靠 badge 数字提示，无"跳到新消息"按钮。消息列表长时新消息不可见
- **修复**: 添加浮动"新消息 N 条 ↓"按钮，点击跳转到底部

### UX-04 [P1] 无响应式设计

- **文件**: `src/frontend/src/layout/AppLayout.tsx`
- **现象**: 三栏布局固定宽度，移动端(<768px)三列挤在一起不可用。无断点切换逻辑
- **修复**: 添加 CSS media query 或移动端适配（单栏/抽屉模式）

### UX-05 [P1] 快速切换对话时 fetchMessages 竞态

- **文件**: `src/frontend/src/store/messageStore.ts` — fetchMessages
- **现象**: 快速在 A→B→C 之间切换，三个 fetch 并发。A/B 的响应可能在 C 之后到达，覆盖 C 的正确数据
- **修复**: 添加请求序列号或 AbortController，旧请求完成时检查当前活跃对话是否仍匹配

### UX-06 [P2] 新建对话默认标题"新对话"硬编码

- **文件**: `src/frontend/src/layout/AppLayout.tsx` — handleCreate
- **现象**: `title: '新对话'` 硬编码，所有新对话同名，无法区分
- **修复**: 弹出输入框让用户命名，或自动生成唯一标题（如 "对话 #N"）

### UX-07 [P2] 对话列表无空状态引导

- **现象**: 新注册用户对话列表为空，无任何引导文字或"创建第一个对话"按钮
- **修复**: 空列表时显示 Empty 组件 + 创建引导

### UX-08 [P2] 发送按钮无 loading 状态

- **文件**: `src/frontend/src/components/chat/ChatInput.tsx`
- **现象**: 发送消息时按钮无 loading 效果，用户可连续点击触发多次 sendMessage
- **修复**: 发送中禁用按钮或显示 spinner

### UX-09 [P2] 群聊创建后不自动打开成员面板

- **现象**: 新建群组后用户不知道如何管理成员，需手动点击成员按钮
- **修复**: 群创建成功后自动打开成员面板

### UX-10 [P2] 好友申请无备注/留言字段

- **文件**: `src/frontend/src/components/friends/FriendPanel.tsx`
- **现象**: 发送好友申请仅能点按钮，无法附言（如"我是XXX"），对方无法判断来源
- **修复**: 添加可选的留言输入框（需后端配合）

### UX-11 [P2] 消息时间戳仅显示时间不显示日期

- **文件**: `src/frontend/src/components/chat/MessageBubble.tsx`
- **现象**: 所有消息仅显示 HH:mm，跨天消息无法区分日期
- **修复**: 当天显示时间，非当天显示"昨天/MM-DD HH:mm"

### UX-12 [P2] 输入框不支持 Shift+Enter 换行

- **文件**: `src/frontend/src/components/chat/ChatInput.tsx`
- **现象**: Enter 直接发送，无法多行输入。长消息只能一段段发
- **修复**: Shift+Enter 换行，Enter 发送

### UX-13 [P2] 消息列表不支持键盘快捷键

- **现象**: 无 Esc 关闭面板、↑↓ 选择消息、Ctrl+F 搜索等键盘操作
- **修复**: 添加常见快捷键支持

### UX-14 [P2] 长消息无折叠/展开功能

- **现象**: AI Agent 返回的长消息（代码块等）占满整个视口，无法折叠
- **修复**: 超过 N 行自动折叠，点击展开

### UX-15 [P2] 无对话内消息搜索功能

- **现象**: 无法在当前对话中搜索关键词，找不到历史消息
- **修复**: 添加 Ctrl+F 弹出搜索栏，高亮匹配消息

### UX-16 [P3] 右键菜单仅显示"撤回"

- **文件**: `src/frontend/src/components/chat/MessageBubble.tsx`
- **现象**: 右键/长按菜单只有"撤回"，缺少"复制文本""转发""引用回复"等常见 IM 操作
- **修复**: 扩展右键菜单选项

### UX-17 [P3] 未读 badge 超 99 无特殊显示

- **现象**: 100+ 条未读仍显示具体数字，占据大量空间
- **修复**: 超过 99 显示 "99+"

### UX-18 [P3] 对话列表项无 hover 预览

- **现象**: 鼠标悬停对话项时不显示最后一条消息摘要，需点击进入才能看到
- **修复**: Tooltip 显示最近消息预览

### UX-19 [P3] 无消息发送失败的全局重试提示

- **现象**: 乐观消息失败后仅在消息旁显示小图标，无全局提示条"有 N 条消息发送失败"
- **修复**: 添加顶部提示条，一键重试所有失败消息

### UX-20 [P3] Emoji 选择器无最近使用/常用分类

- **文件**: `src/frontend/src/components/chat/EmojiPicker.tsx`
- **现象**: Emoji 面板无历史记录，每次从分类中翻找
- **修复**: 添加"最近使用"分类，localStorage 持久化
