package service

import (
	"encoding/json"
	"strings"
)

const (
	daemonOpenPathTool = "__agenthub_open_path__"
)

// DiscoveredSkill 兼容旧 daemon 的字符串能力，也承载真实 SKILL.md 内容。
type DiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Detail      string `json:"detail,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	Auto        bool   `json:"auto,omitempty"`
}

func (s *DiscoveredSkill) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		s.Name = name
		s.Auto = true
		return nil
	}
	type skillAlias DiscoveredSkill
	var parsed skillAlias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*s = DiscoveredSkill(parsed)
	return nil
}

func hasDiscoveredSkillSource(capabilitiesJSON, sourcePath string) bool {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return false
	}
	for _, skill := range parseDiscoveredSkills(capabilitiesJSON) {
		if strings.TrimSpace(skill.SourcePath) == sourcePath {
			return true
		}
	}
	return false
}

func parseDiscoveredSkills(capabilitiesJSON string) []DiscoveredSkill {
	var skills []DiscoveredSkill
	if err := json.Unmarshal([]byte(capabilitiesJSON), &skills); err != nil {
		return nil
	}
	return skills
}

func normalizeCustomSkills(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var incoming []DiscoveredSkill
	if err := json.Unmarshal([]byte(raw), &incoming); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	out := make([]DiscoveredSkill, 0, len(incoming))
	for _, skill := range incoming {
		name := strings.TrimSpace(skill.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, DiscoveredSkill{
			Name:        truncateString(name, 80),
			Description: truncateString(strings.TrimSpace(skill.Description), 200),
		})
	}
	if len(out) == 0 {
		return "", nil
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
