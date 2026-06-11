package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// --- Orchestrator-specific fakes (fakeMsgRepo is in message_test.go) ---

// newTestDaemonHub creates a DaemonHub with a fake connected client for the
// given machineID. The hub can be used with SetDaemonHub to make dispatch
// methods work. Tests must manually call hub.RegisterTaskPromise + hub.ResolveTask
// or use autoResolveTask after creating a daemon task.
func newTestDaemonHub(t *testing.T, machineID string) *ws.DaemonHub {
	t.Helper()
	hub := ws.NewDaemonHub(slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)

	// Insert a fake client so IsConnected returns true.
	client := ws.NewDaemonClient(nil, machineID)
	hub.RegisterTestClient(machineID, client)
	return hub
}

type fakeOrchConvRepo struct {
	conv       *model.Conversation
	convAgents []model.ConversationAgent
	convErr    error
	agentsErr  error
	member     *model.ConversationMember
	memberErr  error
}

func (f *fakeOrchConvRepo) GetByID(_ context.Context, _ string) (*model.Conversation, error) {
	return f.conv, f.convErr
}

func (f *fakeOrchConvRepo) ListAgents(_ context.Context, _, _ string) ([]model.ConversationAgent, error) {
	return f.convAgents, f.agentsErr
}

func (f *fakeOrchConvRepo) GetMember(_ context.Context, _, _ string) (*model.ConversationMember, error) {
	return f.member, f.memberErr
}

func (f *fakeOrchConvRepo) ListMemberIDs(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}

type fakeOrchAgentRepo struct {
	agent    *model.Agent
	agents   map[string]*model.Agent
	agentErr error
	task     *model.DaemonTask
	tasks    []*model.DaemonTask
	taskErr  error
	inConv   bool
	onCreate func(agentID, taskID string)
}

func (f *fakeOrchAgentRepo) GetByID(_ context.Context, id string) (*model.Agent, error) {
	if f.agents != nil {
		if agent, ok := f.agents[id]; ok {
			return agent, f.agentErr
		}
	}
	return f.agent, f.agentErr
}

func (f *fakeOrchAgentRepo) CreateDaemonTask(_ context.Context, _, _, agentID, _, _, _, _ string) (*model.DaemonTask, error) {
	if len(f.tasks) > 0 {
		task := f.tasks[0]
		f.tasks = f.tasks[1:]
		if f.onCreate != nil {
			f.onCreate(agentID, task.ID)
		}
		return task, nil
	}
	if f.task != nil {
		if f.onCreate != nil {
			f.onCreate(agentID, f.task.ID)
		}
		return f.task, nil
	}
	task := &model.DaemonTask{ID: "task-default", Status: "pending"}
	if f.onCreate != nil {
		f.onCreate(agentID, task.ID)
	}
	return task, nil
}

func (f *fakeOrchAgentRepo) GetDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	if len(f.tasks) > 0 {
		return f.tasks[0], f.taskErr
	}
	return f.task, f.taskErr
}

func (f *fakeOrchAgentRepo) IsAgentInConversation(_ context.Context, _, _, _ string) (bool, error) {
	return f.inConv, nil
}

func (f *fakeOrchAgentRepo) SetDaemonTaskOrch(_ context.Context, _, _, _ string) {}

func (f *fakeOrchAgentRepo) CompleteDaemonTask(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}

type failingOrchTaskStore struct{}

func (f failingOrchTaskStore) Create(context.Context, *model.OrchTask) error {
	return errors.New("boom")
}

func (f failingOrchTaskStore) GetByID(context.Context, string) (*model.OrchTask, error) {
	return nil, nil
}

func (f failingOrchTaskStore) UpdateStatus(context.Context, string, string) error {
	return nil
}

func (f failingOrchTaskStore) UpdateDispatchMessageID(context.Context, string, string) error {
	return nil
}

func (f failingOrchTaskStore) UpdateStatusCAS(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (f failingOrchTaskStore) UpdateWorkerResult(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}

func (f failingOrchTaskStore) SetSummaryAndEvaluate(context.Context, string, string) error {
	return nil
}

func (f failingOrchTaskStore) IncrementRound(context.Context, string) error {
	return nil
}

// --- RouteMention tests ---

func TestRouteMention_EmptyContent_ReturnsNil(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty content, got %+v", result)
	}
}

func TestRouteMention_NoMentions_ReturnsNil(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "hello world no mentions here", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
}

func TestRouteMention_UnknownAgent_ReturnsNil(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{
			conv:       &model.Conversation{ID: "c1"},
			convAgents: []model.ConversationAgent{},
		},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "@UnknownAgent please help", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for unknown agent, got %+v", result)
	}
}

func TestRouteMention_ConversationNotFound_ReturnsError(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: nil, convErr: nil},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	_, err := svc.RouteMention(context.Background(), "missing", "u1", "@Agent hello", nil, nil)
	if err == nil {
		t.Fatal("expected error for missing conversation, got nil")
	}
}

