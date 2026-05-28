package repository

import (
	"context"
	"database/sql"
	"errors"
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
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

// ListAgents 查询某个对话中已加入的 Robot 成员。
func (r *ConversationRepo) ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error) {
	list := make([]model.ConversationAgent, 0)
	err := r.db.SelectContext(ctx, &list,
		`SELECT ca.id, ca.conversation_id, ca.agent_id, ca.added_by, ca.role, ca.joined_at,
		        a.name, a.type, a.cli_tool, a.avatar, a.source, a.status, a.version,
		        a.machine_id, a.machine_name, a.last_seen_at, a.capabilities_json
		 FROM conversation_agents ca
		 JOIN conversations c ON c.id = ca.conversation_id
		 JOIN agents a ON a.id = ca.agent_id
		 WHERE ca.conversation_id = $1
		   AND c.user_id = $2
		   AND (a.user_id IS NULL OR a.user_id = $2)
		 ORDER BY ca.joined_at ASC`,
		conversationID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversation agents: %w", err)
	}
	return list, nil
}

// AddAgent 把当前用户可用的 Agent 加入指定对话。
func (r *ConversationRepo) AddAgent(ctx context.Context, conversationID, agentID, userID string) (*model.ConversationAgent, error) {
	var item model.ConversationAgent
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO conversation_agents (conversation_id, agent_id, added_by)
		 SELECT c.id, a.id, $3
		 FROM conversations c
		 JOIN agents a ON a.id = $2
		 WHERE c.id = $1
		   AND c.user_id = $3
		   AND (a.user_id IS NULL OR a.user_id = $3)
		 ON CONFLICT (conversation_id, agent_id) DO UPDATE
		   SET joined_at = conversation_agents.joined_at
		 RETURNING id, conversation_id, agent_id, added_by, role, joined_at`,
		conversationID, agentID, userID,
	).StructScan(&item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("add conversation agent: %w", err)
	}

	list, err := r.ListAgents(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	for _, current := range list {
		if current.ID == item.ID {
			return &current, nil
		}
	}
	return &item, nil
}

// RemoveAgent 从指定对话移除 Robot 成员。
func (r *ConversationRepo) RemoveAgent(ctx context.Context, conversationID, agentID, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM conversation_agents ca
		 USING conversations c
		 WHERE ca.conversation_id = c.id
		   AND ca.conversation_id = $1
		   AND ca.agent_id = $2
		   AND c.user_id = $3`,
		conversationID, agentID, userID,
	)
	if err != nil {
		return false, fmt.Errorf("remove conversation agent: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}
