// Package service: streaming_buffer_test.go
//
// streaming_buffer 单元测试。覆盖 PushEvents / GetState / Delete 三个核心方法。
package service

import (
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

func TestStreamingBuffer_PushAndGetState(t *testing.T) {
	buf := NewStreamingBuffer()
	events := []model.AgentEvent{
		{Type: model.AgentEventText, Content: "hello"},
		{Type: model.AgentEventText, Content: " world"},
	}
	buf.PushEvents("task-1", events)

	state, ok := buf.GetState("task-1")
	if !ok {
		t.Fatalf("expected state found")
	}
	if len(state.Blocks) != 1 {
		t.Fatalf("expected 1 accumulated block, got %d", len(state.Blocks))
	}
	if state.Blocks[0].Text != "hello world" {
		t.Fatalf("expected accumulated text, got %q", state.Blocks[0].Text)
	}
	if state.Status != model.MessageStatusStreaming {
		t.Fatalf("expected streaming status, got %q", state.Status)
	}
}

func TestStreamingBuffer_MultiplePushesAccumulate(t *testing.T) {
	// 多次 PushEvents 应该累积到同一状态
	buf := NewStreamingBuffer()
	buf.PushEvents("task-2", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "a"},
	})
	buf.PushEvents("task-2", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "b"},
	})
	state, _ := buf.GetState("task-2")
	if len(state.Blocks) != 1 {
		t.Fatalf("expected 1 block accumulated across pushes, got %d", len(state.Blocks))
	}
	if state.Blocks[0].Text != "ab" {
		t.Fatalf("expected 'ab', got %q", state.Blocks[0].Text)
	}
}

func TestStreamingBuffer_DeleteReleasesState(t *testing.T) {
	buf := NewStreamingBuffer()
	buf.PushEvents("task-3", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "x"},
	})
	if _, ok := buf.GetState("task-3"); !ok {
		t.Fatalf("expected state before delete")
	}
	buf.Delete("task-3")
	if _, ok := buf.GetState("task-3"); ok {
		t.Fatalf("expected state gone after delete")
	}
}

func TestStreamingBuffer_DeleteMissingIsNoop(t *testing.T) {
	// Delete 幂等：不存在的 taskID 也能调
	buf := NewStreamingBuffer()
	buf.Delete("never-existed")
	buf.Delete("")
}

func TestStreamingBuffer_GetMissingReturnsFalse(t *testing.T) {
	buf := NewStreamingBuffer()
	if _, ok := buf.GetState("nope"); ok {
		t.Fatalf("expected false for missing task")
	}
	if _, ok := buf.GetState(""); ok {
		t.Fatalf("expected false for empty taskID")
	}
}

func TestStreamingBuffer_PushEmptyEventsIsNoop(t *testing.T) {
	buf := NewStreamingBuffer()
	// 空 events: 不应该创建 entry
	buf.PushEvents("task-4", nil)
	if _, ok := buf.GetState("task-4"); ok {
		t.Fatalf("expected no state for empty events push")
	}
	// 空 taskID: 也不应该创建
	buf.PushEvents("", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "x"},
	})
	if _, ok := buf.GetState(""); ok {
		t.Fatalf("expected no state for empty taskID")
	}
}

func TestStreamingBuffer_GetStateReturnsCopy(t *testing.T) {
	// GetState 返回副本：修改返回值不影响 buffer 内部
	buf := NewStreamingBuffer()
	buf.PushEvents("task-5", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "original"},
	})
	state1, _ := buf.GetState("task-5")
	// 修改副本
	state1.Blocks[0].Text = "mutated"
	state1.Status = model.MessageStatusError

	// buffer 内部应保持原值
	state2, _ := buf.GetState("task-5")
	if state2.Blocks[0].Text != "original" {
		t.Fatalf("buffer state mutated via copy: text=%q", state2.Blocks[0].Text)
	}
	if state2.Status != model.MessageStatusStreaming {
		t.Fatalf("buffer status mutated via copy: %q", state2.Status)
	}
}

func TestStreamingBuffer_AccumulatesUntilTerminal(t *testing.T) {
	// 终态事件后继续 PushEvents 应该被 reducer 忽略（终态保护）
	buf := NewStreamingBuffer()
	buf.PushEvents("task-6", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "before"},
		evt(model.AgentEventTurnEnd), // 切到 complete
	})
	// 后续 push：reducer 应该 no-op（终态保护）
	buf.PushEvents("task-6", []model.AgentEvent{
		{Type: model.AgentEventText, Content: "after"},
	})
	state, _ := buf.GetState("task-6")
	if state.Status != model.MessageStatusComplete {
		t.Fatalf("expected complete status, got %q", state.Status)
	}
	if len(state.Blocks) != 1 {
		t.Fatalf("expected 1 block (post-terminal ignored), got %d", len(state.Blocks))
	}
	if state.Blocks[0].Text != "before" {
		t.Fatalf("expected 'before' only, got %q", state.Blocks[0].Text)
	}
}
