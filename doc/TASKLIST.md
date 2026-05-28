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
| B02 | 已读标记不生效——markAsRead 后仍返回已读消息 | P0 | [x] |
| B03 | 创建私聊给非好友返回 500 | P1 | [x] |
| B04 | 创建私聊给自己返回 500 | P1 | [x] |
| B05 | reply_to 不存在的消息返回 500 | P1 | [x] |
| B06 | 附件数据未正确保存 | P2 | [x] |
| B07 | conversations 端点创建的群聊无成员记录 | P2 | [x] |
| B08 | 消息无长度限制 | P3 | [x] |
| B09 | 用户搜索返回自己 | P3 | [x] |
| B10 | togglePin API 方法不匹配(PUT vs POST) | P0 | [x] |
| B11 | API 返回 data:null 导致前端崩溃 | P0 | [x] |
| B12 | 回复功能 UI 存在但数据未发送 | P1 | [x] |
| B13 | 消息撤回前端完全缺失 | P1 | [x] |
| B14 | JWT 过期后会话永久损坏 | P1 | [x] |
| B15 | typingUsers 显示 UUID 而非用户名 | P1 | [x] |
| B16 | retryOptimistic 丢弃附件 | P2 | [x] |
| B17 | deleteConversation 无错误处理 | P2 | [x] |
| B18 | fetchMessages 竞态条件 | P2 | [x] |
| B19 | ProtectedRoute 不响应式 | P3 | [x] |
| B20 | 重复 token 管理 | P3 | [x] |
| B21 | 无效 UUID 路径参数返回 500 | P1 | [x] |
| B22 | 消息撤回后 Redis 缓存未失效 | P1 | [x] |
| B23 | reply_to_message 字段始终为 null | P2 | [x] |
| B24 | 对话列表缺少 members_count | P2 | [x] |
| B25 | 私聊对话缺少 peer_id | P2 | [x] |
| B26 | 私聊 user_id 始终是创建者 ID | P2 | [x] |
| B27 | 消息内容和群名未转义 HTML/JS | P2 | [x] |
| B28 | 好友请求发给不存在用户返回 500 | P3 | [x] |
| B29 | 消息历史 limit 负数未处理 | P3 | [x] |
| B30 | 不存在的群聊 UUID 返回 403 非 404 | P3 | [x] |
| B31 | leave_room 后仍通过成员推送收到消息 | P3 | [-] |
| B32 | 群成员数量硬编码为 9 | P1 | [x] |
| B33 | WebSocket 发消息静默丢弃 DB 持久化错误 | P1 | [x] |
| B34 | 负 offset 值直接传 SQL 无校验 | P2 | [x] |
| B35 | conversation 服务创建群聊非原子(补偿删除可失败) | P1 | [-] |
| B36 | GetUnreadMessages 降级查询返回全部消息而非未读 | P2 | [x] |
| B37 | 删除私聊后用户仍可通过 conv.UserID 绕过发消息 | P2 | [x] |
| B38 | 多个并发 401 导致重复 token 清除+重定向风暴 | P2 | [x] |
| B41 | B38 handling401 标志永不重置，二次 401 静默丢失 | P2 | [x] |
| B42 | WS chat 类型畸形 JSON 静默丢弃无错误反馈 | P2 | [x] |
| B43 | ConversationList noResults 分支不可达(死代码) | P3 | [ ] |
| B44 | 好友请求只单向检查，允许双向重复请求 | P1 | [x] |
| B45 | 归档私聊后对方 GetOrCreatePrivateChat 创建重复对话 | P1 | [x] |
| B46 | ListMemberIDs UNION 返回重复 userID，群主收到重复消息 | P1 | [x] |
| B47 | 私聊非创建者无法归档(只检查 conv.UserID) | P1 | [x] |
| B48 | WS join_room 用 GroupRepo.IsMember 但 REST checkMembership 有 fallback | P1 | [x] |
| B49 | 群创建 owner INSERT 无 ON CONFLICT 非幂等 | P1 | [x] |
| B50 | username 校验 max 冲突：binding 50 vs regex 20 | P2 | [x] |
| B51 | typing 通知广播包含发送者自己(多余流量) | P2 | [x] |
| B52 | RecallMessage 对无 sender_id 的群聊历史消息推断错误 | P2 | [x] |
| B53 | user.stop_stream 前端发送但后端无处理(stop按钮无效) | P1 | [x] |
| B54 | friendStore actionLoading 值不匹配:id vs id+'-accept'(loading永远不显示) | P1 | [x] |
| B55 | ChatWindow 文件上传后不发送附件消息(上传结果丢失) | P1 | [ ] |
| B56 | friendStore accept/reject 成功后不清除 error 状态(旧错误残留) | P2 | [x] |
| B57 | useMessages 缓存 30s 不感知 WS 断连期间丢失的消息 | P2 | [x] |
| B58 | Hub shutdown/handleUnregister 重复 close(sendCh)——panic | P1 | [x] |
| B59 | shutdown bus 满载时 Unregister 丢弃——goroutine+连接泄漏 | P1 | [x] |
| B60 | drain 窗口 wg.Add 无 wg.Done——WaitGroup panic | P1 | [ ] |
| B61 | 背压二次写入无 select/default——dispatch 永久阻塞 | P2 | [x] |
| B62 | WS chat 验证 DB 成员非房间成员——join_room 非强制 | P2 | [-] |
| B63 | 无 refresh token——JWT 过期强制重新登录 | P2 | [ ] |
| B64 | ValidateToken 不校验 user_id 是否存在于 DB——删除用户 token 仍有效 | P2 | [x] |
| B65 | middleware+service 重复 JWT 解析逻辑——有分歧风险 | P2 | [ ] |
| B66 | SearchByContent 内联 escapeLike 未复用共享函数 | P3 | [x] |
| B67 | ILIKE ESCAPE 在部分 PostgreSQL 配置下可能失败 | P2 | [x] |
| B68 | 限流器 c.ClientIP() 信任 X-Forwarded-For——可伪造绕过+StopRateLimiters空实现 | P1 | [x] |
| B69 | 限流仅 IP 粒度——NAT 后多用户共享配额 | P2 | [ ] |
| B70 | MaxBytesReader 硬编码 50MB——超过 20MB 的图片仍完整写入磁盘 | P2 | [x] |
| B71 | 静态文件 filepath.Clean 不充分——路径穿越 | P2 | [x] |
| B72 | MIME 检测基于 512 字节——polyglot 文件绕过 | P3 | [ ] |
| B73 | Auth middleware username claim 未做类型断言——可能存入 nil | P2 | [x] |
| B74 | Upload FileSize 用客户端值 fileHeader.Size 而非实际磁盘大小 | P2 | [x] |
| B75 | authStore login/register 不调用 setToken——刷新后 token 丢失 | P1 | [x] |
| B76 | WS 重连后不恢复房间订阅——断线期间消息静默丢失 | P1 | [x] |
| B77 | GroupMemberPanel handleAddUser 无防重复点击 | P1 | [x] |
| B78 | FriendList handleAddFriend 无防重复点击——重复好友请求 | P1 | [x] |
| B79 | SettingsPanel 主题切换按钮无实际功能 | P1 | [ ] |
| B80 | GroupInfoDrawer info.conversation 可能 undefined 渲染崩溃 | P2 | [x] |
| B81 | ROLE_LABELS/ROLE_COLORS 对未知 role 显示 undefined | P2 | [x] |
| B82 | GroupMemberPanel memberIds 闭包每次渲染重建——debounce 失效 | P2 | [x] |
| B83 | FriendRequest formatTime 对无效日期返回 NaN | P2 | [x] |
| B84 | FriendRequest sendRequest loading 复用全局 loading——UI 误判 | P2 | [x] |
| B85 | WS flushQueue 期间断开——消息顺序错乱 | P2 | [x] |
| B86 | 多 tab 打开 WS 状态不同步 | P3 | [ ] |
| B87 | globals.css * 选择器覆盖所有元素滚动条样式 | P3 | [-] |
| B88 | GroupMemberPanel 退出群聊后不清除成员列表 | P3 | [x] |
| B89 | Friend 与 FriendRequest 类型字段完全重复 | P3 | [ ] |
| B39 | 归档对话错误触发 delete API(双重请求) | P1 | [x] |
| B40 | upload.ts JSON 解析无 try/catch(非 JSON 响应崩溃) | P2 | [x] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 安全问题

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| SEC-01 | 用户名缺少白名单校验(XSS/特殊字符) | P1 | [x] |
| SEC-02 | 消息 content 纯空格被接受 | P2 | [x] |
| SEC-03 | 群名纯空格通过 binding 校验 | P2 | [x] |
| SEC-04 | 上传文件名 XSS 字符未净化 | P3 | [x] |
| SEC-05 | limit 参数无上界/无正数校验 | P3 | [x] |
| SEC-06 | 用户搜索接口缺少独立限流 | P3 | [x] |
| SEC-07 | WebSocket JWT 通过 query string 传入被日志明文记录 | P2 | [x] |
| SEC-08 | SendFriendRequest 的 friend_id 未校验 UUID 格式 | P2 | [x] |
| SEC-09 | CreateGroup 的 member_ids 未逐个校验 UUID 格式 | P2 | [x] |

