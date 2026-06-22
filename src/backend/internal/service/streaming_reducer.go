// Package service: streaming_reducer.go
//
// streaming_reducer 是流式 AgentEvent → StreamingState 的纯函数 reducer（Go 版）。
//
// 设计目标：
//   - 纯函数（不修改入参 state），返回新对象
//   - 与 frontend src/store/streamingReducer.ts 严格对齐：
//     * 字段命名 / 累积策略 / 边界条件 / 终态保护
//     * 同步 TS 的 12 个测试场景（streaming_reducer_test.go）
//   - 双兼容老 snake_case 与新 dot.case 事件类型
//
// 使用方式：
//   state := InitialStreamingState()
//   state = ReduceEvents(events, state)
//   // state.Blocks / state.Status
//
// 注意：与 PR1 实现的 TS 版本是双端权威镜像——修改任一方必须同步另一方 + 同步测试。
package service

import (
	"encoding/json"

	"github.com/agent-hub/backend/internal/model"
)

// StreamingState 是 reducer 累积的状态，与 frontend StreamingState 对齐。
//
// 字段说明：
//   - Blocks  当前累积的 block 列表（按 Index 单调递增）
//   - Status  当前状态（streaming / complete / error / canceled）
//   - TaskID  关联的 daemon task_id（用于 StopButton 取消；可选）
//   - AgentID 产出该消息的 agent ID（用于前端展示 agent name；可选）
//
// 纯函数契约：调用方修改返回的 state 不影响 buffer 内部状态。
// StreamingBuffer 通过 *StreamingState 持有，GetState 返回副本。
type StreamingState struct {
	Blocks  []model.MessageBlock `json:"blocks"`
	Status  string               `json:"status"`
	TaskID  string               `json:"task_id,omitempty"`
	AgentID string               `json:"agent_id,omitempty"`
}

// InitialStreamingState 初始状态：空 block 列表 + status='streaming'。
func InitialStreamingState() StreamingState {
	return StreamingState{
		Blocks: []model.MessageBlock{},
		Status: model.MessageStatusStreaming,
	}
}

// StreamingReducer 纯函数：把单个 AgentEvent 应用到 state，返回新 state。
//
// 累积规则（与 frontend streamingReducer.ts 严格对齐）：
//   - text / text.delta：最后一个 block.kind === 'text' 时累积 text；否则新建
//   - thinking / thinking.delta：同 text，累积到 thinking block
//   - tool_use / tool.call.start：
//     * tool 非空 → 新 tool_use block（带 tool_name / tool_use_id）
//     * tool 为空 → input_json_delta，追加到最后一个 tool_use block 的 Text
//       （注意：当前线上 daemon 把 partial_json 放在 `input` 字段，但 appendDeltas
//       读 `content`；reducer 为了保持运行时行为一致，也优先读 `content`，
//       其次读 `input`（若为 string）。）
//   - tool.call.input：按 tool_use_id 路由（找不到回退到最后一个 tool_use block），
//     追加 delta 到目标 block 的 Text
//   - tool.call.end：no-op（block 边界，不累积）
//   - tool_result / tool.result：总是新 block
//   - error：总是新 block，Status='error'
//   - cancel：Status='canceled'，不产生 block
//   - turn_end / session.end / session_end：Status='complete'，不产生 block
//   - session.start：no-op（agent 元信息走 store 层 meta 通道）
//   - default：未知类型忽略，不破坏流
//
// 终态保护：Status != 'streaming' 时直接返回原 state（与 TS 版一致）。
func StreamingReducer(state StreamingState, event model.AgentEvent) StreamingState {
	// 终态保护：一旦进入 complete/error/canceled，后续事件忽略。
	if state.Status != model.MessageStatusStreaming {
		return state
	}

	switch event.Type {
	case model.AgentEventText, model.AgentEventTextDelta:
		return applyTextDelta(state, event)

	case model.AgentEventThinking, model.AgentEventThinkingDelta:
		return applyThinkingDelta(state, event)

	case model.AgentEventToolUse, model.AgentEventToolCallStart:
		return applyToolUseStart(state, event)

	case model.AgentEventToolCallInput:
		return applyToolCallInput(state, event)

	case model.AgentEventToolCallEnd:
		// block 边界，不产生新 block（与 content_block_stop 同语义）
		return state

	case model.AgentEventToolResultOld, model.AgentEventToolResultNew:
		return applyToolResult(state, event)

	case model.AgentEventError:
		return applyError(state, event)

	case model.AgentEventCancel:
		// 不产生 block，直接切状态
		state.Status = model.MessageStatusCanceled
		return state

	case model.AgentEventTurnEnd, model.AgentEventSessionEndNew, model.AgentEventSessionEndOld:
		state.Status = model.MessageStatusComplete
		return state

	case model.AgentEventSessionStart:
		// session 元数据事件：当前 reducer 不消费
		return state

	default:
		// 未知事件类型忽略，不破坏流
		return state
	}
}

