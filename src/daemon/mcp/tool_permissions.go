package mcp

import (
	"encoding/json"
	"os"
)

var defaultAgentTools = []string{
	"list_group_agents",
	"get_messages",
	"get_agent_skill",
	"list_tasks",
	"create_task",
	"update_task",
	"move_task_status",
}
var noAgentTools = []string{}

var toolsetTemplates = map[string][]string{
	"none":          {},
	"basic":         {"list_group_agents", "get_messages", "get_agent_skill"},
	"tasks":         defaultAgentTools,
	"orchestrator":  append([]string{}, append(defaultAgentTools, "list_conversation_agents", "list_conversations", "get_group_info", "list_group_members", "list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file")...),
	"agent_builder": {"list_agents", "list_group_agents", "get_agent_skill", "list_agent_candidates", "list_machines", "get_agent_detail"},
	"agent_manager": {"list_agents", "get_agent_detail", "update_agent_prompt", "start_agent", "stop_agent", "get_agent_skill"},
	"knowledge":     {"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"},
}

type toolsConfig struct {
	Toolset      string   `json:"toolset"`
	AllowedTools []string `json:"allowed_tools"`
	Tools        []string `json:"tools"`
}

func allowedToolsFromConfig(raw string) map[string]bool {
	cfg := toolsConfig{}
	if raw != "" {
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			return noAgentToolSet()
		}
		if cfg.AllowedTools != nil {
			return toolSet(cfg.AllowedTools)
		}
		if cfg.Tools != nil {
			return toolSet(cfg.Tools)
		}
		if tools, ok := toolsetTemplates[cfg.Toolset]; ok {
			return toolSet(tools)
		}
		return noAgentToolSet()
	}
	return noAgentToolSet()
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
