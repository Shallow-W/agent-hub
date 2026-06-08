# conversation blackboard unify

## Goal

将上一阶段的“群聊上下文黑板”统一为“会话级上下文黑板”：群聊、单聊、Agent 单聊都支持 pin 消息作为长期上下文；每次 Agent 对话时都注入黑板内容。黑板继续分成两部分：用户 Pin 上下文和用户手写上下文。第二部分暂不由 Agent 写入，只允许用户手动编辑。

## What I already know

* 当前 `message_pins` 已经按 `conversation_id + message_id` 存储，后端权限逻辑本身不限制群聊，可以复用到单聊。
* 当前 prompt block 名为 `{群聊上下文黑板}`，需要改成更通用的 `{会话上下文黑板}`。
* 当前第二部分是 `{群聊/任务状态摘要\n未启用}`，需要改成用户可写的手动上下文。

## Requirements

* Pin/Unpin 对所有会话类型可用，包括群聊、普通单聊和 Agent 单聊。
* 新增会话级手写上下文，用户可以读取和保存。
* Agent prompt 中注入 `{会话上下文黑板}`，包含：
  * `{用户 Pin 上下文}`：来自 pinned messages。
  * `{用户手写上下文}`：来自用户手动编辑内容。
* 注入范围覆盖 orchestrator、worker dispatch、直接 @Agent、Agent 单聊。
* 手写上下文限制长度，避免挤爆 prompt。
* 前端提供所有会话通用的黑板入口，展示/编辑用户手写上下文；消息 pin 菜单继续复用。

## Acceptance Criteria

* [x] 单聊消息可以 pin/unpin，history 中 `pinned` 状态正确。
* [x] 用户可以读取/保存会话手写上下文。
* [x] Orchestrator prompt 使用 `{会话上下文黑板}` 并包含用户手写上下文。
* [x] Worker dispatch prompt 包含会话黑板。
* [x] 直接 @Agent 和 Agent 单聊 prompt 包含会话黑板。
* [x] 后端测试、前端 build、真实 API smoke 通过。

## Out of Scope

* 不实现 Agent/Orchestrator 自动写入黑板。
* 不实现黑板历史版本、多人协同编辑、富文本编辑。
* 不做复杂黑板侧栏信息架构，第一版只提供基础编辑入口。

## Technical Notes

* 重点检查：message pin API、orchestrator prompt 构造、Agent 单聊上下文构造、ChatWindow 顶部操作区。
* 已验证：`go test ./...`、`npm run build`、真实临时后端 `GET/PUT /api/conversations/:id/blackboard` smoke。