func TestRouteMention_ListAgentsError_ReturnsError(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{
			conv:      &model.Conversation{ID: "c1"},
			agentsErr: ErrMsgConvNotFound,
		},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	_, err := svc.RouteMention(context.Background(), "c1", "u1", "@Agent hello", nil, nil)
	if err == nil {
		t.Fatal("expected error when ListAgents fails, got nil")
	}
}

func TestDispatchSingleAgent_NotInConversation_ReturnsError(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-1",
		UserID:    &userID,
		Name:      "TestAgent",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{agent: agent, inConv: false, task: &model.DaemonTask{ID: "task-1"}},
		&fakeMsgRepo{},
	)

	_, err := svc.dispatchSingleAgent(context.Background(), "c1", userID, agent, "hello", "", nil)
	if err == nil {
		t.Fatal("expected error when agent not in conversation, got nil")
	}
	if err != ErrMsgAgentNoPerm {
		t.Fatalf("expected ErrMsgAgentNoPerm, got %v", err)
	}
}

func TestDispatchSingleAgent_InConversation_Succeeds(t *testing.T) {
	userID := "u1"
	agent := &model.Agent{
		ID:        "agent-2",
		UserID:    &userID,
		Name:      "TestAgent2",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}

	taskResult := "hello world"

	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{
			agent:  agent,
			inConv: true,
			task:   &model.DaemonTask{ID: "task-2", Status: "completed", Result: taskResult},
		},
		&fakeMsgRepo{},
	)

	// Wire up DaemonHub with fake connected client
	hub := newTestDaemonHub(t, "machine-1")
	svc.SetDaemonHub(hub)

	// Resolve the task promise in background after a short delay
	// (dispatch path registers the promise then waits on it)
	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-2", &ws.TaskResult{
			TaskID: "task-2",
			Result: taskResult,
		})
	}()

	msg, err := svc.dispatchSingleAgent(context.Background(), "c1", userID, agent, "hello", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if msg.Content != taskResult {
		t.Fatalf("expected %q, got %s", taskResult, msg.Content)
	}
}

func TestBuildDispatchContextIncludesParsedMathAssignment(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	orchOutput := "需要多 Agent 协作。第一轮分派如下：\n\n@123\n任务：计算 27 + 38。请只给出答案，并简要说明计算过程。\n\n@1234\n任务：计算 9 × 7。请只给出答案，并简要说明计算过程。\n\n我会在 123 和 1234 都回答后，汇总两边结果并判断是否正确，然后继续出第二轮。"
	dispatch := ParseOrchestratorOutput(orchOutput)
	if dispatch == nil || len(dispatch.Tasks) != 2 {
		t.Fatalf("expected 2 parsed worker tasks, got %#v", dispatch)
	}

	ctx, err := svc.buildDispatchContext(context.Background(), "c1", dispatch.Tasks[0], nil, "Codex")
	if err != nil {
		t.Fatalf("buildDispatchContext error: %v", err)
	}
	if !strings.Contains(ctx, "27 + 38") {
		t.Fatalf("dispatch context missing math assignment: %q", ctx)
	}
	if strings.Contains(ctx, "任务内容为空") {
		t.Fatalf("dispatch context should not contain empty-task wording: %q", ctx)
	}
}

func TestDispatchSingleAgent_SetsReplyToSourceMessage(t *testing.T) {
	userID := "u1"
	replyTo := "msg-user-1"
	agent := &model.Agent{
		ID:        "agent-2",
		UserID:    &userID,
		Name:      "TestAgent2",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}
	msgRepo := &fakeMsgRepo{}
	taskResult := "worker reply"
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{
			agent:  agent,
			inConv: true,
			task:   &model.DaemonTask{ID: "task-reply", Status: "completed", Result: taskResult},
		},
		msgRepo,
	)

	hub := newTestDaemonHub(t, "machine-1")
	svc.SetDaemonHub(hub)
	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-reply", &ws.TaskResult{
			TaskID: "task-reply",
			Result: taskResult,
		})
	}()

	msg, err := svc.dispatchSingleAgent(context.Background(), "c1", userID, agent, "hello", "", &replyTo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ReplyTo == nil || *msg.ReplyTo != replyTo {
		t.Fatalf("expected reply_to %q, got %#v", replyTo, msg.ReplyTo)
	}
	if len(msgRepo.messages) != 1 || msgRepo.messages[0].ReplyTo == nil || *msgRepo.messages[0].ReplyTo != replyTo {
		t.Fatalf("expected persisted reply_to %q, got %#v", replyTo, msgRepo.messages)
	}
}

