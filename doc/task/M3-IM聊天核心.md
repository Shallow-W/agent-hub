# M3 IM 聊天核心

## 目标

实现 IM 聊天界面和消息收发，包含对话列表、聊天窗口、流式消息展示。

## 子任务

### M3-1 对话 CRUD 接口

- `POST /api/conversations` — 创建对话（指定 type 和 agent_ids）
- `GET /api/conversations` — 对话列表（按 updated_at 排序）
- `DELETE /api/conversations/:id` — 删除对话
- `PUT /api/conversations/:id/pin` — 置顶/取消置顶

### M3-2 消息接口

- `POST /api/conversations/:id/messages` — 发送消息
- `GET /api/conversations/:id/messages?before=xxx&limit=50` — 加载历史消息
- 消息通过 WebSocket 实时推送（不走 REST 轮询）

### M3-3 对话列表侧边栏

- 左侧固定侧边栏
- 显示对话列表（标题 + 最后一条消息预览 + 时间）
- 新建对话按钮 → 弹出 Agent 选择
- 支持置顶/归档/删除操作
- 按最近活跃排序

### M3-4 聊天窗口

- 消息列表区域（区分用户消息和 Agent 消息）
- 消息气泡样式（用户靠右，Agent 靠左，显示 Agent 头像/名称）
- 底部输入框 + 发送按钮
- 代码块基础渲染（语法高亮）
- 滚动加载历史消息

### M3-5 流式消息展示

- Agent 回复逐字渲染（通过 WebSocket 接收 streaming 消息）
- 显示 Agent "正在输入..." 状态
- 流式完成后渲染为完整消息

### M3-6 @Agent 选择器

- 单聊模式：选择一个 Agent 开始对话
- 群聊模式：@多个 Agent，显示已选 Agent 标签
- Agent 列表展示名称 + 头像 + 能力标签

### M3-7 多会话并行

- 切换不同对话不丢失状态
- 多个对话可同时接收消息

## 验收标准

- [ ] 可创建对话并在列表中看到
- [ ] 可发送文字消息，消息正确存储和展示
- [ ] Agent 回复逐字流式展示
- [ ] 对话列表按活跃排序，支持置顶
- [ ] 可切换多个对话，消息不串

## 依赖

- M0-1（前端骨架）
- M1-3（登录后进入主界面）
- M2（WebSocket 通道）
