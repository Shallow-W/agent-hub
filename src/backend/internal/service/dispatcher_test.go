package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/domain"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// === Router 单测 ===

// fakeAgentRepoFunc 把 RouterInput.AgentRepo 做成可注入的 closure，
// 便于在测试里精确控制 GetByID 的返回（命中/出错/nil）。
type fakeAgentRepoFunc func(ctx context.Context, id string) (*model.Agent, error)

func (f fakeAgentRepoFunc) GetByID(ctx context.Context, id string) (*model.Agent, error) {
	return f(ctx, id)
}

// makeRouterFixtures 构造一组 convAgents / mentions / mentionMap fixtures。
//   - "Codex"（worker，agentID=codex-1）
//   - "Claude"（orchestrator，agentID=claude-1）
//   - "Ghost"（不在 mentionMap 中，用于测试跳过）
func makeRouterFixtures() (convAgents []model.ConversationAgent, mentions []MentionResult, mentionMap map[string]string) {
	convAgents = []model.ConversationAgent{
		{AgentID: "codex-1", Name: "Codex", Role: string(domain.RoleWorker)},
		{AgentID: "claude-1", Name: "Claude", Role: string(domain.RoleOrchestrator)},
	}
	mentions = []MentionResult{
		{AgentName: "Codex", Task: "写测试"},
		{AgentName: "Ghost", Task: "不存在"},
		{AgentName: "Claude", Task: "协调一下"},
	}
	mentionMap = map[string]string{
		"Codex":  "codex-1",
		"Claude": "claude-1",
		// Ghost 故意不放进 map
	}
	return
}

// TestRouter_SingleWorkerResolvesOneTarget 验证单 worker @mention 解析为单个 worker target。
func TestRouter_SingleWorkerResolvesOneTarget(t *testing.T) {
	convAgents, _, _ := makeRouterFixtures()
	mentions := []MentionResult{{AgentName: "Codex", Task: "写测试"}}
	mentionMap := map[string]string{"Codex": "codex-1"}
	agents := map[string]*model.Agent{
		"codex-1": {ID: "codex-1", Name: "Codex"},
	}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return agents[id], nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "@Codex 写测试",
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Agent.ID != "codex-1" {
		t.Fatalf("expected agent codex-1, got %s", targets[0].Agent.ID)
	}
	if targets[0].Role != DispatchRoleWorker {
		t.Fatalf("expected worker role, got %s", targets[0].Role)
	}
	if targets[0].Task != "写测试" {
		t.Fatalf("expected task '写测试', got %q", targets[0].Task)
	}
	if targets[0].MentionName != "Codex" {
		t.Fatalf("expected mention name Codex, got %q", targets[0].MentionName)
	}
}

// TestRouter_MultiMentionMixedRoles 验证混合 worker + orchestrator 的多 @mention。
// 关键断言：orchestrator target 的 Task 取整条 content（不是 mention 之间的文本）。
func TestRouter_MultiMentionMixedRoles(t *testing.T) {
	convAgents, mentions, mentionMap := makeRouterFixtures()
	content := "@Codex 写测试 然后 @Claude 协调"
	agents := map[string]*model.Agent{
		"codex-1":  {ID: "codex-1", Name: "Codex"},
		"claude-1": {ID: "claude-1", Name: "Claude"},
	}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return agents[id], nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    content,
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets (Ghost skipped), got %d", len(targets))
	}
	// Codex（worker）
	if targets[0].Agent.ID != "codex-1" || targets[0].Role != DispatchRoleWorker {
		t.Fatalf("target[0] expected codex-1/worker, got %s/%s", targets[0].Agent.ID, targets[0].Role)
	}
	if targets[0].Task != "写测试" {
		t.Fatalf("worker task should be mention-scoped text, got %q", targets[0].Task)
	}
	// Claude（orchestrator）—— Task 取整条 content
	if targets[1].Agent.ID != "claude-1" || targets[1].Role != DispatchRoleOrchestrator {
		t.Fatalf("target[1] expected claude-1/orchestrator, got %s/%s", targets[1].Agent.ID, targets[1].Role)
	}
	if targets[1].Task != content {
		t.Fatalf("orchestrator task should be full content, got %q", targets[1].Task)
	}
}

