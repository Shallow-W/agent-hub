// Package service: streaming_reducer_test.go
//
// streaming_reducer 单元测试。与 frontend src/store/__tests__/streamingReducer.test.ts
// 严格对齐（同 12 场景，同累积语义）。
//
// 修改任一端必须同步另一方，保证双端 reducer 行为一致。
package service

import (
	"encoding/json"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

// evt 构造 AgentEvent；测试辅助。payload 直接挂在顶层字段。
func evt(typ string) model.AgentEvent {
	return model.AgentEvent{Type: typ}
}

func TestStreamingReducer_EmptyEventsReturnsInitial(t *testing.T) {
	// 1. 空事件 reduceEvents 返回 initial state
	result := ReduceEvents(nil, InitialStreamingState())
	if len(result.Blocks) != 0 {
		t.Fatalf("expected empty blocks, got %d", len(result.Blocks))
	}
	if result.Status != model.MessageStatusStreaming {
		t.Fatalf("expected streaming status, got %q", result.Status)
	}
}

func TestStreamingReducer_SingleTextCreatesBlock(t *testing.T) {
	// 2. 单 text 事件创建新 text block
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: "hello"},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	b := result.Blocks[0]
	if b.Kind != model.BlockKindText {
		t.Fatalf("expected kind=text, got %q", b.Kind)
	}
	if b.Text != "hello" {
		t.Fatalf("expected text='hello', got %q", b.Text)
	}
	if b.Index != 0 {
		t.Fatalf("expected index=0, got %d", b.Index)
	}
	if result.Status != model.MessageStatusStreaming {
		t.Fatalf("expected streaming status, got %q", result.Status)
	}
}

func TestStreamingReducer_ConsecutiveTextAccumulates(t *testing.T) {
	// 3. 连续 text 事件累积到同一 block
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: "hello"},
		{Type: model.AgentEventText, Content: " world"},
		{Type: model.AgentEventText, Content: "!"},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	if result.Blocks[0].Text != "hello world!" {
		t.Fatalf("expected accumulated text, got %q", result.Blocks[0].Text)
	}
	if result.Blocks[0].Index != 0 {
		t.Fatalf("expected index=0, got %d", result.Blocks[0].Index)
	}
}

func TestStreamingReducer_TextThinkingTextThreeBlocks(t *testing.T) {
	// 4. text→thinking→text 产生 3 个独立 block（单调递增 index）
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: "a"},
		{Type: model.AgentEventThinking, Content: "b"},
		{Type: model.AgentEventText, Content: "c"},
	}, InitialStreamingState())
	if len(result.Blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(result.Blocks))
	}
	wantKinds := []model.BlockKind{model.BlockKindText, model.BlockKindThinking, model.BlockKindText}
	for i, want := range wantKinds {
		if result.Blocks[i].Kind != want {
			t.Fatalf("block %d: expected kind %q, got %q", i, want, result.Blocks[i].Kind)
		}
		if result.Blocks[i].Index != i {
			t.Fatalf("block %d: expected index %d, got %d", i, i, result.Blocks[i].Index)
		}
	}
}

