package scanner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const minDescriptionChars = 12

// SkillInfo 描述本机 Agent 暴露的真实 skill 文件
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Detail      string `json:"detail,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	Auto        bool   `json:"auto,omitempty"`
}

// AgentInfo 描述本机已发现的 Agent CLI
type AgentInfo struct {
	Name         string      `json:"name"`
	CLITool      string      `json:"cli_tool"`
	CommandPath  string      `json:"command_path"`
	Version      string      `json:"version"`
	Capabilities []SkillInfo `json:"capabilities"`
}

// Candidate 是已知 Agent CLI 的扫描配置
type Candidate struct {
	Name         string
	CLITool      string
	Command      string
	Capabilities []SkillInfo
}

// Scanner 扫描 PATH 中可用的 Agent CLI
type Scanner struct {
	candidates []Candidate
	timeout    time.Duration
}

// DefaultCandidates 返回当前项目要求支持的主流 Agent CLI
func DefaultCandidates() []Candidate {
	return []Candidate{
		{
			Name:         "Claude Code",
			CLITool:      "claude",
			Command:      "claude",
			Capabilities: defaultSkills("coding", "review", "orchestration"),
		},
		{
			Name:         "Codex",
			CLITool:      "codex",
			Command:      "codex",
			Capabilities: defaultSkills("coding", "review"),
		},
		{
			Name:         "OpenCode",
			CLITool:      "opencode",
			Command:      "opencode",
			Capabilities: defaultSkills("coding"),
		},
		{
			Name:         "OpenClaw",
			CLITool:      "openclaw",
			Command:      "openclaw",
			Capabilities: defaultSkills("coding"),
		},
	}
}

// New 创建默认扫描器
func New(candidates []Candidate) *Scanner {
	if len(candidates) == 0 {
		candidates = DefaultCandidates()
	}
	return &Scanner{
		candidates: candidates,
		timeout:    3 * time.Second,
	}
}

// Scan 扫描 PATH，并用 --version 验证命令可执行
func (s *Scanner) Scan(ctx context.Context) ([]AgentInfo, error) {
	agents := make([]AgentInfo, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		path, err := exec.LookPath(candidate.Command)
		if err != nil {
			continue
		}
		version := s.readVersion(ctx, candidate.Command)
		capabilities := s.readSkills(candidate)
		if len(capabilities) == 0 {
			capabilities = candidate.Capabilities
		}
		agents = append(agents, AgentInfo{
			Name:         candidate.Name,
			CLITool:      candidate.CLITool,
			CommandPath:  path,
			Version:      version,
			Capabilities: capabilities,
		})
	}
	return agents, nil
}

func (s *Scanner) readVersion(parent context.Context, command string) string {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, command, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (s *Scanner) readSkills(candidate Candidate) []SkillInfo {
	roots := skillRoots(candidate.CLITool)
	skills := make([]SkillInfo, 0)
	seen := make(map[string]bool)
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "SKILL.md" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			skill := parseSkillFile(filepath.Base(filepath.Dir(path)), path, string(content))
			key := strings.ToLower(skill.Name)
			if key == "" || seen[key] {
				return nil
			}
			seen[key] = true
			skills = append(skills, skill)
			return nil
		})
	}
	return skills
}

func skillRoots(cliTool string) []string {
	wd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	roots := make([]string, 0, 4)
	includeProjectRoots := !isAgentHubWorkspace(wd)
	switch cliTool {
	case "claude":
		if includeProjectRoots {
			roots = appendUniqueRoot(roots, filepath.Join(wd, ".claude", "skills"))
		}
		if home != "" {
			roots = appendUniqueRoot(roots, filepath.Join(home, ".claude", "skills"))
			roots = appendUniqueRoot(roots, filepath.Join(home, ".claude", "plugins", "marketplaces"))
			roots = appendUniqueRoot(roots, filepath.Join(home, ".claude", "plugins", "cache"))
		}
	case "codex":
		if includeProjectRoots {
			roots = appendUniqueRoot(roots, filepath.Join(wd, ".agents", "skills"))
		}
		if home != "" {
			roots = appendUniqueRoot(roots, filepath.Join(home, ".codex", "skills"))
		}
	case "opencode", "openclaw":
		if includeProjectRoots {
			roots = appendUniqueRoot(roots, filepath.Join(wd, ".opencode", "skills"))
			roots = appendUniqueRoot(roots, filepath.Join(wd, ".openclaw", "skills"))
		}
		if home != "" {
			roots = appendUniqueRoot(roots, filepath.Join(home, ".opencode", "skills"))
			roots = appendUniqueRoot(roots, filepath.Join(home, ".openclaw", "skills"))
			roots = appendUniqueRoot(roots, filepath.Join(home, ".openclaw", "plugin-skills"))
			roots = append(roots, openClawInstallSkillRoots(home)...)
		}
	}
	return roots
}

func isAgentHubWorkspace(root string) bool {
	if root == "" {
		return false
	}
	daemonPackage := filepath.Join(root, "src", "daemon-npm", "package.json")
	frontendPackage := filepath.Join(root, "src", "frontend", "package.json")
	if _, err := os.Stat(frontendPackage); err != nil {
		return false
	}
	data, err := os.ReadFile(daemonPackage)
	if err != nil {
		return false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return pkg.Name == "@agenthub/daemon"
}

func appendUniqueRoot(roots []string, root string) []string {
	if root == "" {
		return roots
	}
	for _, existing := range roots {
		if existing == root {
			return roots
		}
	}
	return append(roots, root)
}

func openClawInstallSkillRoots(home string) []string {
	data, err := os.ReadFile(filepath.Join(home, ".openclaw", "plugins", "installs.json"))
	if err != nil {
		return nil
	}
	var installs struct {
		InstallRecords map[string]struct {
			InstallPath string `json:"installPath"`
			SourcePath  string `json:"sourcePath"`
		} `json:"installRecords"`
	}
	if err := json.Unmarshal(data, &installs); err != nil {
		return nil
	}
	roots := make([]string, 0, len(installs.InstallRecords))
	for _, record := range installs.InstallRecords {
		if record.InstallPath != "" {
			roots = appendUniqueRoot(roots, filepath.Join(record.InstallPath, "skills"))
		}
		if record.SourcePath != "" {
			roots = appendUniqueRoot(roots, filepath.Join(record.SourcePath, "skills"))
		}
	}
	return roots
}

func parseSkillFile(fallbackName, path, content string) SkillInfo {
	skill := SkillInfo{Name: fallbackName, Detail: content, SourcePath: path, Auto: true}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		skill.Description = normalizeSkillDescription(skill.Name, skill.Description, content)
		return skill
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			if value != "" {
				skill.Name = value
			}
		case "description":
			skill.Description = value
		}
	}
	skill.Description = normalizeSkillDescription(skill.Name, skill.Description, content)
	return skill
}

func normalizeSkillDescription(name, description, content string) string {
	current := strings.TrimSpace(description)
	if isUsefulDescription(current) {
		return current
	}
	return inferSkillDescription(name, content)
}

func isUsefulDescription(description string) bool {
	description = strings.TrimSpace(description)
	if description == "" {
		return false
	}
	count := 0
	for _, r := range description {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		count++
	}
	return count >= minDescriptionChars
}

func inferSkillDescription(name, content string) string {
	body := stripFrontmatter(content)
	lines := strings.Split(body, "\n")
	chunks := make([]string, 0, 3)
	inFence := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		line = cleanMarkdownLine(line)
		if line == "" || strings.EqualFold(line, name) {
			continue
		}
		chunks = append(chunks, line)
		if len(strings.Join(chunks, " ")) >= 120 {
			break
		}
	}
	summary := truncateDescription(strings.Join(chunks, " "))
	if summary != "" {
		return summary
	}
	if name == "" {
		name = "selected"
	}
	return "Provides the " + name + " skill for local Agent workflows."
}

func stripFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}

func cleanMarkdownLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "#")
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "-*+>")
	line = strings.TrimSpace(line)
	if index := strings.Index(line, "]("); index > 0 && strings.HasPrefix(line, "[") {
		closeLabel := strings.Index(line, "]")
		closeURL := strings.Index(line, ")")
		if closeLabel > 1 && closeURL > closeLabel {
			line = line[1:closeLabel] + line[closeURL+1:]
		}
	}
	replacer := strings.NewReplacer("`", "", "**", "", "__", "", "*", "", "_", "")
	return strings.TrimSpace(replacer.Replace(line))
}

func truncateDescription(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= 180 {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:177])) + "..."
}

func defaultSkills(names ...string) []SkillInfo {
	skills := make([]SkillInfo, 0, len(names))
	for _, name := range names {
		skills = append(skills, SkillInfo{Name: name, Auto: true})
	}
	return skills
}