// TestRouter_OrchAllDispatch 验证当消息只 @orchestrator 时，单 target 走 orch 路径。
func TestRouter_OrchAllDispatch(t *testing.T) {
	convAgents, _, _ := makeRouterFixtures()
	content := "@Claude 帮我拆任务"
	mentions := []MentionResult{{AgentName: "Claude", Task: "帮我拆任务"}}
	mentionMap := map[string]string{"Claude": "claude-1"}
	agents := map[string]*model.Agent{
		"claude-1": {ID: "claude-1", Name: "Claude"},
	}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return agents[id], nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    content,
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Role != DispatchRoleOrchestrator {
		t.Fatalf("expected orchestrator role, got %s", targets[0].Role)
	}
	if targets[0].Task != content {
		t.Fatalf("orchestrator task should be full content, got %q", targets[0].Task)
	}
}

// TestRouter_SkipsAgentNotFoundInMentionMap 验证 mentionMap 未命中的 mention 被跳过。
func TestRouter_SkipsAgentNotFoundInMentionMap(t *testing.T) {
	convAgents, _, _ := makeRouterFixtures()
	mentions := []MentionResult{
		{AgentName: "Ghost", Task: "不存在"},
		{AgentName: "Codex", Task: "写测试"},
	}
	mentionMap := map[string]string{"Codex": "codex-1"} // Ghost 不在 map
	agents := map[string]*model.Agent{
		"codex-1": {ID: "codex-1", Name: "Codex"},
	}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return agents[id], nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "x",
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 target (Ghost skipped), got %d", len(targets))
	}
	if targets[0].Agent.ID != "codex-1" {
		t.Fatalf("expected codex-1, got %s", targets[0].Agent.ID)
	}
}

// TestRouter_SkipsAgentGetByIDError 验证 GetByID 返回 error 时跳过该 target。
func TestRouter_SkipsAgentGetByIDError(t *testing.T) {
	convAgents, _, _ := makeRouterFixtures()
	mentions := []MentionResult{
		{AgentName: "Codex", Task: "写测试"},
		{AgentName: "Claude", Task: "协调"},
	}
	mentionMap := map[string]string{"Codex": "codex-1", "Claude": "claude-1"}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		if id == "codex-1" {
			return nil, errors.New("db down")
		}
		return &model.Agent{ID: id, Name: "Claude"}, nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "x",
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 target (codex errored), got %d", len(targets))
	}
	if targets[0].Agent.ID != "claude-1" {
		t.Fatalf("expected claude-1 to survive, got %s", targets[0].Agent.ID)
	}
}

// TestRouter_SkipsNilAgent 验证 GetByID 返回 nil agent 时跳过。
func TestRouter_SkipsNilAgent(t *testing.T) {
	convAgents, _, _ := makeRouterFixtures()
	mentions := []MentionResult{{AgentName: "Codex", Task: "x"}}
	mentionMap := map[string]string{"Codex": "codex-1"}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return nil, nil // agent 不存在
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "x",
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 0 {
		t.Fatalf("expected 0 targets (nil agent skipped), got %d", len(targets))
	}
}

// TestRouter_PreservesMentionOrder 验证 targets 顺序 = mentions 出现顺序。
func TestRouter_PreservesMentionOrder(t *testing.T) {
	convAgents := []model.ConversationAgent{
		{AgentID: "a1", Name: "Alpha", Role: string(domain.RoleWorker)},
		{AgentID: "b1", Name: "Beta", Role: string(domain.RoleWorker)},
		{AgentID: "c1", Name: "Gamma", Role: string(domain.RoleWorker)},
	}
	mentions := []MentionResult{
		{AgentName: "Gamma", Task: "g"},
		{AgentName: "Alpha", Task: "a"},
		{AgentName: "Beta", Task: "b"},
	}
	mentionMap := map[string]string{"Alpha": "a1", "Beta": "b1", "Gamma": "c1"}
	agents := map[string]*model.Agent{
		"a1": {ID: "a1"}, "b1": {ID: "b1"}, "c1": {ID: "c1"},
	}
	repo := fakeAgentRepoFunc(func(_ context.Context, id string) (*model.Agent, error) {
		return agents[id], nil
	})

	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "x",
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  repo.GetByID,
	})

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	wantOrder := []string{"c1", "a1", "b1"} // Gamma, Alpha, Beta
	for i, want := range wantOrder {
		if targets[i].Agent.ID != want {
			t.Fatalf("target[%d] expected %s, got %s", i, want, targets[i].Agent.ID)
		}
	}
}

