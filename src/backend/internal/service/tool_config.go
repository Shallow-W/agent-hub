package service

import (
	"encoding/json"
	"strings"
)

var platformToolsets = map[string][]string{
	"none":          {},
	"basic":         {"list_group_agents", "get_messages"},
	"tasks":         {"list_group_agents", "get_messages", "list_tasks", "create_task", "update_task", "move_task_status"},
	"orchestrator":  {"list_group_agents", "list_conversation_agents", "get_messages", "list_tasks", "create_task", "update_task", "move_task_status", "list_conversations", "get_group_info", "list_group_members"},
	"agent_builder": {"list_agents", "list_group_agents", "list_agent_candidates", "list_machines"},
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
}

type agentToolsConfig struct {
	Toolset      string   `json:"toolset,omitempty"`
	AllowedTools []string `json:"allowed_tools"`
}

func normalizeToolsConfig(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
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
