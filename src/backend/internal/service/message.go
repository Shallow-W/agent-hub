package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/pkg/ws"
)

const maxMessageLen = 10000 // 10KB
const maxBlackboardManualContextLen = 8000

// agentReplyInFlight 防止同一源消息触发重复 agent dispatch。
// key = replyTo (source message ID)。
var agentReplyInFlight sync.Map

const recallTimeLimit = 2 * time.Minute // 消息撤回时间限制

// MessageNotifier 消息推送接口（由 Hub 实现）
type MessageNotifier interface {
	PushToConversation(conversationID string, memberIDs []string, message interface{})
	PushCustomEvent(conversationID string, memberIDs []string, eventType string, data interface{})
	IsOnline(userID string) bool
}

// MessageDeliveryState stores transient delivery state outside the source-of-truth message DB.
// History reads must not use this interface; it is only for offline queues and unread counts.
type MessageDeliveryState interface {
	EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error
	DequeueOfflineAfter(ctx context.Context, userID, conversationID string, after interface{}) ([]model.Message, error)
	ClearUnread(ctx context.Context, userID, conversationID string) error
	IncrementUnread(ctx context.Context, userID, conversationID string) error
}

// MsgRepo 消息服务所需的仓库接口。
// Deprecated: migrate to repository.MessageStore for canonical interface.
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error)
	GetByID(ctx context.Context, id string) (*model.Message, error)
	GetMessageSender(ctx context.Context, messageID string) (string, error)
	SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error)
	SoftDelete(ctx context.Context, messageID string) error
	SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error
	PinMessage(ctx context.Context, conversationID, messageID, userID string) (*model.MessagePin, error)
	UnpinMessage(ctx context.Context, conversationID, messageID string) error
	ListPinnedMessages(ctx context.Context, conversationID string, limit int) ([]model.PinnedMessage, error)
	GetConversationBlackboard(ctx context.Context, conversationID string) (*model.ConversationBlackboard, error)
	UpsertConversationBlackboard(ctx context.Context, conversationID, manualContext, userID string) (*model.ConversationBlackboard, error)
	ListReplies(ctx context.Context, messageID string) ([]model.Message, error)
	HideMessage(ctx context.Context, userID, messageID string) error
	UnhideMessage(ctx context.Context, userID, messageID string) error
	GetHiddenMessageIDs(ctx context.Context, userID, conversationID string) (map[string]bool, error)
}

// artifactsFromTaskResult 将 daemon 上行的产物转换为 model.Artifact。
func artifactsFromTaskResult(results []ws.ArtifactResult) []model.Artifact {
	if len(results) == 0 {
		return nil
	}
	out := make([]model.Artifact, 0, len(results))
	for _, r := range results {
		if r.Type == "" {
			continue
		}
		out = append(out, model.Artifact{
			Version:  1,
			Type:     r.Type,
			Language: r.Language,
			Filename: r.Filename,
			Title:    r.Title,
			URL:      r.URL,
			Content:  r.Content,
		})
	}
	return out
}

func artifactsFromTaskResultOrMarkdown(results []ws.ArtifactResult, content string) []model.Artifact {
	if arts := artifactsFromTaskResult(results); len(arts) > 0 {
		return arts
	}
	return artifactsFromMarkdown(content)
}

func hasCodeArtifact(artifacts []model.Artifact) bool {
	for _, artifact := range artifacts {
		if artifact.Type == "code" {
			return true
		}
	}
	return false
}

func codeArtifactsFromMarkdown(content string) []model.Artifact {
	parsed := artifactsFromMarkdown(content)
	if len(parsed) == 0 {
		return nil
	}
	codeArtifacts := make([]model.Artifact, 0, len(parsed))
	for _, artifact := range parsed {
		if artifact.Type == "code" {
			codeArtifacts = append(codeArtifacts, artifact)
		}
	}
	return codeArtifacts
}

// ConvRepoForMsg 消息服务需要的对话仓库接口（用于权限校验和成员查询）。
// Deprecated: migrate to repository.ConvStore for canonical interface.
type ConvRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	UpdateTimestamp(ctx context.Context, id string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
}

// AgentRepoForMsg 消息服务查询 Agent 用于对话接入。
// Deprecated: migrate to repository.AgentStore for canonical interface.
type AgentRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

