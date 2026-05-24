# M10 Pin 消息上下文

## 目标

用户可 Pin 关键消息，被 Pin 的内容自动注入后续所有 Agent 请求的上下文中。

## 子任务

### M10-1 Pin 消息 UI

- 消息右键菜单或 hover 按钮中添加 "Pin" / "Unpin"
- Pin 的消息在聊天中显示特殊标记（如 📌 图标）
- 侧边栏或顶部展示当前对话的 Pin 消息列表

### M10-2 Pin 消息接口

- `POST /api/conversations/:id/messages/:msgId/pin` — Pin
- `DELETE /api/conversations/:id/messages/:msgId/pin` — Unpin
- `GET /api/conversations/:id/pins` — 获取对话的所有 Pin 消息
- 存储到 `pinned_messages` 表

### M10-3 Pin 内容注入上下文

- 当向 Agent 发送消息时，查询该对话的所有 Pin 消息
- 将 Pin 消息内容拼接到上下文的开头，标记为 `[Pinned Context]`
- 发送给守护进程 → 适配器 → CLI Agent
- 在前端展示 Pin 注入状态提示

## 验收标准

- [ ] 可对任意消息执行 Pin/Unpin 操作
- [ ] Pin 消息有视觉标记
- [ ] 后续对话中 Agent 的回复体现了 Pin 内容的影响
- [ ] 取消 Pin 后，新对话不再包含该内容

## 依赖

- M3-4（聊天窗口 UI）
- M0-4（pinned_messages 表）
- M4-3（适配器，上下文注入点）

## 优先级

P1
