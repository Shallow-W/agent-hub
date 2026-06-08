# fix pinned message metadata and notification

## Goal

修复上下文黑板 Pin 消息列表和消息流提示的两个体验问题：Pin 列表应展示原消息发送人和原消息发送时间；用户 pin/unpin 消息不应触发“新消息”浮层。

## What I already know

* 后端 `ListPinnedMessages` 已返回 `username` 和 `message_created_at`，分别来自原消息 sender 与原消息创建时间。
* 前端黑板列表当前在 sender 缺失时回退到 `pinned_by_name`，会把 pin 人误当作消息发送人。
* `MessageList` 的新消息提示 effect 依赖整个 `messages` 数组引用，pin 状态更新也会触发它。

## Requirements

* Pin 列表元信息必须展示原消息发送人；sender 缺失时按消息角色显示 `助手` / `用户` / `系统`，不能显示 pin 人。
* Pin 列表时间必须继续展示 `message_created_at`，即原消息发送时间。
* pin/unpin 只更新消息 pin 状态，不触发“新消息”按钮/浮层，也不增加未读提示计数。
* 保留真正新消息到达时的原有滚动/新消息提示行为。

## Acceptance Criteria

* [x] Pin 列表不再显示 `pinned_by_name` 作为消息作者。
* [x] Pin 列表时间使用原消息发送时间。
* [x] 对已有消息执行 pin/unpin 时不出现“新消息”按钮。
* [x] 真正新增消息时仍可触发原有滚动/新消息行为。
* [x] 前端构建通过；相关后端测试不退化。

## Out of Scope

* 不改变 pin API 响应结构。
* 不新增 pin 操作历史展示。
