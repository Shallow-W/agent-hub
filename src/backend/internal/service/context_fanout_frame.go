package service

import (
	"context"
	"fmt"
)

// FanoutFrameBuilder 构建 worker 派发时的「群聊背景 + 调度指令」框架段，
// 前置到 current。orchestratorName + task 文本由 ContextInput.Extra 提供。
//
// 与 orchestrator_async.go 中原内联拼装完全等价：
//
//	[群聊背景]
//	- Orchestrator: <name>
//
//	[调度指令]
//	Orch @你，分配了以下任务：
//	<truncated task>
//
//	请完成这个任务并在回复末尾 @<name> 表示完成。
//
// 无 orchestratorName 时返回 current 不变（无法构成框架）。
type FanoutFrameBuilder struct{}

// fanoutFrameExtraKey 是 ContextInput.Extra 中存放 fanout 框架原料的 key。
const fanoutFrameExtraKey = "fanout_frame"

// FanoutFrameInput 是 FanoutFrameBuilder 读取的原料。
type FanoutFrameInput struct {
	OrchestratorName string
	Task             string
}

// Build 实现 ContextBuilder。orchestratorName 为空时返回 current 不变。
func (b *FanoutFrameBuilder) Build(_ context.Context, in ContextInput, current string) string {
	raw, ok := in.Extra[fanoutFrameExtraKey]
	if !ok {
		return current
	}
	fi, ok := raw.(FanoutFrameInput)
	if !ok {
		return current
	}
	if fi.OrchestratorName == "" {
		return current
	}
	frame := fmt.Sprintf("[群聊背景]\n- Orchestrator: %s\n\n[调度指令]\nOrch @你，分配了以下任务：\n%s\n\n请完成这个任务并在回复末尾 @%s 表示完成。",
		fi.OrchestratorName, truncateString(fi.Task, 2000), fi.OrchestratorName)
	return frame + current
}