var (
	ErrMsgConvNotFound      = errors.New("对话不存在")
	ErrMsgConvNoPerm        = errors.New("无权操作此对话")
	ErrMsgTooLong           = errors.New("消息内容过长")
	ErrMsgNotFound          = errors.New("消息不存在")
	ErrMsgNotSender         = errors.New("只能撤回自己的消息")
	ErrMsgRecallExpired     = errors.New("消息已超过2分钟，无法撤回")
	ErrMsgAlreadyDeleted    = errors.New("消息已被撤回")
	ErrMsgEmptyContent      = errors.New("消息内容不能为空")
	ErrMsgReplyNotFound     = errors.New("回复的消息不存在")
	ErrMsgReplyWrongConv    = errors.New("回复的消息不属于当前对话")
	ErrMsgAgentNoPerm       = errors.New("无权使用此 Agent")
	ErrMsgAgentOffline      = errors.New("Agent 未连接电脑，无法执行真实 CLI")
	ErrMsgAgentTimeout      = errors.New("Agent 执行超时")
	ErrMsgBlackboardTooLong = errors.New("黑板内容过长")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo   MsgRepo
	convRepo  ConvRepoForMsg
	agentRepo AgentRepoForMsg
	notifier  MessageNotifier
	delivery  MessageDeliveryState
	orchSvc   *OrchestratorService
	daemonHub port.DaemonDispatcher
	fileURLs  *FileURLBuilder
}

// SendMessageResult 发送消息后的用户消息和可选 Agent 回复。
type SendMessageResult struct {
	UserMessage  *model.Message `json:"user_message"`
	AgentMessage *model.Message `json:"agent_message,omitempty"`
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg, agentRepo AgentRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo, agentRepo: agentRepo, fileURLs: NewFileURLBuilder("")}
}

// SetNotifier 注入消息推送器（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetDeliveryState injects transient delivery state storage.
func (s *MessageService) SetDeliveryState(c MessageDeliveryState) {
	s.delivery = c
}

// SetCacher is kept for compatibility with older wiring code.
func (s *MessageService) SetCacher(c MessageDeliveryState) {
	s.SetDeliveryState(c)
}

// SetOrchestratorService 注入 Orchestrator 服务（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetOrchestratorService(orchSvc *OrchestratorService) {
	s.orchSvc = orchSvc
}

// SetDaemonHub 注入 DaemonDispatcher（避免循环依赖，由 main.go 调用）。
//
// P8b: 字段类型由 *ws.DaemonHub 改为 port.DaemonDispatcher，service 层不再
// 直接依赖 pkg/ws 具体实现。*ws.DaemonHub 通过 Go 结构化类型自动满足该接口，
// 调用方（main.go）依然传 *ws.DaemonHub 指针，无需改动。
func (s *MessageService) SetDaemonHub(dh port.DaemonDispatcher) {
	s.daemonHub = dh
}

// SetFileURLBuilder injects the public URL policy for message attachments.
func (s *MessageService) SetFileURLBuilder(builder *FileURLBuilder) {
	if builder == nil {
		builder = NewFileURLBuilder("")
	}
	s.fileURLs = builder
}

// checkMembership 校验用户是否为会话成员（优先查成员表）
func (s *MessageService) checkMembership(ctx context.Context, conv *model.Conversation, userID string) error {
	member, err := s.convRepo.GetMember(ctx, conv.ID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member != nil {
		return nil
	}
	// Fallback: creator may not yet be in members table
	if conv.UserID == userID {
		return nil
	}
	return ErrMsgConvNoPerm
}

// SendMessage 发送消息：持久化 → 推送 → 缓存
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment) (*SendMessageResult, error) {
	return s.SendMessageWithReply(ctx, convID, userID, role, content, artifactsJSON, attachments, nil, "", nil)
}

// SendMessageWithMentions 发送消息（支持 mentions）
func (s *MessageService) SendMessageWithMentions(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, mentions []string) (*SendMessageResult, error) {
	return s.SendMessageWithReply(ctx, convID, userID, role, content, artifactsJSON, attachments, nil, "", mentions)
}

