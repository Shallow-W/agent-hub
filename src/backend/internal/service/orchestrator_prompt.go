package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// OrchestratorSystemPrompt is the base system prompt for the orchestrator agent.
// It instructs the agent how to decompose tasks, dispatch to workers, and aggregate results.
const OrchestratorSystemPrompt = `你是群聊中的任务协调者（Orchestrator）。你负责理解用户当前消息，结合群聊上下文与当前群聊 Agent 详情，判断是否需要拆解任务并分派给当前群聊中的 Agent。使用中文交流。

## 工作步骤

1. 先阅读用户消息、群聊最近动态和当前群聊 Agent 详情。
2. 判断用户是否明确要求某些 Agent 参与；如果明确要求，优先按用户指定分派。
3. 如果需要分派，拆成互不重复、可执行的小任务，用 @mention 格式输出。
4. 如果只需要一个 Agent 处理，直接 @该 Agent。
5. 如果不需要分派（例如闲聊、澄清、直接回答即可），直接回复用户。

## @mention 分派格式（必须严格遵守）

每个 @mention 独占一段，一段只有一个 @mention。多段之间用空行分隔表示并行。

示例（并行）：
` + "```" + `
@AgentA 设计数据库 schema

@AgentB 编写 API 接口
` + "```" + `

需要顺序执行的任务用 → 前缀：
` + "```" + `
@AgentA 设计数据库 schema

→ @AgentB 根据 @AgentA 的设计编写 API 接口
` + "```" + `

## 关键规则
- 只能分派给“当前群聊 Agent 详情”中列出的 Agent，@mention 中的名称必须完全匹配。
- “当前群聊 Agent 详情”只代表本群聊已加入的 Agent，不代表系统里的全局 Agent 池。
- 不要编造 Agent 的简介、标签或能力；只依据上下文中提供的真实字段判断。
- 不要亲自执行被分派的具体任务。用户要求分派时，你只输出分派结果。
- 分派结果要简洁，不解释你的推理过程。
- 如果用户的指令不完整，先提出一个最小澄清问题。`

// OrchestratorAgentDetail describes an agent available in the current group chat.
type OrchestratorAgentDetail struct {
	Name             string
	Role             string
	Status           string
	CLITool          string
	SystemPrompt     string
	CapabilitiesJSON string
	Tags             string
}

// BuildOrchestratorPrompt builds the full prompt for an orchestrator dispatch.
//
// Deprecated: use BuildOrchestratorPromptWithAgents so the prompt includes
// backend-provided agent details instead of names only.
func BuildOrchestratorPrompt(conversationTitle string, agentList []string, recentMessages string, userMessage string) string {
	agents := make([]OrchestratorAgentDetail, 0, len(agentList))
	for _, name := range agentList {
		agents = append(agents, OrchestratorAgentDetail{Name: name})
	}
	return BuildOrchestratorPromptWithAgents(conversationTitle, agents, recentMessages, userMessage)
}