---

## 前端 UI 问题

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| UI-01 | WebSocket 主动断开后仍自动重连 | P0 | [x] |
| UI-02 | 已登录用户访问 /login 不跳转 | P1 | [x] |
| UI-03 | 展开输入框按钮无功能 | P1 | [x] |
| UI-04 | 搜索结果高亮效果不可见 | P1 | [x] |
| UI-05 | 无对话级别 URL，刷新丢状态 | P2 | [x] |
| UI-06 | 页面刷新后恢复到空状态 | P2 | [x] |
| UI-07 | 多处硬编码颜色不跟随主题 | P2 | [ ] |
| UI-08 | AuthLayout 固定宽度窄屏溢出 | P2 | [x] |
| UI-09 | 多处内联 style 无法被暗色主题覆盖 | P2 | [ ] |
| UI-10 | WebSocket 重连无 jitter 惊群风险 | P2 | [x] |
| UI-11 | 断线期间发送队列不持久化 | P2 | [ ] |
| UI-12 | 无键盘 focus-visible 样式 | P3 | [x] |
| UI-13 | GroupMemberPanel 缺少 aria-label | P3 | [ ] |
| UI-14 | "文件"和"停止任务"按钮无功能 | P3 | [x] |
| UI-15 | friendStore accept/reject 缺少 loading | P3 | [x] |
| UI-16 | hasMore 翻页边界判断不准 | P3 | [ ] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 后端代码质量

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| CODE-01 | rate limiter goroutine 永不退出(泄漏) | P0 | [x] |
| CODE-02 | Hub.clients 值 (*[]*Client) 并发竞争 | P1 | [x] |
| CODE-03 | WebSocket 连接 ctx 未绑定 hub 生命周期 | P1 | [-] |
| CODE-04 | Hub bus channel 发送可无限阻塞 handler | P1 | [-] |
| CODE-05 | config.yaml 缺 upload 和 redis.db 字段 | P1 | [x] |
| CODE-06 | Client.LastActive 无同步并发读写 | P1 | [x] |
| CODE-07 | createDatabase 数据库名通过 Sprintf 拼接 | P2 | [x] |
| CODE-08 | 多语句迁移无事务包裹 | P2 | [x] |
| CODE-09 | WS chat handler 忽略 SendMessage 错误 | P2 | [x] |
| CODE-10 | WS readLoop JSON 解组错误被静默吞噬 | P2 | [x] |
| CODE-11 | ListMemberIDs 不包含会话所有者(通知遗漏) | P2 | [x] |
| CODE-12 | fillReplyTo 后独立查询用户名(N+1) | P2 | [-] |
| CODE-13 | 静态文件服务缺少路径边界检查 | P2 | [x] |
| CODE-14 | postPersist 异步推送无重试/死信队列 | P2 | [ ] |
| CODE-15 | config.example 缺 upload 和 redis.db 字段 | P2 | [x] |
| CODE-16 | 无单用户 WebSocket 连接数限制(DoS风险) | P2 | [x] |
| CODE-17 | Hub Register/Unregister 异步竞态 | P2 | [-] |
| CODE-18 | Client.enqueue 背压时可能阻塞 dispatch | P3 | [ ] |
| CODE-19 | 迁移 006 缺少 DOWN 部分 | P3 | [x] |
| CODE-20 | group handler 错误码 40300 被多个错误复用 | P3 | [x] |
| CODE-21 | Redis 客户端未在 shutdown 时 Close | P3 | [x] |
| CODE-22 | RecallMessage 将 DB 错误误报为"消息不存在" | P3 | [x] |
| CODE-23 | RemoveMember 错误未包装为 sentinel，handler 降级 500 | P2 | [-] |
| CODE-24 | schema_migrations 表创建错误被忽略(迁移全部跳过) | P2 | [x] |
| CODE-25 | postPersist goroutine 与 Hub shutdown 竞态 | P2 | [x] |
| CODE-26 | escape 逻辑在 message/friend repo 重复实现 | P3 | [x] |
| CODE-27 | SetNotifier/SetCacher 无同步保护 | P3 | [-] |
| CODE-28 | member_count 在 list 与 get-by-id 端点不一致 | P3 | [ ] |
| CODE-29 | 错误码 40030/40031 跨 handler 重复(不同语义) | P3 | [ ] |
| CODE-30 | config 零值无校验(JWT secret 空/port=0 直接运行) | P1 | [x] |
| CODE-31 | go-redis/imaging 标记为 indirect 但实际直接导入 | P2 | [x] |
| CODE-32 | go.mod 含幽灵 mongo-driver 依赖 | P3 | [x] |
| CODE-33 | 整个项目零测试覆盖——后端无 _test.go、前端无 .test/.spec 文件 | P0 | [ ] |

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
| BUILD-09 | ChatInput 跨组件导入 EmojiPicker.module.css | P3 | [ ] |
| BUILD-10 | package.json 含冗余 playwright 依赖 | P3 | [ ] |

