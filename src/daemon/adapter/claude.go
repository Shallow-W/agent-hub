package adapter

// NewClaudeAdapter 创建 Claude Code CLI 适配器
func NewClaudeAdapter() *CommandAdapter {
	return NewCommandAdapter("Claude Code", "claude", nil)
}
