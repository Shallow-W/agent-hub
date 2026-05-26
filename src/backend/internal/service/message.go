package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// MessageNotifier 消息推送接口（由 Hub 实现）
type MessageNotifier interface {
	PushToConversation(conversationID string, memberIDs []string, message interface{})
	IsOnline(userID string) bool
}

// MessageCacher 消息缓存接口（由 Redis repo 实现）
type MessageCacher interface {
	CacheMessage(ctx context.Context, conversationID string, msg *model.Message) error
	EnqueueOffline(ctx context.Context, conversationID string, msg *model.Message) error
	GetCachedMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error)
	DequeueOfflineAfter(ctx context.Context, conversationID string, after interface{}) ([]model.Message, error)
	ClearUnread(ctx context.Context, userID, conversationID string) error
}

// MsgRepo 消息服务所需的仓库接口
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error)
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

// SendMessage 发送消息：持久化 → 推送 → 缓存
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string) (*model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if role == "" {
		role = "user"
	}

	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// 异步推送和缓存（失败不影响消息持久化）
	go s.postPersist(convID, userID, msg)

	return msg, nil
}

// postPersist 持久化后的推送和缓存操作
func (s *MessageService) postPersist(conversationID, senderID string, msg *model.Message) {
	ctx := context.Background()

	// 获取会话成员列表
	memberIDs, err := s.convRepo.ListMemberIDs(ctx, conversationID)
	if err != nil {
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
		_ = s.cacher.CacheMessage(ctx, conversationID, msg)

		// 为不在线的成员加入离线队列
		for _, uid := range memberIDs {
			if uid != senderID && !s.notifier.IsOnline(uid) {
				_ = s.cacher.EnqueueOffline(ctx, conversationID, msg)
				break // 每条消息只需入队一次（按会话维度）
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
	member, err := s.convRepo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil {
		return ErrMsgConvNoPerm
	}

	if err := s.msgRepo.MarkConversationRead(ctx, convID, userID); err != nil {
		return err
	}

	// 清除 Redis 未读计数
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
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// 先查 Redis 缓存
	if s.cacher != nil {
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

	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	// 先查 Redis 离线队列
	if s.cacher != nil {
		offline, err := s.cacher.DequeueOfflineAfter(ctx, convID, "-inf")
		if err == nil && len(offline) > 0 {
			return offline, nil
		}
	}

	// fallback: 查 DB（最新 limit 条）
	messages, err := s.msgRepo.GetMessagesAfter(ctx, convID, nil, limit)
	if err != nil {
		return nil, fmt.Errorf("get unread messages: %w", err)
	}
	return messages, nil
}

// ClearUnread 清除未读计数
func (s *MessageService) ClearUnread(ctx context.Context, userID, convID string) error {
	if s.cacher != nil {
		return s.cacher.ClearUnread(ctx, userID, convID)
	}
	return nil
}
