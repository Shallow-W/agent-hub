package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// MessageRepo 消息数据访问
type MessageRepo struct {
	db            *sqlx.DB
	attachmentRepo *AttachmentRepo
}

// NewMessageRepo 创建消息仓库
func NewMessageRepo(db *sqlx.DB, attachmentRepo *AttachmentRepo) *MessageRepo {
	return &MessageRepo{db: db, attachmentRepo: attachmentRepo}
}

// Create 创建新消息并刷新对话时间戳（事务保证原子性，附件在同一事务内写入）
func (r *MessageRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string) (*model.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var m model.Message
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, artifacts_json, reply_to)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at`,
		conversationID, role, content, artifactsJSON, replyTo,
	).StructScan(&m)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	// 同时更新对话 updated_at
	_, err = tx.ExecContext(ctx,
		`UPDATE conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("update conversation timestamp: %w", err)
	}

	// 写入附件
	if r.attachmentRepo != nil && len(attachments) > 0 {
		if err := r.attachmentRepo.CreateAttachments(ctx, tx, m.ID, attachments); err != nil {
			return nil, fmt.Errorf("create attachments: %w", err)
		}
		m.Attachments = attachments
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &m, nil
}

// MarkConversationRead 更新会话成员的已读时间戳
func (r *MessageRepo) MarkConversationRead(ctx context.Context, conversationID, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversation_members SET last_read_at = NOW()
		 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	)
	if err != nil {
		return fmt.Errorf("mark conversation read: %w", err)
	}
	return nil
}

// ListByConversation 分页查询对话消息，支持 before 游标
func (r *MessageRepo) ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error) {
	var list []model.Message

	switch v := before.(type) {
	case time.Time:
		if v.IsZero() {
			query := `SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
				FROM messages WHERE conversation_id = $1
				ORDER BY created_at DESC LIMIT $2`
			err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
			if err != nil {
				return nil, fmt.Errorf("list messages: %w", err)
			}
			return r.fillAttachmentsAndReply(ctx, list)
		}
		query := `SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
			FROM messages WHERE conversation_id = $1 AND created_at < $2
			ORDER BY created_at DESC LIMIT $3`
		err := r.db.SelectContext(ctx, &list, query, conversationID, v, limit)
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
	default:
		query := `SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2`
		err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
	}

	return r.fillAttachmentsAndReply(ctx, list)
}

// GetMessagesAfter 查询指定时间之后的消息（用于离线消息拉取）
func (r *MessageRepo) GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error) {
	var list []model.Message

	switch v := afterTime.(type) {
	case time.Time:
		query := `SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
			FROM messages WHERE conversation_id = $1 AND created_at > $2
			ORDER BY created_at ASC LIMIT $3`
		err := r.db.SelectContext(ctx, &list, query, conversationID, v, limit)
		if err != nil {
			return nil, fmt.Errorf("get messages after: %w", err)
		}
	default:
		query := `SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at ASC LIMIT $2`
		err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
		if err != nil {
			return nil, fmt.Errorf("get messages after: %w", err)
		}
	}

	return r.fillAttachmentsAndReply(ctx, list)
}

// GetByID 按 ID 查找消息
func (r *MessageRepo) GetByID(ctx context.Context, id string) (*model.Message, error) {
	var m model.Message
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, conversation_id, role, content, artifacts_json, reply_to, deleted_at, created_at
		 FROM messages WHERE id = $1`,
		id,
	).StructScan(&m)
	if err != nil {
		return nil, fmt.Errorf("get message by id: %w", err)
	}
	return &m, nil
}

// GetMessageSender 获取消息的发送者（通过 conversation 中的 user_id 或 member 关联推断）
// 对于 role=user 的消息，发送者就是 conversation 的 user_id
// 对于群聊，需要通过 conversation_members 查找
func (r *MessageRepo) GetMessageSender(ctx context.Context, messageID string) (string, error) {
	// 先查消息的 conversation_id 和 role
	var convID, role string
	err := r.db.QueryRowxContext(ctx,
		`SELECT conversation_id, role FROM messages WHERE id = $1`,
		messageID,
	).Scan(&convID, &role)
	if err != nil {
		return "", fmt.Errorf("get message sender: %w", err)
	}

	// 对于 role=user 的私信，发送者就是对话的 user_id
	if role == "user" {
		var ownerID string
		err = r.db.QueryRowxContext(ctx,
			`SELECT user_id FROM conversations WHERE id = $1`,
			convID,
		).Scan(&ownerID)
		if err != nil {
			return "", fmt.Errorf("get conversation owner: %w", err)
		}
		return ownerID, nil
	}

	return "", nil
}

// SoftDelete 软删除消息（撤回）
func (r *MessageRepo) SoftDelete(ctx context.Context, messageID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE messages SET deleted_at = NOW() WHERE id = $1`,
		messageID,
	)
	if err != nil {
		return fmt.Errorf("soft delete message: %w", err)
	}
	return nil
}

// fillAttachmentsAndReply 批量填充消息的附件字段和回复引用
func (r *MessageRepo) fillAttachmentsAndReply(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	// 填充附件
	messages, err := r.fillAttachments(ctx, messages)
	if err != nil {
		return nil, err
	}

	// 填充回复引用
	messages, err = r.fillReplyTo(ctx, messages)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// fillAttachments 批量填充消息的附件字段
func (r *MessageRepo) fillAttachments(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	if r.attachmentRepo == nil || len(messages) == 0 {
		return messages, nil
	}
	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}
	attMap, err := r.attachmentRepo.ListByMessageIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fill attachments: %w", err)
	}
	for i := range messages {
		messages[i].Attachments = attMap[messages[i].ID]
	}
	return messages, nil
}

// fillReplyTo 批量填充回复引用预览
func (r *MessageRepo) fillReplyTo(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	// 收集所有 reply_to ID
	replyIDs := make([]string, 0)
	for _, m := range messages {
		if m.ReplyTo != nil {
			replyIDs = append(replyIDs, *m.ReplyTo)
		}
	}
	if len(replyIDs) == 0 {
		return messages, nil
	}

	// 批量查询引用的消息
	query := `SELECT m.id, m.content, m.deleted_at,
	          COALESCE(cm.user_id, c.user_id) AS sender_id,
	          u.username
	          FROM messages m
	          JOIN conversations c ON c.id = m.conversation_id
	          LEFT JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.role = 'owner'
	          LEFT JOIN users u ON u.id = COALESCE(cm.user_id, c.user_id)
	          WHERE m.id = ANY($1)`
	rows, err := r.db.QueryxContext(ctx, query, replyIDs)
	if err != nil {
		return nil, fmt.Errorf("fill reply to: %w", err)
	}
	defer rows.Close()

	replyMap := make(map[string]*model.ReplyToPreview)
	for rows.Next() {
		var id, content, senderID, username string
		var deletedAt *time.Time
		if err := rows.Scan(&id, &content, &deletedAt, &senderID, &username); err != nil {
			return nil, fmt.Errorf("scan reply: %w", err)
		}
		preview := content
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		if deletedAt != nil {
			preview = "[消息已撤回]"
		}
		replyMap[id] = &model.ReplyToPreview{
			ID:        id,
			Content:   preview,
			SenderID:  senderID,
			Username:  username,
			DeletedAt: deletedAt,
		}
	}

	for i := range messages {
		if messages[i].ReplyTo != nil {
			if preview, ok := replyMap[*messages[i].ReplyTo]; ok {
				messages[i].ReplyToMessage = preview
			}
		}
	}

	return messages, nil
}
