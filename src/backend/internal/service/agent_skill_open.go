package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// OpenDaemonSkillLocation 让归属电脑的 daemon 打开真实 SKILL.md 所在位置。
func (s *AgentService) OpenDaemonSkillLocation(ctx context.Context, userID, agentID, sourcePath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	if userID == "" || agentID == "" || sourcePath == "" {
		return ErrAgentInvalidInput
	}
	agent, err := s.repo.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil || agent.UserID == nil || *agent.UserID != userID {
		return ErrAgentNotFound
	}
	if agent.Source != "daemon" || agent.MachineID == nil || *agent.MachineID == "" {
		return ErrAgentInvalidInput
	}
	if !hasDiscoveredSkillSource(agent.CapabilitiesJSON, sourcePath) {
		return ErrAgentInvalidInput
	}
	payload, err := json.Marshal(map[string]string{"source_path": sourcePath})
	if err != nil {
		return fmt.Errorf("marshal open skill payload: %w", err)
	}
	task, err := s.repo.CreateDaemonTask(ctx, userID, "", agent.ID, *agent.MachineID, daemonOpenPathTool, string(payload), "")
	if err != nil {
		return fmt.Errorf("create open skill task: %w", err)
	}
	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if task.Status == "failed" {
		return fmt.Errorf("open daemon skill location: %s", task.Error)
	}
	return nil
}
