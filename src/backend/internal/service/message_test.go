package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

type fakeMsgRepo struct {
	messages       []model.Message
	pinnedMessages []model.PinnedMessage
	blackboard     *model.ConversationBlackboard
	savedArtifacts map[string][]model.Artifact
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

func (r *fakeMsgRepo) SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error {
	if r.savedArtifacts == nil {
		r.savedArtifacts = make(map[string][]model.Artifact)
	}
	r.savedArtifacts[messageID] = artifacts
	return nil
}

func (r *fakeMsgRepo) PinMessage(ctx context.Context, conversationID, messageID, userID string) (*model.MessagePin, error) {
	return &model.MessagePin{
		ID:             "pin-1",
		ConversationID: conversationID,
		MessageID:      messageID,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
	}, nil
}

func (r *fakeMsgRepo) UnpinMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}

func (r *fakeMsgRepo) ListPinnedMessages(ctx context.Context, conversationID string, limit int) ([]model.PinnedMessage, error) {
	return r.pinnedMessages, nil
}

func (r *fakeMsgRepo) GetConversationBlackboard(ctx context.Context, conversationID string) (*model.ConversationBlackboard, error) {
	if r.blackboard != nil {
		return r.blackboard, nil
	}
	return &model.ConversationBlackboard{ConversationID: conversationID, ManualContext: ""}, nil
}

func (r *fakeMsgRepo) UpsertConversationBlackboard(ctx context.Context, conversationID, manualContext, userID string) (*model.ConversationBlackboard, error) {
	r.blackboard = &model.ConversationBlackboard{
		ConversationID: conversationID,
		ManualContext:  manualContext,
		UpdatedBy:      &userID,
		UpdatedAt:      time.Now(),
	}
	return r.blackboard, nil
}

func (r *fakeMsgRepo) ListReplies(ctx context.Context, messageID string) ([]model.Message, error) {
	var replies []model.Message
	for _, m := range r.messages {
		if m.ReplyTo != nil && *m.ReplyTo == messageID {
			replies = append(replies, m)
		}
	}
	return replies, nil
}

// UpdateMessageCards 记录最近一次写入的 cards_json（测试可断言）。
// 同时把解析后的 cards 结构化字段写回 slice，便于测试从 msgRepo.messages 直接断言 .Cards。
func (r *fakeMsgRepo) UpdateMessageCards(ctx context.Context, messageID, cardsJSON string) error {
	for i := range r.messages {
		if r.messages[i].ID == messageID {
			r.messages[i].CardsJSON = cardsJSON
			var parsed []map[string]any
			if err := json.Unmarshal([]byte(cardsJSON), &parsed); err == nil {
				r.messages[i].Cards = parsed
			}
			return nil
		}
	}
	return nil
}

// HideMessage / UnhideMessage / GetHiddenMessageIDs —— message_hides 功能的 no-op 桩。
func (r *fakeMsgRepo) HideMessage(ctx context.Context, userID, messageID string) error     { return nil }
func (r *fakeMsgRepo) UnhideMessage(ctx context.Context, userID, messageID string) error   { return nil }
func (r *fakeMsgRepo) GetHiddenMessageIDs(ctx context.Context, userID, conversationID string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type fakeConvRepoForMsg struct {
	conv      *model.Conversation
	agents    []model.ConversationAgent
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
	return r.agents, nil
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

type fakeMessageDeliveryState struct{}

func (c *fakeMessageDeliveryState) EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error {
	return nil
}

func (c *fakeMessageDeliveryState) DequeueOfflineAfter(ctx context.Context, userID, conversationID string, after interface{}) ([]model.Message, error) {
	return nil, nil
}

func (c *fakeMessageDeliveryState) ClearUnread(ctx context.Context, userID, conversationID string) error {
	return nil
}

func (c *fakeMessageDeliveryState) IncrementUnread(ctx context.Context, userID, conversationID string) error {
	return nil
}

func TestSendMessageWithAgentCreatesAssistantReply(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "agent"},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent:          &model.Agent{ID: "agent-1", UserID: &userID, Name: "Codex Agent", CLITool: "codex", MachineID: stringPtr("machine-1")},
		inConversation: true,
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	// Wire up DaemonHub for WS-based dispatch
	hub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go hub.Run(hubCtx)
	hub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))
	svc.SetDaemonHub(hub)

	// Resolve the daemon task in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-1", &ws.TaskResult{
			TaskID: "task-1",
			Result: "daemon task result",
		})
	}()

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
	if agentMessage.Content != "daemon task result" {
		t.Fatalf("expected daemon task result, got %s", agentMessage.Content)
	}
	if agentMessage.ArtifactsJSON == "" {
		t.Fatalf("expected agent metadata in artifacts")
	}
}

