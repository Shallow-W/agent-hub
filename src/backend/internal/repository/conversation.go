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
			 RETURNING id, user_id, type, COALESCE(title, '') AS title, pinned, archived_at, created_at, updated_at,
			 ''::text AS peer_id, ''::text AS peer_name, ''::text AS last_message`,
		userID, convType, title,
	).StructScan(&c)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}
	return &c, nil
}

// ListByUserID 分页查询用户的对话列表（包括作为成员的群聊），排除已归档，按 updated_at 降序
// 同时返回私聊对方用户名（peer_name）和最近一条消息内容（last_message）
func (r *ConversationRepo) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error) {
	var list []model.Conversation
	err := r.db.SelectContext(ctx, &list,
		`SELECT c.id, c.user_id, c.type, COALESCE(c.title, '') AS title, c.pinned, c.archived_at, c.created_at, c.updated_at,
			        COALESCE(peer_cm.user_id::text, agent_a.id::text, '') AS peer_id,
				        COALESCE(agent_a.name, peer_u.username, creator_u.username, '') AS peer_name,
			        COALESCE(latest_msg.content, '') AS last_message,
				        CASE WHEN c.type = 'agent' THEN 1
				             ELSE COALESCE((SELECT COUNT(*) FROM conversation_members WHERE conversation_id = c.id), 0)
				        END AS member_count
			 FROM conversations c
			 LEFT JOIN conversation_members peer_cm ON c.type = 'single'
			     AND peer_cm.conversation_id = c.id AND peer_cm.user_id != $1
			 LEFT JOIN users peer_u ON peer_u.id = peer_cm.user_id
			 LEFT JOIN users creator_u ON c.type = 'single' AND creator_u.id = c.user_id AND c.user_id != $1
			 LEFT JOIN conversation_agents agent_ca ON c.type = 'agent' AND agent_ca.conversation_id = c.id
			 LEFT JOIN agents agent_a ON agent_a.id = agent_ca.agent_id
			 LEFT JOIN LATERAL (
			     SELECT content FROM messages
			     WHERE conversation_id = c.id AND deleted_at IS NULL
			     ORDER BY created_at DESC LIMIT 1
			 ) latest_msg ON true
			 WHERE c.archived_at IS NULL
			   AND (c.user_id = $1
			        OR EXISTS (SELECT 1 FROM conversation_members cm
			                   WHERE cm.conversation_id = c.id AND cm.user_id = $1))
			 ORDER BY c.updated_at DESC LIMIT $2 OFFSET $3`,
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
		`SELECT id, user_id, type, COALESCE(title, '') AS title, pinned, archived_at, created_at, updated_at,
			 ''::text AS peer_id, ''::text AS peer_name, ''::text AS last_message
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

