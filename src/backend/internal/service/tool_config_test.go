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