func TestSendMessageAgentChatFallsBackToConversationAgent(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv:   &model.Conversation{ID: "conv-1", UserID: userID, Type: "agent"},
		agents: []model.ConversationAgent{{AgentID: "agent-1"}},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent:          &model.Agent{ID: "agent-1", UserID: &userID, Name: "OpenCode", CLITool: "opencode", MachineID: stringPtr("machine-1")},
		inConversation: true,
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	hub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go hub.Run(hubCtx)
	hub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))
	svc.SetDaemonHub(hub)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-1", &ws.TaskResult{
			TaskID: "task-1",
			Result: "fallback agent result",
		})
	}()

	_, err := svc.SendMessageWithReply(context.Background(), "conv-1", userID, "user", "hello", "", nil, nil, "", nil)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		for _, msg := range msgRepo.messages {
			if msg.Role == "assistant" && msg.Content == "fallback agent result" {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("expected fallback assistant reply, got %#v", msgRepo.messages)
}

func stringPtr(value string) *string {
	return &value
}

func TestArtifactsFromTaskResult(t *testing.T) {
	// 跳过无 type 的产物，version 默认 1，字段透传
	got := artifactsFromTaskResult([]ws.ArtifactResult{
		{Type: "code", Language: "go", Filename: "main.go", Content: "package main"},
		{Type: "", Content: "应被跳过"},
		{Type: "webpage", URL: "https://example.com", Title: "Demo"},
		{Type: "document", Language: "markdown", Filename: "notes.md", Title: "Notes", Content: "# Notes"},
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(got))
	}
	if got[0].Type != "code" || got[0].Language != "go" || got[0].Filename != "main.go" || got[0].Content != "package main" {
		t.Fatalf("code artifact mismatch: %+v", got[0])
	}
	if got[0].Version != 1 {
		t.Fatalf("expected default version 1, got %d", got[0].Version)
	}
	if got[1].Type != "webpage" || got[1].URL != "https://example.com" || got[1].Title != "Demo" {
		t.Fatalf("webpage artifact mismatch: %+v", got[1])
	}
	if got[2].Type != "document" || got[2].Language != "markdown" || got[2].Filename != "notes.md" || got[2].Content != "# Notes" {
		t.Fatalf("document artifact mismatch: %+v", got[2])
	}
}

func TestArtifactsFromMarkdown_CodeFence(t *testing.T) {
	got := artifactsFromMarkdown("here\n```python\n# file: demo.py\nprint('hi')\n```\n")
	if len(got) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(got))
	}
	if got[0].Type != "code" || got[0].Language != "python" || got[0].Filename != "demo.py" || got[0].Content != "print('hi')" {
		t.Fatalf("artifact mismatch: %+v", got[0])
	}
}

func TestGetHistoryBackfillsMarkdownArtifacts(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{
		messages: []model.Message{
			{
				ID:             "msg-1",
				ConversationID: "conv-1",
				Role:           "assistant",
				Content:        "code:\n```python\nprint('hi')\n```",
				CreatedAt:      time.Now(),
			},
		},
	}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "group"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})

	messages, err := svc.GetHistory(context.Background(), "conv-1", userID, nil, 20)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Artifacts) != 1 {
		t.Fatalf("expected returned artifact, got %+v", messages)
	}
	if messages[0].Artifacts[0].Type != "code" || messages[0].Artifacts[0].Content != "print('hi')" {
		t.Fatalf("returned artifact mismatch: %+v", messages[0].Artifacts[0])
	}
	saved := msgRepo.savedArtifacts["msg-1"]
	if len(saved) != 1 || saved[0].Language != "python" {
		t.Fatalf("expected saved artifact, got %+v", saved)
	}
}

func TestGetHistoryBackfillsCodeWhenDocumentArtifactExists(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{
		messages: []model.Message{
			{
				ID:             "msg-1",
				ConversationID: "conv-1",
				Role:           "assistant",
				Content:        "code:\n```python\nprint('hi')\n```",
				CreatedAt:      time.Now(),
				Artifacts: []model.Artifact{
					{ID: "doc-1", Type: "document", Language: "markdown", Content: "# doc"},
				},
			},
		},
	}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "group"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})

	messages, err := svc.GetHistory(context.Background(), "conv-1", userID, nil, 20)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Artifacts) != 2 {
		t.Fatalf("expected document plus code artifact, got %+v", messages)
	}
	if !hasCodeArtifact(messages[0].Artifacts) {
		t.Fatalf("expected code artifact, got %+v", messages[0].Artifacts)
	}
	saved := msgRepo.savedArtifacts["msg-1"]
	if len(saved) != 1 || saved[0].Type != "code" {
		t.Fatalf("expected saved code artifact only, got %+v", saved)
	}
}

