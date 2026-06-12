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

// === P7: Router interface 可替换性单测 ===

// mockRouter 是 Router interface 的测试替身：返回固定 targets，用于验证
// OrchestratorDeps.Router 可以被任意实现替换（P7 抽 interface 的核心价值）。
type mockRouter struct {
	calls   int
	lastIn  RouterInput
	targets []DispatchTarget
}

func (m *mockRouter) Resolve(_ context.Context, in RouterInput) []DispatchTarget {
	m.calls++
	m.lastIn = in
	return m.targets
}

// TestRouter_InterfaceIsReplaceable 验证 NewDefaultRouter 返回的 Router 是 interface，
// 可被任意自定义实现替换（mockRouter），且 OrchestratorService.Router() 透传注入的实现。
//
// 这是 P7 #1 的核心断言：Router 现在是 interface，调用方可注入自定义策略。
func TestRouter_InterfaceIsReplaceable(t *testing.T) {
	want := []DispatchTarget{
		{Agent: &model.Agent{ID: "fixed-1"}, MentionName: "X", Task: "fixed", Role: DispatchRoleWorker},
	}
	mr := &mockRouter{targets: want}

	svc := NewOrchestratorServiceWithDeps(OrchestratorDeps{
		ConvRepo:  &fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		AgentRepo: &fakeOrchAgentRepo{},
		MsgRepo:   &fakeMsgRepo{},
		Router:    mr,
	})

	got := svc.Router()
	if got != mr {
		t.Fatalf("svc.Router() should return the injected mock, got %T", got)
	}

	out := got.Resolve(context.Background(), RouterInput{Content: "abc"})
	if len(out) != 1 || out[0].Agent.ID != "fixed-1" {
		t.Fatalf("expected mock targets, got %+v", out)
	}
	if mr.calls != 1 {
		t.Fatalf("expected mockRouter called once, got %d", mr.calls)
	}
	if mr.lastIn.Content != "abc" {
		t.Fatalf("expected lastIn.Content=%q, got %q", "abc", mr.lastIn.Content)
	}
}

// TestRouter_DefaultRouterImplementsInterface 编译期保证 defaultRouter 满足 Router interface。
func TestRouter_DefaultRouterImplementsInterface(t *testing.T) {
	var _ Router = defaultRouter{}
	var _ Router = NewDefaultRouter()
	var _ Router = NewRouter() // 兼容别名同样返回 interface
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

// TestAgentQueue_CtxCancelWhileWaitingReturnsErr 验证：当槽位已被占用且 ctx 取消时，
// 排队等待的 Run 立即返回 ctx.Err()，既不占用槽位也不执行 fn。
//
// 这是纯增强：原实现忽略 ctx，调用方在槽位繁忙时会无限阻塞；
// 新实现对槽位繁忙场景响应 ctx 取消。
func TestAgentQueue_CtxCancelWhileWaitingReturnsErr(t *testing.T) {
	q := NewAgentQueue()

	// 第一个 Run：占住 agentID="agent-W" 的槽位，直到 release 被关闭。
	holderEntered := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		_ = q.Run(context.Background(), "agent-W", func() error {
			close(holderEntered)
			<-release
			return nil
		})
		close(holderDone)
	}()

	<-holderEntered // 确保持有者已占住槽位

	// 第二个 Run 用可取消 ctx，预期立即返回 ctx.Err()
	ctx, cancel := context.WithCancel(context.Background())
	gotErr := make(chan error, 1)
	fnInvoked := make(chan struct{}, 1)
	go func() {
		gotErr <- q.Run(ctx, "agent-W", func() error {
			// 不应被调用：ctx 取消时应从 select 返回 ctx.Err() 而非执行 fn。
			// 用 buffered chan 记录「被错误调用」事件（避免从非 test goroutine 调 t.Fatalf）。
			select {
			case fnInvoked <- struct{}{}:
			default:
			}
			return nil
		})
	}()

	// 等待一小段，确认第二个 Run 真的卡在 select 上
	select {
	case <-gotErr:
		t.Fatalf("Run returned before ctx cancel: still blocked expected")
	case <-fnInvoked:
		t.Fatalf("fn was invoked before ctx cancel")
	case <-time.After(20 * time.Millisecond):
	}

	cancel() // 触发取消
	select {
	case err := <-gotErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Run did not return after ctx cancel")
	}

	// 释放持有者，确认槽位未泄漏（持有者正常退出）
	close(release)
	<-holderDone

	// 取消后槽位应被正确释放：新 Run 应能立即进入并执行 fn
	ran := make(chan struct{})
	go func() {
		_ = q.Run(context.Background(), "agent-W", func() error {
			close(ran)
			return nil
		})
	}()
	select {
	case <-ran:
	case <-time.After(1 * time.Second):
		t.Fatalf("slot was not released after ctx-cancelled Run returned")
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
	hub := newTestDaemonHub(t, "machine-d1")
	// P8a: setter 已删除，DaemonHub 通过 OrchestratorDeps 注入。
	svc := NewOrchestratorServiceWithDeps(OrchestratorDeps{
		ConvRepo: &fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		AgentRepo: &fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: "task-d1", Status: "completed", Result: taskResult},
		},
		MsgRepo:   &fakeMsgRepo{},
		DaemonHub: hub,
	})

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-d1", &ws.TaskResult{TaskID: "task-d1", Result: taskResult})
	}()

	d := svc.Dispatcher()
	res, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID:          "c1",
		UserID:          userID,
		Agent:           agent,
		Prompt:          "hello",
		ContextMessages: "[ctx]",
		ReplyTo:         nil,
	}, DispatchHooks{})
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

	d := svc.Dispatcher()
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "hello",
	}, DispatchHooks{})
	if err == nil {
		t.Fatalf("expected error when daemon not connected, got nil")
	}
}

