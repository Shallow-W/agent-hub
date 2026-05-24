# M2 WebSocket 通信基础设施

## 目标

建立前后端 WebSocket 长连接通道，支持实时消息推送和流式输出。

## 子任务

### M2-1 后端 WebSocket 管理

- WebSocket 升级端点：`WS /ws?token=xxx`
- 连接鉴权（验证 JWT token）
- 连接管理：按 user_id 维护连接映射
- 心跳机制（ping/pong，30s 间隔）
- 断连检测与清理

### M2-2 前端 WebSocket 客户端

- 封装 WebSocket 连接类/模块
- 自动重连逻辑（指数退避，最大 30s）
- 消息队列（断连期间缓存消息）
- 连接状态管理（connecting/connected/disconnected）

### M2-3 消息类型定义

统一消息格式：

```json
{
  "type": "message.streaming | message.complete | agent.status | error",
  "data": { }
}
```

- `message.streaming`：Agent 流式输出片段（`{conversationId, messageId, content, done: false}`）
- `message.complete`：消息完成（`{conversationId, messageId, content, artifacts, done: true}`）
- `agent.status`：Agent 状态变更（`{agentId, status: "thinking"|"running"|"idle"}`）
- `error`：错误通知（`{code, message}`）

## 验收标准

- [ ] 前端可建立 WebSocket 连接并通过鉴权
- [ ] 后端可向指定用户推送消息
- [ ] 心跳正常工作，断连被检测到
- [ ] 前端断连后自动重连成功
- [ ] 消息类型定义前后端一致

## 依赖

- M0-1（前端骨架）
- M0-2（后端骨架）
- M1-2（JWT token）
