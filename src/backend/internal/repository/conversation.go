package repository

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// ConversationRepo 对话数据访问
type ConversationRepo struct {
	db *sqlx.DB
}

// NewConversationRepo 创建对话仓库
func NewConversationRepo(db *sqlx.DB) *ConversationRepo {
	return &ConversationRepo{db: db}
}

// Create 创建新对话
func (r *ConversationRepo) Create(ctx context.Context, userID, convType, title string) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO conversations (user_id, type, title) VALUES ($1, $2, $3)
		 RETURNING id, user_id, type, title, pinned, created_at, updated_at`,
		userID, convType, title,
	).StructScan(&c)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}
	return &c, nil
}

// ListByUserID 分页查询用户的对话列表，按 updated_at 降序
func (r *ConversationRepo) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error) {
	var list []model.Conversation
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, type, title, pinned, created_at, updated_at
		 FROM conversations WHERE user_id = $1
		 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return list, nil
}

// GetByID 按 ID 查找对话
func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, type, title, pinned, created_at, updated_at
		 FROM conversations WHERE id = $1`,
		id,
	).StructScan(&c)
	if err != nil {
		return nil, fmt.Errorf("get conversation by id: %w", err)
	}
	return &c, nil
}

// Delete 删除对话
func (r *ConversationRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

// UpdatePinned 更新对话置顶状态
func (r *ConversationRepo) UpdatePinned(ctx context.Context, id string, pinned bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET pinned = $1 WHERE id = $2`,
		pinned, id,
	)
	if err != nil {
		return fmt.Errorf("update pinned: %w", err)
	}
	return nil
}

// UpdateTimestamp 刷新对话的 updated_at 为当前时间
func (r *ConversationRepo) UpdateTimestamp(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("update timestamp: %w", err)
	}
	return nil
}