// SendMessageWithReply 发送消息（支持回复引用和 Agent 回复）
func (s *MessageService) SendMessageWithReply(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, agentID string, mentions []string) (*SendMessageResult, error) {
	if len(content) > maxMessageLen {
		return nil, ErrMsgTooLong
	}
	// 允许纯附件消息（无文字内容），仅当内容为空且无附件时拒绝
	if strings.TrimSpace(content) == "" && len(attachments) == 0 {
		return nil, ErrMsgEmptyContent
	}

	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	// 强制角色约束：客户端只允许发送 user 角色消息，防止伪造 assistant 消息
	role = "user"

	// 校验 reply_to 引用的消息
	if replyTo != nil {
		refMsg, err := s.msgRepo.GetByID(ctx, *replyTo)
		if err != nil || refMsg == nil {
			return nil, ErrMsgReplyNotFound
		}
		if refMsg.DeletedAt != nil {
			return nil, ErrMsgReplyNotFound
		}
		if refMsg.ConversationID != convID {
			return nil, ErrMsgReplyWrongConv
		}
	}

	var senderID *string
	if role == "user" {
		senderID = &userID
	}
	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON, attachments, replyTo, senderID, mentions)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	s.enrichMessageFileURLs(msg)

	result := &SendMessageResult{UserMessage: msg}

	// Agent dispatch routing based on conversation type
	switch conv.Type {
	case "agent":
		// Single/agent chat — direct dispatch via agentID
		resolvedAgentID := strings.TrimSpace(agentID)
		if resolvedAgentID == "" {
			resolvedAgentID = s.resolveAgentConversationAgentID(ctx, convID, userID)
		}
		slog.Info("agent chat dispatch resolved", "conversation_id", convID, "agent_id", resolvedAgentID, "provided_agent_id", strings.TrimSpace(agentID) != "")
		if resolvedAgentID != "" {
			go s.asyncAgentReply(convID, userID, resolvedAgentID, content, msg.Attachments, &msg.ID)
		}
	case "group":
		// Group chat — mention routing via Orchestrator
		if s.orchSvc != nil {
			parsedMentions := ParseMentions(content)
			if len(mentions) > 0 || len(parsedMentions) > 0 {
				slog.Info(orchFlowLog, "stage", "message.async_mention_enqueued", "conversation_id", convID, "user_id", userID, "source_message_id", msg.ID, "mention_count", len(parsedMentions))
				go s.asyncMentionDispatch(convID, userID, msg.ID, content, msg.Attachments)
			}
		}
	default:
		// "single" or other types — no agent dispatch
	}

	// 异步推送和缓存（失败不影响消息持久化）
	go s.postPersist(convID, userID, msg)

	return result, nil
}

// postPersist 持久化后的推送和缓存操作
func (s *MessageService) postPersist(conversationID, senderID string, msg *model.Message) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("postPersist recovered from panic", "conversation_id", conversationID, "panic", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取会话成员列表
	memberIDs, err := s.convRepo.ListMemberIDs(ctx, conversationID)
	if err != nil {
		slog.Warn("list members failed in postPersist", "conversation_id", conversationID, "error", err)
		memberIDs = []string{senderID}
	}
	if len(memberIDs) == 0 {
		memberIDs = []string{senderID}
	}

	// 推送给所有会话成员
	if s.notifier != nil {
		s.notifier.PushToConversation(conversationID, memberIDs, msg)
	}

	// Record transient delivery state. The persisted message row remains the
	// source of truth for history reads.
	if s.delivery != nil {
		for _, uid := range memberIDs {
			if uid == senderID {
				continue
			}
			if s.notifier != nil && !s.notifier.IsOnline(uid) {
				if err := s.delivery.EnqueueOffline(ctx, uid, conversationID, msg); err != nil {
					slog.Warn("enqueue offline failed", "user_id", uid, "conversation_id", conversationID, "error", err)
				}
			}
			if err := s.delivery.IncrementUnread(ctx, uid, conversationID); err != nil {
				slog.Warn("increment unread failed", "user_id", uid, "conversation_id", conversationID, "error", err)
			}
		}
	}
}

