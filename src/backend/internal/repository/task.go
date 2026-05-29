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
		COALESCE(a.name, '') AS agent_name
		FROM ` + from + `
		LEFT JOIN users u ON u.id = t.assignee_id
		LEFT JOIN agents a ON a.id = t.agent_id`
}

// List 按当前用户和筛选条件列出任务。
func (r *TaskRepo) List(ctx context.Context, userID string, filter model.TaskFilter) ([]*model.WorkspaceTask, error) {
	args := []interface{}{userID}
	conditions := []string{"t.user_id = $1"}

	if filter.ConversationID != "" {
		args = append(args, filter.ConversationID)
		conditions = append(conditions, fmt.Sprintf("t.conversation_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("t.status = $%d", len(args)))
	}

	query := selectTaskSQL("workspace_tasks t") +
		" WHERE " + strings.Join(conditions, " AND ") +
		" ORDER BY t.updated_at DESC"

	var tasks []*model.WorkspaceTask
	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

// Create 新建任务。
func (r *TaskRepo) Create(ctx context.Context, userID string, input model.TaskCreateInput) (*model.WorkspaceTask, error) {
	var task model.WorkspaceTask
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO workspace_tasks
		 (user_id, conversation_id, assignee_id, agent_id, title, description, status, priority)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, user_id, conversation_id, assignee_id, agent_id, title, description,
		 status, priority, created_at, updated_at, '' AS assignee_name, '' AS agent_name`,
		userID, input.ConversationID, input.AssigneeID, input.AgentID, input.Title,
		input.Description, input.Status, input.Priority,
	).StructScan(&task)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return &task, nil
}

// GetByID 查询当前用户名下的任务。
func (r *TaskRepo) GetByID(ctx context.Context, userID, id string) (*model.WorkspaceTask, error) {
	var task model.WorkspaceTask
	err := r.db.QueryRowxContext(ctx,
		selectTaskSQL("workspace_tasks t")+" WHERE t.user_id = $1 AND t.id = $2",
		userID, id,
	).StructScan(&task)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &task, nil
}

// Update 更新任务内容字段。
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
		return r.GetByID(ctx, userID, id)
	}

	args = append(args, userID, id)
	userArg := len(args) - 1
	idArg := len(args)
	query := `UPDATE workspace_tasks SET ` + strings.Join(sets, ", ") +
		`, updated_at = NOW() WHERE user_id = $` + fmt.Sprint(userArg) +
		` AND id = $` + fmt.Sprint(idArg) + ` RETURNING id`

	var updatedID string
	if err := r.db.QueryRowxContext(ctx, query, args...).Scan(&updatedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update task: %w", err)
	}
	return r.GetByID(ctx, userID, updatedID)
}

// MoveStatus 更新任务状态。
func (r *TaskRepo) MoveStatus(ctx context.Context, userID, id, status string) (*model.WorkspaceTask, error) {
	var updatedID string
	err := r.db.QueryRowxContext(ctx,
		`UPDATE workspace_tasks
		 SET status = $1, updated_at = NOW()
		 WHERE user_id = $2 AND id = $3
		 RETURNING id`,
		status, userID, id,
	).Scan(&updatedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("move task status: %w", err)
	}
	return r.GetByID(ctx, userID, updatedID)
}

// Delete 删除当前用户名下的任务。
func (r *TaskRepo) Delete(ctx context.Context, userID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM workspace_tasks WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	if err != nil {
		return false, fmt.Errorf("delete task: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}
