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
// 注意：与 frontend TS 版本是双端权威镜像——修改任一方必须同步另一方 + 同步测试。
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
//   - tool_result / tool.result：总是新 block，记录 is_error 标记
//     （注意：与早期版本不同，is_error=true 不再切 Status='error'——agent 在
//     工具失败后通常仍会输出总结文本 + agenthub 卡片，提前切 error 会触发
//     终态保护丢弃后续 text.delta / turn_end 事件，导致消息截断。真正的流级
//     失败由独立的 error 事件表达，与 TS 版前端 reducer 严格对齐。）
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
// payload 字段：Content（兼容老格式 Text）；content 为空时 no-op。
// 累积到最后一个 text block（若末尾不是 text 则新建）。
func applyTextDelta(state StreamingState, event model.AgentEvent) StreamingState {
	content := event.Content
	if content == "" {
		content = event.Text
	}
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
	content := event.Content
	if content == "" {
		content = event.Text
	}
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
//     此分支对应 daemon 的 content_block_start 路径（toolUseEvent(name, {}, id)），
//     此时 Input 是空对象占位 `{}`，不应作为 delta 追加——直接忽略 Input 字段。
//   - tool 为空 → input_json_delta（老协议）：
//     当前 daemon 把 partial_json 放 `input` 字段（events.js toolUseEvent('',
//     partial_json) → input = string partial）。reducer 双兼容：优先 `content`，
//     其次 `input`（RawMessage → 解包为 string）。
//     追加到最后一个 tool_use block 的 Text。找不到 tool_use block 容错：忽略。
//
// 注意：Input 改为 json.RawMessage 后（Bug 3 修复），content_block_delta 路径的
// daemon 发送 `input: "<partial_json>"`，序列化为 JSON string `"{"cmd"...""`。
// Unmarshal 到 RawMessage 后 bytes 形如 `"{\"cmd\":..."`（带外层引号和转义）。
// 直接 `string(event.Input)` 会保留这些字面引号和反斜杠 → tool_use.Text 出现双重
// 转义。这里用 json.Unmarshal 把 RawMessage 再解一层：若是 JSON string 则得到
// 原始 Go string；若是 object/array/number 等非 string 形态则 fallback 用原文
// string() 转换（保留旧行为兼容，例如测试直接构造 RawMessage(`{"cmd":"ls"}`) 的 case）。
func applyToolUseStart(state StreamingState, event model.AgentEvent) StreamingState {
	blocks := cloneBlocks(state.Blocks)
	if event.Tool != "" {
		// 工具名非空 → 开启新 tool_use block（content_block_start 路径）。
		// 忽略 Input 字段：content_block_start 时 daemon 发的是 `input: {}` 空对象占位，
		// 不应作为 partial_json delta 追加（真实 input 通过后续 content_block_delta 累积）。
		blocks = append(blocks, model.MessageBlock{
			Index:     nextIndex(blocks),
			Kind:      model.BlockKindToolUse,
			Text:      "",
			ToolName:  event.Tool,
			ToolUseID: event.ToolUseIDOrAlt(),
		})
		state.Blocks = blocks
		return state
	}
	// 空工具名 → input_json_delta（content_block_delta 路径）
	inputDelta := event.Content
	if inputDelta == "" && len(event.Input) > 0 {
		inputDelta = decodeInputRawMessage(event.Input)
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

// decodeInputRawMessage 把 Input RawMessage 解包为 partial_json 字符串。
//
// daemon events.js toolUseEvent('', partial_json) 让 input 字段为 JS string，
// JSON.stringify 后是带引号的 JSON string（如 `"{\"cmd\":\"ls\"}"`）。Unmarshal
// 进 RawMessage 后 bytes 仍是带引号的 JSON 表示。直接 string() 转换会保留外层
// 引号和转义 → tool_use.Text 累积成 `"{"cmd":"ls"}"` 这样的双转义串。
//
// 正确做法：把 RawMessage 当 JSON value 再 Unmarshal 一次到 Go string。若是
// JSON string 形态，得到原始 partial_json；若是其它形态（object/array 等，
// 如测试用 RawMessage(`{"cmd":"ls"}`) 直接构造），fallback 用 string() 原文。
func decodeInputRawMessage(raw json.RawMessage) string {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	// 非 JSON string（如 object/array/number）——保留原文（向后兼容）。
	return string(raw)
}

// applyToolCallInput 处理 tool.call.input（新协议 partial JSON 增量）。
//
// 找到匹配 tool_use_id 的 tool_use block；找不到则回退到最后一个 tool_use block。
// 找不到任何 tool_use block 时 no-op。
func applyToolCallInput(state StreamingState, event model.AgentEvent) StreamingState {
	if event.Delta == "" {
		return state
	}
	blocks := cloneBlocks(state.Blocks)
	idx := findToolUseBlock(blocks, event.ToolUseIDOrAlt())
	if idx == -1 {
		return state
	}
	blocks[idx].Text += event.Delta
	state.Blocks = blocks
	return state
}

// applyToolResult 处理 tool_result / tool.result 事件。
//
// 总是新 block。output / content 双兼容；is_error / isError 双兼容。
// 重要：is_error=true 不再切 Status='error'。Claude/Codex 等 CLI 在工具失败后
// 通常继续输出总结文本和卡片，若此处提前切 error，终态保护会丢弃所有后续事件，
// 导致 task.complete 时落库的 blocks_json 只到 tool_result(error) 为止，
// 前端权威消息丢失 agent 后续的总结文本 + 卡片。
// 流级错误仍由独立的 error 事件（applyError）处理，与 TS 版前端 reducer 一致。
func applyToolResult(state StreamingState, event model.AgentEvent) StreamingState {
	output := event.Output
	if output == "" {
		output = event.Content
	}
	isError := event.IsErrorOrAlt()
	blocks := cloneBlocks(state.Blocks)
	blocks = append(blocks, model.MessageBlock{
		Index:     nextIndex(blocks),
		Kind:      model.BlockKindToolResult,
		Text:      output,
		ToolUseID: event.ToolUseIDOrAlt(),
		IsError:   isError,
	})
	state.Blocks = blocks
	return state
}

// applyError 处理 error 事件。
//
// 总是新 block（kind='error'）+ Status='error'。
// message 字段为空时使用固定文案（与 TS 版一致：'生成失败'）。
func applyError(state StreamingState, event model.AgentEvent) StreamingState {
	message := event.Message
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