// MarkAsRead 标记会话消息已读
func (s *MessageService) MarkAsRead(ctx context.Context, userID, convID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}

	if err := s.msgRepo.MarkConversationRead(ctx, convID, userID); err != nil {
		return err
	}

	if s.delivery != nil {
		_ = s.delivery.ClearUnread(ctx, userID, convID)
		// 同时清空离线消息队列，防止 GetUnreadMessages 再次返回已读消息
		_, _ = s.delivery.DequeueOfflineAfter(ctx, userID, convID, "+inf")
	}

	return nil
}

// GetHistory 获取对话消息历史，支持 before 游标分页
func (s *MessageService) GetHistory(ctx context.Context, convID, userID string, before interface{}, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}

	messages, err := s.msgRepo.ListByConversation(ctx, convID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	// 过滤当前用户已隐藏的消息
	messages = s.filterHidden(ctx, messages, convID, userID)
	s.ensureParsedArtifacts(ctx, messages)
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// filterHidden 从消息列表中移除当前用户已隐藏的消息。
func (s *MessageService) filterHidden(ctx context.Context, messages []model.Message, convID, userID string) []model.Message {
	if userID == "" || len(messages) == 0 {
		return messages
	}
	hidden, err := s.msgRepo.GetHiddenMessageIDs(ctx, userID, convID)
	if err != nil {
		return messages // 查询失败不阻塞，返回完整列表
	}
	if len(hidden) == 0 {
		return messages
	}
	filtered := messages[:0]
	for _, m := range messages {
		if !hidden[m.ID] {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// HideMessage 对当前用户隐藏消息。
func (s *MessageService) HideMessage(ctx context.Context, userID, messageID string) error {
	return s.msgRepo.HideMessage(ctx, userID, messageID)
}

// UnhideMessage 取消隐藏。
func (s *MessageService) UnhideMessage(ctx context.Context, userID, messageID string) error {
	return s.msgRepo.UnhideMessage(ctx, userID, messageID)
}

// GetUnreadMessages 获取离线/未读消息
func (s *MessageService) GetUnreadMessages(ctx context.Context, convID, userID string, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	if s.delivery != nil {
		offline, err := s.delivery.DequeueOfflineAfter(ctx, userID, convID, "-inf")
		if err == nil && len(offline) > 0 {
			s.enrichMessagesFileURLs(offline)
			return offline, nil
		}
	}

	// 降级查询：使用 last_read_at 作为起点，而非返回全部消息
	member, _ := s.convRepo.GetMember(ctx, convID, userID)
	var afterTime interface{}
	if member != nil && member.LastReadAt != nil && *member.LastReadAt != "" {
		afterTime = *member.LastReadAt
	}

	messages, err := s.msgRepo.GetMessagesAfter(ctx, convID, afterTime, limit)
	if err != nil {
		return nil, fmt.Errorf("get unread messages: %w", err)
	}
	messages = s.filterHidden(ctx, messages, convID, userID)
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// SearchMessages 搜索对话消息
func (s *MessageService) SearchMessages(ctx context.Context, conversationID, userID, keyword string) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}
	messages, err := s.msgRepo.SearchByContent(ctx, conversationID, keyword, 20)
	if err != nil {
		return nil, err
	}
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// GetReplies 获取某条消息的所有回复（验证消息归属和状态后再查询）
func (s *MessageService) GetReplies(ctx context.Context, conversationID, messageID string) ([]model.Message, error) {
	// 校验目标消息存在
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMsgNotFound
		}
		return nil, fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return nil, ErrMsgNotFound
	}
	// 消息不属于当前对话
	if msg.ConversationID != conversationID {
		return nil, ErrMsgReplyWrongConv
	}
	// 消息已被软删除
	if msg.DeletedAt != nil {
		return nil, ErrMsgNotFound
	}

	replies, err := s.msgRepo.ListReplies(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("list replies: %w", err)
	}
	s.enrichMessagesFileURLs(replies)
	return replies, nil
}

func (s *MessageService) enrichMessagesFileURLs(messages []model.Message) {
	for i := range messages {
		s.enrichMessageFileURLs(&messages[i])
	}
}

func (s *MessageService) enrichMessageFileURLs(message *model.Message) {
	if message == nil || s.fileURLs == nil {
		return
	}
	for i := range message.Attachments {
		message.Attachments[i].URL = s.fileURLs.UploadURL(message.Attachments[i].FilePath)
		message.Attachments[i].ThumbnailURL = s.fileURLs.UploadURL(message.Attachments[i].ThumbnailPath)
	}
}

func (s *MessageService) ensureParsedArtifacts(ctx context.Context, messages []model.Message) {
	for i := range messages {
		msg := &messages[i]
		if msg.Role != "assistant" || hasCodeArtifact(msg.Artifacts) || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		arts := codeArtifactsFromMarkdown(msg.Content)
		if len(arts) == 0 {
			continue
		}
		if err := s.msgRepo.SaveArtifacts(ctx, msg.ID, arts); err != nil {
			slog.Warn("backfill message artifacts failed", "message_id", msg.ID, "error", err)
			continue
		}
		msg.Artifacts = append(msg.Artifacts, arts...)
	}
}

// PinMessage pins a message into the shared conversation context blackboard.
func (s *MessageService) PinMessage(ctx context.Context, convID, messageID, userID string) (*model.MessagePin, error) {
	if err := s.ensureMessageContextAccess(ctx, convID, messageID, userID); err != nil {
		return nil, err
	}
	pin, err := s.msgRepo.PinMessage(ctx, convID, messageID, userID)
	if err != nil {
		return nil, fmt.Errorf("pin message: %w", err)
	}
	return pin, nil
}

// UnpinMessage removes a message from the shared conversation context blackboard.
func (s *MessageService) UnpinMessage(ctx context.Context, convID, messageID, userID string) error {
	if err := s.ensureMessageContextAccess(ctx, convID, messageID, userID); err != nil {
		return err
	}
	if err := s.msgRepo.UnpinMessage(ctx, convID, messageID); err != nil {
		return fmt.Errorf("unpin message: %w", err)
	}
	return nil
}

// ListPinnedContext returns the user's readable pinned context entries.
func (s *MessageService) ListPinnedContext(ctx context.Context, convID, userID string) ([]model.PinnedMessage, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}
	items, err := s.msgRepo.ListPinnedMessages(ctx, convID, 20)
	if err != nil {
		return nil, fmt.Errorf("list pinned context: %w", err)
	}
	if items == nil {
		items = []model.PinnedMessage{}
	}
	return items, nil
}

// GetConversationBlackboard returns the user-authored long-term context for a conversation.
func (s *MessageService) GetConversationBlackboard(ctx context.Context, convID, userID string) (*model.ConversationBlackboard, error) {
	if err := s.ensureConversationAccess(ctx, convID, userID); err != nil {
		return nil, err
	}
	blackboard, err := s.msgRepo.GetConversationBlackboard(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation blackboard: %w", err)
	}
	if blackboard == nil {
		blackboard = &model.ConversationBlackboard{ConversationID: convID, ManualContext: ""}
	}
	return blackboard, nil
}

// UpdateConversationBlackboard saves user-authored long-term context for a conversation.
func (s *MessageService) UpdateConversationBlackboard(ctx context.Context, convID, userID, manualContext string) (*model.ConversationBlackboard, error) {
	if len([]rune(manualContext)) > maxBlackboardManualContextLen {
		return nil, ErrMsgBlackboardTooLong
	}
	if err := s.ensureConversationAccess(ctx, convID, userID); err != nil {
		return nil, err
	}
	blackboard, err := s.msgRepo.UpsertConversationBlackboard(ctx, convID, manualContext, userID)
	if err != nil {
		return nil, fmt.Errorf("update conversation blackboard: %w", err)
	}
	return blackboard, nil
}

func (s *MessageService) ensureConversationAccess(ctx context.Context, convID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	return s.checkMembership(ctx, conv, userID)
}

func (s *MessageService) ensureMessageContextAccess(ctx context.Context, convID, messageID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}
	if msg == nil || msg.DeletedAt != nil {
		return ErrMsgNotFound
	}
	if msg.ConversationID != convID {
		return ErrMsgReplyWrongConv
	}
	return nil
}