// ReduceEvents 批量 reduce：把 events 数组依次应用到 state。
//
// 与 frontend reduceEvents 一致：
//   - 空数组返回原 state（语义不变）
//   - 非 nil 初始状态：从该状态开始累积
//   - nil / 空初始状态：从 InitialStreamingState 开始
func ReduceEvents(events []model.AgentEvent, initialState StreamingState) StreamingState {
	if len(events) == 0 {
		return initialState
	}
	state := initialState
	if state.Blocks == nil {
		// 防御性：确保 Blocks 非 nil，避免后续 append 写入 nil slice 后返回时序列化出 "null"
		state.Blocks = []model.MessageBlock{}
	}
	if state.Status == "" {
		state.Status = model.MessageStatusStreaming
	}
	for _, ev := range events {
		state = StreamingReducer(state, ev)
	}
	return state
}

// applyTextDelta 处理 text / text.delta 事件。
//
// 解析 event.Data 为 { content: string }；content 为空时 no-op。
// 累积到最后一个 text block（若末尾不是 text 则新建）。
func applyTextDelta(state StreamingState, event model.AgentEvent) StreamingState {
	content := extractStringField(event.Data, "content")
	if content == "" {
		return state
	}
	blocks := cloneBlocks(state.Blocks)
	if n := len(blocks); n > 0 && blocks[n-1].Kind == model.BlockKindText {
		blocks[n-1].Text += content
	} else {
		blocks = append(blocks, model.MessageBlock{
			Index: nextIndex(blocks),
			Kind:  model.BlockKindText,
			Text:  content,
		})
	}
	state.Blocks = blocks
	return state
}

// applyThinkingDelta 处理 thinking / thinking.delta 事件。
//
// 行为同 applyTextDelta，但累积到 kind='thinking' 的 block。
func applyThinkingDelta(state StreamingState, event model.AgentEvent) StreamingState {
	content := extractStringField(event.Data, "content")
	if content == "" {
		return state
	}
	blocks := cloneBlocks(state.Blocks)
	if n := len(blocks); n > 0 && blocks[n-1].Kind == model.BlockKindThinking {
		blocks[n-1].Text += content
	} else {
		blocks = append(blocks, model.MessageBlock{
			Index: nextIndex(blocks),
			Kind:  model.BlockKindThinking,
			Text:  content,
		})
	}
	state.Blocks = blocks
	return state
}

// applyToolUseStart 处理 tool_use / tool.call.start 事件。
//
//   - tool 非空 → 新 tool_use block（带 tool_name / tool_use_id）
//   - tool 为空 → input_json_delta（老协议）：
//     当前 daemon 把 partial_json 放 `input` 字段，appendDeltas 读 `content`；
//     reducer 双兼容：优先 `content`，其次 `input`（若为 string）。
//     追加到最后一个 tool_use block 的 Text。找不到 tool_use block 容错：忽略。
func applyToolUseStart(state StreamingState, event model.AgentEvent) StreamingState {
	blocks := cloneBlocks(state.Blocks)
	toolName := extractStringField(event.Data, "tool")
	if toolName != "" {
		// 工具名非空 → 开启新 tool_use block
		toolUseID := extractStringField(event.Data, "tool_use_id")
		blocks = append(blocks, model.MessageBlock{
			Index:     nextIndex(blocks),
			Kind:      model.BlockKindToolUse,
			Text:      "",
			ToolName:  toolName,
			ToolUseID: toolUseID,
		})
		state.Blocks = blocks
		return state
	}
	// 空工具名 → input_json_delta（老协议）
	inputDelta := extractStringField(event.Data, "content")
	if inputDelta == "" {
		inputDelta = extractStringField(event.Data, "input")
	}
	if inputDelta == "" {
		return state
	}
	if n := len(blocks); n > 0 && blocks[n-1].Kind == model.BlockKindToolUse {
		blocks[n-1].Text += inputDelta
		state.Blocks = blocks
		return state
	}
	// 找不到 tool_use block 容错：忽略（与 TS 版一致）
	return state
}

