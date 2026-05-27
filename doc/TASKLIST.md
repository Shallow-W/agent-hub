# AgentHub 任务列表

> 简略索引。每个任务的详细要求、子任务和验收标准见 `doc/task/` 下对应文件。

## 状态说明

`[ ]` 待开始 | `[~]` 进行中 | `[x]` 已完成 | `[-]` 取消

---

## P0

| # | 任务 | 详情 | 依赖 | 状态 |
|---|------|------|------|------|
| M0 | 项目基础设施（脚手架 + 数据库 + API规范） | [doc/task/M0-基础设施.md](task/M0-基础设施.md) | 无 | [x] |
| M1 | 用户鉴权（注册/登录/JWT） | [doc/task/M1-用户鉴权.md](task/M1-用户鉴权.md) | M0 | [x] |
| M2 | WebSocket 通信基础设施 | [doc/task/M2-WebSocket通信.md](task/M2-WebSocket通信.md) | M0, M1 | [x] |
| M3 | IM 聊天核心（对话列表 + 聊天窗口 + 流式消息） | [doc/task/M3-IM聊天核心.md](task/M3-IM聊天核心.md) | M1, M2 | [x] |
| M4 | 多 Agent 接入（守护进程 + 适配器 + CLI通信） | [doc/task/M4-多Agent接入.md](task/M4-多Agent接入.md) | M0, M2 | [ ] |
| M5 | Orchestrator（意图拆解 + 并行调度 + 聚合） | [doc/task/M5-Orchestrator.md](task/M5-Orchestrator.md) | M4 | [ ] |
| M6 | 单聊端到端跑通 | [doc/task/M6-单聊跑通.md](task/M6-单聊跑通.md) | M3, M4 | [ ] |
| M7 | 群聊端到端跑通 | [doc/task/M7-群聊跑通.md](task/M7-群聊跑通.md) | M5, M6 | [ ] |
| M8 | 自建 Agent（选CLI + 编写Prompt） | [doc/task/M8-自建Agent.md](task/M8-自建Agent.md) | M4, M6 | [ ] |

## P1

| # | 任务 | 详情 | 依赖 | 状态 |
|---|------|------|------|------|
| M9 | 产物预览（结构化卡片：代码/网页/文件） | [doc/task/M9-产物预览.md](task/M9-产物预览.md) | M6 | [ ] |
| M10 | Pin 消息上下文（Pin注入Agent请求） | [doc/task/M10-Pin上下文.md](task/M10-Pin上下文.md) | M3, M4 | [ ] |

---

## Bug 修复

| # | Bug | 严重度 | 状态 |
|---|-----|--------|------|
| B01 | 消息撤回无权限校验——任何人可撤回别人的消息 | P0 | [x] |
| B02 | 已读标记不生效——markAsRead 后仍返回已读消息 | P0 | [ ] |
| B03 | 创建私聊给非好友返回 500 | P1 | [ ] |
| B04 | 创建私聊给自己返回 500 | P1 | [ ] |
| B05 | reply_to 不存在的消息返回 500 | P1 | [x] |
| B06 | 附件数据未正确保存 | P2 | [ ] |
| B07 | conversations 端点创建的群聊无成员记录 | P2 | [ ] |
| B08 | 消息无长度限制 | P3 | [x] |
| B09 | 用户搜索返回自己 | P3 | [ ] |
| B10 | togglePin API 方法不匹配(PUT vs POST) | P0 | [ ] |
| B11 | API 返回 data:null 导致前端崩溃 | P0 | [x] |
| B12 | 回复功能 UI 存在但数据未发送 | P1 | [x] |
| B13 | 消息撤回前端完全缺失 | P1 | [x] |
| B14 | JWT 过期后会话永久损坏 | P1 | [ ] |
| B15 | typingUsers 显示 UUID 而非用户名 | P1 | [x] |
| B16 | retryOptimistic 丢弃附件 | P2 | [ ] |
| B17 | deleteConversation 无错误处理 | P2 | [ ] |
| B18 | fetchMessages 竞态条件 | P2 | [ ] |
| B19 | ProtectedRoute 不响应式 | P3 | [ ] |
| B20 | 重复 token 管理 | P3 | [ ] |
| B21 | 无效 UUID 路径参数返回 500 | P1 | [ ] |
| B22 | 消息撤回后 Redis 缓存未失效 | P1 | [x] |
| B23 | reply_to_message 字段始终为 null | P2 | [x] |
| B24 | 对话列表缺少 members_count | P2 | [x] |
| B25 | 私聊对话缺少 peer_id | P2 | [ ] |
| B26 | 私聊 user_id 始终是创建者 ID | P2 | [ ] |
| B27 | 消息内容和群名未转义 HTML/JS | P2 | [ ] |
| B28 | 好友请求发给不存在用户返回 500 | P3 | [ ] |
| B29 | 消息历史 limit 负数未处理 | P3 | [ ] |
| B30 | 不存在的群聊 UUID 返回 403 非 404 | P3 | [ ] |
| B31 | leave_room 后仍通过成员推送收到消息 | P3 | [ ] |
| B32 | 群成员数量硬编码为 9 | P1 | [x] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 安全问题

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| SEC-01 | 用户名缺少白名单校验(XSS/特殊字符) | P1 | [ ] |
| SEC-02 | 消息 content 纯空格被接受 | P2 | [ ] |
| SEC-03 | 群名纯空格通过 binding 校验 | P2 | [ ] |
| SEC-04 | 上传文件名 XSS 字符未净化 | P3 | [ ] |
| SEC-05 | limit 参数无上界/无正数校验 | P3 | [ ] |
| SEC-06 | 用户搜索接口缺少独立限流 | P3 | [ ] |