// ClearUnread 清除未读计数
func (s *MessageService) ClearUnread(ctx context.Context, userID, convID string) error {
	if s.delivery != nil {
		return s.delivery.ClearUnread(ctx, userID, convID)
	}
	return nil
}

// RecallMessage 撤回消息（软删除）
func (s *MessageService) RecallMessage(ctx context.Context, convID, messageID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}

	// 查询消息
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return ErrMsgNotFound
	}

	// 检查是否已删除
	if msg.DeletedAt != nil {
		return ErrMsgAlreadyDeleted
	}

	// 检查是否为消息发送者
	senderID, err := s.msgRepo.GetMessageSender(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message sender: %w", err)
	}
	if senderID != userID {
		return ErrMsgNotSender
	}

	// 检查时间限制
	if time.Since(msg.CreatedAt) > recallTimeLimit {
		return ErrMsgRecallExpired
	}

	// 软删除
	if err := s.msgRepo.SoftDelete(ctx, messageID); err != nil {
		return fmt.Errorf("recall message: %w", err)
	}

	// 撤回成功后异步推送通知给其他成员（排除发送者）
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("recall push recovered from panic", "conversation_id", convID, "panic", r)
			}
		}()

		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		memberIDs, err := s.convRepo.ListMemberIDs(bgCtx, convID)
		if err != nil {
			slog.Warn("list members for recall push failed", "conversation_id", convID, "error", err)
			return
		}

		// 过滤掉撤回者本人（本地已做乐观更新）
		filtered := make([]string, 0, len(memberIDs))
		for _, uid := range memberIDs {
			if uid != userID {
				filtered = append(filtered, uid)
			}
		}

		if s.notifier != nil && len(filtered) > 0 {
			s.notifier.PushCustomEvent(convID, filtered, "message.recall", map[string]interface{}{
				"message_id":      messageID,
				"conversation_id": convID,
			})
		}
	}()

	return nil
}

