package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

func (s *AgentService) syncDaemonSkillFiles(ctx context.Context, agent *model.Agent, userID, capabilitiesJSON string) error {
	if agent == nil || agent.MachineID == nil || *agent.MachineID == "" {
		return ErrAgentInvalidInput
	}
	payload, err := json.Marshal(map[string]string{
		"capabilities_json": capabilitiesJSON,
	})
	if err != nil {
		return fmt.Errorf("marshal skill sync payload: %w", err)
	}
	task, err := s.repo.CreateDaemonTask(ctx, userID, "", agent.ID, *agent.MachineID, daemonSkillSyncTool, string(payload), "")
	if err != nil {
		return fmt.Errorf("create skill sync task: %w", err)
	}
	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if task.Status == "failed" {
		return fmt.Errorf("sync daemon skill files: %s", task.Error)
	}
	return nil
}

func (s *AgentService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := s.repo.GetDaemonTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get daemon task: %w", err)
		}
		if task != nil && (task.Status == "completed" || task.Status == "failed") {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrMsgAgentTimeout
		case <-ticker.C:
		}
	}
}
