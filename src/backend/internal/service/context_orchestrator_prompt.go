package service

import (
	"context"
)

// OrchestratorPromptBuilder 只在 IsOrchestrator 时生效，
// 把 OrchestratorSystemPrompt 包装为「[系统指令]...」段前置到 current。
// 非 orch 角色时返回 current 不变。
type OrchestratorPromptBuilder struct{}

// Build 实现 ContextBuilder。
func (b *OrchestratorPromptBuilder) Build(ctx context.Context, in ContextInput, current string) string {
	if !in.IsOrchestrator {
		return current
	}
	// 与原 handleOrchestratedDispatch 中的拼装等价：
	// orchCtx := "[系统指令]\n" + OrchestratorSystemPrompt + "\n\n"
	return "[系统指令]\n" + OrchestratorSystemPrompt + "\n\n" + current
}