// TestDispatcher_DispatchMany_EmptyInputReturnsEmpty 验证空输入返回空切片（不 panic）。
func TestDispatcher_DispatchMany_EmptyInputReturnsEmpty(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	d := svc.Dispatcher()
	results, errs := d.DispatchMany(context.Background(), nil, DispatchHooks{})
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

// === P7: Dispatcher 独立性单测 ===
//
// P7 前：Dispatcher 通过 d.svc 反向依赖 OrchestratorService。
// P7 后：Dispatcher 只依赖 DispatcherDeps。本组测试验证 Dispatcher 可脱离
// OrchestratorService 独立构造与使用（这是 #2 解反向依赖的核心断言）。

// TestDispatcher_ConstructsWithDepsOnly 验证 NewDispatcher 不再接受 *OrchestratorService，
// 而是接受 DispatcherDeps。编译期保证 + 运行时确认 Dispatcher 持有 deps。
func TestDispatcher_ConstructsWithDepsOnly(t *testing.T) {
	agentRepo := &fakeOrchAgentRepo{}
	msgRepo := &fakeMsgRepo{}
	d := NewDispatcher(DispatcherDeps{
		AgentRepo: agentRepo,
		MsgRepo:   msgRepo,
		// DaemonHub 留 nil（验证 Dispatcher 可在 hub 未就绪时构造）
	})
	if d.deps.AgentRepo != agentRepo {
		t.Fatal("Dispatcher.deps.AgentRepo should equal injected agentRepo")
	}
	if d.deps.MsgRepo != msgRepo {
		t.Fatal("Dispatcher.deps.MsgRepo should equal injected msgRepo")
	}
	if d.deps.DaemonHub != nil {
		t.Fatalf("Dispatcher.deps.DaemonHub should be nil when not injected, got %v", d.deps.DaemonHub)
	}
}

// TestDispatcher_DispatchFailsWhenDaemonHubNil 验证 Dispatcher 在没有 OrchestratorService
// 的场景下，daemon hub 缺失时返回预期错误（"agent %q 的 daemon 未通过 WS 连接"）。
// 这是 #2 解耦的关键测试：Dispatcher 不需要 svc 即可独立完成 dispatch 流程判断。
func TestDispatcher_DispatchFailsWhenDaemonHubNil(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{ID: "agent-x", Name: "XAgent", CLITool: "claude", MachineID: stringPtr("machine-x")}
	d := NewDispatcher(DispatcherDeps{
		AgentRepo: &fakeOrchAgentRepo{
			agent: agent,
			task:  &model.DaemonTask{ID: "task-x", Status: "completed", Result: "ok"},
		},
		MsgRepo:   &fakeMsgRepo{},
		DaemonHub: nil, // 故意不注入
	})

	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err == nil {
		t.Fatal("expected error when DaemonHub is nil")
	}
}

// === Dispatcher hook 机制单测 ===
//
// 这一组测试验证 P6 新增的 DispatchHooks：
//   - 零值 hooks 等价于直接调 dispatchAndWait
//   - PreDispatch error 中止派发
//   - OnTaskCreated 在 daemon task 创建后、WS 推送前触发
//   - OnMessagePersisted 在消息落库后触发
//   - OnFailed 在 PreDispatch 失败 / 派发失败时触发

// makeDispatcherTestSvc 构造一个用于 Dispatcher hook 测试的最小 svc：
// 已连接的 daemonHub + 能落库的 fake msgRepo。
//
// P8a 后 setter 已删除，DaemonHub 通过 OrchestratorDeps.DaemonHub 一次性注入。
func makeDispatcherTestSvc(t *testing.T, machineID string) (*OrchestratorService, *ws.DaemonHub) {
	t.Helper()
	hub := newTestDaemonHub(t, machineID)
	svc := NewOrchestratorServiceWithDeps(OrchestratorDeps{
		ConvRepo: &fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		AgentRepo: &fakeOrchAgentRepo{
			agent: &model.Agent{
				ID:        "agent-h",
				Name:      "HookAgent",
				CLITool:   "claude",
				MachineID: stringPtr(machineID),
			},
			task: &model.DaemonTask{ID: "task-h", Status: "completed", Result: "ok"},
		},
		MsgRepo:   &fakeMsgRepo{},
		DaemonHub: hub,
	})
	return svc, hub
}

// TestDispatchHooks_ZeroHooksEquivalentToDispatchAndWait 验证零值 hooks 时
// Dispatch 的行为与直接调 dispatchAndWait 完全一致（成功路径返回相同 message）。
func TestDispatchHooks_ZeroHooksEquivalentToDispatchAndWait(t *testing.T) {
	svc, hub := makeDispatcherTestSvc(t, "machine-h0")
	taskResult := "result-zero-hooks"
	svc.agentRepo.(*fakeOrchAgentRepo).task = &model.DaemonTask{ID: "task-h0", Status: "completed", Result: taskResult}

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-h0", &ws.TaskResult{TaskID: "task-h0", Result: taskResult})
	}()

	userID := "u1"
	agent := &model.Agent{ID: "agent-h0", Name: "HookAgent0", CLITool: "claude", MachineID: stringPtr("machine-h0")}
	svc.agentRepo.(*fakeOrchAgentRepo).agent = agent

	d := svc.Dispatcher()
	res, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{})
	if err != nil {
		t.Fatalf("zero-hooks Dispatch returned error: %v", err)
	}
	if res == nil || res.Message == nil {
		t.Fatalf("expected non-nil message")
	}
	if res.Message.Content != taskResult {
		t.Fatalf("expected content %q, got %q", taskResult, res.Message.Content)
	}
}

