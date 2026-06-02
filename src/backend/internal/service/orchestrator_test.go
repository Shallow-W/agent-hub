package service

import (
	"context"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

// --- Orchestrator-specific fakes (fakeMsgRepo is in message_test.go) ---

type fakeOrchConvRepo struct {
	conv       *model.Conversation
	convAgents []model.ConversationAgent
	convErr    error
	agentsErr  error
}

func (f *fakeOrchConvRepo) GetByID(_ context.Context, _ string) (*model.Conversation, error) {
	return f.conv, f.convErr
}

func (f *fakeOrchConvRepo) ListAgents(_ context.Context, _, _ string) ([]model.ConversationAgent, error) {
	return f.convAgents, f.agentsErr
}

type fakeOrchAgentRepo struct {
	agent    *model.Agent
	agentErr error
	task     *model.DaemonTask
	taskErr  error
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
