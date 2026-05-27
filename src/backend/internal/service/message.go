package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string) (*model.Message, error)
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

var (
	ErrMsgConvNotFound = errors.New("对话不存在")
	ErrMsgConvNoPerm   = errors.New("无权操作此对话")
	ErrMsgTooLong      = errors.New("消息内容过长")
	ErrMsgNotFound     = errors.New("消息不存在")
	ErrMsgNotSender    = errors.New("只能撤回自己的消息")
	ErrMsgRecallExpired = errors.New("消息已超过2分钟，无法撤回")
	ErrMsgAlreadyDeleted = errors.New("消息已被撤回")
	ErrMsgReplyNotFound = errors.New("回复的消息不存在")
	ErrMsgReplyWrongConv = errors.New("回复的消息不属于当前对话")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo  MsgRepo
	convRepo ConvRepoForMsg
	notifier MessageNotifier
	cacher   MessageCacher
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo}
}

// SetNotifier 注入消息推送器（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetCacher 注入消息缓存器
func (s *MessageService) SetCacher(c MessageCacher) {
	s.cacher = c
}

// checkMembership 校验用户是否为会话成员（含会话创建者）
func (s *MessageService) checkMembership(ctx context.Context, conv *model.Conversation, userID string) error {
	if conv.UserID == userID {
		return nil
	}
	member, err := s.convRepo.GetMember(ctx, conv.ID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil {
		return ErrMsgConvNoPerm
	}
	return nil
}

// SendMessage 发送消息：持久化 → 推送 → 缓存
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment) (*model.Message, error) {
	return s.SendMessageWithReply(ctx, convID, userID, role, content, artifactsJSON, attachments, nil)
}

// SendMessageWithReply 发送消息（支持回复引用）
func (s *MessageService) SendMessageWithReply(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string) (*model.Message, error) {
	if len(content) > maxMessageLen {
		return nil, ErrMsgTooLong
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
	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON, attachments, replyTo, senderID)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// 异步推送和缓存（失败不影响消息持久化）
	go s.postPersist(convID, userID, msg)

	return msg, nil
}

// postPersist 持久化后的推送和缓存操作
func (s *MessageService) postPersist(conversationID, senderID string, msg *model.Message) {
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

	messages, err := s.msgRepo.GetMessagesAfter(ctx, convID, nil, limit)
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
