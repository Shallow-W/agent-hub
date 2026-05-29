package repository

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// AttachmentRepo 附件数据访问
type AttachmentRepo struct {
	db *sqlx.DB
}

// NewAttachmentRepo 创建附件仓库
func NewAttachmentRepo(db *sqlx.DB) *AttachmentRepo {
	return &AttachmentRepo{db: db}
}

// CreateAttachments 批量创建消息附件（在消息创建事务内调用）
func (r *AttachmentRepo) CreateAttachments(ctx context.Context, tx *sqlx.Tx, messageID string, attachments []model.MessageAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	for i := range attachments {
		attachments[i].MessageID = messageID
		_, err := tx.ExecContext(ctx,
			`INSERT INTO message_attachments (id, message_id, file_name, mime_type, file_size, file_path, thumbnail_path, width, height)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)`,
			messageID,
			attachments[i].FileName,
			attachments[i].MimeType,
			attachments[i].FileSize,
			attachments[i].FilePath,
			attachments[i].ThumbnailPath,
			attachments[i].Width,
			attachments[i].Height,
		)
		if err != nil {
			return fmt.Errorf("insert attachment %d: %w", i, err)
		}
	}
	return nil
}

// ListByMessageIDs 批量查询多条消息的附件
func (r *AttachmentRepo) ListByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]model.MessageAttachment, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, message_id, file_name, mime_type, file_size, file_path, thumbnail_path, width, height, created_at
		 FROM message_attachments WHERE message_id IN (?)`,
		messageIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build in query: %w", err)
	}
	query = r.db.Rebind(query)

	var list []model.MessageAttachment
	if err := r.db.SelectContext(ctx, &list, query, args...); err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}

	result := make(map[string][]model.MessageAttachment, len(messageIDs))
	for _, a := range list {
		result[a.MessageID] = append(result[a.MessageID], a)
	}
	return result, nil
}