// TestRouter_EmptyMentionsReturnsEmpty 验证空 mentions 返回空 targets。
func TestRouter_EmptyMentionsReturnsEmpty(t *testing.T) {
	r := NewRouter()
	targets := r.Resolve(context.Background(), RouterInput{
		Content:    "x",
		ConvAgents: nil,
		Mentions:   nil,
		MentionMap: nil,
		AgentRepo:  fakeAgentRepoFunc(func(_ context.Context, _ string) (*model.Agent, error) { return nil, nil }).GetByID,
	})
	if len(targets) != 0 {
		t.Fatalf("expected 0 targets for empty mentions, got %d", len(targets))
	}
}

// === AgentQueue 单测 ===

// TestAgentQueue_SameAgentSerialized 验证同一 agentID 的 Run 调用串行执行。
// 启动两个 Run，第二个必须等第一个释放槽位后才能进入 fn。
func TestAgentQueue_SameAgentSerialized(t *testing.T) {
	q := NewAgentQueue()

	var order []int
	var mu sync.Mutex
	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})

	// 第一个 Run：立即进入，然后阻塞在 firstRelease 上
	go func() {
		_ = q.Run(context.Background(), "agent-X", func() error {
			mu.Lock()
			order = append(order, 1)
			mu.Unlock()
			close(firstEntered)
			<-firstRelease // 等待测试放行
			return nil
		})
	}()

	<-firstEntered // 确保第一个已进入

	// 第二个 Run：必须排队
	done := make(chan error, 1)
	go func() {
		done <- q.Run(context.Background(), "agent-X", func() error {
			mu.Lock()
			order = append(order, 2)
			mu.Unlock()
			return nil
		})
	}()

	// 给一段时间确认第二个还在排队
	select {
	case <-done:
		t.Fatalf("second Run should block while first holds slot")
	case <-time.After(50 * time.Millisecond):
		// 预期：第二个还在排队
	}

	close(firstRelease) // 放行第一个
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("second Run did not complete after first released")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("expected order [1,2], got %v", order)
	}
}

// TestAgentQueue_DifferentAgentsParallel 验证不同 agentID 的 Run 调用并行执行。
func TestAgentQueue_DifferentAgentsParallel(t *testing.T) {
	q := NewAgentQueue()

	bothEntered := make(chan struct{}, 2)
	release := make(chan struct{})

	run := func(agentID string) {
		_ = q.Run(context.Background(), agentID, func() error {
			bothEntered <- struct{}{}
			<-release
			return nil
		})
	}

	go run("agent-A")
	go run("agent-B")

	// 两个都应能同时进入（并行）
	for i := 0; i < 2; i++ {
		select {
		case <-bothEntered:
		case <-time.After(1 * time.Second):
			t.Fatalf("expected both agents to enter in parallel (got %d)", i)
		}
	}
	close(release)
}

// TestAgentQueue_FnErrorPropagated 验证 fn 的返回值原样透传。
func TestAgentQueue_FnErrorPropagated(t *testing.T) {
	q := NewAgentQueue()
	wantErr := errors.New("boom")
	err := q.Run(context.Background(), "agent-E", func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
}

// TestAgentQueue_FnReturnValuePropagated 验证 fn 返回 nil 时 Run 返回 nil。
func TestAgentQueue_FnReturnValuePropagated(t *testing.T) {
	q := NewAgentQueue()
	err := q.Run(context.Background(), "agent-N", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestAgentQueue_ConcurrentStressDoesNotDeadlock 验证高并发下不 deadlock / panic。
// 启动 50 个 goroutine 各跑 10 次 Run，最后总执行次数应等于 500。
func TestAgentQueue_ConcurrentStressDoesNotDeadlock(t *testing.T) {
	q := NewAgentQueue()
	const goroutines = 50
	const perG = 10
	var counter int64

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				_ = q.Run(context.Background(), agentID, func() error {
					atomic.AddInt64(&counter, 1)
					return nil
				})
			}
		}("agent-" + string(rune('A'+g%5))) // 共 5 个不同 agentID
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("AgentQueue deadlocked under stress")
	}

	if got := atomic.LoadInt64(&counter); got != goroutines*perG {
		t.Fatalf("expected %d executions, got %d", goroutines*perG, got)
	}
}

