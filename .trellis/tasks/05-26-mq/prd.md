# PRD: MQ 消息队列 + 消息存储优化 + 离线消息 + 原子性

## 目标
统一消息管道，解决 HTTP 发消息不推送、WS chat 不持久化的断裂问题，引入 Redis 做离线消息缓冲和热数据缓存。

## 现状问题
1. HTTP POST 发消息 → 存 DB → 不推送给其他成员
2. WS chat 消息 → 转发房间 → 不持久化到 DB
3. 无离线消息机制，用户上线后看不到离线期间的消息
4. 消息存储全靠 PostgreSQL，无缓存层

## 方案

### 1. 统一消息管道
- 消息发送（HTTP 或 WS）统一走 Service 层
- Service 层职责：权限校验 → DB 事务写入 → Hub 推送 → Redis 缓存
- Hub bus 新增 `BusPersistedMsg` 类型，携带持久化后的完整 Message

### 2. Redis 离线消息
- Redis Sorted Set 存储每会话的离线消息队列（key: `offline:{conversationID}`，score: timestamp）
- 用户上线时，Hub 检测离线消息并推送
- 消息被所有成员读取后清理 Redis 缓冲
- 热数据缓存：最近 50 条消息缓存到 Redis（key: `msgs:{conversationID}`）

### 3. MQ 选型
- 单实例用 Go channel（Hub bus 已有机制），不引入外部 MQ
- Hub bus 新增消息类型处理持久化消息的推送

### 4. 消息原子性
- repo.Create 事务：INSERT message + UPDATE conversation.updated_at（已有）
- 新增：DB 写入成功后才触发 Hub 推送 + Redis 写入
- 推送失败不影响消息持久化（最终一致性）

## 改动范围

### 后端
| 文件 | 改动 |
|------|------|
| `pkg/ws/hub.go` | 新增 `BusPersistedMsg`，新增 `PushToConversation` 方法 |
| `internal/handler/websocket.go` | WS chat → 先走 Service 持久化再转发 |
| `internal/handler/message.go` | Send 成功后调 Hub 推送 + 新增离线消息拉取接口 |
| `internal/service/message.go` | SendMessage 增加推送回调 |
| `internal/repository/message.go` | 新增 `GetMessagesAfter` 按时间拉取 |
| `internal/repository/redis.go` (新) | Redis 客户端封装：离线队列读写、消息缓存 |
| `cmd/server/main.go` | DI 调整：初始化 Redis、注入 Hub 到 msgHandler |
| `config/config.yaml` | Redis 配置已存在，无需改动 |
| `migrations/009_*` (新) | 可选：消息表索引优化 |

### 前端
| 文件 | 改动 |
|------|------|
| `store/wsStore.ts` | 处理 `message.complete` 推送，自动 addMessage |
| `api/message.ts` | 新增离线消息拉取 API |
| `hooks/useMessages.ts` | 连接时拉取离线消息 |

## 验收标准
1. HTTP 发消息 → 所有在线成员实时收到 WS 推送
2. WS chat 消息 → 持久化到 DB + 转发给房间成员
3. 用户离线期间收到的消息，上线后能通过 API 拉取
4. 消息发送的 DB 事务和推送解耦，推送失败不丢消息
5. Redis 缓存热数据，减少 DB 查询
