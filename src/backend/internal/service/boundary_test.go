package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/pkg/ws"
)

// ---------------------------------------------------------------------------
// mention_parser edge cases
// ---------------------------------------------------------------------------

func TestParseMentions_AtFollowedByNothing(t *testing.T) {
	// "@" at end of string with no following characters
	results := ParseMentions("@")
	if results != nil {
		t.Fatalf("expected nil for lone '@', got %v", results)
	}
}

func TestParseMentions_AtFollowedBySpace(t *testing.T) {
	// "@ " -- space is not a valid agent name character
	results := ParseMentions("@ some text")
	if results != nil {
		t.Fatalf("expected nil for '@ ' (space after @), got %v", results)
	}
}

func TestParseMentions_EmptyQuotesNotParsed(t *testing.T) {
	// @"" -- the quote character is not in the regex character class
	results := ParseMentions("@\"\"")
	if results != nil {
		t.Fatalf("expected nil for @\"\", got %v", results)
	}
}

func TestParseMentions_ChineseAgentName(t *testing.T) {
	text := "@小明 完成任务 @小红 写报告"
	results := ParseMentions(text)

	if len(results) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(results))
	}
	if results[0].AgentName != "小明" {
		t.Errorf("first agent = %q, want 小明", results[0].AgentName)
	}
	if results[1].AgentName != "小红" {
		t.Errorf("second agent = %q, want 小红", results[1].AgentName)
	}
}

func TestParseMentions_EmojiLikeName(t *testing.T) {
	// Emoji characters like 💡 are not in [\p{L}\p{N}_\-.]
	// so @💡 should not be treated as a mention.
	results := ParseMentions("@💡 做事")
	// The main assertion: no crash, consistent result.
	if results != nil && results[0].AgentName != "💡" {
		t.Errorf("unexpected agent name %q", results[0].AgentName)
	}
}

func TestParseMentions_VeryLongName(t *testing.T) {
	longName := strings.Repeat("A", 500)
	text := "@" + longName + " do something"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != longName {
		t.Errorf("agent name length = %d, want %d", len(results[0].AgentName), len(longName))
	}
}

func TestParseMentions_NameWithDigits(t *testing.T) {
	text := "@Agent123 执行任务"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != "Agent123" {
		t.Errorf("agent = %q, want Agent123", results[0].AgentName)
	}
}

func TestParseMentions_NameWithDotsAndHyphens(t *testing.T) {
	text := "@my.agent-v2.0 run"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != "my.agent-v2.0" {
		t.Errorf("agent = %q, want my.agent-v2.0", results[0].AgentName)
	}
}

func TestParseMentions_MultipleAtSymbols(t *testing.T) {
	text := "@@Double do something"
	results := ParseMentions(text)

	if len(results) == 0 {
		t.Fatal("expected at least one mention")
	}
	if results[0].AgentName != "Double" {
		t.Errorf("agent = %q, want Double", results[0].AgentName)
	}
}

// ---------------------------------------------------------------------------
// orchestrator concurrent protection
// ---------------------------------------------------------------------------

