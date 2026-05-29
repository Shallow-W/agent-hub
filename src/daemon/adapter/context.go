package adapter

import (
	"encoding/json"
	"strings"
)

// ContextMessage 与后端 model.ContextMessage 结构对齐。
type ContextMessage struct {
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	Content string `json:"content"`
}

// FormatContextAsSystemPrompt 把后端传来的 context_messages JSON 转换为
// 结构化的对话历史文本，作为 systemPrompt 传给 Adapter.Start。
// 返回空字符串表示无历史上下文（首条消息场景）。
func FormatContextAsSystemPrompt(contextMessagesJSON string) string {
	if strings.TrimSpace(contextMessagesJSON) == "" {
		return ""
	}
	var msgs []ContextMessage
	if err := json.Unmarshal([]byte(contextMessagesJSON), &msgs); err != nil || len(msgs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("以下是当前对话的历史上下文，你需要基于此进行回复：\n\n")

	for _, m := range msgs {
		switch m.Role {
		case "user":
			name := m.Name
			if name == "" {
				name = "用户"
			}
			sb.WriteString("[")
			sb.WriteString(name)
			sb.WriteString("]: ")
		case "assistant":
			name := m.Name
			if name == "" {
				name = "助手"
			}
			sb.WriteString("[")
			sb.WriteString(name)
			sb.WriteString("]: ")
		default:
			sb.WriteString("[系统]: ")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n请基于以上对话历史，回复最新的用户消息。")
	return sb.String()
}
