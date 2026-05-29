package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

const maxMessageLen = 10000 // 10KB

const recallTimeLimit = 2 * time.Minute // 消息撤回时间限制

// MessageNotifier 消息推送接口（由 Hub 实现）
type MessageNotifier interface {
	PushToConversation(conversationID string, memberIDs []string, message interface{})
	PushCustomEvent(conversationID string, memberIDs []string, eventType string, data interface{})
	IsOnline(userID string) bool
}

// MessageCacher 消息缓存接口（由 Redis repo 实现）
type MessageCacher interface {
	CacheMessage(ctx context.Context, conversationID string, msg *model.Message) error
	EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error
	GetCachedMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error)
	DequeueOfflineAfter(ctx context.Context, userID, conversationID string, after interface{}) ([]model.Message, error)
	ClearUnread(ctx context.Context, userID, conversationID string) error
	IncrementUnread(ctx context.Context, userID, conversationID string) error
	InvalidateCache(ctx context.Context, conversationID string) error
}

// MsgRepo 消息服务所需的仓库接口
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error)
	GetByID(ctx context.Context, id string) (*model.Message, error)
	GetMessageSender(ctx context.Context, messageID string) (string, error)
	SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error)
	SoftDelete(ctx context.Context, messageID string) error
}

// ConvRepoForMsg 消息服务需要的对话仓库接口（用于权限校验和成员查询）
type ConvRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	UpdateTimestamp(ctx context.Context, id string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
}

// AgentRepoForMsg 消息服务查询 Agent 用于对话接入。
type AgentRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

var (
	ErrMsgConvNotFound = errors.New("对话不存在")
	ErrMsgConvNoPerm   = errors.New("无权操作此对话")
	ErrMsgTooLong      = errors.New("消息内容过长")
	ErrMsgNotFound     = errors.New("消息不存在")
	ErrMsgNotSender    = errors.New("只能撤回自己的消息")
	ErrMsgRecallExpired = errors.New("消息已超过2分钟，无法撤回")
	ErrMsgAlreadyDeleted = errors.New("消息已被撤回")
	ErrMsgEmptyContent  = errors.New("消息内容不能为空")
	ErrMsgReplyNotFound = errors.New("回复的消息不存在")
	ErrMsgReplyWrongConv = errors.New("回复的消息不属于当前对话")
	ErrMsgAgentNoPerm  = errors.New("无权使用此 Agent")
	ErrMsgAgentOffline = errors.New("Agent 未连接电脑，无法执行真实 CLI")
	ErrMsgAgentTimeout = errors.New("Agent 执行超时")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo   MsgRepo
	convRepo  ConvRepoForMsg
	agentRepo AgentRepoForMsg
	notifier  MessageNotifier
	cacher    MessageCacher
}

// SendMessageResult 发送消息后的用户消息和可选 Agent 回复。
type SendMessageResult struct {
	UserMessage  *model.Message `json:"user_message"`
	AgentMessage *model.Message `json:"agent_message,omitempty"`
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg, agentRepo AgentRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo, agentRepo: agentRepo}
}

// SetNotifier 注入消息推送器（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetCacher 注入消息缓存器
func (s *MessageService) SetCacher(c MessageCacher) {
	s.cacher = c
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
	if strings.TrimSpace(content) == "" {
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

	if role == "" {
		role = "user"
	}

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

	result := &SendMessageResult{UserMessage: msg}

	// Agent 回复
	if strings.TrimSpace(agentID) != "" {
		agentMsg, err := s.createAgentReply(ctx, convID, userID, agentID, content)
		if err != nil {
			return nil, err
		}
		result.AgentMessage = agentMsg
	}

	// 异步推送和缓存（失败不影响消息持久化）
	go s.postPersist(convID, userID, msg)

	// 刷新对话的 updated_at
	if err := s.convRepo.UpdateTimestamp(ctx, convID); err != nil {
		return nil, fmt.Errorf("update conversation timestamp: %w", err)
	}

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

	// 缓存到 Redis
	if s.cacher != nil {
		if err := s.cacher.CacheMessage(ctx, conversationID, msg); err != nil {
			slog.Warn("cache message failed", "conversation_id", conversationID, "error", err)
		}

		// 为不在线的成员加入离线队列 + 递增未读计数
		for _, uid := range memberIDs {
			if uid == senderID {
				continue
			}
			if s.notifier != nil && !s.notifier.IsOnline(uid) {
				if err := s.cacher.EnqueueOffline(ctx, uid, conversationID, msg); err != nil {
					slog.Warn("enqueue offline failed", "user_id", uid, "conversation_id", conversationID, "error", err)
				}
			}
			if err := s.cacher.IncrementUnread(ctx, uid, conversationID); err != nil {
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

	if s.cacher != nil {
		_ = s.cacher.ClearUnread(ctx, userID, convID)
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
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// 仅在无游标时使用缓存（缓存是最新N条，不含历史分页）
	if s.cacher != nil && before == nil {
		cached, err := s.cacher.GetCachedMessages(ctx, convID, limit)
		if err == nil && len(cached) > 0 {
			return cached, nil
		}
	}

	messages, err := s.msgRepo.ListByConversation(ctx, convID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return messages, nil
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

	if s.cacher != nil {
		offline, err := s.cacher.DequeueOfflineAfter(ctx, userID, convID, "-inf")
		if err == nil && len(offline) > 0 {
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
	return s.msgRepo.SearchByContent(ctx, conversationID, keyword, 20)
}

// ClearUnread 清除未读计数
func (s *MessageService) ClearUnread(ctx context.Context, userID, convID string) error {
	if s.cacher != nil {
		return s.cacher.ClearUnread(ctx, userID, convID)
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

	// 清除该会话的 Redis 缓存，避免撤回后仍返回旧内容
	if s.cacher != nil {
		if err := s.cacher.InvalidateCache(ctx, convID); err != nil {
			slog.Warn("invalidate cache after recall failed", "conversation_id", convID, "error", err)
		}
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

// buildContextMessages 拉取最近 40 条消息，组装为结构化上下文 JSON。
// 包含发送者名称和 agent 身份，供 daemon 侧区分不同 Agent 的回复。
func (s *MessageService) buildContextMessages(ctx context.Context, convID string) string {
	const contextLimit = 40
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, contextLimit)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	// ListByConversation 返回倒序（最新在前），需要反转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	ctx2 := make([]model.ContextMessage, 0, len(msgs))
	for _, m := range msgs {
		cm := model.ContextMessage{Role: m.Role, Content: m.Content}
		if m.Role == "user" {
			cm.Name = m.Username
		} else if m.Role == "assistant" && m.ArtifactsJSON != "" {
			var a struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
			}
			if json.Unmarshal([]byte(m.ArtifactsJSON), &a) == nil {
				cm.Name = a.AgentName
				cm.AgentID = a.AgentID
			}
		}
		ctx2 = append(ctx2, cm)
	}
	b, err := json.Marshal(ctx2)
	if err != nil {
		return ""
	}
	return string(b)
}

// createAgentReply 生成 Agent 回复消息
func (s *MessageService) createAgentReply(ctx context.Context, convID, userID, agentID, userContent string) (*model.Message, error) {
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

	contextMessages := s.buildContextMessages(ctx, convID)
	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, userContent, contextMessages)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}
	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts, err := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal agent message artifacts: %w", err)
	}

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, string(artifacts), nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	return msg, nil
}

func (s *MessageService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := s.agentRepo.GetDaemonTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get daemon task: %w", err)
		}
		if task != nil && (task.Status == "completed" || task.Status == "failed") {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrMsgAgentTimeout
		case <-ticker.C:
		}
	}
}
