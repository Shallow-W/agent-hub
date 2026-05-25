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

// Create 创建新消息
func (r *MessageRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error) {
	var m model.Message
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, artifacts_json)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, conversation_id, role, content, artifacts_json, created_at`,
		conversationID, role, content, artifactsJSON,
	).StructScan(&m)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return &m, nil
}

// ListByConversation 分页查询对话消息，支持 before 游标
func (r *MessageRepo) ListByConversation(ctx context.Context, conversationID string, before time.Time, limit int) ([]model.Message, error) {
	var list []model.Message

	// before 为零值时不加游标条件
	query := `SELECT id, conversation_id, role, content, artifacts_json, created_at
		FROM messages WHERE conversation_id = $1 AND created_at < $2
		ORDER BY created_at DESC LIMIT $3`
	args := []interface{}{conversationID, before, limit}

	if before.IsZero() {
		query = `SELECT id, conversation_id, role, content, artifacts_json, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2`
		args = []interface{}{conversationID, limit}
	}

	err := r.db.SelectContext(ctx, &list, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return list, nil
}