// agentHandoff 表示一条 agent 的任务交接单，只收集 agent 的回复摘要。
type agentHandoff struct {
	AgentName   string `json:"agent_name"`
	AgentID     string `json:"agent_id"`
	UserRequest string `json:"user_request"`
	Result      string `json:"result"`
}

// buildAgentHandoffs 只收集其他 agent 的回复摘要，构建精简交接单。
// 遍历最近消息，找到每条 assistant 消息（含 agent_name），向前找触发它的最近一条
// user 消息作为 user_request，结果截断到 500 字符，最多保留 5 条。
func (s *MessageService) buildAgentHandoffs(ctx context.Context, convID string) string {
	const fetchLimit = 40
	const maxHandoffs = 5
	const maxResultLen = 500

	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, fetchLimit)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	// ListByConversation 返回倒序（最新在前），需要反转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	handoffs := make([]agentHandoff, 0, maxHandoffs)
	for i, m := range msgs {
		if m.Role != "assistant" || m.ArtifactsJSON == "" {
			continue
		}
		var a struct {
			AgentID   string `json:"agent_id"`
			AgentName string `json:"agent_name"`
		}
		if err := json.Unmarshal([]byte(m.ArtifactsJSON), &a); err != nil || a.AgentName == "" {
			continue
		}

		// 向前找触发该 agent 的最近一条 user 消息
		userRequest := ""
		for j := i - 1; j >= 0; j-- {
			if msgs[j].Role == "user" {
				userRequest = msgs[j].Content
				break
			}
		}

		result := m.Content
		if len([]rune(result)) > maxResultLen {
			runes := []rune(result)
			result = string(runes[:maxResultLen]) + "..."
		}

		handoffs = append(handoffs, agentHandoff{
			AgentName:   a.AgentName,
			AgentID:     a.AgentID,
			UserRequest: userRequest,
			Result:      result,
		})
		if len(handoffs) >= maxHandoffs {
			break
		}
	}

	if len(handoffs) == 0 {
		return ""
	}
	b, err := json.Marshal(handoffs)
	if err != nil {
		return ""
	}
	return string(b)
}