func TestStreamingReducer_ToolUseAndResultTwoBlocks(t *testing.T) {
	// 5. tool_use(name) + tool_use(partial content) + tool_result 累积成 2 个 block
	// 当前线上 daemon 把 input_json_delta 的 partial_json 作为 toolUseEvent('', partial_json) 发出。
	// reducer 把这部分追加到最近一个 tool_use block 的 Text（同一 block），
	// 而不是产生新 block。所以最终 block 数：1 个 tool_use + 1 个 tool_result = 2。
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolUse,
			Tool:      "Bash",
			ToolUseID: "tu_1",
		},
		// 模拟线上字段（input）——reducer 兼容路径（优先 content，其次 input）
		// Note: Input 是 json.RawMessage（[]byte），Bug 3 修复后兼容 daemon 的
		// `input: {}` 对象 / `input: "<partial>"` 字符串两种 shape。
		{
			Type:  model.AgentEventToolUse,
			Tool:  "",
			Input: json.RawMessage(`{"cmd":"ls"}`),
		},
		{
			Type:      model.AgentEventToolResultOld,
			Output:    "done",
			ToolUseID: "tu_1",
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %+v", len(result.Blocks), result.Blocks)
	}
	tu := result.Blocks[0]
	if tu.Kind != model.BlockKindToolUse {
		t.Fatalf("expected kind=tool_use, got %q", tu.Kind)
	}
	if tu.ToolName != "Bash" {
		t.Fatalf("expected tool_name=Bash, got %q", tu.ToolName)
	}
	if tu.ToolUseID != "tu_1" {
		t.Fatalf("expected tool_use_id=tu_1, got %q", tu.ToolUseID)
	}
	// 第二个事件 partial 追加到第一个 tool_use block 的 Text
	if tu.Text != `{"cmd":"ls"}` {
		t.Fatalf("expected accumulated partial json %q, got %q", `{"cmd":"ls"}`, tu.Text)
	}
	tr := result.Blocks[1]
	if tr.Kind != model.BlockKindToolResult {
		t.Fatalf("expected kind=tool_result, got %q", tr.Kind)
	}
	if tr.Text != "done" {
		t.Fatalf("expected output='done', got %q", tr.Text)
	}
	if tr.IsError {
		t.Fatalf("expected is_error=false")
	}
	if result.Blocks[0].Index != 0 || result.Blocks[1].Index != 1 {
		t.Fatalf("expected monotonic indices [0,1], got [%d,%d]",
			result.Blocks[0].Index, result.Blocks[1].Index)
	}
}

func TestStreamingReducer_NewProtocolToolCallInput(t *testing.T) {
	// 5b. 新协议 tool.call.start + tool.call.input + tool.call.end 累积
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolCallStart,
			Tool:      "Read",
			ToolUseID: "tu_2",
		},
		{
			Type:      model.AgentEventToolCallInput,
			ToolUseID: "tu_2",
			Delta:     `{"file_path":"/a`,
		},
		{
			Type:      model.AgentEventToolCallInput,
			ToolUseID: "tu_2",
			Delta:     `.txt"}`,
		},
		{
			Type:      model.AgentEventToolCallEnd,
			ToolUseID: "tu_2",
		},
		{
			Type:      model.AgentEventToolResultNew,
			ToolUseID: "tu_2",
			Content:   "file body",
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %+v", len(result.Blocks), result.Blocks)
	}
	tu := result.Blocks[0]
	if tu.Kind != model.BlockKindToolUse {
		t.Fatalf("expected kind=tool_use, got %q", tu.Kind)
	}
	if tu.ToolName != "Read" {
		t.Fatalf("expected tool_name=Read, got %q", tu.ToolName)
	}
	if tu.ToolUseID != "tu_2" {
		t.Fatalf("expected tool_use_id=tu_2, got %q", tu.ToolUseID)
	}
	if tu.Text != `{"file_path":"/a.txt"}` {
		t.Fatalf("expected accumulated input %q, got %q",
			`{"file_path":"/a.txt"}`, tu.Text)
	}
	tr := result.Blocks[1]
	if tr.Kind != model.BlockKindToolResult {
		t.Fatalf("expected kind=tool_result, got %q", tr.Kind)
	}
	if tr.Text != "file body" {
		t.Fatalf("expected output='file body', got %q", tr.Text)
	}
}

