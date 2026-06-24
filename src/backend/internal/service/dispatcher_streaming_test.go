// Package service: dispatcher_streaming_test.go
//
// 本文件覆盖群聊流式接入（本任务）的 Dispatcher 行为：
//   - 成功路径：dispatch payload 包含 message_id，streaming placeholder 被 FinalizeStreaming(Complete)
//   - 失败路径 1：daemon 未连接 → placeholder 被 FinalizeStreaming(Error)
//   - 失败路径 2：daemon task failed → placeholder 被 FinalizeStreaming(Error)
//   - 失败路径 3：wait 失败（await channel nil）→ placeholder 被 FinalizeStreaming(Error)
//
// 这些测试单独验证 StreamingPipeline 接入，不需要真实 OrchestratorService。
// 通过直接 NewDispatcher(DispatcherDeps{...}) + 显式注入 StreamingPipelineDeps 实现。
package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/pkg/ws"
)

// fakeStreamingNotifier 是测试用的最小 MessageNotifier 实现。
// 记录所有 PushCustomEvent 调用，断言 broadcastStreamingTerminal 是否触发。
type fakeStreamingNotifier struct {
	mu          sync.Mutex
	customCalls []struct {
		convID     string
		eventType  string
		memberIDs  []string
	}
}

func (n *fakeStreamingNotifier) PushToConversation(_ string, _ []string, _ interface{}) {}

func (n *fakeStreamingNotifier) PushCustomEvent(convID string, memberIDs []string, eventType string, _ interface{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.customCalls = append(n.customCalls, struct {
		convID    string
		eventType string
		memberIDs []string
	}{convID: convID, eventType: eventType, memberIDs: append([]string(nil), memberIDs...)})
}

func (n *fakeStreamingNotifier) IsOnline(_ string) bool { return false }

func (n *fakeStreamingNotifier) CustomEventCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.customCalls)
}

// fakeTaskAgentIndex 记录 RegisterTaskAgent / DeleteTaskAgent 调用。
type fakeTaskAgentIndex struct {
	registered map[string]string // taskID -> agentName
	deleted    map[string]bool
}

func newFakeTaskAgentIndex() *fakeTaskAgentIndex {
	return &fakeTaskAgentIndex{
		registered: make(map[string]string),
		deleted:    make(map[string]bool),
	}
}

func (f *fakeTaskAgentIndex) RegisterTaskAgent(taskID, agentName string) {
	f.registered[taskID] = agentName
}

func (f *fakeTaskAgentIndex) DeleteTaskAgent(taskID string) {
	f.deleted[taskID] = true
}

// fakeStreamingConvRepo 是流式管线用到的最小 ConvRepo（仅 ListMemberIDs）。
type fakeStreamingConvRepo struct {
	memberIDs []string
}

func (r *fakeStreamingConvRepo) ListMemberIDs(_ context.Context, _ string) ([]string, error) {
	return r.memberIDs, nil
}

// buildStreamingDispatcher 构造装配了 StreamingPipeline 的 Dispatcher。
// 调用方可通过参数覆盖 hub / msgRepo / notifier 的默认行为。
func buildStreamingDispatcher(
	t *testing.T,
	hub port.DaemonDispatcher,
	msgRepo *fakeMsgRepo,
	notifier *fakeStreamingNotifier,
) *Dispatcher {
	t.Helper()
	convRepo := &fakeStreamingConvRepo{memberIDs: []string{"u1", "u2"}}
	buf := NewStreamingBuffer()
	pipeline := StreamingPipelineDeps{
		MsgRepo:         msgRepo,
		DaemonHub:       hub,
		StreamingBuffer: buf,
		Notifier:        notifier,
		ConvRepo:        convRepo,
	}
	taskAgent := newFakeTaskAgentIndex()
	return NewDispatcher(DispatcherDeps{
		AgentRepo: &fakeOrchAgentRepo{
			agent: &model.Agent{ID: "agent-s", Name: "StreamAgent", CLITool: "claude", MachineID: stringPtr("machine-s")},
			task:  &model.DaemonTask{ID: "task-s", Status: "completed", Result: "stream ok"},
		},
		DaemonHub:     hub,
		MsgRepo:       msgRepo,
		Streaming:     &pipeline,
		TaskAgent:     taskAgent,
		TaskCardQueue: nil,
	})
}