---

## 前端 UI 问题

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| UI-01 | WebSocket 主动断开后仍自动重连 | P0 | [ ] |
| UI-02 | 已登录用户访问 /login 不跳转 | P1 | [ ] |
| UI-03 | 展开输入框按钮无功能 | P1 | [ ] |
| UI-04 | 搜索结果高亮效果不可见 | P1 | [x] |
| UI-05 | 无对话级别 URL，刷新丢状态 | P2 | [ ] |
| UI-06 | 页面刷新后恢复到空状态 | P2 | [ ] |
| UI-07 | 多处硬编码颜色不跟随主题 | P2 | [ ] |
| UI-08 | AuthLayout 固定宽度窄屏溢出 | P2 | [ ] |
| UI-09 | 多处内联 style 无法被暗色主题覆盖 | P2 | [ ] |
| UI-10 | WebSocket 重连无 jitter 惊群风险 | P2 | [ ] |
| UI-11 | 断线期间发送队列不持久化 | P2 | [ ] |
| UI-12 | 无键盘 focus-visible 样式 | P3 | [ ] |
| UI-13 | GroupMemberPanel 缺少 aria-label | P3 | [ ] |
| UI-14 | "文件"和"停止任务"按钮无功能 | P3 | [ ] |
| UI-15 | friendStore accept/reject 缺少 loading | P3 | [ ] |
| UI-16 | hasMore 翻页边界判断不准 | P3 | [ ] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 后端代码质量

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| CODE-01 | rate limiter goroutine 永不退出(泄漏) | P0 | [ ] |
| CODE-02 | Hub.clients 值 (*[]*Client) 并发竞争 | P1 | [ ] |
| CODE-03 | WebSocket 连接 ctx 未绑定 hub 生命周期 | P1 | [ ] |
| CODE-04 | Hub bus channel 发送可无限阻塞 handler | P1 | [ ] |
| CODE-05 | config.yaml 缺 upload 和 redis.db 字段 | P1 | [ ] |
| CODE-06 | Client.LastActive 无同步并发读写 | P1 | [ ] |
| CODE-07 | createDatabase 数据库名通过 Sprintf 拼接 | P2 | [ ] |
| CODE-08 | 多语句迁移无事务包裹 | P2 | [ ] |
| CODE-09 | WS chat handler 忽略 SendMessage 错误 | P2 | [ ] |
| CODE-10 | WS readLoop JSON 解组错误被静默吞噬 | P2 | [ ] |
| CODE-11 | ListMemberIDs 不包含会话所有者(通知遗漏) | P2 | [ ] |
| CODE-12 | fillReplyTo 后独立查询用户名(N+1) | P2 | [ ] |
| CODE-13 | 静态文件服务缺少路径边界检查 | P2 | [ ] |
| CODE-14 | postPersist 异步推送无重试/死信队列 | P2 | [ ] |
| CODE-15 | config.example 缺 upload 和 redis.db 字段 | P2 | [ ] |
| CODE-16 | 无单用户 WebSocket 连接数限制(DoS风险) | P2 | [ ] |
| CODE-17 | Hub Register/Unregister 异步竞态 | P2 | [ ] |
| CODE-18 | Client.enqueue 背压时可能阻塞 dispatch | P3 | [ ] |
| CODE-19 | 迁移 006 缺少 DOWN 部分 | P3 | [ ] |
| CODE-20 | group handler 错误码 40300 被多个错误复用 | P3 | [ ] |
| CODE-21 | Redis 客户端未在 shutdown 时 Close | P3 | [ ] |
| CODE-22 | RecallMessage 将 DB 错误误报为"消息不存在" | P3 | [ ] |