---

## 前端运行时问题

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| RT-01 | Conversation 类型缺 archived_at 字段 | P2 | [x] |
| RT-02 | createGroup 返回字段名不匹配(id vs conversation_id) | P1 | [x] |
| RT-03 | accept/reject 好友请求后端返回 null，前端类型错误 | P2 | [x] |
| RT-04 | 缺少 ErrorBoundary——渲染错误全页白屏 | P1 | [x] |
| RT-05 | 网络错误无全局 toast 通知 | P2 | [x] |
| RT-06 | WS error 类型消息只 console.error 无用户提示 | P2 | [x] |
| RT-07 | MessageList 缺少虚拟滚动，长对话卡顿 | P2 | [ ] |
| RT-08 | renderMarkdown 每次渲染重计算无 memoize | P2 | [x] |
| RT-09 | 缺少 404 路由兜底 | P2 | [x] |
| RT-10 | 前端未使用分页参数，对话超20条不可见 | P1 | [x] |

---

## 部署就绪度

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| DEPLOY-01 | 无 CI/CD 配置 | P1 | [ ] |
| DEPLOY-02 | 无 Dockerfile | P1 | [ ] |
| DEPLOY-03 | 无 README.md | P1 | [ ] |
| DEPLOY-04 | go.mod 声明不存在的 Go 1.26.3 | P1 | [x] |
| DEPLOY-05 | docker-compose 缺 Redis 服务 | P2 | [ ] |
| DEPLOY-06 | config.example 与实际 config 不同步 | P2 | [ ] |
| DEPLOY-07 | JWT secret 无强制校验(默认弱密钥) | P2 | [ ] |
| DEPLOY-08 | 无 metrics/Prometheus 端点 | P2 | [ ] |
| DEPLOY-09 | 日志级别硬编码不可配置 | P2 | [ ] |
| DEPLOY-10 | Rate limit 参数硬编码不可配置 | P2 | [ ] |
| DEPLOY-11 | 无 HTTPS/TLS 配置 | P2 | [ ] |
| DEPLOY-12 | daemon 代码为空壳 placeholder | P2 | [ ] |
| DEPLOY-13 | 健康检查不验证 DB/Redis 连接状态 | P3 | [ ] |