// TestDispatchHooks_PreDispatchAbortsOnErr 验证 PreDispatch 返回 error 时
// 立即中止派发，OnTaskCreated / OnMessagePersisted 都不会被调用，
// OnFailed 会被调用一次（携带 PreDispatch 的错误）。
func TestDispatchHooks_PreDispatchAbortsOnErr(t *testing.T) {
	svc, _ := makeDispatcherTestSvc(t, "machine-h1")
	userID := "u1"
	agent := &model.Agent{ID: "agent-h1", Name: "HookAgent1", CLITool: "claude", MachineID: stringPtr("machine-h1")}
	svc.agentRepo.(*fakeOrchAgentRepo).agent = agent

	preErr := errors.New("permission denied")
	var preCalled, taskCreated, msgPersisted, failed bool

	d := svc.Dispatcher()
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{
		PreDispatch: func(_ context.Context, _ DispatchInput) error {
			preCalled = true
			return preErr
		},
		OnTaskCreated: func(_ context.Context, _ *model.DaemonTask) {
			taskCreated = true
		},
		OnMessagePersisted: func(_ context.Context, _ *model.Message) {
			msgPersisted = true
		},
		OnFailed: func(_ context.Context, _ DispatchInput, ferr error) {
			failed = true
			if !errors.Is(ferr, preErr) {
				t.Fatalf("OnFailed got unexpected err: %v", ferr)
			}
		},
	})
	if !errors.Is(err, preErr) {
		t.Fatalf("expected preErr to propagate, got %v", err)
	}
	if !preCalled {
		t.Fatal("PreDispatch should be called")
	}
	if taskCreated {
		t.Fatal("OnTaskCreated should NOT be called when PreDispatch fails")
	}
	if msgPersisted {
		t.Fatal("OnMessagePersisted should NOT be called when PreDispatch fails")
	}
	if !failed {
		t.Fatal("OnFailed should be called when PreDispatch fails")
	}
}

