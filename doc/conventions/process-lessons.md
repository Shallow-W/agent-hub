# 流程问题与教训

本文档记录开发过程中遇到的流程性问题，防止同类错误再次发生。

---

## 1. 后端代码变更后必须重启服务器

**问题**：修改后端 Go 代码并 commit 后，忘记重新编译和重启后端服务器。前端调用新接口返回 404。

**案例**：添加 `POST /api/conversations/private` 端点后，运行中的服务器仍是旧二进制，前端点击好友报 "创建私聊失败"。

**规则**：每次后端代码 commit 后，必须：
```bash
cd /Users/shallow/Desktop/repo/agent-hub/src/backend && go build -o bin/server ./cmd/server/
# kill 旧进程，启动新二进制
kill $(lsof -ti :8080) && ./bin/server
```

**根本原因**：CLAUDE.md 规则 5 中的 Review 流程只做了静态检查（编译通过），缺少运行时验证。

---

## 2. API 返回 null 而非空数组

**问题**：后端 API 在无数据时返回 `{"data": null}` 而非 `{"data": []}`。前端直接赋值给数组类型的 state，后续 `.filter()` / `.length` 崩溃。

**案例**：`GET /api/friends` 无好友时返回 `data: null`，`friendStore` 设置 `friends = null`，`FriendList` 中 `friends.filter(...)` 抛出 `TypeError: Cannot read properties of null`。

**规则**：
- 前端 store 所有 API 响应赋值处，必须加 `?? []` 兜底
- 后端 handler 应在无数据时返回空数组而非 null

**检测方法**：测试环节必须覆盖"无数据"的边界情况。

---

## 3. Bash 命令必须包含 cd 路径

**问题**：Go 和前端命令在 Bash tool 的 `command` 参数中省略了 `cd` 前缀，导致在项目根目录执行而非 `src/backend` 或 `src/frontend`，反复报 `cannot find main module`。

**规则**：所有 Go 命令必须使用：
```bash
cd /Users/shallow/Desktop/repo/agent-hub/src/backend && go build ./...
```
所有前端 npm/node 命令必须使用：
```bash
cd /Users/shallow/Desktop/repo/agent-hub/src/frontend && npx tsc --noEmit
```

**注意**：`description` 参数只是描述标签，不影响实际执行目录。路径必须写在 `command` 参数中。

---

## 4. 新功能必须端到端测试

**问题**：实现好友系统功能后，只验证了编译通过（`go build` + `tsc --noEmit`），没有测试实际运行时行为。导致多个运行时 bug 未被发现。

**案例**：
- `onStartChat` 忽略 friendId 参数（功能完全不工作）
- 好友列表从未 fetch（UI 始终为空）
- API 返回 null 导致前端崩溃

**规则**：每个新功能/bug修复完成后，必须：
1. 编译检查（后端 `go build`，前端 `tsc --noEmit`）
2. API 级别测试（curl 或类似工具验证端点返回正确）
3. 边界情况测试（空数据、无效输入、并发请求）
4. 前端 UI 验证（至少确认不崩溃）

---

## 5. 功能完整性审查

**问题**：团队/子代理实现功能时，只完成了 API 层和组件层，遗漏了关键的"接线"代码（如 useEffect 数据加载、事件处理参数传递）。

**规则**：功能实现的 checklist：
- [ ] 后端 API 端点可 curl 调通
- [ ] 前端 store 在组件挂载/页面切换时正确触发数据加载
- [ ] 用户交互（点击、提交）正确传递参数到 handler
- [ ] handler 调用正确的 API 并处理响应/错误
- [ ] UI 在空数据/加载中/错误状态下不崩溃

---

## 6. API 响应结构不一致导致的幽灵消息

**问题**：用户发送带回复的消息后，左侧出现一条幽灵消息，显示为"助手 NaN:NaN"，内容仅为引用的原文。普通消息发送也存在同样问题（幽灵消息为空气泡，不易察觉）。

**根因分析**：

后端 `MessageHandler.Send` 返回 `SendMessageResult` 结构：

```go
type SendMessageResult struct {
    UserMessage  *model.Message `json:"user_message"`
    AgentMessage *model.Message `json:"agent_message,omitempty"`
}
```

经 `middleware.CreatedResponse` 包装后，前端收到：
```json
{ "code": 0, "data": { "user_message": {...}, "agent_message": null } }
```

但前端 `api/message.ts` 的 `sendMessage` 直接将 `json.data` 当作 `Message` 类型返回。`client.ts` 的 `request<T>` 函数执行 `return json.data as T`，所以 `msg` 实际是 `{ user_message: {...}, agent_message: null }`，而非 `Message` 对象。

`messageStore.ts` 收到后：
1. `msg.id` → `undefined`（真正的 id 在 `msg.user_message.id`）
2. `msg.content` → `undefined`
3. `msg.role` → `undefined`
4. `patchedMsg` 逻辑将 `reply_to_message` 拼到这个错误对象上 → 引用内容被渲染为独立气泡
5. `MessageBubble` 渲染：`role` 缺失走 assistant 分支 → 显示"助手"，`created_at` 缺失 → `new Date(undefined)` → "NaN:NaN"

由于幽灵消息 `id: undefined`，后续 WS 推送的真实消息无法通过 `addMessage` 去重（`findIndex` 比对 `undefined` vs 真实 ID 不匹配），导致两条消息同时存在。

**修复**：在 `api/message.ts` 提取 `result.user_message`：

```ts
const result = await post<SendMessageResult>(...);
return result.user_message;
```

**教训**：

- **后端返回包装结构时，前端必须正确解包**。当 API 响应是 `{ user_message, agent_message }` 这种包装结构而非直接实体时，`post<T>` 的泛型 `T` 应为包装类型，然后手动提取所需字段
- **TypeScript 类型系统无法在运行时防止此类错误**。`as T` 只是编译期断言，实际运行时数据结构不符时不会报错
- **检查清单**：新增 API 调用时，必须确认后端实际返回结构与前端类型定义一致

**影响范围**：所有通过 HTTP API 发送的消息（包括普通消息和回复消息）。WS 推送路径不受影响（WS handler 直接使用推送数据构造 Message 对象）。
