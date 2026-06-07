package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// TaskRepo 负责任务看板的数据访问。
type TaskRepo struct {
	db *sqlx.DB
}

// NewTaskRepo 创建任务仓库。
func NewTaskRepo(db *sqlx.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func selectTaskSQL(from string) string {
	return `SELECT t.id, t.user_id, t.conversation_id, t.assignee_id, t.agent_id,
		t.title, t.description, t.status, t.priority, t.created_at, t.updated_at,
		COALESCE(u.username, '') AS assignee_name,
		COALESCE(a.name, '') AS agent_name,
		t.orch_task_id, t.worker_name
		FROM ` + from + `
		LEFT JOIN users u ON u.id = t.assignee_id
		LEFT JOIN agents a ON a.id = t.agent_id`
}

// List 按筛选条件列出任务。conversation_id 必填，按会话查询共享任务。
func (r *TaskRepo) List(ctx context.Context, userID string, filter model.TaskFilter) ([]*model.WorkspaceTask, error) {
	var args []interface{}
	conditions := []string{}
	argIdx := 0

	if filter.ConversationID != "" {
		argIdx++
		args = append(args, filter.ConversationID)
		conditions = append(conditions, fmt.Sprintf("t.conversation_id = $%d", argIdx))
	}
	if filter.Status != "" {
		argIdx++
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("t.status = $%d", argIdx))
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	query := selectTaskSQL("workspace_tasks t") +
		where + " ORDER BY t.updated_at DESC"

	var tasks []*model.WorkspaceTask
	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

// Create 新建任务。userID 为空时 user_id 列写入 NULL。
func (r *TaskRepo) Create(ctx context.Context, userID string, input model.TaskCreateInput) (*model.WorkspaceTask, error) {
	var task model.WorkspaceTask
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO workspace_tasks
		 (user_id, conversation_id, assignee_id, agent_id, title, description, status, priority, orch_task_id, worker_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, user_id, conversation_id, assignee_id, agent_id, title, description,
		 status, priority, created_at, updated_at, '' AS assignee_name, '' AS agent_name,
		 orch_task_id, worker_name`,
		nilIfEmpty(userID), input.ConversationID, input.AssigneeID, input.AgentID, input.Title,
		input.Description, input.Status, input.Priority, input.OrchTaskID, input.WorkerName,
	).StructScan(&task)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return &task, nil
}

// GetByID 按 ID 查询任务（会话内共享，不按 user_id 过滤）。
func (r *TaskRepo) GetByID(ctx context.Context, userID, id string) (*model.WorkspaceTask, error) {
	query := selectTaskSQL("workspace_tasks t") + " WHERE t.id = $1"
	var task model.WorkspaceTask
	err := r.db.QueryRowxContext(ctx, query, id).StructScan(&task)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &task, nil
}

// GetByOrchTaskAndWorker 按 orch_task_id + worker_name 查找任务。
func (r *TaskRepo) GetByOrchTaskAndWorker(ctx context.Context, orchTaskID, workerName string) (*model.WorkspaceTask, error) {
	query := selectTaskSQL("workspace_tasks t") + " WHERE t.orch_task_id = $1 AND t.worker_name = $2"
	var task model.WorkspaceTask
	err := r.db.QueryRowxContext(ctx, query, orchTaskID, workerName).StructScan(&task)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get task by orch: %w", err)
	}
	return &task, nil
}

// Update 更新任务内容字段。userID 为空时仅按 ID 匹配。
func (r *TaskRepo) Update(ctx context.Context, userID, id string, input model.TaskUpdateInput) (*model.WorkspaceTask, error) {
	sets := make([]string, 0, 6)
	args := make([]interface{}, 0, 8)

	addSet := func(column string, value interface{}) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if input.Title != nil {
		addSet("title", *input.Title)
	}
	if input.Description != nil {
		addSet("description", *input.Description)
	}
	if input.Priority != nil {
		addSet("priority", *input.Priority)
	}
	if input.AssigneeID != nil {
		addSet("assignee_id", *input.AssigneeID)
	}
	if input.AgentID != nil {
		addSet("agent_id", *input.AgentID)
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, "", id)
	}

	args = append(args, id)
	idArg := len(args)
	query := `UPDATE workspace_tasks SET ` + strings.Join(sets, ", ") +
		`, updated_at = NOW() WHERE id = $` + fmt.Sprint(idArg)
	if userID != "" {
		args = append(args, userID)
		userArg := len(args)
		query += ` AND user_id = $` + fmt.Sprint(userArg)
	}
	query += ` RETURNING id`

	var updatedID string
	if err := r.db.QueryRowxContext(ctx, query, args...).Scan(&updatedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update task: %w", err)
	}
	return r.GetByID(ctx, "", updatedID)
}

// MoveStatus 更新任务状态。userID 为空时仅按 ID 匹配。
func (r *TaskRepo) MoveStatus(ctx context.Context, userID, id, status string) (*model.WorkspaceTask, error) {
	query := `UPDATE workspace_tasks SET status = $1, updated_at = NOW() WHERE id = $2`
	args := []interface{}{status, id}
	if userID != "" {
		query += ` AND user_id = $3`
		args = append(args, userID)
	}
	query += ` RETURNING id`

	var updatedID string
	err := r.db.QueryRowxContext(ctx, query, args...).Scan(&updatedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("move task status: %w", err)
	}
	return r.GetByID(ctx, "", updatedID)
}

// Delete 删除任务。userID 为空时仅按 ID 匹配。
func (r *TaskRepo) Delete(ctx context.Context, userID, id string) (bool, error) {
	query := `DELETE FROM workspace_tasks WHERE id = $1`
	args := []interface{}{id}
	if userID != "" {
		query += ` AND user_id = $2`
		args = append(args, userID)
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("delete task: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// nilIfEmpty 空字符串转为 nil *string（用于 NULLable 列）。
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
