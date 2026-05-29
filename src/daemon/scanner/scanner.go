package scanner

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// AgentInfo 描述本机已发现的 Agent CLI
type AgentInfo struct {
	Name         string   `json:"name"`
	CLITool      string   `json:"cli_tool"`
	CommandPath  string   `json:"command_path"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

// Candidate 是已知 Agent CLI 的扫描配置
type Candidate struct {
	Name         string
	CLITool      string
	Command      string
	Capabilities []string
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
			Capabilities: []string{"coding", "review", "orchestration"},
		},
		{
			Name:         "Codex",
			CLITool:      "codex",
			Command:      "codex",
			Capabilities: []string{"coding", "review"},
		},
		{
			Name:         "OpenCode",
			CLITool:      "opencode",
			Command:      "opencode",
			Capabilities: []string{"coding"},
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
		agents = append(agents, AgentInfo{
			Name:         candidate.Name,
			CLITool:      candidate.CLITool,
			CommandPath:  path,
			Version:      version,
			Capabilities: candidate.Capabilities,
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