// TestDispatcher_StreamingSuccess_PayloadContainsMessageID 验证：
// Dispatcher 装配了 StreamingPipeline 时，dispatch payload 包含 message_id 字段
// （这是让 daemon-npm shouldStream=true 的关键字段）。
func TestDispatcher_StreamingSuccess_PayloadContainsMessageID(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-s1",
		Name:      "StreamAgent1",
		CLITool:   "claude",
		MachineID: stringPtr("machine-s1"),
	}

	promiseCh := make(chan *ws.TaskResult, 1)
	promiseCh <- &ws.TaskResult{TaskID: "task-fake-stream", Result: "streamed"}

	var capturedPayload ws.WSMessage
	var capturedMachineID string
	var sendMu sync.Mutex
	fakeHub := &fakeDaemonDispatcher{
		isConnected: func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult {
			return promiseCh
		},
		sendToMachine: func(machineID string, msg ws.WSMessage) error {
			sendMu.Lock()
			defer sendMu.Unlock()
			capturedMachineID = machineID
			capturedPayload = msg
			return nil
		},
		awaitTaskResult: func(_ string) chan *ws.TaskResult {
			return promiseCh
		},
		removeTaskPromise: func(_ string) {},
	}
	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	d := buildStreamingDispatcher(t, fakeHub, msgRepo, notifier)

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "hello",
	}, DispatchHooks{})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	sendMu.Lock()
	defer sendMu.Unlock()
	data, ok := capturedPayload.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected dispatch payload Data to be map[string]interface{}, got %T", capturedPayload.Data)
	}
	msgID, exists := data["message_id"]
	if !exists {
		t.Fatal("dispatch payload missing message_id field (group chat streaming will not work)")
	}
	if msgIDStr, ok := msgID.(string); !ok || msgIDStr == "" {
		t.Fatalf("expected non-empty string message_id, got %v", msgID)
	}
	if capturedMachineID != "machine-s1" {
		t.Fatalf("expected SendToMachine called with machine-s1, got %q", capturedMachineID)
	}
}

// TestDispatcher_StreamingSuccess_FinalizeStreamingCalled 验证成功路径：
// placeholder streaming message 被切到 complete，content 写入 strippedContent。
func TestDispatcher_StreamingSuccess_FinalizeStreamingCalled(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-s2",
		Name:      "StreamAgent2",
		CLITool:   "claude",
		MachineID: stringPtr("machine-s2"),
	}

	// taskID 保持一致：CreateDaemonTask 返回的 task.ID 与 promiseCh 注入的 TaskID
	// 必须相同，否则 streamingHandleForTask(task.ID) 会 lookup 失败（handle 注册
	// 在 CreateDaemonTask 返回的 task.ID 下）。
	const taskID = "task-s2"
	promiseCh := make(chan *ws.TaskResult, 1)
	promiseCh <- &ws.TaskResult{TaskID: taskID, Result: "stream success content"}
	fakeHub := &fakeDaemonDispatcher{
		isConnected:         func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult { return promiseCh },
		sendToMachine:       func(_ string, _ ws.WSMessage) error { return nil },
		awaitTaskResult:     func(_ string) chan *ws.TaskResult { return promiseCh },
		removeTaskPromise:   func(_ string) {},
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	convRepo := &fakeStreamingConvRepo{memberIDs: []string{"u1", "u2"}}
	buf := NewStreamingBuffer()
	pipeline := StreamingPipelineDeps{
		MsgRepo:         msgRepo,
		DaemonHub:       fakeHub,
		StreamingBuffer: buf,
		Notifier:        notifier,
		ConvRepo:        convRepo,
	}
	d := NewDispatcher(DispatcherDeps{
		AgentRepo: &fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: taskID, Status: "pending"},
		},
		DaemonHub: fakeHub,
		MsgRepo:   msgRepo,
		Streaming: &pipeline,
	})

	msg, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "hello",
	}, DispatchHooks{})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if msg == nil || msg.Message == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Message.Status != model.MessageStatusComplete {
		t.Fatalf("expected message status complete, got %q", msg.Message.Status)
	}
	if msg.Message.Content != "stream success content" {
		t.Fatalf("expected content to be 'stream success content', got %q", msg.Message.Content)
	}
}