---

## 数据库 Schema

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| DB-01 | messages.sender_id 可空，部分消息无发送者 | P2 | [ ] |
| DB-02 | 迁移 006 创建重复索引(002/004/005 已创建) | P3 | [ ] |
| DB-03 | conversation_members.last_read_at 无索引 | P2 | [x] |
| DB-04 | 迁移 012 sender_id backfill 仅覆盖 user 角色 | P2 | [ ] |
| DB-05 | ListByUserID 热查询缺 archived_at 索引 | P2 | [x] |
| DB-06 | conversations.type 无 CHECK 约束 | P2 | [x] |
| DB-07 | friends.status 无 CHECK 约束 | P2 | [x] |
| DB-08 | CASCADE 删除用户时销毁群聊(应 SET NULL) | P1 | [ ] |
| DB-09 | 可空 DB 列映射为非指针 Go 类型(StructScan 崩溃) | P1 | [ ] |
| DB-10 | ANY($1)+[]string 在 sqlx 下可能运行时失败 | P2 | [x] |
| DB-11 | GroupRepo.AddMember 缺 ON CONFLICT 幂等保护 | P2 | [x] |
| DB-12 | 仓库方法重复且行为不一致(AddMember/GetUserByID) | P3 | [ ] |
| DB-13 | user.go 用 err==sql.ErrNoRows 而非 errors.Is | P3 | [x] |

