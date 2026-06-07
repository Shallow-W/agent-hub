package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// OrchestratorSystemPrompt is the base system prompt for the orchestrator agent.
// It instructs the agent how to decompose tasks, dispatch to workers, and aggregate results.
const OrchestratorSystemPrompt = `你是群聊中的任务协调者（Orchestrator）。你不亲自执行任何工作，只负责拆解任务并分派给群聊中的 Agent。使用中文交流。

## 工作步骤

1. 先调用 MCP 工具 list_group_agents 查询当前群聊中有哪些智能体、它们的名称和角色
2. 分析用户需求，决定分派方案
3. 用 @mention 格式输出分派结果（见下方格式）
4. 如果不需要分派（例如直接回答即可），直接回复用户

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
- 必须先调 list_group_agents 确认可用 agent 名称，@mention 中的名称必须完全匹配
- 只需一个 Agent 时直接 @该Agent，无需多余说明
- 禁止亲自执行具体工作`

// BuildOrchestratorPrompt builds the full prompt for an orchestrator dispatch.
// conversationTitle: the group chat name
// agentList: list of agent names available in this group chat (for @mention reference)
// recentMessages: compressed summary of recent group chat messages
// userMessage: the user's current message
func BuildOrchestratorPrompt(conversationTitle string, agentList []string, recentMessages string, userMessage string) string {
	var sb strings.Builder

	sb.WriteString("[当前群聊]\n")
	sb.WriteString("群聊名称：")
	sb.WriteString(conversationTitle)
	sb.WriteString("\n可用 Agent：")
	sb.WriteString(strings.Join(agentList, "、"))
	sb.WriteString("\n\n")

	sb.WriteString("[群聊最近动态]\n")
	sb.WriteString(recentMessages)
	sb.WriteString("\n\n")

	sb.WriteString("[用户消息]\n")
	sb.WriteString(userMessage)
	sb.WriteString("\n\n")

	sb.WriteString("请分析用户需求，决定是否需要拆解任务并分派给相应的 Agent。\n")
	sb.WriteString("如果只需要单个 Agent 处理，直接 @该Agent。\n")
	sb.WriteString("如果需要多 Agent 协作，按照分派格式拆解任务。")

	return sb.String()
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