// === Dispatcher 单测 ===
//
// Dispatcher 是 dispatchAndWait 的薄封装。这里用一个集成式单测验证：
//   - Dispatch 走完 task 创建 → WS dispatch → 等待 → 落库 全链路
//   - 错误透传（daemon 未连接时返回包装错误）
//
// 复用 orchestrator_test.go 的 newTestDaemonHub / fakeOrchAgentRepo / fakeMsgRepo。

// TestDispatcher_DispatchReturnsMessage 验证 Dispatch 成功返回落库的 assistant 消息。
func TestDispatcher_DispatchReturnsMessage(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-d1",
		UserID:    &userID,
		Name:      "DispAgent",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-d1"),
	}
	taskResult := "dispatched ok"
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: "task-d1", Status: "completed", Result: taskResult},
		},
		&fakeMsgRepo{},
	)
	hub := newTestDaemonHub(t, "machine-d1")
	svc.SetDaemonHub(hub)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-d1", &ws.TaskResult{TaskID: "task-d1", Result: taskResult})
	}()

	d := NewDispatcher(svc)
	res, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID:          "c1",
		UserID:          userID,
		Agent:           agent,
		Prompt:          "hello",
		ContextMessages: "[ctx]",
		ReplyTo:         nil,
	})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if res == nil || res.Message == nil {
		t.Fatalf("expected non-nil message")
	}
	if res.Message.Content != taskResult {
		t.Fatalf("expected content %q, got %q", taskResult, res.Message.Content)
	}
}

// TestDispatcher_DispatchDaemonNotConnectedReturnsError 验证 daemon 未连接时返回包装错误。
func TestDispatcher_DispatchDaemonNotConnectedReturnsError(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-d2",
		UserID:    &userID,
		Name:      "OfflineAgent",
		CLITool:   "claude",
		MachineID: stringPtr("machine-d2"),
	}
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: "task-d2", Status: "completed", Result: "x"},
		},
		&fakeMsgRepo{},
	)
	// 不 SetDaemonHub → svc.daemonHub 为 nil → dispatchAndWait 返回 daemon 未连接错误

	d := NewDispatcher(svc)
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "hello",
	})
	if err == nil {
		t.Fatalf("expected error when daemon not connected, got nil")
	}
}

// TestDispatcher_DispatchMany_EmptyInputReturnsEmpty 验证空输入返回空切片（不 panic）。
func TestDispatcher_DispatchMany_EmptyInputReturnsEmpty(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	d := NewDispatcher(svc)
	results, errs := d.DispatchMany(context.Background(), nil)
	if len(results) != 0 || len(errs) != 0 {
		t.Fatalf("expected empty results/errs, got %d/%d", len(results), len(errs))
	}
}

// TestFormatDispatchError_WrapsError 验证 FormatDispatchError 包装原错误。
func TestFormatDispatchError_WrapsError(t *testing.T) {
	orig := errors.New("boom")
	wrapped := FormatDispatchError("Codex", orig)
	if !errors.Is(wrapped, orig) {
		t.Fatalf("expected wrapped error to wrap orig, got %v", wrapped)
	}
	if wrapped.Error() == orig.Error() {
		t.Fatalf("expected wrapped error message to differ, got %q", wrapped.Error())
	}
}

// TestFormatDispatchError_NilReturnsNil 验证 nil 错误透传。
func TestFormatDispatchError_NilReturnsNil(t *testing.T) {
	if err := FormatDispatchError("Codex", nil); err != nil {
		t.Fatalf("expected nil for nil input, got %v", err)
	}
}
