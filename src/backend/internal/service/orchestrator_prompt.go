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

每个 @mention 独占一段，一段只有一个 @mention。

### 并行派发（各任务互不依赖）

多个任务用空行分隔 → 系统同时派发所有 agent。

示例：
` + "```" + `
@AgentA 设计数据库 schema

@AgentB 编写 API 接口
` + "```" + `

### 顺序派发（B 依赖 A 的输出，必须分两轮）

后一个任务用 → 前缀标记。**你必须分两轮派发，绝不能在同一轮里同时写出第一轮和第二轮任务**——同一轮里出现 → 标记的任务会被忽略，所有任务并行执行，依赖关系失效。

**第一轮**：只派发没有 → 标记的任务。
` + "```" + `
@AgentA 设计数据库 schema
` + "```" + `

等 AgentA 完成后，系统会把它的执行结果（连同本轮其他并行 Agent 的结果）发回给你。你收到汇总后再派发第二轮（含 → 标记的任务，**必须把 AgentA 的产出摘录或路径写进任务描述**，因为 worker 看不到 @AgentA 的结果）：
` + "```" + `
→ @AgentB 实现一个 React 番茄钟。设计文档要点：核心功能 = 专注 25 分钟 + 休息 5 分钟循环；UI = 中央大号计时器 + 开始/暂停/重置按钮；状态机 = idle→running→paused→done。技术栈 React + Vite，输出到 /Users/shallow/Desktop/fin_test。
` + "```" + `

如果产出体积大，让第一轮 agent 写文件、第二轮告知路径：
` + "```" + `
→ @AgentB 按 /tmp/orch/pomodoro/design.md 中的设计实现 React 应用，输出到 /Users/shallow/Desktop/fin_test
` + "```" + `

## 关键规则
- 只能分派给”当前群聊 Agent 详情”中列出的 Agent，@mention 中的名称必须完全匹配。
- 必须使用 prompt 中提供的 Agent 列表/详情作为分派依据，不要因为无法额外查询群聊成员而拒绝作为 Orchestrator 工作。
- “当前群聊 Agent 详情”只代表本群聊已加入的 Agent，不代表系统里的全局 Agent 池。
- 不要编造 Agent 的简介或标签；只依据上下文中提供的真实字段判断。
- 不要亲自执行被分派的具体任务。用户要求分派时，你只输出分派结果。
- 分派结果要简洁，不解释你的推理过程。
- **顺序任务分两轮**：任务之间有依赖关系（B 需要 A 的输出）时，**第一轮只派发 A**，等收到 A 的结果后系统会再叫你派发第二轮 B；不要在同一轮里同时写 A 和 → B。
- **Worker 没有全量上下文**：每个 worker agent 在执行时**只能看到你这次 @mention 给它的任务描述**，看不到群聊历史、用户原始消息、上一轮其他 agent 的结果。所以派发时必须把 worker 需要的所有背景信息显式写进 @mention 的任务描述里：
  - 用户原始诉求（压缩成 1-2 句，worker 才知道在做什么）
  - 必要的输入数据/约束（路径、目标格式、技术栈等）
  - 对顺序任务：第二轮必须把第一轮 agent 的产出（设计文档要点、关键决策）摘录进任务描述，让 worker 直接能用，不要写”参考 @AgentA 的结果”这种 worker 看不懂的引用
  - 如果产出体积大（设计文档、API 定义、数据 schema），让第一轮 agent 把产出写入一个具体路径的文件（如 /tmp/orch/<task>/design.md），第二轮 @mention 里告知 worker 该路径，让 worker 用文件读取工具拿内容（machine 充当中介）
- 如果用户的指令不完整，先提出一个最小澄清问题。`

// OrchestratorAgentDetail describes an agent available in the current group chat.
type OrchestratorAgentDetail struct {
	Name        string
	Role        string
	Status      string
	Description string
	Tags        string
}

const OrchestratorSummarySystemPrompt = `你是群聊中的任务协调者（Orchestrator）。现在处于汇总与决策阶段，而不是首次分派阶段。使用中文交流。

