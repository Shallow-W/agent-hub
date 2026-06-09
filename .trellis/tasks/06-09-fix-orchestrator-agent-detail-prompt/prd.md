# fix orchestrator agent detail prompt

## Goal

修正 orchestrator 分派提示词里的当前群聊 Agent 详情，避免把 daemon skill/capabilities 和 CLI 工具信息直接暴露给 orchestrator，减少冗余和误导。Agent 详情应只保留对分派有用的轻量画像：名称、角色、状态、简介、标签。

## What I already know

* 生成位置在 `src/backend/internal/service/orchestrator_prompt.go`。
* 当前 `writeAgentDetail` 输出 `CLI工具` 和 `能力`，其中能力来自 `CapabilitiesJSON`，会把 skill 详情原样塞进 prompt。
* 当前 `简介` 使用的是 `SystemPrompt`，会把“你可以通过平台提供的管理工具...”这类工具说明暴露给 orchestrator。
* 当前 agent/conversation agent model 暂无独立 `description` 字段。

## Requirements

* `当前群聊 Agent 详情` 不再输出 `CLI工具`。
* `当前群聊 Agent 详情` 不再输出 `能力`/`CapabilitiesJSON`。
* `简介` 不应输出管理工具操作说明或完整 system prompt。
* 在当前缺少 agent description 字段的情况下，简介优先使用可安全派发判断的短描述；无法得到时显示 `未配置`。
* 更新 orchestrator prompt 测试，覆盖不泄漏 CLI、capabilities、管理工具提示。

## Acceptance Criteria

* [x] 生成的 orchestrator prompt 中不包含 `CLI工具：`。
* [x] 生成的 orchestrator prompt 中不包含 `能力：` 和 skill/capabilities JSON 内容。
* [x] 生成的 orchestrator prompt 中不包含 `你可以通过平台提供的管理工具执行以下操作`。
* [x] 仍保留 Agent 名称、角色、状态、简介、标签。
* [x] 相关 Go 测试通过。

## Out of Scope

* 本次不新增 `agents.description` DB 字段。
* 本次不改 Agent 创建/编辑 UI。
* 本次不重做工具权限系统。
