package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

type fakeMsgRepo struct {
	messages []model.Message
}

func (r *fakeMsgRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error) {
	msg := model.Message{
		ID:             role + "-1",
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		ArtifactsJSON:  artifactsJSON,
		CreatedAt:      time.Now(),
	}
	r.messages = append(r.messages, msg)
	return &msg, nil
}

func (r *fakeMsgRepo) ListByConversation(ctx context.Context, conversationID string, before time.Time, limit int) ([]model.Message, error) {
	return r.messages, nil
}

type fakeConvRepoForMsg struct {
	conv      *model.Conversation
	timestamp bool
}

func (r *fakeConvRepoForMsg) GetByID(ctx context.Context, id string) (*model.Conversation, error) {
	return r.conv, nil
}

func (r *fakeConvRepoForMsg) UpdateTimestamp(ctx context.Context, id string) error {
	r.timestamp = true
	return nil
}

type fakeAgentRepoForMsg struct {
	agent *model.Agent
	task  *model.DaemonTask
}

func (r *fakeAgentRepoForMsg) GetByID(ctx context.Context, id string) (*model.Agent, error) {
	if r.agent != nil && r.agent.ID == id {
		return r.agent, nil
	}
	return nil, nil
}

func (r *fakeAgentRepoForMsg) CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt string) (*model.DaemonTask, error) {
	r.task = &model.DaemonTask{
		ID:             "task-1",
		UserID:         userID,
		ConversationID: conversationID,
		AgentID:        agentID,
		MachineID:      machineID,
		CLITool:        cliTool,
		Prompt:         prompt,
		Status:         "pending",
	}
	return r.task, nil
}

func (r *fakeAgentRepoForMsg) GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error) {
	if r.task == nil {
		return nil, nil
	}
	r.task.Status = "completed"
	r.task.Result = "真实 CLI 回复"
	return r.task, nil
}

func TestSendMessageWithAgentCreatesAssistantReply(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent: &model.Agent{ID: "agent-1", UserID: &userID, Name: "Codex Agent", CLITool: "codex", MachineID: stringPtr("machine-1")},
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	result, err := svc.SendMessage(context.Background(), "conv-1", userID, "user", "hello", "", "agent-1")
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	if result.UserMessage == nil || result.AgentMessage == nil {
		t.Fatalf("expected user and agent messages, got %#v", result)
	}
	if result.AgentMessage.Role != "assistant" {
		t.Fatalf("expected assistant reply, got %s", result.AgentMessage.Role)
	}
	if result.AgentMessage.Content != "真实 CLI 回复" {
		t.Fatalf("expected daemon task result, got %s", result.AgentMessage.Content)
	}
	if result.AgentMessage.ArtifactsJSON == "" {
		t.Fatalf("expected agent metadata in artifacts")
	}
	if !convRepo.timestamp {
		t.Fatalf("expected conversation timestamp refreshed")
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestSendMessageRejectsForeignAgent(t *testing.T) {
	userID := "user-1"
	ownerID := "user-2"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent: &model.Agent{ID: "agent-1", UserID: &ownerID, Name: "Other Agent", CLITool: "claude"},
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	_, err := svc.SendMessage(context.Background(), "conv-1", userID, "user", "hello", "", "agent-1")
	if !errors.Is(err, ErrMsgAgentNoPerm) {
		t.Fatalf("expected ErrMsgAgentNoPerm, got %v", err)
	}
}
