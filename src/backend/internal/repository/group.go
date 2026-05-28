package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// GroupRepo 群聊数据访问
type GroupRepo struct {
	db *sqlx.DB
}

// NewGroupRepo 创建群聊仓库
func NewGroupRepo(db *sqlx.DB) *GroupRepo {
	return &GroupRepo{db: db}
}

// CreateGroup 创建群聊（事务内插入 conversation + members）
func (r *GroupRepo) CreateGroup(ctx context.Context, ownerID, name string, memberIDs []string) (*model.Conversation, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var conv model.Conversation
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO conversations (user_id, type, title) VALUES ($1, 'group', $2)
		 RETURNING id, user_id, type, title, pinned, archived_at, created_at, updated_at`,
		ownerID, name,
	).StructScan(&conv)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}

	// 插入 owner
	_, err = tx.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, 'owner')
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
		conv.ID, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert owner member: %w", err)
	}

	// 插入普通成员
	for _, mid := range memberIDs {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, 'member')
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			conv.ID, mid,
		)
		if err != nil {
			return nil, fmt.Errorf("insert member %s: %w", mid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &conv, nil
}

// AddMember 添加群成员
func (r *GroupRepo) AddMember(ctx context.Context, conversationID, userID, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, $3)`,
		conversationID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("insert member: %w", err)
	}
	return nil
}

// RemoveMember 移除群成员
func (r *GroupRepo) RemoveMember(ctx context.Context, conversationID, userID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// ListMembers 列出群成员（附带用户名）
func (r *GroupRepo) ListMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error) {
	var list []*model.ConversationMember
	err := r.db.SelectContext(ctx, &list,
		`SELECT cm.id, cm.conversation_id, cm.user_id, cm.role, cm.joined_at,
		        u.username
		 FROM conversation_members cm JOIN users u ON u.id = cm.user_id
		 WHERE cm.conversation_id = $1
		 ORDER BY cm.joined_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return list, nil
}

// GetMember 查询单个群成员
func (r *GroupRepo) GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error) {
	var m model.ConversationMember
	err := r.db.QueryRowxContext(ctx,
		`SELECT cm.id, cm.conversation_id, cm.user_id, cm.role, cm.joined_at,
		        u.username
		 FROM conversation_members cm JOIN users u ON u.id = cm.user_id
		 WHERE cm.conversation_id = $1 AND cm.user_id = $2`,
		conversationID, userID,
	).StructScan(&m)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

// IsMember 检查用户是否为群成员
func (r *GroupRepo) IsMember(ctx context.Context, conversationID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowxContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id = $1 AND user_id = $2)`,
		conversationID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check member: %w", err)
	}
	return exists, nil
}

// GetConversationByID 按 ID 查找对话
func (r *GroupRepo) GetConversationByID(ctx context.Context, id string) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, type, title, pinned, archived_at, created_at, updated_at
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

// GetUserByID 按 ID 查找用户
func (r *GroupRepo) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, username, created_at FROM users WHERE id = $1`,
		id,
	).StructScan(&u)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// SearchUsers 按用户名前缀搜索用户
func (r *GroupRepo) SearchUsers(ctx context.Context, query string, limit int) ([]*model.User, error) {
	var list []*model.User
	escaped := escapeLike(query) + "%"
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, username, created_at FROM users WHERE username LIKE $1 ESCAPE '\' LIMIT $2`,
		escaped, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	return list, nil
}
