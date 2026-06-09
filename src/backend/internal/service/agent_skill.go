package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	daemonOpenPathTool = "__agenthub_open_path__"
)

// DiscoveredSkill 兼容旧 daemon 的字符串能力，也承载真实 SKILL.md 内容。
type DiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
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
			Trigger:     truncateString(strings.TrimSpace(skill.Trigger), 200),
			Detail:      truncateString(strings.TrimSpace(skill.Detail), 2000),
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

func BuildAgentSkillContext(raw string, taskText string) string {
	skills := parseDiscoveredSkills(raw)
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[平台 Skills]\n")
	sb.WriteString("以下是用户为当前 Agent 分配的平台 Skills。先参考索引判断是否需要使用；只有命中详情时才按详情执行。\n")
	sb.WriteString("{Skill 索引\n")
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(skill.Description)
		if desc == "" {
			desc = "未配置"
		}
		trigger := strings.TrimSpace(skill.Trigger)
		if trigger == "" {
			trigger = "按任务语义判断"
		}
		fmt.Fprintf(&sb, "- %s：%s；触发：%s\n",
			truncateString(normalizePromptLine(name), 80),
			truncateString(normalizePromptLine(desc), 200),
			truncateString(normalizePromptLine(trigger), 200),
		)
	}
	sb.WriteString("}\n")

	matched := matchedSkills(skills, taskText)
	if len(matched) > 0 {
		sb.WriteString("{命中的平台 Skill 详情\n")
		for _, skill := range matched {
			detail := strings.TrimSpace(skill.Detail)
			if detail == "" {
				continue
			}
			fmt.Fprintf(&sb, "## %s\n%s\n",
				truncateString(normalizePromptLine(skill.Name), 80),
				truncateString(detail, 2000),
			)
		}
		sb.WriteString("}\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

func matchedSkills(skills []DiscoveredSkill, taskText string) []DiscoveredSkill {
	text := strings.ToLower(strings.TrimSpace(taskText))
	if text == "" {
		return nil
	}
	matched := make([]DiscoveredSkill, 0, len(skills))
	for _, skill := range skills {
		if strings.TrimSpace(skill.Detail) == "" {
			continue
		}
		if skillMatchesText(skill, text) {
			matched = append(matched, skill)
		}
	}
	return matched
}

func skillMatchesText(skill DiscoveredSkill, text string) bool {
	for _, part := range []string{skill.Name, skill.Trigger} {
		for _, token := range skillMatchTokens(part) {
			if strings.Contains(text, token) {
				return true
			}
		}
	}
	return false
}

func skillMatchTokens(value string) []string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '/' || r == '、' || r == '\n' || r == '\t'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}
