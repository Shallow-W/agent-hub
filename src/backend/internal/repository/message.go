package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// MessageRepo 消息数据访问
type MessageRepo struct {
	db             *sqlx.DB
	attachmentRepo *AttachmentRepo
	artifactRepo   *ArtifactRepo
}

// NewMessageRepo 创建消息仓库
func NewMessageRepo(db *sqlx.DB, attachmentRepo *AttachmentRepo, artifactRepo *ArtifactRepo) *MessageRepo {
	return &MessageRepo{db: db, attachmentRepo: attachmentRepo, artifactRepo: artifactRepo}
}

// messageCols 通用消息查询列（含 JOIN users 获取 username）
const messageCols = `m.id, m.conversation_id, m.role, m.content, COALESCE(m.artifacts_json, '') AS artifacts_json, m.reply_to, m.deleted_at, m.created_at, m.sender_id,
COALESCE(u.username, '') AS username,
EXISTS (
	SELECT 1 FROM message_pins mp
	WHERE mp.message_id = m.id AND mp.conversation_id = m.conversation_id AND mp.enabled = TRUE
) AS pinned`

// messageFrom 通用 FROM 子句
const messageFrom = `messages m LEFT JOIN users u ON u.id = m.sender_id`

// Create 创建新消息并刷新对话时间戳（事务保证原子性，附件在同一事务内写入）
func (r *MessageRepo) Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 序列化 mentions 为 JSON
	var mentionsJSON *string
	if len(mentions) > 0 {
		b, _ := json.Marshal(mentions)
		s := string(b)
		mentionsJSON = &s
	}

	var m model.Message
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, artifacts_json, reply_to, sender_id, mentions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, conversation_id, role, content, COALESCE(artifacts_json, '') AS artifacts_json, reply_to, deleted_at, created_at, sender_id, FALSE AS pinned`,
		conversationID, role, content, artifactsJSON, replyTo, senderID, mentionsJSON,
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

	// 填充 username（事务外查询，不影响原子性）
	if senderID != nil && *senderID != "" {
		var username string
		err = r.db.QueryRowxContext(ctx,
			`SELECT username FROM users WHERE id = $1`, *senderID,
		).Scan(&username)
		if err == nil {
			m.Username = username
		}
	}

	// 发送后补齐附件与回复引用预览（失败不影响消息发送）
	if filled, err := r.fillAttachmentsAndReply(ctx, []model.Message{m}); err == nil && len(filled) > 0 {
		m = filled[0]
	}

	// 填充 mentions（直接从内存设置，无需再查 DB）
	m.Mentions = mentions

	return &m, nil
}

// SaveArtifacts 保存 assistant 消息的结构化产物（产物来源于 daemon 解析的回复）。
// 与消息创建解耦：在 assistant 消息持久化后调用，落到独立 artifacts 表。
func (r *MessageRepo) SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error {
	if r.artifactRepo == nil || len(artifacts) == 0 {
		return nil
	}
	return r.artifactRepo.CreateArtifacts(ctx, messageID, artifacts)
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
			err := r.db.SelectContext(ctx, &list,
				`SELECT `+messageCols+` FROM `+messageFrom+
					` WHERE m.conversation_id = $1 AND m.deleted_at IS NULL ORDER BY m.created_at DESC LIMIT $2`,
				conversationID, limit,
			)
			if err != nil {
				return nil, fmt.Errorf("list messages: %w", err)
			}
			return r.fillAttachmentsAndReply(ctx, list)
		}
		err := r.db.SelectContext(ctx, &list,
			`SELECT `+messageCols+` FROM `+messageFrom+
				` WHERE m.conversation_id = $1 AND m.deleted_at IS NULL AND m.created_at < $2 ORDER BY m.created_at DESC LIMIT $3`,
			conversationID, v, limit,
		)
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
	default:
		err := r.db.SelectContext(ctx, &list,
			`SELECT `+messageCols+` FROM `+messageFrom+
				` WHERE m.conversation_id = $1 AND m.deleted_at IS NULL ORDER BY m.created_at DESC LIMIT $2`,
			conversationID, limit,
		)
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
		err := r.db.SelectContext(ctx, &list,
			`SELECT `+messageCols+` FROM `+messageFrom+
				` WHERE m.conversation_id = $1 AND m.deleted_at IS NULL AND m.created_at > $2 ORDER BY m.created_at ASC LIMIT $3`,
			conversationID, v, limit,
		)
		if err != nil {
			return nil, fmt.Errorf("get messages after: %w", err)
		}
	default:
		err := r.db.SelectContext(ctx, &list,
			`SELECT `+messageCols+` FROM `+messageFrom+
				` WHERE m.conversation_id = $1 AND m.deleted_at IS NULL ORDER BY m.created_at ASC LIMIT $2`,
			conversationID, limit,
		)
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
		`SELECT `+messageCols+` FROM `+messageFrom+` WHERE m.id = $1`,
		id,
	).StructScan(&m)
	if err != nil {
		return nil, fmt.Errorf("get message by id: %w", err)
	}
	return &m, nil
}

// GetMessageSender 获取消息的发送者
func (r *MessageRepo) GetMessageSender(ctx context.Context, messageID string) (string, error) {
	// 优先使用 sender_id 字段
	var senderID *string
	err := r.db.QueryRowxContext(ctx,
		`SELECT sender_id FROM messages WHERE id = $1`,
		messageID,
	).Scan(&senderID)
	if err != nil {
		return "", fmt.Errorf("get message sender: %w", err)
	}
	if senderID != nil && *senderID != "" {
		return *senderID, nil
	}

	// 回退：通过 conversation 关联推断
	var convID, role string
	err = r.db.QueryRowxContext(ctx,
		`SELECT conversation_id, role FROM messages WHERE id = $1`,
		messageID,
	).Scan(&convID, &role)
	if err != nil {
		return "", fmt.Errorf("get message sender fallback: %w", err)
	}

	if role == "user" {
		var ownerID, convType string
		err = r.db.QueryRowxContext(ctx,
			`SELECT user_id, type FROM conversations WHERE id = $1`,
			convID,
		).Scan(&ownerID, &convType)
		if err != nil {
			return "", fmt.Errorf("get conversation owner: %w", err)
		}
		// 群聊中 sender_id 为空时无法确定发送者，不允许撤回
		if convType == "group" {
			return "", nil
		}
		return ownerID, nil
	}

	return "", nil
}

// SearchByContent 按关键词搜索对话消息（大小写不敏感）
func (r *MessageRepo) SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error) {
	keyword = escapeLike(keyword)

	var list []model.Message
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+messageCols+` FROM `+messageFrom+
			` WHERE m.conversation_id = $1 AND m.content ILIKE '%' || $2 || '%' ESCAPE '=' AND m.deleted_at IS NULL`+
			` ORDER BY m.created_at DESC LIMIT $3`,
		conversationID, keyword, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return r.fillAttachmentsAndReply(ctx, list)
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

// PinMessage adds a message to the conversation's shared pinned context.
func (r *MessageRepo) PinMessage(ctx context.Context, conversationID, messageID, userID string) (*model.MessagePin, error) {
	var pin model.MessagePin
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO message_pins (conversation_id, message_id, created_by, enabled)
		 VALUES ($1, $2, $3, TRUE)
		 ON CONFLICT (conversation_id, message_id)
		 DO UPDATE SET enabled = TRUE, created_by = EXCLUDED.created_by, updated_at = NOW()
		 RETURNING id, conversation_id, message_id, created_by, created_at`,
		conversationID, messageID, userID,
	).StructScan(&pin)
	if err != nil {
		return nil, fmt.Errorf("pin message: %w", err)
	}
	return &pin, nil
}

