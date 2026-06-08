package mcp

import (
	"encoding/json"
	"os"
)

var defaultAgentTools = []string{
	"list_group_agents",
	"get_messages",
	"list_tasks",
	"create_task",
	"update_task",
	"move_task_status",
}
var noAgentTools = []string{}

var toolsetTemplates = map[string][]string{
	"none":          {},
	"basic":         {"list_group_agents", "get_messages"},
	"tasks":         defaultAgentTools,
	"orchestrator":  append([]string{}, append(defaultAgentTools, "list_conversations", "get_group_info", "list_group_members")...),
	"agent_builder": {"list_agents", "list_group_agents", "list_agent_candidates", "list_machines"},
}

type toolsConfig struct {
	Toolset      string   `json:"toolset"`
	AllowedTools []string `json:"allowed_tools"`
	Tools        []string `json:"tools"`
}

func allowedToolsFromConfig(raw string) map[string]bool {
	cfg := toolsConfig{}
	if raw != "" && json.Unmarshal([]byte(raw), &cfg) == nil {
		if cfg.AllowedTools != nil {
			return toolSet(cfg.AllowedTools)
		}
		if cfg.Tools != nil {
			return toolSet(cfg.Tools)
		}
		if tools, ok := toolsetTemplates[cfg.Toolset]; ok {
			return toolSet(tools)
		}
	}
	return toolSet(defaultAgentTools)
}

func noAgentToolSet() map[string]bool {
	return toolSet(noAgentTools)
}

func toolSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		if name != "" {
			set[name] = true
		}
	}
	return set
}

func filterTools(tools []Tool, allowed map[string]bool) []Tool {
	filtered := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if allowed[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func AgentIDFromEnv() string {
	return os.Getenv("AGENTHUB_AGENT_ID")
}