// createAgentReply 生成 Agent 回复消息
func (s *MessageService) createAgentReply(ctx context.Context, convID, userID, agentID, userContent, contextMessages string, replyTo *string) (*model.Message, error) {
	if s.agentRepo == nil {
		return nil, ErrAgentNotFound
	}
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.UserID != nil && *agent.UserID != userID {
		return nil, ErrMsgAgentNoPerm
	}
	ok, err := s.agentRepo.IsAgentInConversation(ctx, convID, agent.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("check conversation agent: %w", err)
	}
	if !ok {
		return nil, ErrMsgAgentNoPerm
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}
	if agent.Status == "stopped" {
		return nil, fmt.Errorf("agent %q 已被用户停止", agent.Name)
	}

	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, userContent, contextMessages)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}

	// Push via WS and wait for channel-based result
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		return nil, fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
	}
	s.daemonHub.RegisterTaskPromise(task.ID)
	slog.Info("createAgentReply: BEFORE SendToMachine", "conversation_id", convID, "agent_id", agent.ID, "daemon_task_id", task.ID, "reply_to", stringValue(replyTo))
	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":          task.ID,
			"cli_tool":         agent.CLITool,
			"prompt":           userContent,
			"context_messages": contextMessages,
			"agent_id":         agent.ID,
			"conversation_id":  convID,
			"user_id":          userID,
		},
	}); err != nil {
		return nil, fmt.Errorf("dispatch to daemon: %w", err)
	}
	slog.Info("createAgentReply: AFTER SendToMachine", "conversation_id", convID, "agent_id", agent.ID, "daemon_task_id", task.ID)

	ch := s.daemonHub.AwaitTaskResult(task.ID)
	if ch == nil {
		return nil, fmt.Errorf("daemon not connected for task %s", task.ID)
	}
	defer s.daemonHub.RemoveTaskPromise(task.ID)

	ctx, cancel := context.WithTimeout(ctx, 400*time.Second)
	defer cancel()

	var result *ws.TaskResult
	select {
	case result = <-ch:
	case <-ctx.Done():
		return nil, ErrMsgAgentTimeout
	}

	if result.Error != "" {
		return nil, fmt.Errorf("daemon task failed: %s", result.Error)
	}

	artifacts, err := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal agent message artifacts: %w", err)
	}

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", result.Result, string(artifacts), nil, replyTo, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}

	// 持久化 daemon 解析出的结构化产物到独立 artifacts 表（失败不影响消息本身）
	if arts := artifactsFromTaskResultOrMarkdown(result.Artifacts, result.Result); len(arts) > 0 {
		if err := s.msgRepo.SaveArtifacts(ctx, msg.ID, arts); err != nil {
			slog.Warn("save agent reply artifacts failed", "message_id", msg.ID, "error", err)
		} else {
			msg.Artifacts = arts
		}
	}
	return msg, nil
}

// broadcastAgentTyping 通过 WebSocket 广播 agent 正在处理任务的状态

// asyncMentionDispatch 异步执行 @mention 路由，不阻塞 HTTP 响应。
func (s *MessageService) asyncMentionDispatch(convID, userID, sourceMessageID, content string, attachments []model.MessageAttachment) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn(orchFlowLog, "stage", "message.async_mention_panic", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "panic", r)
		}
	}()

	slog.Info(orchFlowLog, "stage", "message.async_mention_start", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "content_len", len(content), "content_preview", orchPreview(content))
	s.broadcastAgentTyping(convID, true)
	defer s.broadcastAgentTyping(convID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	orchResult, err := s.orchSvc.RouteMention(ctx, convID, userID, content, attachments, &sourceMessageID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "message.async_mention_route_failed", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "error", err)
		s.postAgentFailure(ctx, convID, userID, "Agent 调用失败："+shortAgentError(err), &sourceMessageID)
		return
	}
	if orchResult == nil || len(orchResult.AgentMessages) == 0 {
		slog.Info(orchFlowLog, "stage", "message.async_mention_no_messages", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID)
		return
	}
	slog.Info(orchFlowLog, "stage", "message.async_mention_messages_ready", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "message_count", len(orchResult.AgentMessages))
	for _, agentMsg := range orchResult.AgentMessages {
		slog.Info(orchFlowLog, "stage", "message.async_mention_push", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "message_id", agentMsg.ID, "reply_to", stringValue(agentMsg.ReplyTo), "content_len", len(agentMsg.Content))
		s.postPersist(convID, userID, agentMsg)
	}
}