func TestCreateAgentReplyPersistsArtifacts(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "agent"},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent:          &model.Agent{ID: "agent-1", UserID: &userID, Name: "Codex Agent", CLITool: "codex", MachineID: stringPtr("machine-1")},
		inConversation: true,
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)
	daemonHub := ws.NewDaemonHub(slog.Default())
	daemonHub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))
	svc.SetDaemonHub(daemonHub)

	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if agentRepo.task != nil && daemonHub.AwaitTaskResult(agentRepo.task.ID) != nil {
				daemonHub.ResolveTask(agentRepo.task.ID, &ws.TaskResult{
					TaskID: agentRepo.task.ID,
					Result: "done",
					Artifacts: []ws.ArtifactResult{
						{Type: "code", Language: "python", Content: "print('hi')"},
					},
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if _, err := svc.SendMessageWithReply(context.Background(), "conv-1", userID, "user", "hello", "", nil, nil, "agent-1", nil); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	var assistantMsg *model.Message
	for i := 0; i < 15; i++ {
		for j := range msgRepo.messages {
			if msgRepo.messages[j].Role == "assistant" {
				assistantMsg = &msgRepo.messages[j]
				break
			}
		}
		if assistantMsg != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if assistantMsg == nil {
		t.Fatalf("expected async assistant reply, got %#v", msgRepo.messages)
	}
	saved := msgRepo.savedArtifacts[assistantMsg.ID]
	if len(saved) != 1 || saved[0].Language != "python" || saved[0].Content != "print('hi')" {
		t.Fatalf("expected persisted artifact, got %+v", saved)
	}
}

func TestGetHistoryEnrichesAttachmentURLs(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{
		messages: []model.Message{
			{
				ID:             "msg-1",
				ConversationID: "conv-1",
				Role:           "user",
				Content:        "image",
				CreatedAt:      time.Now(),
				Attachments: []model.MessageAttachment{
					{
						FileName:      "demo.png",
						MimeType:      "image/png",
						FilePath:      "uploads/originals/demo.png",
						ThumbnailPath: "uploads/thumbnails/demo.jpg",
					},
				},
			},
		},
	}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "single"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})
	svc.SetFileURLBuilder(NewFileURLBuilder("http://111.228.35.61:8080"))

	messages, err := svc.GetHistory(context.Background(), "conv-1", userID, nil, 20)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Attachments) != 1 {
		t.Fatalf("expected one attachment, got %+v", messages)
	}
	att := messages[0].Attachments[0]
	if att.URL != "http://111.228.35.61:8080/api/uploads/originals/demo.png" {
		t.Fatalf("attachment URL = %q", att.URL)
	}
	if att.ThumbnailURL != "http://111.228.35.61:8080/api/uploads/thumbnails/demo.jpg" {
		t.Fatalf("thumbnail URL = %q", att.ThumbnailURL)
	}
}

func TestGetHistoryUsesRepositoryEvenWithDeliveryState(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{
		messages: []model.Message{
			{
				ID:             "db-msg",
				ConversationID: "conv-1",
				Role:           "user",
				Content:        "fresh db message",
				CreatedAt:      time.Now(),
			},
		},
	}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "single"},
	}
	svc := NewMessageService(msgRepo, convRepo, &fakeAgentRepoForMsg{})
	svc.SetCacher(&fakeMessageDeliveryState{})

	messages, err := svc.GetHistory(context.Background(), "conv-1", userID, nil, 200)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one repository message, got %+v", messages)
	}
	if messages[0].ID != "db-msg" {
		t.Fatalf("GetHistory returned %q, want repository message", messages[0].ID)
	}
}

func TestSendMessageRejectsForeignAgent(t *testing.T) {
	userID := "user-1"
	ownerID := "user-2"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "agent"},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent: &model.Agent{ID: "agent-1", UserID: &ownerID, Name: "Other Agent", CLITool: "claude"},
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)

	_, err := svc.createAgentReply(context.Background(), "conv-1", userID, "agent-1", "hello", "", nil)
	if !errors.Is(err, ErrMsgAgentNoPerm) {
		t.Fatalf("expected ErrMsgAgentNoPerm, got %v", err)
	}
}

