package adapter

import (
	"encoding/json"
	"strings"
)

// AgentHandoff 表示一条其他 agent 的任务交接摘要。
type AgentHandoff struct {
	AgentName   string `json:"agent_name"`
	AgentID     string `json:"agent_id"`
	UserRequest string `json:"user_request"`
	Result      string `json:"result"`
}

// FormatContextAsSystemPrompt 把后端传来的 context_messages JSON（[]AgentHandoff 格式）
// 转换为结构化的多 agent 协作背景文本，作为 systemPrompt 传给 Adapter.Start。
// 返回空字符串表示无历史上下文（首条消息场景）。
func FormatContextAsSystemPrompt(contextMessagesJSON string) string {
	if strings.TrimSpace(contextMessagesJSON) == "" {
		return ""
	}
	var handoffs []AgentHandoff
	if err := json.Unmarshal([]byte(contextMessagesJSON), &handoffs); err != nil || len(handoffs) == 0 {
		return strings.TrimSpace(contextMessagesJSON)
	}

	var sb strings.Builder
	sb.WriteString("你正在参与一个多智能体协作任务。\n\n其他智能体的工作进展：\n")

	for _, h := range handoffs {
		name := h.AgentName
		if name == "" {
			name = "未知智能体"
		}
		sb.WriteString("- [")
		sb.WriteString(name)
		sb.WriteString("] 收到请求「")
		sb.WriteString(h.UserRequest)
		sb.WriteString("」后，完成了：")
		sb.WriteString(h.Result)
		sb.WriteString("\n")
	}

	sb.WriteString("\n---\n请基于以上背景，处理当前请求。")
	return sb.String()
}
