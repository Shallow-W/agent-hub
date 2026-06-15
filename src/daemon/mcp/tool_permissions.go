package mcp

import (
	"encoding/json"
	"os"
)

// ToolsetInfo describes a built-in toolset template.
type ToolsetInfo struct {
	Name        string
	Label       string
	Description string
	ToolNames   []string
}

// DefaultToolsets returns the built-in toolset templates — single source of truth
// replacing the 3 separate hardcoded maps (toolsetTemplates, agentCreationToolsets,
// toolsetDescriptions) that previously had to be kept in sync manually.
func DefaultToolsets() []ToolsetInfo {
	return []ToolsetInfo{
		{Name: "none", Label: "无工具", Description: "不分配任何平台工具"},
		{Name: "basic", Label: "基础群聊", Description: "包含群 Agent 列表、消息读取、Skill 查看等基础工具",
			ToolNames: []string{"list_group_agents", "get_messages", "get_agent_skill"}},
		{Name: "tasks", Label: "任务协作", Description: "包含任务看板的完整增删改查能力",
			ToolNames: []string{"list_group_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status"}},
		{Name: "orchestrator", Label: "Orchestrator", Description: "编排器模板，包含会话、任务、群组管理和知识库搜索",
			ToolNames: []string{"list_group_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status", "list_conversation_agents", "list_conversations", "get_group_info", "list_group_members", "list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file", "create_agent", "update_agent", "delete_agent", "list_toolsets"}},
		{Name: "agent_builder", Label: "Agent 创建", Description: "Agent 发现和详情查询工具",
			ToolNames: []string{"list_agents", "list_group_agents", "get_agent_skill", "list_agent_candidates", "list_machines", "get_agent_detail", "create_agent", "update_agent", "delete_agent", "list_toolsets"}},
		{Name: "agent_manager", Label: "Agent 管理", Description: "Agent 详情、提示词更新、启停控制",
			ToolNames: []string{"list_agents", "get_agent_detail", "update_agent_prompt", "start_agent", "stop_agent", "get_agent_skill"}},
		{Name: "knowledge", Label: "知识库", Description: "知识库列表、文件列表和关键词搜索",
			ToolNames: []string{"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"}},
	}
}

// ToolsetStore manages toolset templates — single source of truth with backend sync
// capability. Replaces the 3 separate hardcoded maps that previously required manual
// synchronization.
type ToolsetStore struct {
	toolsets []ToolsetInfo
	byName   map[string]*ToolsetInfo
}

func NewToolsetStore() *ToolsetStore {
	s := &ToolsetStore{}
	s.Reset(DefaultToolsets())
	return s
}

func (s *ToolsetStore) Reset(toolsets []ToolsetInfo) {
	s.toolsets = toolsets
	s.byName = make(map[string]*ToolsetInfo, len(toolsets))
	for i := range toolsets {
		s.byName[toolsets[i].Name] = &toolsets[i]
	}
}

func (s *ToolsetStore) Lookup(name string) ([]string, bool) {
	t, ok := s.byName[name]
	if !ok {
		return nil, false
	}
	return t.ToolNames, true
}

// List returns all toolset descriptions for the list_toolsets MCP tool.
func (s *ToolsetStore) List() []map[string]interface{} {
	result := make([]map[string]interface{}, len(s.toolsets))
	for i, t := range s.toolsets {
		result[i] = map[string]interface{}{
			"name": t.Name, "label": t.Label, "description": t.Description,
		}
	}
	return result
}

type toolsConfig struct {
	Toolset      string   `json:"toolset"`
	AllowedTools []string `json:"allowed_tools"`
	Tools        []string `json:"tools"`
}

// AllowedToolsFromConfig parses a tools_config JSON string and returns the
// allowed tool set. Resolves by: allowed_tools → tools → toolset name lookup.
func (s *ToolsetStore) AllowedToolsFromConfig(raw string) map[string]bool {
	cfg := toolsConfig{}
	if raw != "" {
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			return map[string]bool{}
		}
		if cfg.AllowedTools != nil {
			return toolSet(cfg.AllowedTools)
		}
		if cfg.Tools != nil {
			return toolSet(cfg.Tools)
		}
		if tools, ok := s.Lookup(cfg.Toolset); ok {
			return toolSet(tools)
		}
	}
	return map[string]bool{}
}

// ToolsConfigJSON builds the tools_config JSON for agent creation/update.
func (s *ToolsetStore) ToolsConfigJSON(toolset string, allowedTools []string) string {
	data, _ := json.Marshal(map[string]interface{}{
		"toolset": toolset, "allowed_tools": allowedTools,
	})
	return string(data)
}

// SyncFromAPI fetches toolset templates from the backend and updates the store.
// Falls back to current data on error.
func (s *ToolsetStore) SyncFromAPI(api *APIClient) error {
	data, err := api.doGet("/api/tools/builtin-templates", nil)
	if err != nil {
		return err
	}
	items, ok := data.([]interface{})
	if !ok {
		return nil
	}
	var toolsets []ToolsetInfo
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		label, _ := m["label"].(string)
		desc, _ := m["description"].(string)
		var names []string
		if raw, ok := m["tool_names"].([]interface{}); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					names = append(names, s)
				}
			}
		}
		toolsets = append(toolsets, ToolsetInfo{
			Name: name, Label: label, Description: desc, ToolNames: names,
		})
	}
	if len(toolsets) > 0 {
		s.Reset(toolsets)
	}
	return nil
}

func toolSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		if n != "" {
			set[n] = true
		}
	}
	return set
}

func filterTools(tools []Tool, allowed map[string]bool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if allowed[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func AgentIDFromEnv() string { return os.Getenv("AGENTHUB_AGENT_ID") }