> 详情: [doc/task/Bugfix-测试发现的Bug.md](task/Bugfix-测试发现的Bug.md)

---

## 前端缺失功能

### P0 — 核心功能缺失

| ID | 功能 | 后端状态 | 状态 |
|----|------|----------|------|
| MISS-001 | 群聊重命名 UI | 已有 API | [x] |
| MISS-002 | 个人资料编辑/展示 | 已有 API(placeholder UI) | [ ] |
| MISS-003 | 设置页面实现 | 大部分前端本地(主题按钮无功能) | [ ] |

### P1 — 重要功能缺失

| ID | 功能 | 后端状态 | 状态 |
|----|------|----------|------|
| MISS-004 | 群成员角色管理 UI | 部分已有 | [ ] |
| MISS-005 | 好友删除 | 需新增 API | [ ] |
| MISS-006 | /api/users/search 对接 | 已有 API | [x] |
| MISS-007 | 归档对话列表/查看 | 已有 API | [x] |
| MISS-008 | GetGroupInfo 对接 | 已有 API | [x] |
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

## 文档准确性

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| DOC-01 | API 文档覆盖率仅 26%(31端点仅8个有文档) | P1 | [ ] |
| DOC-02 | 错误响应格式文档与代码不一致(嵌套vs扁平) | P1 | [ ] |
| DOC-03 | WebSocket 消息类型命名文档与代码不匹配 | P1 | [ ] |
| DOC-04 | Conversation type 文档 "direct" vs 代码 "single" | P2 | [ ] |
| DOC-05 | username 校验规则三处冲突(min=2 vs 3) | P2 | [x] |
| DOC-06 | ErrGroupNotFound 可能未定义 | P2 | [x] |
| DOC-07 | ConversationMember.JoinedAt 类型 string 应为 time.Time | P2 | [ ] |
| DOC-08 | ConversationMember 缺少 last_read_at 字段 | P2 | [ ] |
| DOC-09 | service/message.go 超 300 行限制(402行) | P3 | [ ] |
| DOC-10 | Commit message 格式违反 50 字符限制 | P3 | [ ] |

---

