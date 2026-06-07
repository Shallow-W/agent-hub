package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// TaskRepo 定义任务服务依赖的数据访问能力。
type TaskRepo interface {
	List(ctx context.Context, userID string, filter model.TaskFilter) ([]*model.WorkspaceTask, error)
	Create(ctx context.Context, userID string, input model.TaskCreateInput) (*model.WorkspaceTask, error)
	GetByID(ctx context.Context, userID, id string) (*model.WorkspaceTask, error)
	GetByOrchTaskAndWorker(ctx context.Context, orchTaskID, workerName string) (*model.WorkspaceTask, error)
	Update(ctx context.Context, userID, id string, input model.TaskUpdateInput) (*model.WorkspaceTask, error)
	MoveStatus(ctx context.Context, userID, id, status string) (*model.WorkspaceTask, error)
	Delete(ctx context.Context, userID, id string) (bool, error)
	FailAllByOrchTask(ctx context.Context, orchTaskID string) error
}

// TaskBoardSync 定义 Orchestrator 到 TaskBoard 的同步接口。
type TaskBoardSync interface {
	CreateOrchWorkerTask(ctx context.Context, convID, userID, agentID, title, desc, orchTaskID, workerName string) (*model.WorkspaceTask, error)
	UpdateOrchWorkerStatus(ctx context.Context, orchTaskID, workerName, status string) error
	FailAllTasksForOrchTask(ctx context.Context, orchTaskID string) error
}

var (
	ErrTaskNotFound = errors.New("任务不存在")
	ErrTaskInvalid  = errors.New("任务参数不合法")
)

// TaskService 处理任务看板业务逻辑。
type TaskService struct {
	repo TaskRepo
}

// NewTaskService 创建任务服务。
func NewTaskService(repo TaskRepo) *TaskService {
	return &TaskService{repo: repo}
}

// GetByID 按 ID 查询任务。
func (s *TaskService) GetByID(ctx context.Context, userID, id string) (*model.WorkspaceTask, error) {
	task, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

// List 查询任务。
func (s *TaskService) List(ctx context.Context, userID string, filter model.TaskFilter) ([]*model.WorkspaceTask, error) {
	if filter.Status != "" && !isTaskStatus(filter.Status) {
		return nil, ErrTaskInvalid
	}
	tasks, err := s.repo.List(ctx, userID, filter)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	if tasks == nil {
		tasks = []*model.WorkspaceTask{}
	}
	return tasks, nil
}

// Create 创建任务。
func (s *TaskService) Create(ctx context.Context, userID string, input model.TaskCreateInput) (*model.WorkspaceTask, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" || len(input.Title) > 120 {
		return nil, ErrTaskInvalid
	}
	if input.Status == "" {
		input.Status = "todo"
	}
	if input.Priority == "" {
		input.Priority = "medium"
	}
	if !isTaskStatus(input.Status) || !isTaskPriority(input.Priority) {
		return nil, ErrTaskInvalid
	}
	task, err := s.repo.Create(ctx, userID, input)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

// Update 更新任务内容。
func (s *TaskService) Update(ctx context.Context, userID, id string, input model.TaskUpdateInput) (*model.WorkspaceTask, error) {
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || len(title) > 120 {
			return nil, ErrTaskInvalid
		}
		input.Title = &title
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		input.Description = &description
	}
	if input.Priority != nil && !isTaskPriority(*input.Priority) {
		return nil, ErrTaskInvalid
	}
	task, err := s.repo.Update(ctx, userID, id, input)
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

// MoveStatus 流转任务状态。
func (s *TaskService) MoveStatus(ctx context.Context, userID, id, status string) (*model.WorkspaceTask, error) {
	if !isTaskStatus(status) {
		return nil, ErrTaskInvalid
	}
	task, err := s.repo.MoveStatus(ctx, userID, id, status)
	if err != nil {
		return nil, fmt.Errorf("move task status: %w", err)
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

// Delete 删除任务。
func (s *TaskService) Delete(ctx context.Context, userID, id string) error {
	ok, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if !ok {
		return ErrTaskNotFound
	}
	return nil
}

// CreateOrchWorkerTask Orch 派发时创建任务卡片。
// 幂等：如果上一轮已创建同名 worker 卡片，则重置状态为 todo 而非重复插入。
func (s *TaskService) CreateOrchWorkerTask(ctx context.Context, convID, userID, agentID, title, desc, orchTaskID, workerName string) (*model.WorkspaceTask, error) {
	// 检查上一轮是否已存在
	existing, err := s.repo.GetByOrchTaskAndWorker(ctx, orchTaskID, workerName)
	if err != nil {
		return nil, fmt.Errorf("lookup existing orch worker task: %w", err)
	}
		if existing != nil {
		// 更新 title 和 description 为新一轮的值
		if _, err := s.repo.Update(ctx, "", existing.ID, model.TaskUpdateInput{
			Title:       &title,
			Description: &desc,
		}); err != nil {
			return nil, fmt.Errorf("update orch worker task: %w", err)
		}
		// 重置为 todo，开始新一轮
		updated, err := s.repo.MoveStatus(ctx, "", existing.ID, "todo")
		if err != nil {
			return nil, fmt.Errorf("reset orch worker task status: %w", err)
		}
		return updated, nil
		}

	task, err := s.repo.Create(ctx, userID, model.TaskCreateInput{
		ConversationID: &convID,
		AgentID:        &agentID,
		Title:          title,
		Description:    desc,
		Status:         "todo",
		Priority:       "medium",
		OrchTaskID:     &orchTaskID,
		WorkerName:     &workerName,
	})
	if err != nil {
		return nil, fmt.Errorf("create orch worker task: %w", err)
	}
	return task, nil
}

// UpdateOrchWorkerStatus Worker 状态变化时更新。
func (s *TaskService) UpdateOrchWorkerStatus(ctx context.Context, orchTaskID, workerName, status string) error {
	task, err := s.repo.GetByOrchTaskAndWorker(ctx, orchTaskID, workerName)
	if err != nil {
		return fmt.Errorf("find orch worker task: %w", err)
	}
	if task == nil {
		slog.Debug("UpdateOrchWorkerStatus: task not found", "orch_task_id", orchTaskID, "worker", workerName)
		return nil
	}
	_, err = s.repo.MoveStatus(ctx, "", task.ID, status)
	if err != nil {
		return fmt.Errorf("update orch worker status: %w", err)
	}
	return nil
}

// FailAllTasksForOrchTask 将指定 orch_task_id 下所有未完成的 WorkspaceTask 标记为 blocked。
func (s *TaskService) FailAllTasksForOrchTask(ctx context.Context, orchTaskID string) error {
	if err := s.repo.FailAllByOrchTask(ctx, orchTaskID); err != nil {
		return fmt.Errorf("fail all tasks for orch task: %w", err)
	}
	return nil
}

func isTaskStatus(status string) bool {
	switch status {
	case "todo", "in_progress", "blocked", "done", "cancelled":
		return true
	default:
		return false
	}
}

func isTaskPriority(priority string) bool {
	switch priority {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}