// UpdateTitle 更新对话标题
func (r *ConversationRepo) UpdateTitle(ctx context.Context, id, title string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET title = $1 WHERE id = $2`,
		title, id,
	)
	if err != nil {
		return fmt.Errorf("update title: %w", err)
	}
	return nil
}

// Archive 设置 archived_at 为当前时间（软删除）
func (r *ConversationRepo) Archive(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET archived_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("archive conversation: %w", err)
	}
	return nil
}

// Unarchive 取消归档会话
func (r *ConversationRepo) Unarchive(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET archived_at = NULL WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("unarchive conversation: %w", err)
	}
	return nil
}

// ListArchivedByUserID 分页查询用户已归档的对话列表，按 archived_at 降序
func (r *ConversationRepo) ListArchivedByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error) {
	var list []model.Conversation
	err := r.db.SelectContext(ctx, &list,
		`SELECT c.id, c.user_id, c.type, COALESCE(c.title, '') AS title, c.pinned, c.archived_at, c.created_at, c.updated_at,
			        COALESCE(peer_cm.user_id::text, agent_a.id::text, '') AS peer_id,
			        COALESCE(agent_a.name, peer_u.username, creator_u.username, '') AS peer_name,
			        COALESCE(latest_msg.content, '') AS last_message,
						CASE WHEN c.type = 'agent' THEN 1
						     ELSE COALESCE((SELECT COUNT(*) FROM conversation_members WHERE conversation_id = c.id), 0)
						END AS member_count
			 FROM conversations c
			 LEFT JOIN conversation_members peer_cm ON c.type = 'single'
			     AND peer_cm.conversation_id = c.id AND peer_cm.user_id != $1
			 LEFT JOIN users peer_u ON peer_u.id = peer_cm.user_id
			 LEFT JOIN users creator_u ON c.type = 'single' AND creator_u.id = c.user_id AND c.user_id != $1
			 LEFT JOIN conversation_agents agent_ca ON c.type = 'agent' AND agent_ca.conversation_id = c.id
			 LEFT JOIN agents agent_a ON agent_a.id = agent_ca.agent_id
			 LEFT JOIN LATERAL (
			     SELECT content FROM messages
			     WHERE conversation_id = c.id AND deleted_at IS NULL
			     ORDER BY created_at DESC LIMIT 1
			 ) latest_msg ON true
			 WHERE c.archived_at IS NOT NULL
			   AND (c.user_id = $1
			        OR EXISTS (SELECT 1 FROM conversation_members cm
			                   WHERE cm.conversation_id = c.id AND cm.user_id = $1))
			 ORDER BY c.archived_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list archived conversations: %w", err)
	}
	return list, nil
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
			   AND (c.user_id = $2 OR EXISTS (
			       SELECT 1 FROM conversation_members cm
			       WHERE cm.conversation_id = c.id AND cm.user_id = $2
			   ))
			   AND (a.user_id IS NULL OR a.user_id = $2)
			 ORDER BY ca.joined_at ASC`,
		conversationID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversation agents: %w", err)
	}
	return list, nil
}

// GetMember 查询用户在某会话中的成员信息
func (r *ConversationRepo) GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error) {
	var m model.ConversationMember
	err := r.db.QueryRowxContext(ctx,
		`SELECT cm.id, cm.conversation_id, cm.user_id, cm.role, cm.joined_at, cm.last_read_at, u.username
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

// DeleteMember 删除用户在会话中的成员记录
func (r *ConversationRepo) DeleteMember(ctx context.Context, conversationID, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	return nil
}