func TestHandleOrchestratedDispatchReturnsMessageWhenLifecycleCreateFails(t *testing.T) {
	userID := "u1"
	orch := &model.Agent{
		ID:        "orch-1",
		UserID:    &userID,
		Name:      "员工2",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}
	worker := &model.Agent{
		ID:        "codex-1",
		UserID:    &userID,
		Name:      "Codex",
		Type:      "custom",
		CLITool:   "claude",
		MachineID: stringPtr("machine-1"),
	}
	type createdTask struct {
		agentID string
		taskID  string
	}
	created := make(chan createdTask, 4)
	toResolve := make(chan createdTask, 4)
	msgRepo := &fakeMsgRepo{}
	agentRepo := &fakeOrchAgentRepo{
		agents: map[string]*model.Agent{
			"orch-1":  orch,
			"codex-1": worker,
		},
		inConv: true,
		tasks: []*model.DaemonTask{
			{ID: "orch-task", Status: "pending"},
			{ID: "worker-task", Status: "pending"},
		},
		onCreate: func(agentID, taskID string) {
			event := createdTask{agentID: agentID, taskID: taskID}
			created <- event
			toResolve <- event
		},
	}
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		agentRepo,
		msgRepo,
	)
	svc.SetOrchTaskRepo(failingOrchTaskStore{})

	hub := newTestDaemonHub(t, "machine-1")
	svc.SetDaemonHub(hub)
	go func() {
		for event := range toResolve {
			time.Sleep(10 * time.Millisecond)
			result := "2"
			if event.agentID == "orch-1" {
				result = "@Codex 请计算 1+1。"
			}
			hub.ResolveTask(event.taskID, &ws.TaskResult{
				TaskID: event.taskID,
				Result: result,
			})
		}
	}()

	msgs, err := svc.handleOrchestratedDispatch(
		context.Background(),
		"c1",
		userID,
		orch,
		"@员工2 分派任务",
		[]model.ConversationAgent{
			{AgentID: "orch-1", Name: "员工2", Role: "orchestrator"},
			{AgentID: "codex-1", Name: "Codex", Role: "worker"},
		},
		"",
		stringPtr("source-msg"),
	)

	if err != nil {
		t.Fatalf("expected lifecycle failure to be non-fatal after dispatch message persistence, got %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected dispatch message to be returned for websocket push, got %d", len(msgs))
	}
	if msgs[0].Content != "@Codex 请计算 1+1。" {
		t.Fatalf("unexpected message content: %q", msgs[0].Content)
	}
	if msgs[0].ReplyTo == nil || *msgs[0].ReplyTo != "source-msg" {
		t.Fatalf("expected dispatch message to reply to source message, got %#v", msgs[0].ReplyTo)
	}
	if len(msgRepo.messages) == 0 {
		t.Fatal("expected orchestrator dispatch message to be persisted")
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case event := <-created:
			if event.agentID == "codex-1" {
				return
			}
		case <-deadline:
			t.Fatal("expected worker daemon task to be created even when orch lifecycle creation fails")
		}
	}
}

func TestBuildAgentConfigTextIncludesPlatformSkillContext(t *testing.T) {
	agent := &model.Agent{
		ID:           "agent-3",
		Name:         "SkillAgent",
		CustomSkills: `[{"name":"权限审查","description":"检查工具权限","trigger":"权限","detail":"确认 MCP 白名单和拒绝路径。"}]`,
	}
	got := BuildAgentConfigText(agent, "[群聊背景]\nhello", "请检查工具权限")
	if !strings.Contains(got, "[平台 Skills]") {
		t.Fatalf("expected platform skills section, got %s", got)
	}
	if !strings.Contains(got, "权限审查：检查工具权限") {
		t.Fatalf("expected skill index, got %s", got)
	}
	if !strings.Contains(got, "get_agent_skill") {
		t.Fatalf("expected skill lookup tool instruction, got %s", got)
	}
	if strings.Contains(got, "确认 MCP 白名单和拒绝路径。") {
		t.Fatalf("expected skill detail to stay out of prompt, got %s", got)
	}
	if !strings.Contains(got, "[群聊背景]") {
		t.Fatalf("expected original context preserved, got %s", got)
	}
}

func TestBuildConversationBlackboardContext_IncludesPinnedMessages(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{
			pinnedMessages: []model.PinnedMessage{
				{
					ConversationID: "c1",
					MessageID:      "m1",
					Role:           "user",
					Content:        "第一行\n第二行",
					Username:       "wjc",
				},
			},
			blackboard: &model.ConversationBlackboard{
				ConversationID: "c1",
				ManualContext:  "请始终使用中文回答",
			},
		},
	)

	result := svc.BuildConversationBlackboardContext(context.Background(), "c1")
	if !strings.Contains(result, "{会话上下文黑板") {
		t.Fatal("expected blackboard section")
	}
	if !strings.Contains(result, "{用户 Pin 上下文") {
		t.Fatal("expected user pin subsection")
	}
	if !strings.Contains(result, "- wjc: 第一行 第二行") {
		t.Fatalf("expected normalized pinned message, got %q", result)
	}
	if !strings.Contains(result, "{用户手写上下文") {
		t.Fatalf("expected manual context section, got %q", result)
	}
	if !strings.Contains(result, "请始终使用中文回答") {
		t.Fatalf("expected manual context content, got %q", result)
	}
}
