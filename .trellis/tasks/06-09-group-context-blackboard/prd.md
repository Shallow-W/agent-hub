# 实现群聊上下文黑板第一阶段

## Goal

实现群聊共享上下文黑板第一阶段：用户可以手动 pin 关键消息作为长期上下文；所有 Agent（orchestrator、worker、单 Agent @mention）在处理群聊消息时都能收到这块共享上下文。暂不实现 orchestrator 自动总结写入黑板。

## What I already know

* 当前 orchestrator 能拿到群聊最近动态，但 worker 的分派上下文较弱。
* 用户希望上下文黑板分为两部分：
  * 用户 Pin 上下文：用户手动 pin 的关键消息，长期共享。
  * 群聊/任务状态摘要：后续由 orchestrator 总结写入，本阶段暂不做。
* 第一阶段需要让所有 Agent 共享用户 pin 上下文。

## Requirements

* 支持 pin / unpin 群聊消息。
* 能查询当前群聊的 pinned context 列表。
* Agent prompt 中注入 `{群聊上下文黑板}`，其中包含 `{用户 Pin 上下文}`。
* 注入范围覆盖 orchestrator、worker dispatch、直接 @单 Agent。
* 限制上下文长度，避免 pin 内容挤爆 prompt。
* 前端提供基础 pin/unpin 入口和可见状态。

## Acceptance Criteria

* [x] 用户可以 pin 一条消息，并能再次 unpin。
* [x] 群聊 pin 列表来自后端持久化数据。
* [x] Orchestrator prompt 包含用户 pin 上下文。
* [x] Worker dispatch prompt 包含用户 pin 上下文。
* [x] 直接 @Agent prompt 包含用户 pin 上下文。
* [x] 后端和前端检查通过。

## Out of Scope

* 不实现 orchestrator 自动总结黑板。
* 不实现黑板版本历史。
* 不实现复杂排序/分组/用户备注编辑，除非现有 UI 很容易承载。

## Technical Notes

* 待检查 messages API、消息列表组件、orchestrator prompt 构造点和 worker dispatch context 构造点。
* 已验证：`go test ./...`、`npm run build`、真实临时后端 `POST pin` / `GET messages` / `GET pinned-context` / `DELETE pin` 流程。