// UnpinMessage removes a message from the conversation's shared pinned context.
func (r *MessageRepo) UnpinMessage(ctx context.Context, conversationID, messageID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE message_pins
		 SET enabled = FALSE, updated_at = NOW()
		 WHERE conversation_id = $1 AND message_id = $2 AND enabled = TRUE`,
		conversationID, messageID,
	)
	if err != nil {
		return fmt.Errorf("unpin message: %w", err)
	}
	return nil
}

// ListPinnedMessages returns active pinned messages for a conversation.
func (r *MessageRepo) ListPinnedMessages(ctx context.Context, conversationID string, limit int) ([]model.PinnedMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []model.PinnedMessage
	err := r.db.SelectContext(ctx, &list,
		`SELECT
			mp.id AS pin_id,
			mp.conversation_id,
			mp.message_id,
			m.role,
			m.content,
			COALESCE(m.artifacts_json, '') AS artifacts_json,
			m.sender_id,
			COALESCE(sender.username, '') AS username,
			m.created_at AS message_created_at,
			mp.created_by AS pinned_by,
			COALESCE(pinner.username, '') AS pinned_by_name,
			mp.created_at AS pinned_at
		 FROM message_pins mp
		 JOIN messages m ON m.id = mp.message_id AND m.conversation_id = mp.conversation_id
		 LEFT JOIN users sender ON sender.id = m.sender_id
		 LEFT JOIN users pinner ON pinner.id = mp.created_by
		 WHERE mp.conversation_id = $1 AND mp.enabled = TRUE AND m.deleted_at IS NULL
		 ORDER BY mp.created_at ASC
		 LIMIT $2`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list pinned messages: %w", err)
	}
	return list, nil
}

// GetConversationBlackboard returns the user-authored blackboard for a conversation.
func (r *MessageRepo) GetConversationBlackboard(ctx context.Context, conversationID string) (*model.ConversationBlackboard, error) {
	var blackboard model.ConversationBlackboard
	err := r.db.QueryRowxContext(ctx,
		`SELECT conversation_id, manual_context, updated_by, updated_at
		 FROM conversation_blackboards
		 WHERE conversation_id = $1`,
		conversationID,
	).StructScan(&blackboard)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &model.ConversationBlackboard{ConversationID: conversationID, ManualContext: ""}, nil
		}
		return nil, fmt.Errorf("get conversation blackboard: %w", err)
	}
	return &blackboard, nil
}

// UpsertConversationBlackboard saves the user-authored blackboard for a conversation.
func (r *MessageRepo) UpsertConversationBlackboard(ctx context.Context, conversationID, manualContext, userID string) (*model.ConversationBlackboard, error) {
	var blackboard model.ConversationBlackboard
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO conversation_blackboards (conversation_id, manual_context, updated_by)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (conversation_id)
		 DO UPDATE SET manual_context = EXCLUDED.manual_context, updated_by = EXCLUDED.updated_by, updated_at = NOW()
		 RETURNING conversation_id, manual_context, updated_by, updated_at`,
		conversationID, manualContext, userID,
	).StructScan(&blackboard)
	if err != nil {
		return nil, fmt.Errorf("upsert conversation blackboard: %w", err)
	}
	return &blackboard, nil
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

	// 填充产物
	messages, err = r.fillArtifacts(ctx, messages)
	if err != nil {
		return nil, err
	}

	// 填充回复引用
	messages, err = r.fillReplyTo(ctx, messages)
	if err != nil {
		return nil, err
	}

	// 填充 mentions
	messages, err = r.fillMentions(ctx, messages)
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