// TestExtractCardsFromContent 覆盖 fenced block 解析、合并、占位符剥离等行为。
func TestExtractCardsFromContent(t *testing.T) {
	cases := []struct {
		name                      string
		input                     string
		expectedCardsLen          int
		expectedStrippedContains  []string
		expectedStrippedNotContain []string
		expectedCardsJSONContains []string
		expectCardIDNonEmpty      bool // for block_without_id case
	}{
		{
			name: "single_block_single_card",
			input: "我先解释一下：\n```json\n" +
				`{"cards":[{"type":"info","id":"i1","fields":{}}]}` + "\n```\n尾文本",
			expectedCardsLen:          1,
			expectedStrippedContains:  []string{"[CARD:i1]", "我先解释一下：", "尾文本"},
			expectedStrippedNotContain: []string{"```json", "\"cards\""},
			expectedCardsJSONContains: []string{`"type":"info"`, `"id":"i1"`},
		},
		{
			name: "single_block_multi_card",
			input: "开头\n```json\n" +
				`{"cards":[{"type":"info","id":"c1"},{"type":"diff","id":"c2"}]}` + "\n```\n结尾",
			expectedCardsLen:          2,
			expectedStrippedContains:  []string{"[CARD:c1]", "[CARD:c2]"},
			expectedStrippedNotContain: []string{"```json", "\"cards\""},
			expectedCardsJSONContains: []string{`"id":"c1"`, `"id":"c2"`},
		},
		{
			name: "multi_block_multi_card",
			input: "头部\n```json\n" +
				`{"cards":[{"type":"info","id":"a1"}]}` + "\n```\n中间文字\n```JSON\n" +
				`{"cards":[{"type":"diff","id":"b1"}]}` + "\n```\n尾部",
			expectedCardsLen:          2,
			expectedStrippedContains:  []string{"[CARD:a1]", "[CARD:b1]", "中间文字"},
			expectedStrippedNotContain: []string{"```json", "```JSON", "\"cards\""},
			expectedCardsJSONContains: []string{`"id":"a1"`, `"id":"b1"`},
		},
		{
			name: "block_in_middle",
			input: "前文说明\n```json\n" +
				`{"cards":[{"type":"info","id":"mid1"}]}` + "\n```\n后文说明",
			expectedCardsLen:          1,
			expectedStrippedContains:  []string{"[CARD:mid1]", "前文说明", "后文说明"},
			expectedStrippedNotContain: []string{"```json", "\"cards\""},
			expectedCardsJSONContains: []string{`"id":"mid1"`},
		},
		{
			name: "block_without_id",
			input: "```json\n" +
				`{"cards":[{"type":"info","fields":{}}]}` + "\n```\n",
			expectedCardsLen:         1,
			expectCardIDNonEmpty:     true,
			expectedStrippedContains: []string{"[CARD:"},
		},
		{
			name: "invalid_json",
			input: "开头\n```json\nthis is not json{]\n```\n结尾",
			expectedCardsLen:          0,
			expectedStrippedContains:  []string{"```json", "this is not json{]", "结尾"},
			expectedStrippedNotContain: []string{"[CARD:"},
		},
		{
			name: "no_block",
			input:                     "纯文本回复，没有卡片",
			expectedCardsLen:          0,
			expectedStrippedContains:  []string{"纯文本回复，没有卡片"},
			expectedStrippedNotContain: []string{"[CARD:"},
		},
		{
			name: "whole_content_json_not_matched",
			input:                     `{"cards":[{"type":"info","id":"x1"}]}`,
			expectedCardsLen:          0,
			expectedStrippedContains:  []string{`{"cards":[`},
			expectedStrippedNotContain: []string{"[CARD:"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cards, stripped, cardsJSON := extractCardsFromContent(tc.input)
			if len(cards) != tc.expectedCardsLen {
				t.Fatalf("expected %d cards, got %d (cards=%v)", tc.expectedCardsLen, len(cards), cards)
			}
			for _, sub := range tc.expectedStrippedContains {
				if !strings.Contains(stripped, sub) {
					t.Errorf("stripped should contain %q, got: %s", sub, stripped)
				}
			}
			for _, sub := range tc.expectedStrippedNotContain {
				if strings.Contains(stripped, sub) {
					t.Errorf("stripped should NOT contain %q, got: %s", sub, stripped)
				}
			}
			if tc.expectedCardsLen > 0 {
				if cardsJSON == "" {
					t.Fatalf("expected non-empty cardsJSON")
				}
				for _, sub := range tc.expectedCardsJSONContains {
					if !strings.Contains(cardsJSON, sub) {
						t.Errorf("cardsJSON should contain %q, got: %s", sub, cardsJSON)
					}
				}
			} else {
				if cardsJSON != "" {
					t.Errorf("expected empty cardsJSON for no cards case, got %q", cardsJSON)
				}
			}
			if tc.expectCardIDNonEmpty {
				if len(cards) == 0 {
					t.Fatalf("expected at least 1 card")
				}
				id, _ := cards[0]["id"].(string)
				if id == "" {
					t.Errorf("expected auto-generated id, got empty")
				}
			}
		})
	}
}

