package service

import (
	"context"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeTaskRepo struct {
	task *model.WorkspaceTask
}

func (r *fakeTaskRepo) List(context.Context, string, model.TaskFilter) ([]*model.WorkspaceTask, error) {
	return []*model.WorkspaceTask{}, nil
}

func (r *fakeTaskRepo) Create(_ context.Context, userID string, input model.TaskCreateInput) (*model.WorkspaceTask, error) {
	r.task = &model.WorkspaceTask{
		ID:          "task-1",
		UserID:      &userID,
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		Priority:    input.Priority,
	}
	return r.task, nil
}

func (r *fakeTaskRepo) GetByID(context.Context, string, string) (*model.WorkspaceTask, error) {
	return r.task, nil
}

func (r *fakeTaskRepo) Update(_ context.Context, _, _ string, input model.TaskUpdateInput) (*model.WorkspaceTask, error) {
	if r.task == nil {
		return nil, nil
	}
	if input.Title != nil {
		r.task.Title = *input.Title
	}
	if input.Priority != nil {
		r.task.Priority = *input.Priority
	}
	return r.task, nil
}

func (r *fakeTaskRepo) MoveStatus(_ context.Context, _, _, status string) (*model.WorkspaceTask, error) {
	if r.task == nil {
		return nil, nil
	}
	r.task.Status = status
	return r.task, nil
}

func (r *fakeTaskRepo) Delete(context.Context, string, string) (bool, error) {
	return r.task != nil, nil
}

func TestTaskServiceCreateDefaults(t *testing.T) {
	svc := NewTaskService(&fakeTaskRepo{})
	task, err := svc.Create(context.Background(), "user-1", model.TaskCreateInput{Title: "  设计任务看板  "})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Title != "设计任务看板" {
		t.Fatalf("expected trimmed title, got %q", task.Title)
	}
	if task.Status != "todo" || task.Priority != "medium" {
		t.Fatalf("unexpected defaults: %s/%s", task.Status, task.Priority)
	}
}

func TestTaskServiceRejectsInvalidStatus(t *testing.T) {
	svc := NewTaskService(&fakeTaskRepo{})
	_, err := svc.MoveStatus(context.Background(), "user-1", "task-1", "unknown")
	if err != ErrTaskInvalid {
		t.Fatalf("expected ErrTaskInvalid, got %v", err)
	}
}