func TestStreamingReducer_TurnEndSetsComplete(t *testing.T) {
	// 6. turn_end 切 status 到 complete（不产生 block）
	result := ReduceEvents([]model.AgentEvent{
		evt(model.AgentEventTurnEnd),
	}, InitialStreamingState())
	if result.Status != model.MessageStatusComplete {
		t.Fatalf("expected complete status, got %q", result.Status)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_SessionEndVariantsSetComplete(t *testing.T) {
	// 6b. session.end / session_end 同样切 complete（双兼容）
	for _, typ := range []string{model.AgentEventSessionEndNew, model.AgentEventSessionEndOld} {
		result := ReduceEvents([]model.AgentEvent{
			evt(typ),
		}, InitialStreamingState())
		if result.Status != model.MessageStatusComplete {
			t.Fatalf("expected complete for type %q, got %q", typ, result.Status)
		}
	}
}

func TestStreamingReducer_ErrorAppendsBlockAndSetsError(t *testing.T) {
	// 7. error 事件追加 error block + status=error
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: "partial"},
		{Type: model.AgentEventError, Message: "boom"},
	}, InitialStreamingState())
	if len(result.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result.Blocks))
	}
	e := result.Blocks[1]
	if e.Kind != model.BlockKindError {
		t.Fatalf("expected kind=error, got %q", e.Kind)
	}
	if e.Text != "boom" {
		t.Fatalf("expected message='boom', got %q", e.Text)
	}
	if !e.IsError {
		t.Fatalf("expected is_error=true")
	}
	if result.Status != model.MessageStatusError {
		t.Fatalf("expected status=error, got %q", result.Status)
	}
}

func TestStreamingReducer_CancelSetsCanceled(t *testing.T) {
	// 8. cancel 事件切 status 到 canceled
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventCancel, Reason: "用户取消"},
	}, InitialStreamingState())
	if result.Status != model.MessageStatusCanceled {
		t.Fatalf("expected canceled status, got %q", result.Status)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_PureFunctionDoesNotMutateInput(t *testing.T) {
	// 9. 纯函数性：reduce 不修改入参 state
	initial := StreamingState{
		Blocks: []model.MessageBlock{
			{Index: 0, Kind: model.BlockKindText, Text: "pre"},
		},
		Status: model.MessageStatusStreaming,
	}
	// 快照（深拷贝）
	snapshot := StreamingState{
		Blocks: []model.MessageBlock{
			{Index: 0, Kind: model.BlockKindText, Text: "pre"},
		},
		Status: model.MessageStatusStreaming,
	}
	result := StreamingReducer(initial, model.AgentEvent{
		Type:    model.AgentEventText,
		Content: "-post",
	})

	// 入参 state 不变
	if len(initial.Blocks) != 1 || initial.Blocks[0].Text != "pre" {
		t.Fatalf("input state mutated: blocks=%+v", initial.Blocks)
	}
	// 返回新对象，blocks 是新 slice
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	if result.Blocks[0].Text != "pre-post" {
		t.Fatalf("expected accumulated text 'pre-post', got %q", result.Blocks[0].Text)
	}
	// 快照与原 initial 等价（验证 initial 未被改）
	if len(snapshot.Blocks) != len(initial.Blocks) {
		t.Fatalf("snapshot drift: %d vs %d", len(snapshot.Blocks), len(initial.Blocks))
	}
	if snapshot.Blocks[0].Text != initial.Blocks[0].Text {
		t.Fatalf("snapshot text drift: %q vs %q",
			snapshot.Blocks[0].Text, initial.Blocks[0].Text)
	}
}

func TestStreamingReducer_TerminalStateProtection(t *testing.T) {
	// 10. 终态保护：status 非 streaming 时后续事件忽略
	ended := StreamingState{
		Blocks: []model.MessageBlock{},
		Status: model.MessageStatusComplete,
	}
	result := StreamingReducer(ended, model.AgentEvent{
		Type:    model.AgentEventText,
		Content: "x",
	})
	// 终态后 reduce 直接返回原 state
	if result.Status != model.MessageStatusComplete {
		t.Fatalf("expected still complete, got %q", result.Status)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_ToolResultDualFieldCompat(t *testing.T) {
	// 11. tool_result 双字段兼容：output 或 content 都能读到
	r1 := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventToolResultOld, Output: "via-output"},
	}, InitialStreamingState())
	r2 := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventToolResultNew, Content: "via-content"},
	}, InitialStreamingState())
	if r1.Blocks[0].Text != "via-output" {
		t.Fatalf("expected output via-output, got %q", r1.Blocks[0].Text)
	}
	if r2.Blocks[0].Text != "via-content" {
		t.Fatalf("expected content via-content, got %q", r2.Blocks[0].Text)
	}
}

