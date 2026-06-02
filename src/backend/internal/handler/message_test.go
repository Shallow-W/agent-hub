package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestContext builds a gin.Context backed by an httptest.Recorder.
func newTestContext(method, path string, body interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	c.Request = req
	return c, w
}

// setUserID injects a user_id into the gin context (simulates auth middleware).
func setUserID(c *gin.Context, userID string) {
	c.Set("user_id", userID)
}

// decodeResponse decodes the unified Response envelope.
func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) middleware.Response {
	t.Helper()
	var resp middleware.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v, body: %s", err, w.Body.String())
	}
	return resp
}

// buildSendHandler creates a MessageHandler wrapping a real MessageService.
func buildSendHandler(svc *service.MessageService) *MessageHandler {
	return NewMessageHandler(svc)
}

// ---------------------------------------------------------------------------
// Handler tests: MessageHandler.Send
// ---------------------------------------------------------------------------

func TestSend_MissingConversationID_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil} // svc unused for this path

	c, w := newTestContext(http.MethodPost, "/conversations/messages", gin.H{
		"content": "hello",
	})
	// Do NOT set param "id"
	handler.Send(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40020 {
		t.Errorf("code = %d, want 40020", resp.Code)
	}
}

func TestSend_InvalidJSON_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte("{bad json")))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40021 {
		t.Errorf("code = %d, want 40021", resp.Code)
	}
}

func TestSend_EmptyContentNoAttachments_Returns400(t *testing.T) {
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{
		conv:   &model.Conversation{ID: "conv-1", UserID: "user-1"},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: "user-1", Role: "owner"},
	}
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	c, w := newTestContext(http.MethodPost, "/conversations/conv-1/messages", gin.H{
		"content":     "   ",
		"attachments": []interface{}{},
	})
	setUserID(c, "user-1")
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40042 {
		t.Errorf("code = %d, want 40042", resp.Code)
	}
}

func TestSend_ConvNotFound_Returns404(t *testing.T) {
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{conv: nil} // conversation not found
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	c, w := newTestContext(http.MethodPost, "/conversations/missing/messages", gin.H{
		"content": "hello",
	})
	setUserID(c, "user-1")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}

	handler.Send(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40420 {
		t.Errorf("code = %d, want 40420", resp.Code)
	}
}

func TestSend_NoPermission_Returns403(t *testing.T) {
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner-user"},
		member: nil, // user is not a member
	}
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	c, w := newTestContext(http.MethodPost, "/conversations/conv-1/messages", gin.H{
		"content": "hello",
	})
	setUserID(c, "intruder")
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40320 {
		t.Errorf("code = %d, want 40320", resp.Code)
	}
}

func TestSend_Success_Returns201(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{
		conv:   &model.Conversation{ID: "conv-1", UserID: userID},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: userID, Role: "owner"},
	}
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	c, w := newTestContext(http.MethodPost, "/conversations/conv-1/messages", gin.H{
		"content": "hello world",
	})
	setUserID(c, userID)
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 0 {
		t.Errorf("code = %d, want 0 (success)", resp.Code)
	}
}

