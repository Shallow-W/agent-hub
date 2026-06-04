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

func (r *fakeMsgRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error) {
	msg := model.Message{
		ID:             role + "-1",
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		ArtifactsJSON:  artifactsJSON,
		CreatedAt:      time.Now(),
		Attachments:    attachments,
		ReplyTo:        replyTo,
		SenderID:       senderID,
		Mentions:       mentions,
	}
	r.messages = append(r.messages, msg)
	return &msg, nil
}

func (r *fakeMsgRepo) ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepo) MarkConversationRead(ctx context.Context, conversationID, userID string) error {
	return nil
}

func (r *fakeMsgRepo) GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepo) GetByID(ctx context.Context, id string) (*model.Message, error) {
	for i := range r.messages {
		if r.messages[i].ID == id {
			return &r.messages[i], nil
		}
	}
	return nil, nil
}

func (r *fakeMsgRepo) GetMessageSender(ctx context.Context, messageID string) (string, error) {
	return "user-1", nil
}

func (r *fakeMsgRepo) SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepo) SoftDelete(ctx context.Context, messageID string) error {
	return nil
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

func (r *fakeConvRepoForMsg) GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error) {
	if r.conv != nil && r.conv.ID == conversationID && r.conv.UserID == userID {
		return &model.ConversationMember{ConversationID: conversationID, UserID: userID, Role: "owner"}, nil
	}
	return nil, nil
}

func (r *fakeConvRepoForMsg) ListMemberIDs(ctx context.Context, conversationID string) ([]string, error) {
	if r.conv == nil {
		return []string{}, nil
	}
	return []string{r.conv.UserID}, nil
}

func (r *fakeConvRepoForMsg) ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error) {
	return nil, nil
}

type fakeAgentRepoForMsg struct {
	agent          *model.Agent
	task           *model.DaemonTask
	inConversation bool
}

func (r *fakeAgentRepoForMsg) GetByID(ctx context.Context, id string) (*model.Agent, error) {
	if r.agent != nil && r.agent.ID == id {
		return r.agent, nil
	}
	return nil, nil
}

func (r *fakeAgentRepoForMsg) IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error) {
	return r.inConversation, nil
}

func (r *fakeAgentRepoForMsg) CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error) {
	r.task = &model.DaemonTask{
		ID:              "task-1",
		UserID:          userID,
		ConversationID:  conversationID,
		AgentID:         agentID,
		MachineID:       machineID,
		CLITool:         cliTool,
		Prompt:          prompt,
		ContextMessages: contextMessages,
		Status:          "pending",
	}
	return r.task, nil
}

func (r *fakeAgentRepoForMsg) GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error) {
	if r.task == nil {
		return nil, nil
	}
	r.task.Status = "completed"
	r.task.Result = "鐪熷疄 CLI 鍥炲"
	return r.task, nil
}

func TestSendMessageWithAgentCreatesAssistantReply(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent:          &model.Agent{ID: "agent-1", UserID: &userID, Name: "Codex Agent", CLITool: "codex", MachineID: stringPtr("machine-1")},
		inConversation: true,
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	result, err := svc.SendMessageWithReply(context.Background(), "conv-1", userID, "user", "hello", "", nil, nil, "agent-1", nil)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	if result.UserMessage == nil {
		t.Fatalf("expected user message, got %#v", result)
	}
	if result.AgentMessage != nil {
		t.Fatalf("expected async agent reply, got immediate message %#v", result.AgentMessage)
	}
	var agentMessage *model.Message
	for i := 0; i < 10; i++ {
		for j := range msgRepo.messages {
			if msgRepo.messages[j].Role == "assistant" {
				agentMessage = &msgRepo.messages[j]
				break
			}
		}
		if agentMessage != nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if agentMessage == nil {
		t.Fatalf("expected async assistant reply, got messages %#v", msgRepo.messages)
	}
	if agentMessage.Content != "鐪熷疄 CLI 鍥炲" {
		t.Fatalf("expected daemon task result, got %s", agentMessage.Content)
	}
	if agentMessage.ArtifactsJSON == "" {
		t.Fatalf("expected agent metadata in artifacts")
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

	_, err := svc.createAgentReply(context.Background(), "conv-1", userID, "agent-1", "hello", "")
	if !errors.Is(err, ErrMsgAgentNoPerm) {
		t.Fatalf("expected ErrMsgAgentNoPerm, got %v", err)
	}
}