func TestStreamingReducer_IsErrorDualFieldCompat(t *testing.T) {
	// 12. is_error / isError 双字段兼容
	r1 := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventToolResultOld, Output: "x", IsError: true},
	}, InitialStreamingState())
	r2 := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventToolResultOld, Output: "x", IsErrorAlt: true},
	}, InitialStreamingState())
	if !r1.Blocks[0].IsError {
		t.Fatalf("expected is_error=true for snake_case")
	}
	if !r2.Blocks[0].IsError {
		t.Fatalf("expected is_error=true for camelCase")
	}
	// is_error=true 不再切 Status='error'——agent 在工具失败后通常继续输出总结文本
	// 和卡片，提前切 error 会触发终态保护丢弃后续事件（regression：streaming 截断 bug）。
	if r1.Status != model.MessageStatusStreaming {
		t.Fatalf("tool_result(is_error) must not flip status; got %q", r1.Status)
	}
	if r2.Status != model.MessageStatusStreaming {
		t.Fatalf("tool_result(isError) must not flip status; got %q", r2.Status)
	}
}

func TestStreamingReducer_TextAfterToolResultErrorStillAccumulates(t *testing.T) {
	// 12b. 回归测试：agent 在 tool_result(is_error=true) 之后仍会输出总结文本，
	// reducer 必须继续累积，且最终 status 跟随 turn_end 切到 complete。
	// 这是 streaming 截断 bug 的核心契约：tool_result error 不等价于流级 error。
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: "before-tool"},
		{Type: model.AgentEventToolUse, Tool: "Bash", ToolUseID: "tu_a"},
		{Type: model.AgentEventToolResultOld, ToolUseID: "tu_a", Output: "boom", IsError: true},
		{Type: model.AgentEventText, Content: "after-tool-summary"},
		{Type: model.AgentEventTurnEnd},
	}, InitialStreamingState())

	if result.Status != model.MessageStatusComplete {
		t.Fatalf("expected status=complete (turn_end after tool error), got %q", result.Status)
	}
	// 期望 4 个 block: text / tool_use / tool_result(error) / text(summary)
	if len(result.Blocks) != 4 {
		t.Fatalf("expected 4 blocks (text/tool_use/tool_result/text), got %d: %+v", len(result.Blocks), result.Blocks)
	}
	last := result.Blocks[3]
	if last.Kind != model.BlockKindText || last.Text != "after-tool-summary" {
		t.Fatalf("expected final block to be summary text, got kind=%q text=%q", last.Kind, last.Text)
	}
	tr := result.Blocks[2]
	if tr.Kind != model.BlockKindToolResult || !tr.IsError {
		t.Fatalf("expected tool_result(is_error) at index 2, got kind=%q is_error=%v", tr.Kind, tr.IsError)
	}
}

