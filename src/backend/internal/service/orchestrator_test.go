package service

import (
	"context"
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
	agentErr error
	task     *model.DaemonTask
	taskErr  error
	inConv   bool
}

func (f *fakeOrchAgentRepo) GetByID(_ context.Context, _ string) (*model.Agent, error) {
	return f.agent, f.agentErr
}

func (f *fakeOrchAgentRepo) CreateDaemonTask(_ context.Context, _, _, _, _, _, _, _ string) (*model.DaemonTask, error) {
	return f.task, nil
}

func (f *fakeOrchAgentRepo) GetDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	return f.task, f.taskErr
}

func (f *fakeOrchAgentRepo) IsAgentInConversation(_ context.Context, _, _, _ string) (bool, error) {
	return f.inConv, nil
}

func (f *fakeOrchAgentRepo) SetDaemonTaskOrch(_ context.Context, _, _, _ string) {}

func (f *fakeOrchAgentRepo) CompleteDaemonTask(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}

// --- RouteMention tests ---

func TestRouteMention_EmptyContent_ReturnsNil(t *testing.T) {
	svc := NewOrchestratorService(
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
	)

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "", nil)
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

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "hello world no mentions here", nil)
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

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "@UnknownAgent please help", nil)
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

	_, err := svc.RouteMention(context.Background(), "missing", "u1", "@Agent hello", nil)
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

	_, err := svc.RouteMention(context.Background(), "c1", "u1", "@Agent hello", nil)
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

	_, err := svc.dispatchSingleAgent(context.Background(), "c1", userID, agent, "hello", "")
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

	msg, err := svc.dispatchSingleAgent(context.Background(), "c1", userID, agent, "hello", "")
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

func TestInjectAgentConfigIncludesPlatformSkillContext(t *testing.T) {
	agent := &model.Agent{
		ID:           "agent-3",
		Name:         "SkillAgent",
		CustomSkills: `[{"name":"权限审查","description":"检查工具权限","trigger":"权限","detail":"确认 MCP 白名单和拒绝路径。"}]`,
	}
	svc := NewOrchestratorService(nil, nil, nil)
	got := svc.InjectAgentConfig(agent, "[群聊背景]\nhello", "u1", "请检查工具权限")
	if !strings.Contains(got, "[平台 Skills]") {
		t.Fatalf("expected platform skills section, got %s", got)
	}
	if !strings.Contains(got, "权限审查：检查工具权限") {
		t.Fatalf("expected skill index, got %s", got)
	}
	if !strings.Contains(got, "确认 MCP 白名单和拒绝路径。") {
		t.Fatalf("expected matched skill detail, got %s", got)
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