// TestCreateAgentReplyMergesDaemonAndTextCards 验证 daemon 上行 cards（如 deploy info）
// 与 agent 正文 ```json{"cards":[...]}``` block 解析出的卡片被合并、剥离后存入 DB。
func TestCreateAgentReplyMergesDaemonAndTextCards(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepo{}
	convRepo := &fakeConvRepoForMsg{
		conv: &model.Conversation{ID: "conv-1", UserID: userID, Type: "agent"},
	}
	agentRepo := &fakeAgentRepoForMsg{
		agent:          &model.Agent{ID: "agent-1", UserID: &userID, Name: "Codex Agent", CLITool: "codex", MachineID: stringPtr("machine-1")},
		inConversation: true,
	}
	svc := NewMessageService(msgRepo, convRepo, agentRepo)
	daemonHub := ws.NewDaemonHub(slog.Default())
	daemonHub.RegisterTestClient("machine-1", ws.NewDaemonClient(nil, "machine-1"))
	svc.SetDaemonHub(daemonHub)

	// daemon 上行的卡片（例如 deploy_project 成功的 info 卡）+ 正文里嵌的 diff 卡
	daemonCards := []map[string]any{
		{"type": "info", "id": "daemon-deploy", "fields": map[string]any{"url": "https://example.com"}},
	}
	textResult := "回复开头\n```json\n" + `{"cards":[{"type":"diff","id":"d1","title":"本次修改","workDir":"/repo","files":["App.tsx"]}]}` + "\n```\n尾文本"

	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if agentRepo.task != nil && daemonHub.AwaitTaskResult(agentRepo.task.ID) != nil {
				daemonHub.ResolveTask(agentRepo.task.ID, &ws.TaskResult{
					TaskID: agentRepo.task.ID,
					Result: textResult,
					Cards:  daemonCards,
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if _, err := svc.SendMessageWithReply(context.Background(), "conv-1", userID, "user", "hello", "", nil, nil, "agent-1", nil); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	var agentMsg *model.Message
	for i := 0; i < 20; i++ {
		for j := range msgRepo.messages {
			if msgRepo.messages[j].Role == "assistant" {
				agentMsg = &msgRepo.messages[j]
				break
			}
		}
		if agentMsg != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if agentMsg == nil {
		t.Fatalf("expected async assistant reply, got %#v", msgRepo.messages)
	}

	// 合并后应有 2 张卡（daemon 1 张 + 文本 1 张）
	if len(agentMsg.Cards) != 2 {
		t.Fatalf("expected 2 merged cards (daemon+text), got %d: %+v", len(agentMsg.Cards), agentMsg.Cards)
	}

	// 正文里不应再出现原始 block 文本
	if strings.Contains(agentMsg.Content, `{"cards"`) {
		t.Errorf("content should not contain raw cards JSON block, got: %s", agentMsg.Content)
	}
	if strings.Contains(agentMsg.Content, "```json") {
		t.Errorf("content should not contain fenced json block, got: %s", agentMsg.Content)
	}

	// 应含占位符 + 前后文字
	if !strings.Contains(agentMsg.Content, "[CARD:d1]") {
		t.Errorf("content should contain [CARD:d1] placeholder, got: %s", agentMsg.Content)
	}
	if !strings.Contains(agentMsg.Content, "回复开头") || !strings.Contains(agentMsg.Content, "尾文本") {
		t.Errorf("content should preserve surrounding text, got: %s", agentMsg.Content)
	}

	// CardsJSON 应同时含两种 type
	if !strings.Contains(agentMsg.CardsJSON, `"type":"info"`) {
		t.Errorf("CardsJSON should contain daemon info card, got: %s", agentMsg.CardsJSON)
	}
	if !strings.Contains(agentMsg.CardsJSON, `"type":"diff"`) {
		t.Errorf("CardsJSON should contain agent diff card, got: %s", agentMsg.CardsJSON)
	}
}