func TestStreamingReducer_ErrorDefaultMessage(t *testing.T) {
	// error 事件无 message 字段时 fallback 到 '生成失败'
	result := ReduceEvents([]model.AgentEvent{
		evt(model.AgentEventError),
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	if result.Blocks[0].Text != "生成失败" {
		t.Fatalf("expected default message, got %q", result.Blocks[0].Text)
	}
}

func TestStreamingReducer_EmptyContentTextIsNoop(t *testing.T) {
	// text 事件 content 为空时不产生新 block（防御性）
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Content: ""},
	}, InitialStreamingState())
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty content, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_PartialInputWithoutToolUseIsDropped(t *testing.T) {
	// tool_use 空 tool 名 + 无前置 tool_use block → 丢弃（与 appendDeltas 一致）
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventToolUse, Tool: "", Content: `{"partial":"json"}`},
	}, InitialStreamingState())
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks for orphan partial input, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_ToolCallInputWithoutMatchingFallback(t *testing.T) {
	// tool.call.input 找不到匹配 tool_use_id 时回退到最后一个 tool_use block
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolUse,
			Tool:      "Read",
			ToolUseID: "tu_a",
		},
		// 找不到 tu_b → 回退到最后一个 tool_use block（tu_a）
		{
			Type:      model.AgentEventToolCallInput,
			ToolUseID: "tu_b",
			Delta:     `{"x":1}`,
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	if result.Blocks[0].Text != `{"x":1}` {
		t.Fatalf("expected accumulated text on fallback block, got %q", result.Blocks[0].Text)
	}
}

func TestStreamingReducer_ToolCallInputNoToolUseBlockDrops(t *testing.T) {
	// tool.call.input 找不到任何 tool_use block → no-op
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolCallInput,
			ToolUseID: "tu_x",
			Delta:     `{"x":1}`,
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks for orphan input, got %d", len(result.Blocks))
	}
}