// applyToolCallInput 处理 tool.call.input（新协议 partial JSON 增量）。
//
// 找到匹配 tool_use_id 的 tool_use block；找不到则回退到最后一个 tool_use block。
// 找不到任何 tool_use block 时 no-op。
func applyToolCallInput(state StreamingState, event model.AgentEvent) StreamingState {
	delta := extractStringField(event.Data, "delta")
	if delta == "" {
		return state
	}
	blocks := cloneBlocks(state.Blocks)
	toolUseID := extractStringField(event.Data, "tool_use_id")
	idx := findToolUseBlock(blocks, toolUseID)
	if idx == -1 {
		return state
	}
	blocks[idx].Text += delta
	state.Blocks = blocks
	return state
}

// applyToolResult 处理 tool_result / tool.result 事件。
//
// 总是新 block。output / content 双兼容；is_error / isError 双兼容。
// is_error=true 时同时切 Status='error'（与 TS 版一致）。
func applyToolResult(state StreamingState, event model.AgentEvent) StreamingState {
	output := extractStringField(event.Data, "output")
	if output == "" {
		output = extractStringField(event.Data, "content")
	}
	isError := extractBoolField(event.Data, "is_error")
	if !isError {
		isError = extractBoolField(event.Data, "isError")
	}
	toolUseID := extractStringField(event.Data, "tool_use_id")
	blocks := cloneBlocks(state.Blocks)
	blocks = append(blocks, model.MessageBlock{
		Index:     nextIndex(blocks),
		Kind:      model.BlockKindToolResult,
		Text:      output,
		ToolUseID: toolUseID,
		IsError:   isError,
	})
	state.Blocks = blocks
	if isError {
		state.Status = model.MessageStatusError
	}
	return state
}

// applyError 处理 error 事件。
//
// 总是新 block（kind='error'）+ Status='error'。
// message 字段为空时使用固定文案（与 TS 版一致：'生成失败'）。
func applyError(state StreamingState, event model.AgentEvent) StreamingState {
	message := extractStringField(event.Data, "message")
	if message == "" {
		message = "生成失败"
	}
	blocks := cloneBlocks(state.Blocks)
	blocks = append(blocks, model.MessageBlock{
		Index:   nextIndex(blocks),
		Kind:    model.BlockKindError,
		Text:    message,
		IsError: true,
	})
	state.Blocks = blocks
	state.Status = model.MessageStatusError
	return state
}

// nextIndex 计算新 block 的 index：空数组返回 0，否则 last.index + 1。
func nextIndex(blocks []model.MessageBlock) int {
	if len(blocks) == 0 {
		return 0
	}
	return blocks[len(blocks)-1].Index + 1
}

// findToolUseBlock 查找 tool_use block：
// 优先匹配 tool_use_id（从后向前），找不到则回退到最后一个 tool_use block。
// 仍找不到返回 -1。
func findToolUseBlock(blocks []model.MessageBlock, toolUseID string) int {
	if toolUseID != "" {
		for i := len(blocks) - 1; i >= 0; i-- {
			if blocks[i].Kind == model.BlockKindToolUse && blocks[i].ToolUseID == toolUseID {
				return i
			}
		}
	}
	// 回退：最后一个 tool_use block
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Kind == model.BlockKindToolUse {
			return i
		}
	}
	return -1
}

// cloneBlocks 复制一份 blocks slice（深拷贝元素，避免修改入参）。
//
// 纯函数契约：reducer 不能修改入参 state 的任何子结构。
// 返回的新 slice 元素是副本（struct 值拷贝），修改新 slice 的元素不影响原 slice。
func cloneBlocks(blocks []model.MessageBlock) []model.MessageBlock {
	if blocks == nil {
		return []model.MessageBlock{}
	}
	out := make([]model.MessageBlock, len(blocks))
	copy(out, blocks)
	return out
}

// extractStringField 从 json.RawMessage 提取 string 字段。
//
// 容错策略：
//   - data 为 nil / 空 → 返回 ""
//   - JSON 解析失败 → 返回 ""
//   - 字段不存在 / 类型不是 string → 返回 ""
func extractStringField(data json.RawMessage, field string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// extractBoolField 从 json.RawMessage 提取 bool 字段。
// 容错策略同 extractStringField，缺省返回 false。
func extractBoolField(data json.RawMessage, field string) bool {
	if len(data) == 0 {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	v, ok := m[field]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}