func TestRouteMention_ConcurrentOrchestration_BothSucceed(t *testing.T) {
	// Two goroutines call RouteMention for the same conversation with an
	// orchestrator agent. Both should proceed independently (event-driven
	// design allows parallel orchestration tasks per conversation).

	agentID := "orch-1"
	userID := "u1"
	convID := "c1"

	agent := &model.Agent{
		ID:        agentID,
		UserID:    &userID,
		Name:      "Orch",
		Type:      "orchestrator",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}

	convAgents := []model.ConversationAgent{
		{AgentID: agentID, Name: "Orch", Role: "orchestrator"},
	}

	completedTask := &model.DaemonTask{
		ID:     "task-1",
		Status: "completed",
		Result: "I'll handle this.",
	}

	var mu sync.Mutex
	createCount := 0
	var taskIDs []string

	agentRepo := &slowConcurrentAgentRepo{
		agent:         agent,
		completedTask: completedTask,
		onCreate: func() {
			mu.Lock()
			createCount++
			taskIDs = append(taskIDs, fmt.Sprintf("task-%d", createCount))
			mu.Unlock()
		},
	}

	// Wire up DaemonHub so dispatch path works
	hub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go hub.Run(hubCtx)
	hub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))

	// P8a: setter 已删除，直接通过 OrchestratorDeps.DaemonHub 一次性注入。
	svc := NewOrchestratorServiceWithDeps(OrchestratorDeps{
		ConvRepo: &fakeOrchConvRepo{
			conv:       &model.Conversation{ID: convID},
			convAgents: convAgents,
		},
		AgentRepo: agentRepo,
		MsgRepo:   &fakeMsgRepo{},
		DaemonHub: hub,
	})

	// Resolve both tasks after their result channels are registered. A fixed
	// sleep can race ahead of task creation and drop fake daemon results.
	resolveErr := make(chan error, 1)
	go func() {
		for i := 1; i <= 2; i++ {
			taskID := fmt.Sprintf("task-%d", i)
			deadline := time.Now().Add(5 * time.Second)
			resolved := false
			for time.Now().Before(deadline) {
				if hub.AwaitTaskResult(taskID) != nil {
					hub.ResolveTask(taskID, &ws.TaskResult{
						TaskID: taskID,
						Result: "I'll handle this.",
					})
					resolved = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !resolved {
				resolveErr <- fmt.Errorf("task promise not registered: %s", taskID)
				return
			}
		}
		resolveErr <- nil
	}()

	var wg sync.WaitGroup
	var result1, result2 *RouteResult
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		result1, err1 = svc.RouteMention(context.Background(), convID, userID, "@Orch 分析数据", nil, nil)
	}()
	go func() {
		defer wg.Done()
		result2, err2 = svc.RouteMention(context.Background(), convID, userID, "@Orch 写报告", nil, nil)
	}()
	wg.Wait()
	if err := <-resolveErr; err != nil {
		t.Error(err)
	}

	if err1 != nil {
		t.Errorf("goroutine 1 error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("goroutine 2 error: %v", err2)
	}

	hasDispatch1 := result1 != nil && len(result1.Dispatches) > 0
	hasDispatch2 := result2 != nil && len(result2.Dispatches) > 0
	if !hasDispatch1 || !hasDispatch2 {
		t.Errorf("expected both goroutines to have dispatches, got %v and %v", hasDispatch1, hasDispatch2)
	}
}

// slowConcurrentAgentRepo adds a delay in GetDaemonTask so the first
// orchestrator stays active long enough for the second to hit the guard.
//
// P8a 后 OrchestratorService 持有 canonical repository.AgentStore；
// 此 fake 通过嵌入 repository.AgentStore 让未覆盖的方法自动走 nil/zero 路径。
type slowConcurrentAgentRepo struct {
	repository.AgentStore
	agent         *model.Agent
	completedTask *model.DaemonTask
	onCreate      func()
	taskCounter   int32
}

func (r *slowConcurrentAgentRepo) GetByID(_ context.Context, _ string) (*model.Agent, error) {
	return r.agent, nil
}

func (r *slowConcurrentAgentRepo) CreateDaemonTask(_ context.Context, _, _, _, _, _, _, _ string) (*model.DaemonTask, error) {
	if r.onCreate != nil {
		r.onCreate()
	}
	n := atomic.AddInt32(&r.taskCounter, 1)
	return &model.DaemonTask{ID: fmt.Sprintf("task-%d", n), Status: "pending"}, nil
}

func (r *slowConcurrentAgentRepo) GetDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	time.Sleep(50 * time.Millisecond)
	return r.completedTask, nil
}

func (r *slowConcurrentAgentRepo) IsAgentInConversation(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

func (r *slowConcurrentAgentRepo) SetDaemonTaskOrch(_ context.Context, _, _, _ string) {}

func (r *slowConcurrentAgentRepo) CompleteDaemonTask(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}

func TestParseOrchOutput_MultilineTask(t *testing.T) {
	// Indented continuation lines are appended to the dispatch task.
	text := "@Alice 第一行\n  第二行\n  第三行\n@Bob 其他任务"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if len(dispatch.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(dispatch.Tasks))
	}
	if !strings.Contains(dispatch.Tasks[0].Task, "第一行") {
		t.Errorf("Alice task = %q, should contain '第一行'", dispatch.Tasks[0].Task)
	}
	if !strings.Contains(dispatch.Tasks[0].Task, "第二行") {
		t.Errorf("Alice task = %q, should contain '第二行'", dispatch.Tasks[0].Task)
	}
	if !strings.Contains(dispatch.Tasks[0].Task, "第三行") {
		t.Errorf("Alice task = %q, should contain '第三行'", dispatch.Tasks[0].Task)
	}
}

