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
	db *sqlx.DB
}

// NewMessageRepo 创建消息仓库
func NewMessageRepo(db *sqlx.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// Create 创建新消息并刷新对话时间戳（事务保证原子性）
func (r *MessageRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var m model.Message
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, artifacts_json)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, conversation_id, role, content, artifacts_json, created_at`,
		conversationID, role, content, artifactsJSON,
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
			query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
				FROM messages WHERE conversation_id = $1
				ORDER BY created_at DESC LIMIT $2`
			err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
			if err != nil {
				return nil, fmt.Errorf("list messages: %w", err)
			}
			return list, nil
		}
		query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
			FROM messages WHERE conversation_id = $1 AND created_at < $2
			ORDER BY created_at DESC LIMIT $3`
		err := r.db.SelectContext(ctx, &list, query, conversationID, v, limit)
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
	default:
		query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2`
		err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
	}

	return list, nil
}

// GetMessagesAfter 查询指定时间之后的消息（用于离线消息拉取）
func (r *MessageRepo) GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error) {
	var list []model.Message

	switch v := afterTime.(type) {
	case time.Time:
		query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
			FROM messages WHERE conversation_id = $1 AND created_at > $2
			ORDER BY created_at ASC LIMIT $3`
		err := r.db.SelectContext(ctx, &list, query, conversationID, v, limit)
		if err != nil {
			return nil, fmt.Errorf("get messages after: %w", err)
		}
	default:
		query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at ASC LIMIT $2`
		err := r.db.SelectContext(ctx, &list, query, conversationID, limit)
		if err != nil {
			return nil, fmt.Errorf("get messages after: %w", err)
		}
	}

	return list, nil
}
