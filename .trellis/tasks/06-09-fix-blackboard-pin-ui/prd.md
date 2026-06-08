# fix blackboard pin ui

## Goal

修复上下文黑板第一版的可用性问题：用户现在很难发现消息 pin 入口，且黑板弹窗没有清楚分成“Pin 的消息”和“自由手写上下文”两部分。本任务补齐 UI，让 pin 和黑板结构可见、可操作。

## What I already know

* 后端已有 `POST/DELETE /api/conversations/:id/messages/:messageId/pin`。
* 前端目前只有右键菜单能 pin，用户不容易发现。
* 黑板弹窗目前只展示 `manual_context` 文本框，没有展示 pinned messages。
* `GET /api/conversations/:id/pinned-context` 已返回 pinned message 列表。

## Requirements

* 每条普通消息 hover 时提供可见 pin/unpin 按钮，右键菜单继续保留。
* 上下文黑板弹窗必须分成两部分：
  * Pin 的消息：展示当前会话 pinned message 列表，支持从列表中取消 pin。
  * 自由消息/手写上下文：用户编辑 `manual_context`。
* 保存手写上下文不应影响 pinned messages。
* pin/unpin 成功后，黑板弹窗里的 pinned 列表和消息气泡状态都同步更新。
* 空 pinned 列表要有明确空态，不要让用户误以为没加载出来。

## Acceptance Criteria

* [x] 用户不用右键也能看到并点击消息 pin/unpin。
* [x] 黑板弹窗明确展示两部分：Pinned 消息 + 自由手写上下文。
* [x] 在黑板弹窗中取消 pin 后，消息列表状态同步变为未 pin。
* [x] `npm run build` 通过。

## Out of Scope

* 不做复杂排序/搜索 pinned messages。
* 不做 pinned message 编辑备注。
* 不改后端 prompt 注入逻辑，除非发现 UI 同步必须补 API。