func TestStreamingReducer_TextContentFromAltField(t *testing.T) {
	// 老格式兼容：text 事件用 text 字段（而非 content）也能被读到
	result := ReduceEvents([]model.AgentEvent{
		{Type: model.AgentEventText, Text: "from-alt"},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 || result.Blocks[0].Text != "from-alt" {
		t.Fatalf("expected alt text field, got blocks=%+v", result.Blocks)
	}
}

func TestStreamingReducer_ToolUseIDAltCamelCase(t *testing.T) {
	// toolUseID camelCase 兼容路径
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:         model.AgentEventToolCallStart,
			Tool:         "Read",
			ToolUseIDAlt: "tu_camel",
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 || result.Blocks[0].ToolUseID != "tu_camel" {
		t.Fatalf("expected toolUseID alt fallback, got blocks=%+v", result.Blocks)
	}
}

// === Bug 3 fix tests ===
// Bug 3: JS daemon 的 toolUseEvent 在 content_block_start 路径发 `input: {}`（空对象），
// 旧 `Input string` 字段 unmarshal object 报错 → daemon.go:377 丢整批 events。
// 改为 json.RawMessage 后兼容 object / string 两种 shape。

func TestStreamingReducer_ToolUseStartWithObjectInputNoPanic(t *testing.T) {
	// content_block_start 路径：tool 非空 + Input 是 `{}` 对象占位
	// 预期：不 panic，创建空 tool_use block，Input 字段被忽略（不作为 delta 追加）。
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolUse,
			Tool:      "Bash",
			ToolUseID: "tu_obj_1",
			Input:     json.RawMessage(`{}`),
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %+v", len(result.Blocks), result.Blocks)
	}
	b := result.Blocks[0]
	if b.Kind != model.BlockKindToolUse {
		t.Fatalf("expected kind=tool_use, got %q", b.Kind)
	}
	if b.ToolName != "Bash" {
		t.Fatalf("expected tool_name=Bash, got %q", b.ToolName)
	}
	if b.ToolUseID != "tu_obj_1" {
		t.Fatalf("expected tool_use_id=tu_obj_1, got %q", b.ToolUseID)
	}
	if b.Text != "" {
		t.Fatalf("expected empty Text (Input object placeholder should NOT be appended), got %q", b.Text)
	}
}

func TestStreamingReducer_ToolUseDeltaWithStringInputRawMessage(t *testing.T) {
	// content_block_delta 路径：tool 空 + Input 是 JSON string（被 RawMessage 保留为原文）
	// 预期：Input RawMessage 转 string 后追加到上一个 tool_use block 的 Text。
	result := ReduceEvents([]model.AgentEvent{
		{
			Type:      model.AgentEventToolUse,
			Tool:      "Bash",
			ToolUseID: "tu_str_1",
		},
		{
			Type:  model.AgentEventToolUse,
			Tool:  "",
			Input: json.RawMessage(`{"cmd":"ls"}`),
		},
	}, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block (delta accumulates), got %d: %+v", len(result.Blocks), result.Blocks)
	}
	if result.Blocks[0].Text != `{"cmd":"ls"}` {
		t.Fatalf("expected Text=%q, got %q", `{"cmd":"ls"}`, result.Blocks[0].Text)
	}
}

func TestStreamingReducer_ObjectInputBatchUnmarshalSucceeds(t *testing.T) {
	// 回归 Bug 3：模拟 daemon.go:377 反序列化场景——
	// 整批 events 含 object shape Input 时不能因为 unmarshal 失败被丢弃。
	raw := []byte(`[
		{"type":"tool_use","tool":"Bash","tool_use_id":"tu_a","input":{}},
		{"type":"tool_use","tool":"","input":"{\"cmd\":\"ls\"}"},
		{"type":"tool_result","tool_use_id":"tu_a","output":"done"}
	]`)
	var events []model.AgentEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("Unmarshal must succeed for object-shape input (Bug 3): %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// reduce 后应得到 1 个 tool_use block（input delta 累积）+ 1 个 tool_result block
	result := ReduceEvents(events, InitialStreamingState())
	if len(result.Blocks) != 2 {
		t.Fatalf("expected 2 blocks after reduce, got %d: %+v", len(result.Blocks), result.Blocks)
	}
}

// TestStreamingReducer_StringShapeInputUnwrappedCorrectly 验证 daemon content_block_delta
// 路径下 string-shape input 被正确解码（不带外层引号/转义）。
//
// daemon events.js toolUseEvent('', partial_json) 让 input 字段成为 JS string，
// JSON.stringify 后变成带引号的 JSON string。reducer 必须把 RawMessage 解包成
// 原始 partial_json，而不是直接 string() 转换保留字面引号和反斜杠。
// 否则 tool_use.Text 会累积成 `"{\"cmd\":\"ls\"}"`（双转义）而非 `{"cmd":"ls"}`。
func TestStreamingReducer_StringShapeInputUnwrappedCorrectly(t *testing.T) {
	raw := []byte(`[
		{"type":"tool_use","tool":"Bash","tool_use_id":"tu_s","input":{}},
		{"type":"tool_use","tool":"","input":"{\"cmd\":\"ls\"}"}
	]`)
	var events []model.AgentEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	result := ReduceEvents(events, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 tool_use block, got %d: %+v", len(result.Blocks), result.Blocks)
	}
	want := `{"cmd":"ls"}`
	if result.Blocks[0].Text != want {
		t.Fatalf("expected Text=%q, got %q (double-escape bug if mismatched)", want, result.Blocks[0].Text)
	}
}

// TestStreamingReducer_PartialStringInputAccumulatesCorrectly 多个 partial_json 片段
// 累积后必须拼成原始完整 JSON（不带额外转义）。
func TestStreamingReducer_PartialStringInputAccumulatesCorrectly(t *testing.T) {
	raw := []byte(`[
		{"type":"tool_use","tool":"Read","tool_use_id":"tu_p","input":{}},
		{"type":"tool_use","tool":"","input":"{\"file\":"},
		{"type":"tool_use","tool":"","input":"\"/a.txt\"}"}
	]`)
	var events []model.AgentEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	result := ReduceEvents(events, InitialStreamingState())
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 tool_use block, got %d", len(result.Blocks))
	}
	want := `{"file":"/a.txt"}`
	if result.Blocks[0].Text != want {
		t.Fatalf("expected accumulated Text=%q, got %q", want, result.Blocks[0].Text)
	}
}