// BuildOrchestratorPromptWithAgents builds the full prompt for an orchestrator dispatch.
// conversationTitle: the group chat name
// agents: backend-provided agent details available in this group chat
// recentMessages: compressed summary of recent group chat messages
// userMessage: the user's current message
func BuildOrchestratorPromptWithAgents(conversationTitle string, agents []OrchestratorAgentDetail, recentMessages string, userMessage string) string {
	var sb strings.Builder

	sb.WriteString("{当前群聊\n")
	sb.WriteString("群聊名称：")
	sb.WriteString(conversationTitle)
	sb.WriteString("\n可用 Agent：")
	sb.WriteString(strings.Join(agentNames(agents), "、"))
	sb.WriteString("\n}\n\n")

	sb.WriteString("{当前群聊 Agent 详情\n")
	if len(agents) == 0 {
		sb.WriteString("无\n")
	} else {
		for _, agent := range agents {
			writeAgentDetail(&sb, agent)
		}
	}
	sb.WriteString("}\n\n")

	sb.WriteString("{群聊最近动态\n")
	if strings.TrimSpace(recentMessages) == "" {
		sb.WriteString("无\n")
	} else {
		sb.WriteString(recentMessages)
		if !strings.HasSuffix(recentMessages, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("}\n\n")

	sb.WriteString("{用户消息\n")
	sb.WriteString(userMessage)
	sb.WriteString("\n}\n\n")

	sb.WriteString("{Orchestrator 指令\n")
	sb.WriteString("请分析用户当前消息，决定是否需要拆解任务并分派给相应的 Agent。\n")
	sb.WriteString("如果用户明确指定了 Agent，优先分派给用户指定的 Agent。\n")
	sb.WriteString("如果只需要单个 Agent 处理，直接 @该Agent 并给出任务。\n")
	sb.WriteString("如果需要多 Agent 协作，按 @mention 分派格式拆解为清晰、互不重复的小任务。\n")
	sb.WriteString("只能使用“当前群聊 Agent 详情”里真实存在的 Agent 名称；不要编造 Agent 能力、标签、群外 Agent 或不存在的成员。\n")
	sb.WriteString("当用户要求你分派任务时，不要亲自完成这些任务，只输出分派结果。\n")
	sb.WriteString("}\n")

	return sb.String()
}

func agentNames(agents []OrchestratorAgentDetail) []string {
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		names = append(names, agent.Name)
	}
	return names
}

func writeAgentDetail(sb *strings.Builder, agent OrchestratorAgentDetail) {
	fmt.Fprintf(sb, "- 名称：%s\n", fallbackText(agent.Name))
	fmt.Fprintf(sb, "  角色：%s\n", fallbackText(agent.Role))
	fmt.Fprintf(sb, "  状态：%s\n", fallbackText(agent.Status))
	fmt.Fprintf(sb, "  CLI工具：%s\n", fallbackText(agent.CLITool))
	fmt.Fprintf(sb, "  简介：%s\n", fallbackText(agent.SystemPrompt))
	fmt.Fprintf(sb, "  能力：%s\n", fallbackText(agent.CapabilitiesJSON))
	fmt.Fprintf(sb, "  标签：%s\n", fallbackText(agent.Tags))
}

func fallbackText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未配置"
	}
	return value
}

// roundHistoryEntry is used to deserialize round_history JSONB.
type roundHistoryEntry struct {
	Round         int               `json:"round"`
	WorkerStatus  map[string]string `json:"worker_status"`
	WorkerResults map[string]string `json:"worker_results"`
}

// BuildSummaryPrompt builds the prompt for the summary+decision phase.
// After all workers complete (or after all rounds), the orchestrator receives
// this prompt and must either conclude or dispatch more work via @mention.
func BuildSummaryPrompt(orchTask *model.OrchTask) string {
	var sb strings.Builder
	sb.WriteString(OrchestratorSystemPrompt)
	sb.WriteString("\n\n---\n\n")

	sb.WriteString("[汇总与决策任务]\n")
	if orchTask.Round == 0 {
		sb.WriteString("所有 Agent 已完成你分配的任务，以下是它们的执行结果。\n")
	} else {
		fmt.Fprintf(&sb, "这是第 %d 轮任务执行结果。以下是历史轮次和本轮 Agent 的执行结果。\n", orchTask.Round+1)
	}

	sb.WriteString("\n[原始用户请求]\n")
	sb.WriteString(truncateString(orchTask.OriginalMessage, 1000))
	sb.WriteString("\n\n")

	// Include previous rounds context from round_history
	if orchTask.RoundHistory != "" {
		var history []roundHistoryEntry
		if json.Unmarshal([]byte(orchTask.RoundHistory), &history) == nil {
			for _, entry := range history {
				fmt.Fprintf(&sb, "[第 %d 轮结果]\n", entry.Round+1)
				for name, result := range entry.WorkerResults {
					fmt.Fprintf(&sb, "- %s: %s\n", name, truncateString(result, 1000))
				}
				sb.WriteString("\n")
			}
		}
	}

	// Current round results
	sb.WriteString("[本轮 Agent 执行结果]\n")
	var workerResults map[string]string
	if orchTask.WorkerResults != "" {
		_ = json.Unmarshal([]byte(orchTask.WorkerResults), &workerResults)
	}
	for name, result := range workerResults {
		fmt.Fprintf(&sb, "### %s\n%s\n\n", name, truncateString(result, 2000))
	}

	// Decision guidance
	sb.WriteString("[决策指引]\n")
	sb.WriteString("请先汇总各 Agent 的成果。\n")
	sb.WriteString("如果你认为任务已全部完成，直接给出最终结论即可（不要包含任何 @mention）。\n")
	sb.WriteString("如果需要进一步工作，请使用 @mention 格式分派新的任务（可以使用与之前相同的 Agent）。\n")

	if orchTask.Round >= model.MaxOrchRounds-1 {
		sb.WriteString("\n注意：已达到最大轮次限制，这是最后一轮，请直接给出最终结论。\n")
	}

	return sb.String()
}