---

## 前端构建/规范

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| BUILD-01 | 缺少 ESLint 配置 | P3 | [ ] |
| BUILD-02 | vendor-antd chunk 超 500 kB | P2 | [ ] |
| BUILD-03 | 多个依赖有 Major 版本更新 | P3 | [ ] |
| BUILD-04 | screenshot.mjs 幽灵 playwright 依赖 | P3 | [ ] |
| BUILD-05 | 无 .env 环境文件 | P3 | [ ] |
| BUILD-06 | useWebSocket.ts TODO 残留 | P3 | [ ] |
| BUILD-07 | 大量内联 style 违反编码规范(40+处) | P1 | [ ] |
| BUILD-08 | messageStore.ts 超 300 行限制 | P2 | [ ] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 前端缺失功能

### P0 — 核心功能缺失

| ID | 功能 | 后端状态 | 状态 |
|----|------|----------|------|
| MISS-001 | 群聊重命名 UI | 已有 API | [ ] |
| MISS-002 | 个人资料编辑/展示 | 需新增 API | [ ] |
| MISS-003 | 设置页面实现 | 大部分前端本地 | [ ] |

### P1 — 重要功能缺失

| ID | 功能 | 后端状态 | 状态 |
|----|------|----------|------|
| MISS-004 | 群成员角色管理 UI | 部分已有 | [ ] |
| MISS-005 | 好友删除 | 需新增 API | [ ] |
| MISS-006 | /api/users/search 对接 | 已有 API | [ ] |
| MISS-007 | 归档对话列表/查看 | 需新增 API | [ ] |
| MISS-008 | GetGroupInfo 对接 | 已有 API | [ ] |
| MISS-009 | 消息转发 | 需新增 API | [ ] |
| MISS-010 | @提及功能 | 需新增 API | [ ] |
| MISS-011 | 消息已读回执展示 | 部分已有 | [ ] |

### P2 — 次要功能缺失

| ID | 功能 | 后端状态 | 状态 |
|----|------|----------|------|
| MISS-012 | 群公告/群描述 | 需新增字段 | [ ] |
| MISS-013 | 群头像设置 | 需新增字段 | [ ] |
| MISS-014 | 转让群主 | 需新增 API | [ ] |
| MISS-015 | 好友备注名 | 需新增字段 | [ ] |
| MISS-016 | 好友分组/黑名单 | 部分准备 | [ ] |
| MISS-017 | 好友申请撤回 | 需新增 API | [ ] |
| MISS-018 | 对话分组/标签 | 需新增模型 | [ ] |
| MISS-019 | 全局消息搜索 | 需新增 API | [ ] |
| MISS-020 | 声音/震动通知设置 | 纯前端 | [ ] |
| MISS-021 | 浏览器推送通知 | 纯前端 | [ ] |
| MISS-022 | 对话列表搜索/过滤 | 已有参数 | [ ] |
| MISS-023 | 群组搜索 | 纯前端 | [ ] |
| MISS-024 | 语言设置(i18n) | 纯前端 | [ ] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 关键路径

```
M0 → M1 → M2 → M3 ──→ M6(单聊跑通) → M7(群聊跑通) → M8(自建Agent)
           ↓     ↘                                    ↘
           M4 ────→ M5 ──────────────────────────→ M7       M9(产物预览)
                                                           M10(Pin上下文)
```

**最短跑通**：M0 → M1 → M2 → M3 → M4 → M6
