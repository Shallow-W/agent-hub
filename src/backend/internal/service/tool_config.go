package service

import (
	"encoding/json"
	"strings"
)

var platformToolsets = map[string][]string{
	"none":          {},
	"basic":         {"list_group_agents", "get_messages", "get_agent_skill"},
	"tasks":         {"list_group_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status"},
	"orchestrator":  {"list_group_agents", "list_conversation_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status", "list_conversations", "get_group_info", "list_group_members", "list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file", "create_agent", "update_agent", "delete_agent", "list_toolsets", "list_platform_skills"},
	"agent_builder": {"list_agents", "list_group_agents", "get_agent_skill", "list_agent_candidates", "list_machines", "get_agent_detail", "create_agent", "update_agent", "update_agent_prompt", "list_platform_skills"},
	"agent_manager": {"list_agents", "get_agent_detail", "update_agent", "update_agent_prompt", "start_agent", "stop_agent", "delete_agent", "get_agent_skill", "list_platform_skills"},
	"knowledge":     {"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"},
}

var platformToolCatalog = map[string]bool{
	"list_conversations":       true,
	"list_conversation_agents": true,
	"get_messages":             true,
	"create_group":             true,
	"list_agents":              true,
	"list_group_agents":        true,
	"list_tasks":               true,
	"create_task":              true,
	"update_task":              true,
	"move_task_status":         true,
	"delete_task":              true,
	"get_group_info":           true,
	"list_group_members":       true,
	"list_machines":            true,
	"list_agent_candidates":    true,
	"get_agent_skill":          true,
	"get_agent_detail":         true,
	"update_agent_prompt":      true,
	"start_agent":              true,
	"stop_agent":               true,
	"list_knowledge_bases":     true,
	"list_knowledge_files":     true,
	"search_knowledge":         true,
	"read_knowledge_file":      true,
	"create_agent":             true,
	"update_agent":             true,
	"delete_agent":             true,
		"list_toolsets":            true,
		"list_platform_skills":    true,
}

type agentToolsConfig struct {
	Toolset      string   `json:"toolset,omitempty"`
	AllowedTools []string `json:"allowed_tools"`
}

func normalizeToolsConfig(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return `{"toolset":"none","allowed_tools":[]}`, nil
	}

	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		// Legacy markdown/free-text configs are preserved for display. Runtime
		// treats them as no tool authorization.
		return raw, nil
	}

	if _, ok := platformToolsets[cfg.Toolset]; !ok {
		cfg.Toolset = ""
	}
	cfg.AllowedTools = normalizeToolNames(cfg.AllowedTools)

	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeToolNames(names []string) []string {
	if names == nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || !platformToolCatalog[name] || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}
