// Package model: agent_event.go
//
// AgentEvent 是三端共享的流式事件契约（daemon JS 源、frontend TS 镜像、backend Go 镜像）。
// PR4 会实现 Go 版 streamingReducer；此文件仅定义类型与 Type 常量，不含 reducer 逻辑。
//
// 与 frontend src/types/agentEvent.ts AgentEventType 严格对齐：
// 新命名（dot.case）+ 老命名（snake_case）双兼容。
//
// 字段说明：
//   - Type    事件类型（必填，见 AgentEvent* 常量）
//   - Seq     daemon 自增序号（可选，reducer 不依赖）
//   - Adapter 产出事件的 adapter 名（'claude' / 'codex' / ...）
//   - Ts      时间戳（可选）
//   - Data    按 Type 收窄解析的 payload；调用方自行 unmarshal 到对应 struct
package model

import (
	"encoding/json"
	"time"
)

// AgentEvent 是三端共享的流式事件契约。
//
// 注意：Data 字段保留原始 JSON，调用方按 Type 收窄解析。
// 这样避免在 Go 镜像里定义 10 个不同 struct，同时保留协议可演进性
// （新增 Type 时旧 consumer 仍能透传）。
type AgentEvent struct {
	Type    string          `json:"type"`
	Seq     int64           `json:"seq,omitempty"`
	Adapter string          `json:"adapter,omitempty"`
	Ts      time.Time       `json:"ts,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Type 常量（与 frontend agentEvent.ts AgentEventType 对齐）。
//
// 命名分组：
//   - 新命名（dot.case）：未来 PR5 daemon 切换到此命名
//   - 老命名（snake_case）：兼容当前 daemon 线上协议
const (
	// 新命名（dot.case）
	AgentEventSessionStart   = "session.start"
	AgentEventSessionEndNew  = "session.end"
	AgentEventTextDelta      = "text.delta"
	AgentEventThinkingDelta  = "thinking.delta"
	AgentEventToolCallStart  = "tool.call.start"
	AgentEventToolCallInput  = "tool.call.input"
	AgentEventToolCallEnd    = "tool.call.end"
	AgentEventToolResultNew  = "tool.result"
	AgentEventError          = "error"
	AgentEventCancel         = "cancel"

	// 老命名（snake_case）—— 兼容当前 daemon 线上协议
	AgentEventText          = "text"
	AgentEventThinking      = "thinking"
	AgentEventToolUse       = "tool_use"
	AgentEventToolResultOld = "tool_result"
	AgentEventTurnEnd       = "turn_end"
	AgentEventSessionEndOld = "session_end"
)

// IsTextDelta 判断事件是否为文本增量（text 或 text.delta）。
// 便于 reducer 调用方在不解析 Data 的情况下快速分支。
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
