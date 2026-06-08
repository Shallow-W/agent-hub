package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// OrchTaskCardRepo 负责 orch_task_cards 表的数据访问。
type OrchTaskCardRepo struct {
	db *sqlx.DB
}

// NewOrchTaskCardRepo 创建任务卡片仓库。
func NewOrchTaskCardRepo(db *sqlx.DB) *OrchTaskCardRepo {
	return &OrchTaskCardRepo{db: db}
}

// Create 创建一张任务卡片，RETURNING 全部字段。
func (r *OrchTaskCardRepo) Create(ctx context.Context, card *model.OrchTaskCard) error {
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO orch_task_cards
		 (conversation_id, orch_task_id, sender_id, sender_name, sender_avatar,
		  worker_id, worker_name, worker_avatar, task_content, task_summary,
		  worker_result, status, priority, task_hash, dispatched_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id, conversation_id, orch_task_id,
		 sender_id, sender_name, sender_avatar,
		 worker_id, worker_name, worker_avatar,
		 task_content, task_summary, worker_result,
		 status, priority, task_hash,
		 dispatched_at, started_at, completed_at,
		 created_at, updated_at`,
		card.ConversationID, card.OrchTaskID,
		card.SenderID, card.SenderName, card.SenderAvatar,
		card.WorkerID, card.WorkerName, card.WorkerAvatar,
		card.TaskContent, card.TaskSummary,
		card.WorkerResult, card.Status, card.Priority, card.TaskHash,
		card.DispatchedAt,
	).StructScan(card)
	if err != nil {
		return fmt.Errorf("insert orch task card: %w", err)
	}
	return nil
}

// GetByTaskHash 按 task_hash 查找卡片（幂等检查）。
func (r *OrchTaskCardRepo) GetByTaskHash(ctx context.Context, taskHash string) (*model.OrchTaskCard, error) {
	var card model.OrchTaskCard
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, conversation_id, orch_task_id,
		 sender_id, sender_name, sender_avatar,
		 worker_id, worker_name, worker_avatar,
		 task_content, task_summary, worker_result,
		 status, priority, task_hash,
		 dispatched_at, started_at, completed_at,
		 created_at, updated_at
		 FROM orch_task_cards WHERE task_hash = $1`, taskHash,
	).StructScan(&card)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get orch task card by hash: %w", err)
	}
	return &card, nil
}

// ListByConversation 查询群聊的所有任务卡片。
func (r *OrchTaskCardRepo) ListByConversation(ctx context.Context, conversationID string) ([]*model.OrchTaskCard, error) {
	var cards []*model.OrchTaskCard
	err := r.db.SelectContext(ctx, &cards,
		`SELECT id, conversation_id, orch_task_id,
		 sender_id, sender_name, sender_avatar,
		 worker_id, worker_name, worker_avatar,
		 task_content, task_summary, worker_result,
		 status, priority, task_hash,
		 dispatched_at, started_at, completed_at,
		 created_at, updated_at
		 FROM orch_task_cards
		 WHERE conversation_id = $1
		 ORDER BY updated_at DESC`, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list orch task cards: %w", err)
	}
	return cards, nil
}

// UpdateStatus 更新卡片状态。
// 当 status == "in_progress" 时自动 SET started_at = NOW()
// 当 status == "done" 或 "failed" 时自动 SET completed_at = NOW()
func (r *OrchTaskCardRepo) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE orch_task_cards SET status = $1, updated_at = NOW()`
	switch status {
	case "in_progress":
		query += `, started_at = NOW()`
	case "done", "failed":
		query += `, completed_at = NOW()`
	}
	query += ` WHERE id = $2`

	res, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update orch task card status: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateWorkerResult 存储 worker 回答，同时设 completed_at。
// 状态由 UpdateStatus 单独管理。
func (r *OrchTaskCardRepo) UpdateWorkerResult(ctx context.Context, id, result string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_task_cards
		 SET worker_result = $1, completed_at = NOW(), updated_at = NOW()
		 WHERE id = $2`,
		result, id,
	)
	if err != nil {
		return fmt.Errorf("update orch task card worker result: %w", err)
	}
	return nil
}

// FailAllByOrchTask 批量标记指定 orch_task_id 下所有未完成的卡片为 failed。
func (r *OrchTaskCardRepo) FailAllByOrchTask(ctx context.Context, orchTaskID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_task_cards
		 SET status = 'failed', completed_at = NOW(), updated_at = NOW()
		 WHERE orch_task_id = $1 AND status NOT IN ('done', 'failed')`,
		orchTaskID,
	)
	if err != nil {
		return fmt.Errorf("fail all orch task cards: %w", err)
	}
	return nil
}