func TestSend_WithAttachments_Success(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{
		conv:   &model.Conversation{ID: "conv-1", UserID: userID},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: userID, Role: "owner"},
	}
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	c, w := newTestContext(http.MethodPost, "/conversations/conv-1/messages", gin.H{
		"content": "",
		"attachments": []model.MessageAttachment{
			{ID: "att-1", FileName: "photo.png", MimeType: "image/png", FileSize: 1024},
		},
	})
	setUserID(c, userID)
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestSend_MessageTooLong_Returns413(t *testing.T) {
	userID := "user-1"
	msgRepo := &fakeMsgRepoForHandler{}
	convRepo := &fakeConvRepoForHandler{
		conv:   &model.Conversation{ID: "conv-1", UserID: userID},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: userID, Role: "owner"},
	}
	agentRepo := &fakeAgentRepoForHandler{}
	svc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	handler := buildSendHandler(svc)

	// content longer than maxMessageLen (10000)
	longContent := make([]byte, 10001)
	for i := range longContent {
		longContent[i] = 'a'
	}

	c, w := newTestContext(http.MethodPost, "/conversations/conv-1/messages", gin.H{
		"content": string(longContent),
	})
	setUserID(c, userID)
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Send(c)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40026 {
		t.Errorf("code = %d, want 40026", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Handler tests: MessageHandler.History
// ---------------------------------------------------------------------------

func TestHistory_MissingConversationID_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}
	c, w := newTestContext(http.MethodGet, "/conversations/messages", nil)
	handler.History(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40022 {
		t.Errorf("code = %d, want 40022", resp.Code)
	}
}

func TestHistory_InvalidBeforeFormat_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}
	c, w := newTestContext(http.MethodGet, "/conversations/conv-1/messages?before=not-a-date", nil)
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.History(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40023 {
		t.Errorf("code = %d, want 40023", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Handler tests: MessageHandler.Search
// ---------------------------------------------------------------------------

func TestSearch_MissingKeyword_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}
	c, w := newTestContext(http.MethodGet, "/conversations/conv-1/messages/search", nil)
	c.Params = gin.Params{{Key: "id", Value: "conv-1"}}

	handler.Search(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40030 {
		t.Errorf("code = %d, want 40030", resp.Code)
	}
}

func TestSearch_MissingConversationID_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}
	c, w := newTestContext(http.MethodGet, "/conversations/messages/search?keyword=test", nil)
	// No param "id"
	handler.Search(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40029 {
		t.Errorf("code = %d, want 40029", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Handler tests: MessageHandler.Recall
// ---------------------------------------------------------------------------

func TestRecall_MissingIDs_Returns400(t *testing.T) {
	handler := &MessageHandler{svc: nil}
	c, w := newTestContext(http.MethodDelete, "/conversations/messages/", nil)
	// Neither convID nor messageID set
	handler.Recall(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, w)
	if resp.Code != 40027 {
		t.Errorf("code = %d, want 40027", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Fake repo implementations for handler-level tests
// These satisfy the repo interfaces that service.MessageService depends on.
// ---------------------------------------------------------------------------

// fakeMsgRepoForHandler satisfies service.MsgRepo
type fakeMsgRepoForHandler struct {
	messages []model.Message
}

func (r *fakeMsgRepoForHandler) Create(_ context.Context, convID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error) {
	msg := model.Message{
		ID:             role + "-1",
		ConversationID: convID,
		Role:           role,
		Content:        content,
		ArtifactsJSON:  artifactsJSON,
		Attachments:    attachments,
		ReplyTo:        replyTo,
		SenderID:       senderID,
		Mentions:       mentions,
		CreatedAt:      time.Now(),
	}
	r.messages = append(r.messages, msg)
	return &msg, nil
}

func (r *fakeMsgRepoForHandler) ListByConversation(_ context.Context, _ string, _ interface{}, _ int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepoForHandler) MarkConversationRead(_ context.Context, _, _ string) error { return nil }

func (r *fakeMsgRepoForHandler) GetMessagesAfter(_ context.Context, _ string, _ interface{}, _ int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepoForHandler) GetByID(_ context.Context, _ string) (*model.Message, error) {
	return nil, nil
}

func (r *fakeMsgRepoForHandler) GetMessageSender(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (r *fakeMsgRepoForHandler) SearchByContent(_ context.Context, _, _ string, _ int) ([]model.Message, error) {
	return r.messages, nil
}

func (r *fakeMsgRepoForHandler) SoftDelete(_ context.Context, _ string) error { return nil }

// fakeConvRepoForHandler satisfies service.ConvRepoForMsg
type fakeConvRepoForHandler struct {
	conv    *model.Conversation
	member  *model.ConversationMember
	convErr error
}

func (r *fakeConvRepoForHandler) GetByID(_ context.Context, _ string) (*model.Conversation, error) {
	return r.conv, r.convErr
}

func (r *fakeConvRepoForHandler) UpdateTimestamp(_ context.Context, _ string) error { return nil }

func (r *fakeConvRepoForHandler) GetMember(_ context.Context, _, _ string) (*model.ConversationMember, error) {
	if r.member != nil {
		return r.member, nil
	}
	return nil, nil
}

func (r *fakeConvRepoForHandler) ListMemberIDs(_ context.Context, _ string) ([]string, error) {
	if r.conv == nil {
		return []string{}, nil
	}
	return []string{r.conv.UserID}, nil
}

func (r *fakeConvRepoForHandler) ListAgents(_ context.Context, _, _ string) ([]model.ConversationAgent, error) {
	return nil, nil
}

// fakeAgentRepoForHandler satisfies service.AgentRepoForMsg
type fakeAgentRepoForHandler struct{}

func (r *fakeAgentRepoForHandler) GetByID(_ context.Context, _ string) (*model.Agent, error) {
	return nil, nil
}

func (r *fakeAgentRepoForHandler) IsAgentInConversation(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

func (r *fakeAgentRepoForHandler) CreateDaemonTask(_ context.Context, _, _, _, _, _, _, _ string) (*model.DaemonTask, error) {
	return &model.DaemonTask{ID: "task-1", Status: "completed", Result: "ok"}, nil
}

func (r *fakeAgentRepoForHandler) GetDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	return &model.DaemonTask{ID: "task-1", Status: "completed", Result: "ok"}, nil
}
