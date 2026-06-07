package service

import (
	"context"
	"log/slog"
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

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "")
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

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "hello world no mentions here")
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

	result, err := svc.RouteMention(context.Background(), "c1", "u1", "@UnknownAgent please help")
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

	_, err := svc.RouteMention(context.Background(), "missing", "u1", "@Agent hello")
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

	_, err := svc.RouteMention(context.Background(), "c1", "u1", "@Agent hello")
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
