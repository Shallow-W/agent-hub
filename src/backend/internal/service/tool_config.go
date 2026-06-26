package service

import (
	"context"
	"encoding/json"
	"strings"
)

// ToolsetStore 是 toolset 模板校验所需的最小接口。
// 实现侧（如 *repository.ToolDefinitionRepo）通过查询 builtin_toolset_templates
// 表来判断 toolset 名是否合法。
type ToolsetStore interface {
	// IsValidToolset 返回给定 toolset 名是否在 builtin_toolset_templates 中存在。
	IsValidToolset(ctx context.Context, name string) (bool, error)
}

type agentToolsConfig struct {
	Toolset      string   `json:"toolset,omitempty"`
	AllowedTools []string `json:"allowed_tools"`
}

// normalizeToolsConfig validates and normalizes a JSON tool config string.
// registry 用于校验工具名；toolsetStore 用于校验 toolset 名是否在 DB 中存在；
// 任一为 nil 时对应校验环节跳过。
func normalizeToolsConfig(ctx context.Context, raw string, registry ToolRegistryReader, toolsetStore ToolsetStore) (string, error) {
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

	// 校验 toolset 名：
	//   - "none" 始终合法
	//   - 有 ToolsetStore：交给 DB-backed 校验
	//   - 无 ToolsetStore（测试场景）：放行任何非空 toolset，以保持 Agent 单测的
	//     行为（"custom" 之类的自由文本会被上层模板系统进一步解析）。
	if cfg.Toolset == "" {
		// 保持空，下面会保留现状
	} else if cfg.Toolset == "none" {
		// "none" 始终合法
	} else if toolsetStore != nil {
		ok, err := toolsetStore.IsValidToolset(ctx, cfg.Toolset)
		if err != nil {
			return "", err
		}
		if !ok {
			cfg.Toolset = ""
		}
	}
	cfg.AllowedTools = normalizeToolNames(cfg.AllowedTools, registry)

	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// normalizeToolNames filters and deduplicates tool names, keeping only those
// recognized by the registry. When registry is nil, all names pass through.
func normalizeToolNames(names []string, registry ToolRegistryReader) []string {
	if names == nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if registry != nil {
			if _, ok := registry.Lookup(name); !ok {
				continue
			}
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}