// TestDispatcher_StreamingFailure_DaemonNotConnected 验证 daemon 未连接时：
// placeholder 被 FinalizeStreaming(Error)，且广播 message.complete 终态事件。
func TestDispatcher_StreamingFailure_DaemonNotConnected(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-f1",
		Name:      "FailAgent1",
		CLITool:   "claude",
		MachineID: stringPtr("machine-f1"),
	}

	// fakeHub 报告未连接，SendToMachine 不应被调用
	fakeHub := &fakeDaemonDispatcher{
		isConnected: func(_ string) bool { return false },
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	d := buildStreamingDispatcher(t, fakeHub, msgRepo, notifier)

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err == nil {
		t.Fatal("expected error when daemon not connected")
	}

	// streaming placeholder 应被 FinalizeStreaming(Error)
	// fakeMsgRepo.FinalizeStreaming 把 status 写到对应 message
	var found bool
	for _, m := range msgRepo.messages {
		if m.Status == model.MessageStatusError {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected streaming placeholder to be finalized with Error status")
	}

	// 应广播 1 次 message.complete 终态事件（status=error）
	if got := notifier.CustomEventCount(); got != 1 {
		t.Fatalf("expected 1 terminal broadcast, got %d", got)
	}
}

// TestDispatcher_StreamingFailure_TaskFailed 验证 daemon task failed 时：
// placeholder 被 FinalizeStreaming(Error) + 广播终态。
func TestDispatcher_StreamingFailure_TaskFailed(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-f2",
		Name:      "FailAgent2",
		CLITool:   "claude",
		MachineID: stringPtr("machine-f2"),
	}

	promiseCh := make(chan *ws.TaskResult, 1)
	promiseCh <- &ws.TaskResult{TaskID: "task-f2", Error: "daemon crashed"}
	fakeHub := &fakeDaemonDispatcher{
		isConnected:       func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult { return promiseCh },
		sendToMachine:     func(_ string, _ ws.WSMessage) error { return nil },
		awaitTaskResult:   func(_ string) chan *ws.TaskResult { return promiseCh },
		removeTaskPromise: func(_ string) {},
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	d := buildStreamingDispatcher(t, fakeHub, msgRepo, notifier)

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err == nil {
		t.Fatal("expected error when daemon task failed")
	}

	var found bool
	for _, m := range msgRepo.messages {
		if m.Status == model.MessageStatusError {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected streaming placeholder to be finalized with Error status on task failure")
	}
	if got := notifier.CustomEventCount(); got != 1 {
		t.Fatalf("expected 1 terminal broadcast on task failure, got %d", got)
	}
}

// TestDispatcher_StreamingFailure_WaitFails 验证 waitDaemonTask 失败（channel nil）时：
// placeholder 被 FinalizeStreaming(Error) + 广播终态。
func TestDispatcher_StreamingFailure_WaitFails(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-f3",
		Name:      "FailAgent3",
		CLITool:   "claude",
		MachineID: stringPtr("machine-f3"),
	}

	// AwaitTaskResult 返回 nil → waitDaemonTask 返回 "daemon not connected for task %s"
	fakeHub := &fakeDaemonDispatcher{
		isConnected:       func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult { return make(chan *ws.TaskResult, 1) },
		sendToMachine:     func(_ string, _ ws.WSMessage) error { return nil },
		awaitTaskResult:   func(_ string) chan *ws.TaskResult { return nil },
		removeTaskPromise: func(_ string) {},
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	d := buildStreamingDispatcher(t, fakeHub, msgRepo, notifier)

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err == nil {
		t.Fatal("expected error when await channel is nil")
	}

	var found bool
	for _, m := range msgRepo.messages {
		if m.Status == model.MessageStatusError {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected streaming placeholder to be finalized with Error status on wait failure")
	}
	if got := notifier.CustomEventCount(); got != 1 {
		t.Fatalf("expected 1 terminal broadcast on wait failure, got %d", got)
	}
}

// TestDispatcher_StreamingFailure_DispatchFails 验证 SendToMachine 失败时：
// placeholder 被 FinalizeStreaming(Error) + 广播终态。
func TestDispatcher_StreamingFailure_DispatchFails(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-f4",
		Name:      "FailAgent4",
		CLITool:   "claude",
		MachineID: stringPtr("machine-f4"),
	}
	sendErr := errors.New("ws connection closed")

	fakeHub := &fakeDaemonDispatcher{
		isConnected:       func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult { return make(chan *ws.TaskResult, 1) },
		sendToMachine:     func(_ string, _ ws.WSMessage) error { return sendErr },
		awaitTaskResult:   func(_ string) chan *ws.TaskResult { return nil },
		removeTaskPromise: func(_ string) {},
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	d := buildStreamingDispatcher(t, fakeHub, msgRepo, notifier)

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err == nil {
		t.Fatal("expected error when SendToMachine fails")
	}

	var found bool
	for _, m := range msgRepo.messages {
		if m.Status == model.MessageStatusError {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected streaming placeholder to be finalized with Error status on dispatch failure")
	}
	if got := notifier.CustomEventCount(); got != 1 {
		t.Fatalf("expected 1 terminal broadcast on dispatch failure, got %d", got)
	}
}

// TestDispatcher_Streaming_BufferCleanedAfterDispatch 验证 streamingBuffer entry 在
// task 结束后被释放（defer），防止 sync.Map 无限增长。
func TestDispatcher_Streaming_BufferCleanedAfterDispatch(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-f5",
		Name:      "BufAgent",
		CLITool:   "claude",
		MachineID: stringPtr("machine-f5"),
	}

	const taskID = "task-f5"
	promiseCh := make(chan *ws.TaskResult, 1)
	promiseCh <- &ws.TaskResult{TaskID: taskID, Result: "ok"}
	fakeHub := &fakeDaemonDispatcher{
		isConnected:         func(_ string) bool { return true },
		registerTaskPromise: func(_ string) chan *ws.TaskResult { return promiseCh },
		sendToMachine:       func(_ string, _ ws.WSMessage) error { return nil },
		awaitTaskResult:     func(_ string) chan *ws.TaskResult { return promiseCh },
		removeTaskPromise:   func(_ string) {},
	}

	msgRepo := &fakeMsgRepo{}
	notifier := &fakeStreamingNotifier{}
	convRepo := &fakeStreamingConvRepo{memberIDs: []string{"u1"}}
	buf := NewStreamingBuffer()
	pipeline := StreamingPipelineDeps{
		MsgRepo:         msgRepo,
		DaemonHub:       fakeHub,
		StreamingBuffer: buf,
		Notifier:        notifier,
		ConvRepo:        convRepo,
	}
	d := NewDispatcher(DispatcherDeps{
		AgentRepo: &fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: taskID, Status: "pending"},
		},
		DaemonHub: fakeHub,
		MsgRepo:   msgRepo,
		Streaming: &pipeline,
	})

	// 给 buffer 注入一个 task entry（模拟 daemon task.progress 已累积的状态）
	buf.PushEvents(taskID, []model.AgentEvent{{Type: model.AgentEventTextDelta, Text: "hello"}})
	if _, ok := buf.GetState(taskID); !ok {
		t.Fatal("expected buffer entry to exist after PushEvents")
	}

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	// 给 defer 一点时间执行（defer 在函数返回时执行，但 sync.Map 的 Delete 同步）
	time.Sleep(10 * time.Millisecond)

	if _, ok := buf.GetState(taskID); ok {
		t.Fatal("expected buffer entry to be deleted after dispatch completion (memory leak)")
	}
}