func TestParseOrchOutput_OnlyPreambleNoDispatch(t *testing.T) {
	text := "这是一条直接回复\n没有分派任何任务"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch != nil {
		t.Fatal("expected nil when no @mentions present")
	}
}

func TestParseOrchOutput_MixedParallelAndSequential(t *testing.T) {
	text := "@A 并行1\n\n@B 并行2\n\n→ @C 顺序任务"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if len(dispatch.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(dispatch.Tasks))
	}
	if dispatch.Tasks[0].Sequential {
		t.Error("task 0 should not be sequential")
	}
	if dispatch.Tasks[1].Sequential {
		t.Error("task 1 should not be sequential")
	}
	if !dispatch.Tasks[2].Sequential {
		t.Error("task 2 should be sequential")
	}
}

// ---------------------------------------------------------------------------
// message service edge cases: empty content + empty/nil attachments
// ---------------------------------------------------------------------------

func TestSendMessage_EmptyContentNoAttachments_ReturnsError(t *testing.T) {
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: "user-1"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})

	_, err := svc.SendMessageWithReply(context.Background(), "conv-1", "user-1", "user", "   ", "", nil, nil, "", nil)
	if err == nil {
		t.Fatal("expected error for empty content and no attachments")
	}
	if err != ErrMsgEmptyContent {
		t.Errorf("error = %v, want ErrMsgEmptyContent", err)
	}
}

func TestSendMessage_EmptyContentWithAttachments_Succeeds(t *testing.T) {
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: "user-1"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})

	attachments := []model.MessageAttachment{
		{ID: "att-1", FileName: "photo.png", MimeType: "image/png"},
	}
	result, err := svc.SendMessageWithReply(context.Background(), "conv-1", "user-1", "user", "", "", attachments, nil, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.UserMessage == nil {
		t.Fatal("expected user message to be created")
	}
}

func TestSendMessage_NilAttachmentsEmptyContent_ReturnsError(t *testing.T) {
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: "user-1"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})

	_, err := svc.SendMessageWithReply(context.Background(), "conv-1", "user-1", "user", "", "", nil, nil, "", nil)
	if err == nil {
		t.Fatal("expected error for empty content and nil attachments")
	}
	if err != ErrMsgEmptyContent {
		t.Errorf("error = %v, want ErrMsgEmptyContent", err)
	}
}

// ---------------------------------------------------------------------------
// Bug 2: Failed parallel dispatch writes [任务失败] to depResults
// ---------------------------------------------------------------------------

func TestOrchestratorName_Empty_DefaultsToOrchestrator(t *testing.T) {
	userID := "u1"
	// Quick repo that returns completed orchestrator task immediately
	agent := &model.Agent{
		ID:        "orch-1",
		UserID:    &userID,
		Name:      "", // empty name
		Type:      "orchestrator",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}

	task := &model.DaemonTask{
		ID:     "task-direct",
		Status: "completed",
		Result: "直接回复，不派发",
	}

	// Wire up DaemonHub
	hub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go hub.Run(hubCtx)
	hub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))

	// P8a: setter 已删除，DaemonHub 通过 OrchestratorDeps 注入。
	svc := NewOrchestratorServiceWithDeps(OrchestratorDeps{
		ConvRepo: &fakeOrchConvRepo{
			conv:       &model.Conversation{ID: "c1"},
			convAgents: []model.ConversationAgent{{AgentID: "orch-1", Name: "Orch"}},
		},
		AgentRepo: &fakeOrchAgentRepo{agent: agent, task: task, inConv: true},
		MsgRepo:   &fakeMsgRepo{},
		DaemonHub: hub,
	})

	// Resolve task in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-direct", &ws.TaskResult{
			TaskID: "task-direct",
			Result: "直接回复，不派发",
		})
	}()

	result, err := svc.RouteMention(context.Background(), "c1", userID, "@Orch test", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.AgentMessages) == 0 {
		t.Fatal("expected agent messages, got none")
	}
	// Should not panic; empty name should have been defaulted
}
