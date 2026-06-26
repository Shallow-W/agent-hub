// Package model: agent_event_test.go
//
// AgentEvent 反序列化 / 序列化测试。
//
// 重点覆盖 Bug 3 修复：JS daemon 的 toolUseEvent 在 content_block_start 路径
// 发 `input: {}`（空对象占位），旧 `Input string` 字段 unmarshal 报错 →
// daemon.go:377 整批 events 被丢弃。改为 json.RawMessage 后兼容 object / string 两种 shape。
package model

import (
	"encoding/json"
	"testing"
)

// TestAgentEventUnmarshal_ObjectInputSucceeds 验证 Bug 3 修复：
// `input: {}` 不能让 Unmarshal 失败（否则整批 events 被 daemon handler 丢弃）。
func TestAgentEventUnmarshal_ObjectInputSucceeds(t *testing.T) {
	raw := []byte(`{"type":"tool_use","tool":"Bash","tool_use_id":"tu_1","input":{}}`)
	var ev AgentEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("Unmarshal must succeed for object-shape input (Bug 3): %v", err)
	}
	if ev.Tool != "Bash" {
		t.Fatalf("expected tool=Bash, got %q", ev.Tool)
	}
	if ev.ToolUseID != "tu_1" {
		t.Fatalf("expected tool_use_id=tu_1, got %q", ev.ToolUseID)
	}
	// RawMessage 应该保留 `{}` 原文
	if string(ev.Input) != `{}` {
		t.Fatalf("expected Input RawMessage={} , got %q", string(ev.Input))
	}
}

// TestAgentEventUnmarshal_StringInputSucceeds 验证 content_block_delta 路径
// （input 是 JSON string）也能被 RawMessage 兼容保留。
func TestAgentEventUnmarshal_StringInputSucceeds(t *testing.T) {
	raw := []byte(`{"type":"tool_use","tool":"","input":"{\"cmd\":\"ls\"}"}`)
	var ev AgentEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("Unmarshal string-shape input failed: %v", err)
	}
	// RawMessage 保留序列化后的 JSON string 原文（含引号）
	expected := `"{\"cmd\":\"ls\"}"`
	if string(ev.Input) != expected {
		t.Fatalf("expected Input RawMessage=%q, got %q", expected, string(ev.Input))
	}
}

// TestAgentEventUnmarshal_BatchWithMixedInputShapes 验证整批 events
// 含混合 shape（object + string + 缺省）时反序列化全部成功——这是 Bug 3 真实场景。
func TestAgentEventUnmarshal_BatchWithMixedInputShapes(t *testing.T) {
	raw := []byte(`[
		{"type":"tool_use","tool":"Bash","tool_use_id":"tu_a","input":{}},
		{"type":"tool_use","tool":"","input":"{\"cmd\":\"ls\"}"},
		{"type":"tool_result","tool_use_id":"tu_a","output":"done"}
	]`)
	var events []AgentEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("batch Unmarshal must succeed (Bug 3): %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Tool != "Bash" {
		t.Fatalf("events[0].Tool mismatch: %q", events[0].Tool)
	}
	if string(events[0].Input) != `{}` {
		t.Fatalf("events[0].Input RawMessage mismatch: %q", string(events[0].Input))
	}
}

// TestAgentEventMarshal_RawMessageSerializable 验证 RawMessage 字段
// 序列化后与原始 JSON value 一致（不会产生额外引号/转义）。
func TestAgentEventMarshal_RawMessageSerializable(t *testing.T) {
	ev := AgentEvent{
		Type:      AgentEventToolUse,
		Tool:      "Bash",
		ToolUseID: "tu_m",
		Input:     json.RawMessage(`{"k":"v"}`),
	}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// Input 字段在 JSON 输出中应保留为 `"input":{"k":"v"}`（不是字符串化的）
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-Unmarshal failed: %v", err)
	}
	inputRaw, ok := got["input"]
	if !ok {
		t.Fatalf("expected input field in output: %s", out)
	}
	if string(inputRaw) != `{"k":"v"}` {
		t.Fatalf("expected input={\"k\":\"v\"}, got %q", string(inputRaw))
	}
}

// TestAgentEventMarshal_NilInputOmitted 验证 nil/empty RawMessage 被 omitempty 省略。
func TestAgentEventMarshal_NilInputOmitted(t *testing.T) {
	ev := AgentEvent{
		Type: AgentEventText,
		Content: "hi",
	}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-Unmarshal failed: %v", err)
	}
	if _, ok := got["input"]; ok {
		t.Fatalf("input field should be omitted when nil, got output: %s", out)
	}
}
