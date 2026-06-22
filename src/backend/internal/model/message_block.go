// Package model: message_block.go
//
// MessageBlock 是流式消息的结构化积木。与 frontend types/message.ts:MessageBlock
// 严格对齐（字段命名 / JSON tag / 兼容空值）。
//
// 设计原则：
//   - 同一 kind 连续 delta 聚合成一个 block（streamingReducer 负责）
//   - text / thinking / tool_use / tool_result / error 共 5 种 kind
//   - tool_use 的入参（partial JSON）累积到 Text 字段，与 frontend 一致
//
// JSON 字段命名沿用 frontend snake_case 习惯（tool_name / tool_use_id / is_error），
// 与 daemon 上行协议字段名保持一致，便于直接 marshal 到 WS payload。
package model

// BlockKind 流式 block 类型枚举。
//
// 取值与 frontend BlockKind 严格一致。任意 LLM 协议（Claude / Codex / ...）
// 的事件最终都归一到这 5 种 kind。
type BlockKind string

const (
	BlockKindText       BlockKind = "text"
	BlockKindThinking   BlockKind = "thinking"
	BlockKindToolUse    BlockKind = "tool_use"
	BlockKindToolResult BlockKind = "tool_result"
	BlockKindError      BlockKind = "error"
)

// MessageBlock 单个累积 block。
//
// 字段说明（与 frontend MessageBlock 对齐）：
//   - Index       block 在 message 内的序号（单调递增，用作 React key / 排序）
//   - Kind        block 类型（见 BlockKind 常量）
//   - Text        累积内容：text/thinking/tool_use 入参/tool_result 输出/error 消息
//   - ToolName    kind=tool_use 的工具名（如 "Read"）
//   - ToolUseID   kind=tool_use 的 tool_use_id（与 tool_result 对齐用）
//   - IsError     kind=tool_result / error 时为 true
//
// 注意：tool_use 的 partial JSON 输入累积到 Text 字段（不是独立 InputJSON 字段），
// 这与 frontend streamingReducer 行为一致（PR1 已锁死）。
type MessageBlock struct {
	Index     int       `json:"index" db:"index"`
	Kind      BlockKind `json:"kind" db:"kind"`
	Text      string    `json:"text,omitempty" db:"text"`
	ToolName  string    `json:"tool_name,omitempty" db:"tool_name"`
	ToolUseID string    `json:"tool_use_id,omitempty" db:"tool_use_id"`
	IsError   bool      `json:"is_error,omitempty" db:"is_error"`
}