// asyncAgentReply 异步执行 agentID 路径回复，不阻塞 HTTP 响应。
// asyncAgentReply 异步执行 agentID 路径回复，不阻塞 HTTP 响应。
func (s *MessageService) asyncAgentReply(convID, userID, agentID, content string, attachments []model.MessageAttachment, replyTo *string) {
	slog.Info("asyncAgentReply ENTER", "conversation_id", convID, "agent_id", agentID, "reply_to", stringValue(replyTo), "goroutine", "started")

	defer func() {
		if r := recover(); r != nil {
			slog.Warn("asyncAgentReply recovered", "panic", r)
		}
	}()

	slog.Info("asyncAgentReply CALL createAgentReply", "conversation_id", convID, "agent_id", agentID)

	s.broadcastAgentTyping(convID, true)
	defer s.broadcastAgentTyping(convID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 单聊 Agent 不需要群聊风格的 handoff 上下文：
	// Claude Code 通过 --session-id/--resume 自行维护对话历史，
	// 无需服务端额外发送历史摘要。
	//
	// 通过 direct reply chain 构建上下文：[Attachment, Blackboard, KB, AgentConfig]
	// 最终输出顺序：agentConfig + kb + blackboard + attach（与重构前完全一致）。
	// 仅当 orchSvc 可用且 agent 解析成功时才走完整链；否则只走 attachment 段。
	var contextMessages string
	if s.orchSvc != nil {
		agent, err := s.agentRepo.GetByID(ctx, agentID)
		if err == nil && agent != nil {
			contextMessages = s.orchSvc.DirectReplyChain().Build(ctx, ContextInput{
				ConvID:      convID,
				UserID:      userID,
				Agent:       agent,
				Content:     content,
				Attachments: attachments,
			})
		} else {
			// agent 解析失败时仅注入附件段（保持原降级行为，不注入 agent config / blackboard / kb）
			// 直接调纯函数 BuildAttachmentText（与 AttachmentBuilder 共享同一实现）
			contextMessages = BuildAttachmentText(ctx, attachments, s.orchSvc.uploadDir, attachmentTextMaxRunes)
		}
	}

	agentMsg, err := s.createAgentReply(ctx, convID, userID, agentID, content, contextMessages, replyTo)
	if err != nil {
		slog.Warn("agent reply failed", "convID", convID, "agentID", agentID, "error", err)
		s.postAgentFailure(ctx, convID, userID, "Agent 调用失败："+shortAgentError(err), replyTo)
		return
	}
	s.postPersist(convID, userID, agentMsg)
}

func (s *MessageService) postAgentFailure(ctx context.Context, convID, userID, content string, replyTo *string) {
	meta, _ := json.Marshal(map[string]string{"agent_name": "AgentHub"})
	msg, err := s.msgRepo.Create(ctx, convID, "assistant", content, string(meta), nil, replyTo, nil, nil)
	if err != nil {
		slog.Warn("create agent failure message failed", "convID", convID, "error", err)
		return
	}
	s.postPersist(convID, userID, msg)
}

func shortAgentError(err error) string {
	if err == nil {
		return "未知错误"
	}
	text := err.Error()
	if strings.Contains(text, "You've hit your usage limit") {
		return "Codex CLI 当前额度已用尽，请到 Codex 设置查看 usage，或等待额度恢复。"
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		text = strings.TrimSpace(lines[0])
	}
	if len([]rune(text)) > 240 {
		runes := []rune(text)
		text = string(runes[:240]) + "..."
	}
	return text
}

func (s *MessageService) broadcastAgentTyping(convID string, typing bool) {
	if s.notifier == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	memberIDs, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil || len(memberIDs) == 0 {
		return
	}

	eventType := "agent.typing_stop"
	if typing {
		eventType = "agent.typing_start"
	}

	s.notifier.PushCustomEvent(convID, memberIDs, eventType, map[string]string{
		"conversation_id": convID,
	})
}

func (s *MessageService) resolveAgentConversationAgentID(ctx context.Context, convID, userID string) string {
	agents, err := s.convRepo.ListAgents(ctx, convID, userID)
	if err != nil || len(agents) == 0 {
		if err != nil {
			slog.Warn("resolve agent conversation agent failed", "conversation_id", convID, "error", err)
		}
		return ""
	}
	return strings.TrimSpace(agents[0].AgentID)
}
