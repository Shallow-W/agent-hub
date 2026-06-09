package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToolsConfig_FiltersUnknownTools(t *testing.T) {
	raw := `{"toolset":"custom","allowed_tools":["list_tasks","unknown","list_tasks"]}`
	got, err := normalizeToolsConfig(raw)
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
	raw := "## legacy tool docs"
	got, err := normalizeToolsConfig(raw)
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	if got != raw {
		t.Fatalf("expected legacy text preserved, got %q", got)
	}
}

func TestNormalizeToolsConfig_EmptyMeansNoTools(t *testing.T) {
	got, err := normalizeToolsConfig("")
	if err != nil {
		t.Fatalf("normalizeToolsConfig error: %v", err)
	}
	if got != `{"toolset":"none","allowed_tools":[]}` {
		t.Fatalf("expected no-tools config, got %q", got)
	}
}

func TestNormalizeToolsConfig_PreservesNoneAndEmptyAllowedTools(t *testing.T) {
	got, err := normalizeToolsConfig(`{"toolset":"none","allowed_tools":[]}`)
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
	got, err := normalizeToolsConfig(`{"toolset":"basic"}`)
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

func TestPlatformToolCatalogIncludesTemplateTools(t *testing.T) {
	for toolset, tools := range platformToolsets {
		for _, tool := range tools {
			if !platformToolCatalog[tool] {
				t.Fatalf("toolset %s references unknown tool %s", toolset, tool)
			}
		}
	}
}
