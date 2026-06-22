// Package model: agent_event.go
//
// AgentEvent 是三端共享的流式事件契约（daemon JS 源、frontend TS 镜像、backend Go 镜像）。
//
// 与 frontend src/types/agentEvent.ts AgentEvent discriminated union 严格对齐：
// payload 字段直接挂在 event 顶层（不用 Data 嵌套），不同 Type 使用不同字段，
// 未使用的字段保持零值（omitempty 保证序列化干净）。
//
// 字段说明：
//   - Type    事件类型（必填，见 AgentEvent* 常量）
//   - Seq     daemon 自增序号（可选，reducer 不依赖）
//   - Adapter 产出事件的 adapter 名（'claude' / 'codex' / ...）
//   - Ts      时间戳（可选）
//   - payload 字段（按 Type 使用不同字段，见下方注释）
package model

import "time"

// AgentEvent 是三端共享的流式事件契约。
//
// payload 字段直接在 event 顶层（与 TS AgentEvent discriminated union 对齐，
// 也与 daemon cli/events.js 的 flat 事件构造方式对齐）。
// 不同 Type 使用不同字段，未使用的字段保持零值。
//
// 字段 → Type 映射：
//   - Content         text / thinking（也兼容 tool_result.content）
//   - Text            兼容老格式 text.payload.text
//   - Tool            tool_use（工具名）
//   - ToolUseID       tool_use / tool_result 关联 ID
//   - ToolUseIDAlt    兼容 daemon camelCase 'toolUseID'
//   - Input           tool_use partial_json（老协议）
//   - Delta           tool.call.input.delta（新协议）
//   - Output          tool_result.output
//   - Message         error.message
//   - Reason          cancel.reason
//   - IsError         tool_result.is_error
//   - IsErrorAlt      兼容 camelCase 'isError'
//   - Result          turn_end.result（reducer 当前不消费，保留透传）
//   - Code            session_end.code（reducer 当前不消费，保留透传）
type AgentEvent struct {
	Type        string      `json:"type"`
	Seq         int64       `json:"seq,omitempty"`
	Adapter     string      `json:"adapter,omitempty"`
	Ts          time.Time   `json:"ts,omitempty"`
	Content     string      `json:"content,omitempty"`
	Text        string      `json:"text,omitempty"`
	Tool        string      `json:"tool,omitempty"`
	ToolUseID   string      `json:"tool_use_id,omitempty"`
	ToolUseIDAlt string     `json:"toolUseID,omitempty"`
	Input       string      `json:"input,omitempty"`
	Delta       string      `json:"delta,omitempty"`
	Output      string      `json:"output,omitempty"`
	Message     string      `json:"message,omitempty"`
	Reason      string      `json:"reason,omitempty"`
	IsError     bool        `json:"is_error,omitempty"`
	IsErrorAlt  bool        `json:"isError,omitempty"`
	Result      interface{} `json:"result,omitempty"`
	Code        int         `json:"code,omitempty"`
}

// Type 常量（与 frontend agentEvent.ts AgentEventType 对齐）。
//
// 命名分组：
//   - 新命名（dot.case）：未来 PR5 daemon 切换到此命名
//   - 老命名（snake_case）：兼容当前 daemon 线上协议
const (
	// 新命名（dot.case）
	AgentEventSessionStart  = "session.start"
	AgentEventSessionEndNew = "session.end"
	AgentEventTextDelta     = "text.delta"
	AgentEventThinkingDelta = "thinking.delta"
	AgentEventToolCallStart = "tool.call.start"
	AgentEventToolCallInput = "tool.call.input"
	AgentEventToolCallEnd   = "tool.call.end"
	AgentEventToolResultNew = "tool.result"
	AgentEventError         = "error"
	AgentEventCancel        = "cancel"

	// 老命名（snake_case）—— 兼容当前 daemon 线上协议
	AgentEventText          = "text"
	AgentEventThinking      = "thinking"
	AgentEventToolUse       = "tool_use"
	AgentEventToolResultOld = "tool_result"
	AgentEventTurnEnd       = "turn_end"
	AgentEventSessionEndOld = "session_end"
)

// ToolUseIDOrAlt 返回 tool_use_id（优先 snake_case，回退 camelCase）。
// 便于 reducer / handler 双兼容处理。
func (e AgentEvent) ToolUseIDOrAlt() string {
	if e.ToolUseID != "" {
		return e.ToolUseID
	}
	return e.ToolUseIDAlt
}

// IsErrorOrAlt 返回 is_error（优先 snake_case，回退 camelCase）。
func (e AgentEvent) IsErrorOrAlt() bool {
	return e.IsError || e.IsErrorAlt
}

// IsTextDelta 判断事件是否为文本增量（text 或 text.delta）。
// 便于 reducer 调用方在不解析 payload 的情况下快速分支。
func (e AgentEvent) IsTextDelta() bool {
	return e.Type == AgentEventText || e.Type == AgentEventTextDelta
}

// IsThinkingDelta 判断事件是否为思考增量（thinking 或 thinking.delta）。
func (e AgentEvent) IsThinkingDelta() bool {
	return e.Type == AgentEventThinking || e.Type == AgentEventThinkingDelta
}

// IsTerminal 判断事件是否为终态信号（turn_end / session.end / session_end / error / cancel）。
// reducer 据此切换 StreamingState.Status。
func (e AgentEvent) IsTerminal() bool {
	switch e.Type {
	case AgentEventTurnEnd,
		AgentEventSessionEndNew,
		AgentEventSessionEndOld,
		AgentEventError,
		AgentEventCancel:
		return true
	default:
		return false
	}
}