// fillArtifacts 批量填充消息的产物字段
func (r *MessageRepo) fillArtifacts(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	if r.artifactRepo == nil || len(messages) == 0 {
		return messages, nil
	}
	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}
	artMap, err := r.artifactRepo.ListByMessageIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fill artifacts: %w", err)
	}
	for i := range messages {
		messages[i].Artifacts = artMap[messages[i].ID]
	}
	return messages, nil
}

// fillMentions 批量填充消息的 mentions 字段
func (r *MessageRepo) fillMentions(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}
	query, args, err := sqlx.In(
		`SELECT id, mentions FROM messages WHERE id IN (?) AND mentions IS NOT NULL`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("build mentions query: %w", err)
	}
	query = r.db.Rebind(query)
	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fill mentions: %w", err)
	}
	defer rows.Close()

	mentionMap := make(map[string][]string)
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		var list []string
		if json.Unmarshal([]byte(raw), &list) == nil {
			mentionMap[id] = list
		}
	}
	for i := range messages {
		if m, ok := mentionMap[messages[i].ID]; ok {
			messages[i].Mentions = m
		}
	}
	return messages, nil
}

// fillReplyTo 批量填充回复引用预览
func (r *MessageRepo) fillReplyTo(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	replyIDs := collectReplyIDs(messages)
	if len(replyIDs) == 0 {
		return messages, nil
	}

	// 批量查询引用的消息。assistant 消息没有 sender_id，显示名从 artifacts_json.agent_name 取；
	// 不能回退到 conversations.user_id，否则 worker 回复 Orch 消息时会显示成群主/用户。
	query, args, err := sqlx.In(`SELECT m.id, m.content, m.deleted_at,
	          COALESCE(m.sender_id::text, '') AS sender_id,
	          COALESCE(u.username, '') AS username,
	          m.role,
	          COALESCE(m.artifacts_json, '') AS artifacts_json
	          FROM messages m
	          LEFT JOIN users u ON u.id = m.sender_id
	          WHERE m.id IN (?)`, replyIDs)
	if err != nil {
		return nil, fmt.Errorf("build reply query: %w", err)
	}
	query = r.db.Rebind(query)
	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fill reply to: %w", err)
	}
	defer rows.Close()

	replyMap := make(map[string]*model.ReplyToPreview)
	for rows.Next() {
		var id, content, senderID, username, role, artifactsJSON string
		var deletedAt *time.Time
		if err := rows.Scan(&id, &content, &deletedAt, &senderID, &username, &role, &artifactsJSON); err != nil {
			return nil, fmt.Errorf("scan reply: %w", err)
		}
		preview := truncateRunes(content, 50)
		if deletedAt != nil {
			preview = "[消息已撤回]"
		}
		replyMap[id] = &model.ReplyToPreview{
			ID:        id,
			Content:   preview,
			SenderID:  senderID,
			Username:  replyPreviewUsername(role, username, artifactsJSON),
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

func collectReplyIDs(messages []model.Message) []string {
	replyIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, m := range messages {
		if m.ReplyTo == nil {
			continue
		}
		id := strings.TrimSpace(*m.ReplyTo)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		replyIDs = append(replyIDs, id)
	}
	return replyIDs
}

func replyPreviewUsername(role, username, artifactsJSON string) string {
	if role != "assistant" {
		return username
	}
	var meta struct {
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal([]byte(artifactsJSON), &meta); err == nil && meta.AgentName != "" {
		return meta.AgentName
	}
	return username
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
