package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const daemonSkillSyncTool = "__agenthub_skill_sync__"

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

func syncSkillFiles(capabilitiesJSON string) error {
	var skills []DiscoveredSkill
	if err := json.Unmarshal([]byte(capabilitiesJSON), &skills); err != nil {
		return nil
	}
	for _, skill := range skills {
		sourcePath := strings.TrimSpace(skill.SourcePath)
		if sourcePath == "" {
			continue
		}
		if err := writeSkillFile(sourcePath, skill.Detail); err != nil {
			return err
		}
	}
	return nil
}

func validateDaemonSkillFiles(previousJSON, nextJSON string) error {
	allowed := make(map[string]bool)
	for _, skill := range parseDiscoveredSkills(previousJSON) {
		sourcePath := strings.TrimSpace(skill.SourcePath)
		if sourcePath != "" {
			allowed[sourcePath] = true
		}
	}
	for _, skill := range parseDiscoveredSkills(nextJSON) {
		sourcePath := strings.TrimSpace(skill.SourcePath)
		if sourcePath == "" {
			continue
		}
		if !allowed[sourcePath] {
			return ErrAgentInvalidInput
		}
	}
	return nil
}

func parseDiscoveredSkills(capabilitiesJSON string) []DiscoveredSkill {
	var skills []DiscoveredSkill
	if err := json.Unmarshal([]byte(capabilitiesJSON), &skills); err != nil {
		return nil
	}
	return skills
}

func writeSkillFile(sourcePath, detail string) error {
	cleanPath, err := filepath.Abs(filepath.Clean(sourcePath))
	if err != nil {
		return fmt.Errorf("resolve skill path: %w", err)
	}
	if filepath.Base(cleanPath) != "SKILL.md" {
		return ErrAgentInvalidInput
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("stat skill file: %w", err)
	}
	if info.IsDir() {
		return ErrAgentInvalidInput
	}
	if err := os.WriteFile(cleanPath, []byte(detail), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return nil
}
