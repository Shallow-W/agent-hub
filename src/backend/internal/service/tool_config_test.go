package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agent-hub/backend/internal/port"
)

// mockToolRegistry implements ToolRegistryReader for tests.
type mockToolRegistry struct {
	names map[string]bool
}

func (m *mockToolRegistry) Lookup(name string) (port.MCPToolSpec, bool) {
	if m.names[name] {
		// Return a minimal spec that satisfies the interface.
		// The Lookup caller only cares about existence (ok boolean).
		return nil, true
	}
	return nil, false
}

func (m *mockToolRegistry) List() []port.MCPToolSpec { return nil }

// mockToolsetStore implements ToolsetStore for tests.
type mockToolsetStore struct {
	names map[string]bool
}

func (m *mockToolsetStore) IsValidToolset(_ context.Context, name string) (bool, error) {
	return m.names[name], nil
}

// testRegistry builds a mock ToolRegistryReader containing all tool names.
func testRegistry() ToolRegistryReader {
	return &mockToolRegistry{names: map[string]bool{
		"list_conversations":       true,
		"list_conversation_agents": true,
		"get_messages":             true,
		"create_group":             true,
		"list_agents":              true,
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
		"list_platform_skills":     true,
		"deploy_artifact":          true,
		"deploy_artifact_github":   true,
	}}
}

// testToolsetStore builds a mock ToolsetStore mirroring the seeded builtin_toolset_templates.
func testToolsetStore() ToolsetStore {
	return &mockToolsetStore{names: map[string]bool{
		"none":          true,
		"basic":         true,
		"tasks":         true,
		"orchestrator":  true,
		"agent_builder": true,
		"agent_manager": true,
		"knowledge":     true,
	}}
}

func TestNormalizeToolsConfig_FiltersUnknownTools(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	raw := `{"toolset":"custom","allowed_tools":["list_tasks","unknown","list_tasks"]}`
	got, err := normalizeToolsConfig(ctx, raw, reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Toolset != "" {
		t.Fatalf("expected custom toolset to normalize to empty, got %q", cfg.Toolset)
	}
	if len(cfg.AllowedTools) != 1 || cfg.AllowedTools[0] != "list_tasks" {
		t.Fatalf("unexpected allowed tools: %#v", cfg.AllowedTools)
	}
}

func TestNormalizeToolsConfig_PreservesLegacyText(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	raw := "## legacy tool docs"
	got, err := normalizeToolsConfig(ctx, raw, reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	if got != raw {
		t.Fatalf("expected legacy text preserved, got %q", got)
	}
}

func TestNormalizeToolsConfig_EmptyMeansNoTools(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	got, err := normalizeToolsConfig(ctx, "", reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	if got != `{"toolset":"none","allowed_tools":[]}` {
		t.Fatalf("expected no-tools config, got %q", got)
	}
}

func TestNormalizeToolsConfig_PreservesNoneAndEmptyAllowedTools(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	got, err := normalizeToolsConfig(ctx, `{"toolset":"none","allowed_tools":[]}`, reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Toolset != "none" {
		t.Fatalf("expected none toolset, got %q", cfg.Toolset)
	}
	if cfg.AllowedTools == nil || len(cfg.AllowedTools) != 0 {
		t.Fatalf("expected explicit empty allowed tools, got %#v", cfg.AllowedTools)
	}
}

func TestNormalizeToolsConfig_PreservesTemplateWithoutExplicitAllowedTools(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	got, err := normalizeToolsConfig(ctx, `{"toolset":"basic"}`, reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Toolset != "basic" {
		t.Fatalf("expected basic toolset, got %q", cfg.Toolset)
	}
	if cfg.AllowedTools != nil {
		t.Fatalf("expected absent allowed tools to stay nil, got %#v", cfg.AllowedTools)
	}
}

func TestNormalizeToolsConfig_NilToolsetStoreSkipsValidation(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	// 即使 toolsetStore 为 nil，custom toolset 名也应放行（向后兼容）。
	got, err := normalizeToolsConfig(ctx, `{"toolset":"custom"}`, reg, nil)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Toolset != "custom" {
		t.Fatalf("expected custom toolset preserved when store is nil, got %q", cfg.Toolset)
	}
}

func TestNormalizeToolsConfig_AllowsKnowledgeTools(t *testing.T) {
	ctx := context.Background()
	reg := testRegistry()
	ts := testToolsetStore()
	raw := `{"toolset":"knowledge","allowed_tools":["list_knowledge_bases","list_knowledge_files","search_knowledge","read_knowledge_file"]}`
	got, err := normalizeToolsConfig(ctx, raw, reg, ts)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	var cfg agentToolsConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Toolset != "knowledge" {
		t.Fatalf("expected knowledge toolset, got %q", cfg.Toolset)
	}
	want := []string{"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"}
	if len(cfg.AllowedTools) != len(want) {
		t.Fatalf("allowed tools = %#v, want %#v", cfg.AllowedTools, want)
	}
	for i, tool := range want {
		if cfg.AllowedTools[i] != tool {
			t.Fatalf("allowed tool[%d] = %q, want %q", i, cfg.AllowedTools[i], tool)
		}
	}
}