// TestDispatchHooks_OnTaskCreatedFiresBeforeWS 验证 OnTaskCreated 在 daemon task
// 创建成功后、WS 推送前被触发，可拿到 task.ID。
func TestDispatchHooks_OnTaskCreatedFiresBeforeWS(t *testing.T) {
	svc, hub := makeDispatcherTestSvc(t, "machine-h2")
	taskResult := "ok"
	taskID := "task-h2"
	svc.agentRepo.(*fakeOrchAgentRepo).task = &model.DaemonTask{ID: taskID, Status: "completed", Result: taskResult}
	userID := "u1"
	agent := &model.Agent{ID: "agent-h2", Name: "HookAgent2", CLITool: "claude", MachineID: stringPtr("machine-h2")}
	svc.agentRepo.(*fakeOrchAgentRepo).agent = agent

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask(taskID, &ws.TaskResult{TaskID: taskID, Result: taskResult})
	}()

	var capturedTaskID string
	d := svc.Dispatcher()
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{
		OnTaskCreated: func(_ context.Context, task *model.DaemonTask) {
			capturedTaskID = task.ID
		},
	})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if capturedTaskID != taskID {
		t.Fatalf("OnTaskCreated expected task.ID=%q, got %q", taskID, capturedTaskID)
	}
}

// TestDispatchHooks_OnMessagePersistedFiresAfterCreate 验证 OnMessagePersisted
// 在消息落库后被调用，能拿到落库的 message。
func TestDispatchHooks_OnMessagePersistedFiresAfterCreate(t *testing.T) {
	svc, hub := makeDispatcherTestSvc(t, "machine-h3")
	taskResult := "summary-payload"
	taskID := "task-h3"
	svc.agentRepo.(*fakeOrchAgentRepo).task = &model.DaemonTask{ID: taskID, Status: "completed", Result: taskResult}
	userID := "u1"
	agent := &model.Agent{ID: "agent-h3", Name: "HookAgent3", CLITool: "claude", MachineID: stringPtr("machine-h3")}
	svc.agentRepo.(*fakeOrchAgentRepo).agent = agent

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask(taskID, &ws.TaskResult{TaskID: taskID, Result: taskResult})
	}()

	var capturedContent string
	d := svc.Dispatcher()
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{
		OnMessagePersisted: func(_ context.Context, msg *model.Message) {
			capturedContent = msg.Content
		},
	})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if capturedContent != taskResult {
		t.Fatalf("OnMessagePersisted expected content %q, got %q", taskResult, capturedContent)
	}
}

// TestDispatchHooks_OnFailedFiresOnDaemonDown 验证派发失败（daemon 未连接）时
// OnFailed 被调用且携带原始错误。
func TestDispatchHooks_OnFailedFiresOnDaemonDown(t *testing.T) {
	// 故意不 SetDaemonHub → svc.daemonHub 为 nil
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{
			agent: &model.Agent{ID: "agent-h4", Name: "HookAgent4", CLITool: "claude", MachineID: stringPtr("machine-h4")},
			task:  &model.DaemonTask{ID: "task-h4", Status: "completed", Result: "x"},
		},
		&fakeMsgRepo{},
	)
	userID := "u1"
	agent := &model.Agent{ID: "agent-h4", Name: "HookAgent4", CLITool: "claude", MachineID: stringPtr("machine-h4")}

	var failedErr error
	d := svc.Dispatcher()
	_, err := d.Dispatch(context.Background(), DispatchInput{
		ConvID: "c1",
		UserID: userID,
		Agent:  agent,
		Prompt: "p",
	}, DispatchHooks{
		OnFailed: func(_ context.Context, _ DispatchInput, ferr error) {
			failedErr = ferr
		},
	})
	if err == nil {
		t.Fatal("expected error when daemon hub is nil")
	}
	if failedErr == nil {
		t.Fatal("OnFailed should be called when daemon hub is nil")
	}
	if !errors.Is(failedErr, err) {
		t.Fatalf("OnFailed err should match Dispatch err: got %v want %v", failedErr, err)
	}
}
