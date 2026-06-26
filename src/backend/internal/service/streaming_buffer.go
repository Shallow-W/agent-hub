// Package service: streaming_buffer.go
//
// streaming_buffer 在 backend 内存里累积 StreamingState，按 task_id 索引。
//
// 工作流：
//   1. daemon Hub 收到 task.progress → 调 PushEvents 把 events reduce 到累积状态
//   2. task.complete 时 message service 调 GetState 拿累积 blocks → 序列化为 blocks_json
//   3. FinalizeStreaming 后调 Delete 释放内存
//
// 设计权衡：
//   - 不持久化：backend 重启丢失（接受，watchdog 60s 兜底）
//   - sync.Map：task_id 级别并发安全；同一 task_id 的事件按 daemon 顺序单 goroutine
//     处理（WS read loop），所以 *StreamingState 内部修改安全
//   - 内存稳定：累积 blocks ≪ events 数组（blocks 是聚合后的状态）
package service

import (
	"sync"

	"github.com/agent-hub/backend/internal/model"
)

// StreamingBuffer 累积 task_id 对应的 StreamingState。
//
// 约束：
//   - PushEvents：task_id 为空或 events 为空时 no-op
//   - GetState：返回副本（调用方修改不影响 buffer 内部状态）
//   - Delete：幂等（不存在的 task_id 也能调用）
type StreamingBuffer struct {
	states sync.Map // taskID -> *StreamingState
}

// NewStreamingBuffer 创建空 buffer。
func NewStreamingBuffer() *StreamingBuffer {
	return &StreamingBuffer{}
}

// PushEvents 把 events reduce 到 taskID 对应的累积状态。
//
// 若 taskID 首次出现，初始化为 InitialStreamingState。
// 同一 taskID 的事件按调用顺序依次应用（不并发，依赖 WS read loop 单 goroutine）。
//
// 注意：sync.Map 只保证 map 层面并发安全，*StreamingState 内部修改需要调用方
// 保证同一 taskID 不并发。当前调用路径：
//   daemon.go handleTaskProgress → PushEvents
//   handleTaskProgress 由 daemonHub.readLoop 单 goroutine 串行调用，满足约束。
func (b *StreamingBuffer) PushEvents(taskID string, events []model.AgentEvent) {
	if taskID == "" || len(events) == 0 {
		return
	}
	actual, _ := b.states.LoadOrStore(taskID, ptrStreamingState(InitialStreamingState()))
	state := actual.(*StreamingState)
	newState := ReduceEvents(events, *state)
	*state = newState
}

// GetState 返回 taskID 的累积状态深拷贝。
//
// 返回值：调用方修改不影响 buffer 内部（slice header 拷贝 + 元素值拷贝）。
// 第二返回值 false 表示 taskID 不存在或 taskID 为空。
//
// 注意：必须深拷贝 Blocks slice 元素。因为 reducer 通过 cloneBlocks 保证
// 每次 PushEvents 时 buffer 内部 *state 的 Blocks 指向新底层数组，但
// GetState 返回的副本若指向同一底层数组，调用方修改 block 元素会影响
// buffer 内部状态（即使 slice header 是独立的）。所以这里用 cloneBlocks
// 切断底层共享。
func (b *StreamingBuffer) GetState(taskID string) (StreamingState, bool) {
	if taskID == "" {
		return StreamingState{}, false
	}
	v, ok := b.states.Load(taskID)
	if !ok {
		return StreamingState{}, false
	}
	state := v.(*StreamingState)
	// 深拷贝 Blocks 元素，避免调用方修改 buffer 内部状态
	copyState := StreamingState{
		Blocks:  cloneBlocks(state.Blocks),
		Status:  state.Status,
		TaskID:  state.TaskID,
		AgentID: state.AgentID,
	}
	return copyState, true
}

// Delete 释放 taskID 对应的 buffer entry。
// 幂等：taskID 不存在或为空时 no-op。
func (b *StreamingBuffer) Delete(taskID string) {
	if taskID == "" {
		return
	}
	b.states.Delete(taskID)
}

// ptrStreamingState 返回 state 的指针。仅用于内部 LoadOrStore。
func ptrStreamingState(state StreamingState) *StreamingState {
	return &state
}