// ListMemberIDs 返回会话所有成员 ID
func (r *ConversationRepo) ListMemberIDs(ctx context.Context, conversationID string) ([]string, error) {
	var ids []string
	err := r.db.SelectContext(ctx, &ids,
		`SELECT user_id FROM conversation_members WHERE conversation_id = $1
			 UNION
			 SELECT user_id FROM conversations WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list member ids: %w", err)
	}
	return ids, nil
}

// AddMember 添加用户为会话成员
func (r *ConversationRepo) AddMember(ctx context.Context, conversationID, userID, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, $3)
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
		conversationID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// FindPrivateChat 查找两个用户之间的私聊会话
func (r *ConversationRepo) FindPrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.QueryRowxContext(ctx,
		`SELECT c.id, c.user_id, c.type, COALESCE(c.title, '') AS title, c.pinned, c.archived_at, c.created_at, c.updated_at,
			 ''::text AS peer_name, ''::text AS last_message
			 FROM conversations c
			 INNER JOIN conversation_members cm ON cm.conversation_id = c.id
			 WHERE c.type = 'single'
			   AND (
			     (c.user_id = $1 AND cm.user_id = $2)
			     OR
			     (c.user_id = $2 AND cm.user_id = $1)
			   )
			 LIMIT 1`,
		userID, friendID,
	).StructScan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find private chat: %w", err)
	}
	return &c, nil
}

// CreatePrivateChat 创建私聊会话并添加好友为成员（事务保证原子性）
func (r *ConversationRepo) CreatePrivateChat(ctx context.Context, userID, friendID, title string) (*model.Conversation, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 事务内重新检查：防止并发请求重复创建
	var exists bool
	tx.QueryRowxContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversations c
			 INNER JOIN conversation_members cm ON cm.conversation_id = c.id
			 WHERE c.type = 'single' AND c.archived_at IS NULL
			   AND ((c.user_id = $1 AND cm.user_id = $2) OR (c.user_id = $2 AND cm.user_id = $1)))`,
		userID, friendID,
	).Scan(&exists)
	if exists {
		tx.Rollback()
		return r.FindPrivateChat(ctx, userID, friendID)
	}

	var c model.Conversation
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO conversations (user_id, type, title) VALUES ($1, 'single', $2)
			 RETURNING id, user_id, type, COALESCE(title, '') AS title, pinned, archived_at, created_at, updated_at,
			 ''::text AS peer_id, ''::text AS peer_name, ''::text AS last_message`,
		userID, title,
	).StructScan(&c)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}

	// 插入双方为成员，使用 UPSERT 保证幂等
	for _, uid := range []string{userID, friendID} {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, role) VALUES ($1, $2, 'member')
				 ON CONFLICT (conversation_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
			c.ID, uid,
		)
		if err != nil {
			return nil, fmt.Errorf("add member: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &c, nil
}

// FindAgentChat 查找当前用户和指定 Agent 的一对一会话。
func (r *ConversationRepo) FindAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.QueryRowxContext(ctx,
		`SELECT c.id, c.user_id, c.type, COALESCE(c.title, a.name) AS title, c.pinned,
			 c.archived_at, c.created_at, c.updated_at, a.id::text AS peer_id,
			 a.name AS peer_name, COALESCE(latest_msg.content, '') AS last_message, 1 AS member_count
			 FROM conversations c
			 JOIN conversation_agents ca ON ca.conversation_id = c.id
			 JOIN agents a ON a.id = ca.agent_id
			 LEFT JOIN LATERAL (
			     SELECT content FROM messages
			     WHERE conversation_id = c.id AND deleted_at IS NULL
			     ORDER BY created_at DESC LIMIT 1
			 ) latest_msg ON true
			 WHERE c.type = 'agent'
			   AND c.user_id = $1
			   AND ca.agent_id = $2
			   AND c.archived_at IS NULL
			   AND (a.user_id IS NULL OR a.user_id = $1)
			 LIMIT 1`,
		userID, agentID,
	).StructScan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find agent chat: %w", err)
	}
	return &c, nil
}

// CreateAgentChat 创建当前用户和指定 Agent 的一对一会话。
func (r *ConversationRepo) CreateAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin agent chat tx: %w", err)
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowxContext(ctx,
		`SELECT EXISTS(
			 SELECT 1 FROM conversations c
			 JOIN conversation_agents ca ON ca.conversation_id = c.id
			 JOIN agents a ON a.id = ca.agent_id
			 WHERE c.type = 'agent' AND c.user_id = $1 AND ca.agent_id = $2
			   AND c.archived_at IS NULL AND (a.user_id IS NULL OR a.user_id = $1)
		 )`,
		userID, agentID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check agent chat exists: %w", err)
	}
	if exists {
		tx.Rollback()
		return r.FindAgentChat(ctx, userID, agentID)
	}

	var c model.Conversation
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO conversations (user_id, type, title)
			 SELECT $1, 'agent', a.name FROM agents a
			 WHERE a.id = $2 AND (a.user_id IS NULL OR a.user_id = $1)
			 RETURNING id, user_id, type, COALESCE(title, '') AS title, pinned,
			 archived_at, created_at, updated_at, $2::text AS peer_id,
			 COALESCE(title, '') AS peer_name, ''::text AS last_message, 1 AS member_count`,
		userID, agentID,
	).StructScan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("insert agent chat: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO conversation_agents (conversation_id, agent_id, added_by)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (conversation_id, agent_id) DO NOTHING`,
		c.ID, agentID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("add agent chat member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit agent chat tx: %w", err)
	}
	return &c, nil
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
			   AND c.type = 'group'
			   AND (c.user_id = $3 OR EXISTS (
			       SELECT 1 FROM conversation_members cm
			       WHERE cm.conversation_id = c.id
			         AND cm.user_id = $3
			         AND cm.role IN ('owner', 'admin')
			   ))
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
			   AND c.type = 'group'
			   AND (c.user_id = $3 OR EXISTS (
			       SELECT 1 FROM conversation_members cm
			       WHERE cm.conversation_id = c.id
			         AND cm.user_id = $3
			         AND cm.role IN ('owner', 'admin')
			   ))`,
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