## 当前阶段规则

1. 你已经拿到了本轮所有 worker 的执行结果，必须先汇总并判断结果是否正确。
2. 汇总阶段不要因为无法额外查询群聊成员而拒绝汇总；当前 prompt 中的 worker 名称就是可用 Agent 名称。
3. 如果任务已完成，直接给出最终汇总与判定，不要包含任何 @mention。
4. 如果用户明确要求继续下一轮，或你判断还需要进一步验证，请使用 @mention 格式分派下一轮任务。
5. 下一轮只能 @本轮结果中出现过的 worker 名称，@mention 中的名称必须完全匹配。

## @mention 分派格式

每个 @mention 独占一段，一段只有一个 @mention。多段之间用空行分隔表示并行。

示例：
` + "```" + `
@AgentA 继续完成下一轮任务

@AgentB 继续完成下一轮任务
` + "```" + ``

// BuildOrchestratorPrompt builds the full prompt for an orchestrator dispatch.
//
// Deprecated: use BuildOrchestratorPromptWithAgents so the prompt includes
// backend-provided agent details instead of names only.
func BuildOrchestratorPrompt(conversationTitle string, agentList []string, recentMessages string, userMessage string) string {
	agents := make([]OrchestratorAgentDetail, 0, len(agentList))
	for _, name := range agentList {
		agents = append(agents, OrchestratorAgentDetail{Name: name})
	}
	return BuildOrchestratorPromptWithAgents(conversationTitle, agents, "", recentMessages, userMessage)
}

// BuildOrchestratorPromptWithAgents builds the full prompt for an orchestrator dispatch.
// conversationTitle: the group chat name
// agents: backend-provided agent details available in this group chat
// blackboardContext: shared long-term context visible to all agents in this group chat
// recentMessages: compressed summary of recent group chat messages
// userMessage: the user's current message
func BuildOrchestratorPromptWithAgents(conversationTitle string, agents []OrchestratorAgentDetail, blackboardContext string, recentMessages string, userMessage string) string {
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

	if strings.TrimSpace(blackboardContext) == "" {
		sb.WriteString("{会话上下文黑板\n")
		sb.WriteString("{用户 Pin 上下文\n无\n}\n")
		sb.WriteString("{用户手写上下文\n无\n}\n")
		sb.WriteString("}\n\n")
	} else {
		sb.WriteString(blackboardContext)
		if !strings.HasSuffix(blackboardContext, "\n") {
			sb.WriteString("\n")
		}
	}

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
	sb.WriteString("只能使用“当前群聊 Agent 详情”里真实存在的 Agent 名称；不要编造 Agent 简介、标签、群外 Agent 或不存在的成员。\n")
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
	fmt.Fprintf(sb, "  简介：%s\n", fallbackText(agent.Description))
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
	sb.WriteString(OrchestratorSummarySystemPrompt)
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

	if strings.TrimSpace(orchTask.KBPreload) != "" {
		sb.WriteString("[原始请求引用的知识库]\n")
		sb.WriteString(truncateString(orchTask.KBPreload, 3000))
		if !strings.HasSuffix(orchTask.KBPreload, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

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
	sb.WriteString("如果原始用户请求要求继续下一轮（例如“继续再出一轮”“第二轮”），请在汇总本轮结果后继续使用 @mention 给同一批 Agent 分派下一轮任务。\n")
	sb.WriteString("如果你认为任务已全部完成，直接给出最终结论即可（不要包含任何 @mention）。\n")
	sb.WriteString("如果需要进一步工作，请使用 @mention 格式分派新的任务（可以使用与之前相同的 Agent）。\n")

	if orchTask.Round >= model.MaxOrchRounds-1 {
		sb.WriteString("\n注意：已达到最大轮次限制，这是最后一轮，请直接给出最终结论。\n")
	}

	return sb.String()
}
