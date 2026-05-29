package adapter

// NewCodexAdapter 创建 Codex CLI 适配器
func NewCodexAdapter() *CommandAdapter {
	return NewCommandAdapter("Codex", "codex", nil)
}

// NewOpenCodeAdapter 创建 OpenCode CLI 适配器
func NewOpenCodeAdapter() *CommandAdapter {
	return NewCommandAdapter("OpenCode", "opencode", nil)
}
