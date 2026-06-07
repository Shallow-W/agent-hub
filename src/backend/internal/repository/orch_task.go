package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type OrchTaskRepo struct {
	db *sqlx.DB
}

func NewOrchTaskRepo(db *sqlx.DB) *OrchTaskRepo {
	return &OrchTaskRepo{db: db}
}

func (r *OrchTaskRepo) Create(ctx context.Context, task *model.OrchTask) error {
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO orch_tasks (id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, kb_preload, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		task.ID, task.ConversationID, task.UserID, task.OrchAgentID, task.Status,
		task.DispatchPlan, task.WorkerStatus, task.WorkerResults, task.Summary, task.OriginalMessage, task.KBPreload,
		task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert orch task: %w", err)
	}
	return nil
}

func (r *OrchTaskRepo) GetByID(ctx context.Context, id string) (*model.OrchTask, error) {
	var t model.OrchTask
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, kb_preload, created_at, updated_at
		 FROM orch_tasks WHERE id = $1`, id,
	).StructScan(&t)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get orch task: %w", err)
	}
	return &t, nil
}

func (r *OrchTaskRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_tasks SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, status,
	)
	if err != nil {
		return fmt.Errorf("update orch task status: %w", err)
	}
	return nil
}

// UpdateWorkerResult 更新单个 worker 的状态和结果，返回 true 表示所有 worker 都已完成。
// Uses explicit transaction to ensure SELECT FOR UPDATE + UPDATE atomicity.
func (r *OrchTaskRepo) UpdateWorkerResult(ctx context.Context, id, workerName, status, result string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var t model.OrchTask
	err = tx.QueryRowxContext(ctx,
		`SELECT worker_status, worker_results FROM orch_tasks WHERE id = $1 FOR UPDATE`, id,
	).StructScan(&t)
	if err != nil {
		return false, fmt.Errorf("lock orch task: %w", err)
	}

	var wsMap map[string]string
	if t.WorkerStatus != "" {
		_ = json.Unmarshal([]byte(t.WorkerStatus), &wsMap)
	}
	if wsMap == nil {
		wsMap = make(map[string]string)
	}
	wsMap[workerName] = status

	var wrMap map[string]string
	if t.WorkerResults != "" {
		_ = json.Unmarshal([]byte(t.WorkerResults), &wrMap)
	}
	if wrMap == nil {
		wrMap = make(map[string]string)
	}
	if result != "" {
		wrMap[workerName] = result
	}

	wsJSON, _ := json.Marshal(wsMap)
	wrJSON, _ := json.Marshal(wrMap)

	_, err = tx.ExecContext(ctx,
		`UPDATE orch_tasks SET worker_status = $2, worker_results = $3, updated_at = NOW() WHERE id = $1`,
		id, string(wsJSON), string(wrJSON),
	)
	if err != nil {
		return false, fmt.Errorf("update orch worker result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	allDone := true
	for _, s := range wsMap {
		if s != "completed" && s != "failed" {
			allDone = false
			break
		}
	}
	return allDone, nil
}

func (r *OrchTaskRepo) SetSummary(ctx context.Context, id, summary string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_tasks SET summary = $2, status = $3, updated_at = NOW() WHERE id = $1`,
		id, summary, model.OrchTaskCompleted,
	)
	if err != nil {
		return fmt.Errorf("set orch summary: %w", err)
	}
	return nil
}

// UpdateStatusCAS atomically transitions status only if current status matches fromStatus.
// Returns true if the transition succeeded.
func (r *OrchTaskRepo) UpdateStatusCAS(ctx context.Context, id, fromStatus, toStatus string) (bool, error) {
	var err error
	if fromStatus == "" {
		_, err = r.db.ExecContext(ctx,
			`UPDATE orch_tasks SET status = $2, updated_at = NOW() WHERE id = $1`,
			id, toStatus,
		)
	} else {
		result, execErr := r.db.ExecContext(ctx,
			`UPDATE orch_tasks SET status = $3, updated_at = NOW() WHERE id = $1 AND status = $2`,
			id, fromStatus, toStatus,
		)
		if execErr != nil {
			return false, fmt.Errorf("cas update orch task status: %w", execErr)
		}
		rows, _ := result.RowsAffected()
		return rows > 0, nil
	}
	if err != nil {
		return false, fmt.Errorf("update orch task status: %w", err)
	}
	return true, nil
}
