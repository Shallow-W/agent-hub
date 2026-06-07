package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/pkg/ws"
	"github.com/google/uuid"
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
	if agent == nil || agent.MachineID == nil || *agent.MachineID == "" {
		return ErrAgentNotFound
	}
	if !hasDiscoveredSkillSource(agent.CapabilitiesJSON, sourcePath) {
		return ErrAgentInvalidInput
	}

	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		return ErrAgentOffline
	}

	payload, err := json.Marshal(map[string]string{"source_path": sourcePath})
	if err != nil {
		return fmt.Errorf("marshal open skill payload: %w", err)
	}

	taskID := uuid.NewString()
	ch := s.daemonHub.RegisterTaskPromise(taskID)
	defer s.daemonHub.RemoveTaskPromise(taskID)

	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"id":              taskID,
			"cli_tool":        daemonOpenPathTool,
			"prompt":          string(payload),
			"agent_id":        agentID,
			"conversation_id": "",
		},
	}); err != nil {
		return fmt.Errorf("send open skill task: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case result := <-ch:
		if result == nil || result.Error != "" {
			errMsg := "open skill location failed"
			if result != nil && result.Error != "" {
				errMsg = result.Error
			}
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	case <-ctx.Done():
		return ErrMsgAgentTimeout
	}
}