## 前端性能

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| PERF-01 | messages store 无限增长，永不清理 | P1 | [x] |
| PERF-02 | AppLayout 订阅整个 unreadCounts 对象 | P1 | [x] |
| PERF-03 | ChatWindow 订阅全部 typingUsers | P1 | [x] |
| PERF-04 | MessageBubble 未使用 React.memo | P2 | [x] |
| PERF-05 | smooth scroll 在流式消息时引起抖动 | P2 | [x] |
| PERF-06 | 对话切换重复 API 请求 | P2 | [x] |
| PERF-07 | transition:all 导致布局抖动 | P2 | [x] |
| PERF-08 | renderMarkdown 无缓存(useMemo) | P2 | [x] |
| PERF-09 | conversationStore 订阅粒度过粗 | P3 | [ ] |
| PERF-10 | friendStore 订阅全量 friends 数组 | P3 | [ ] |
| PERF-11 | 未使用 React.lazy 懒加载 | P3 | [ ] |
| PERF-12 | WebSocket 消息处理未做 batching | P3 | [ ] |

---

## 前端状态/逻辑问题（第七轮）

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| FE-01 | useWebSocket 中 currentUserId 闭包过期导致 typing 判断错误 | P1 | [x] |
| FE-02 | archive handler 错误调用 deleteConversation(归档变删除) | P1 | [x] |
| FE-03 | upload.ts 绕过共享 request()函数，缺 retry/错误标准化 | P2 | [x] |
| FE-04 | conversationStore.togglePin/createConversation 无错误处理 | P2 | [x] |
| FE-05 | retryOptimistic 中 attachments 类型强转隐藏类型不匹配 | P2 | [x] |
| FE-06 | useConversation 每次挂载触发重复 API 调用(无去重) | P2 | [x] |
| FE-07 | 无 AbortController 取消机制，请求不可中断 | P2 | [ ] |
| FE-08 | SettingsPanel 内部 selectedKey 不同步外部导航变化 | P3 | [ ] |
| FE-09 | friendStore 共享 loading 标志导致状态不一致 | P3 | [ ] |
| FE-10 | ResizeHandle 组件卸载时拖拽事件监听泄漏 | P3 | [x] |
| FE-11 | messageStore.recall 动态 import antd 可掩盖原始错误 | P3 | [ ] |

---

## 前端 UX 问题（第十轮）

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| UX-01 | 删除对话无二次确认弹窗，误点即删 | P1 | [x] |
| UX-02 | 创建群组后群不自动激活，需手动点击 | P1 | [x] |
| UX-03 | 切换对话无"新消息"指示器/跳转按钮 | P1 | [x] |
| UX-04 | 无响应式设计，移动端布局不可用 | P1 | [ ] |
| UX-05 | 快速切换对话时 fetchMessages 竞态（旧请求覆盖新数据） | P1 | [x] |
| UX-06 | 新建对话默认标题"新对话"硬编码，无输入框 | P2 | [ ] |
| UX-07 | 对话列表无空状态引导（新用户不知如何开始） | P2 | [ ] |
| UX-08 | 发送按钮无 loading 状态，重复点击可触发多次发送 | P2 | [x] |
| UX-09 | 群聊创建后不自动打开成员面板 | P2 | [x] |
| UX-10 | 好友申请无备注/留言字段 | P2 | [ ] |
| UX-11 | 消息时间戳仅显示时间不显示日期（跨天消息混乱） | P2 | [ ] |
| UX-12 | 输入框不支持 Shift+Enter 换行 | P2 | [x] |
| UX-13 | 消息列表不支持键盘快捷键（Esc关闭面板等） | P2 | [ ] |
| UX-14 | 长消息无折叠/展开功能 | P2 | [ ] |
| UX-15 | 无消息搜索功能（对话内搜索） | P2 | [ ] |
| UX-16 | 右键菜单仅显示"撤回"，缺少"复制""转发"等常见操作 | P3 | [ ] |
| UX-17 | 未读消息数 badge 超 99 无特殊显示（如 99+） | P3 | [ ] |
| UX-18 | 对话列表项无 hover 预览（最后一条消息摘要） | P3 | [ ] |
| UX-19 | 无消息发送失败的全局重试提示 | P3 | [ ] |
| UX-20 | Emoji 选择器无最近使用/常用分类 | P3 | [ ] |

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
