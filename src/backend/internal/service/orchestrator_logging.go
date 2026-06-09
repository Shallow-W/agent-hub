package service

import (
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

const orchFlowLog = "orch_flow"

func orchPreview(text string) string {
	return truncateString(normalizePromptLine(text), 220)
}

func mentionLogNames(mentions []MentionResult) []string {
	names := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		names = append(names, mention.AgentName)
	}
	return names
}

func convAgentLogNames(agents []model.ConversationAgent) []string {
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		role := agent.Role
		if role == "" {
			role = "member"
		}
		names = append(names, fmt.Sprintf("%s(%s)", agent.Name, role))
	}
	return names
}

func dispatchTaskLogNames(tasks []DispatchTask) []string {
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, task.AgentName)
	}
	return names
}

func resolvedDispatchLogNames(tasks []ResolvedDispatchTask) []string {
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, fmt.Sprintf("%s:%s", task.AgentName, task.AgentID))
	}
	return names
}
