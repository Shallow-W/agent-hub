package service

import (
	"context"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// AgentConfigInjector 把 Agent 的 system_prompt / tools_config / 平台 Skills 前置到 current。
// 这是「包装器」型 builder：输出 = agentConfig + current。
// Agent 为 nil 时返回 current 不变。
type AgentConfigInjector struct{}

// Build 实现 ContextBuilder。
func (b *AgentConfigInjector) Build(ctx context.Context, in ContextInput, current string) string {
	if in.Agent == nil {
		return current
	}
	return BuildAgentConfigText(in.Agent, current, in.Content)
}

// BuildAgentConfigText 把 agent 的系统提示词 / 工具配置 / 平台 Skills 拼到 contextStr 前面。
// 逻辑与 OrchestratorService.InjectAgentConfig 等价（供 façade 复用）。
// 注意：Orchestrator 系统指令由 OrchestratorPromptBuilder 单独注入，此处只处理 agent 自定义配置。
func BuildAgentConfigText(agent *model.Agent, contextStr string, taskText string) string {
	var sb strings.Builder
	if agent.SystemPrompt != "" {
		sb.WriteString("[系统指令]\n")
		sb.WriteString(agent.SystemPrompt)
		sb.WriteString("\n\n")
	}
	if agent.ToolsConfig != "" {
		sb.WriteString("[可用工具]\n")
		sb.WriteString(agent.ToolsConfig)
		sb.WriteString("\n\n")
	}
	if skillCtx := BuildAgentSkillContext(agent.CustomSkills, taskText); skillCtx != "" {
		sb.WriteString(skillCtx)
		if !strings.HasSuffix(skillCtx, "\n\n") {
			sb.WriteString("\n\n")
		}
	}
	sb.WriteString(contextStr)
	return sb.String()
}
