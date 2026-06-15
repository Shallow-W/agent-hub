package service

import "github.com/agent-hub/backend/internal/model"

// ResolvedDispatchTask is a dispatch assignment after resolving the target
// Agent name against the current group chat membership.
type ResolvedDispatchTask struct {
	DispatchTask
	AgentID string
}

// WorkerDispatchPlan separates valid worker assignments from references the
// orchestrator produced but the current group chat cannot execute.
type WorkerDispatchPlan struct {
	Tasks         []ResolvedDispatchTask
	UnknownTasks  []DispatchTask
	agentNameToID map[string]string
}

func BuildWorkerDispatchPlan(tasks []DispatchTask, convAgents []model.ConversationAgent) WorkerDispatchPlan {
	plan := WorkerDispatchPlan{
		Tasks:         make([]ResolvedDispatchTask, 0, len(tasks)),
		agentNameToID: buildAgentNameIDMap(convAgents),
	}
	for _, task := range tasks {
		agentID, ok := resolveDispatchAgentID(plan.agentNameToID, task.AgentName)
		if !ok {
			plan.UnknownTasks = append(plan.UnknownTasks, task)
			continue
		}
		plan.Tasks = append(plan.Tasks, ResolvedDispatchTask{
			DispatchTask: task,
			AgentID:      agentID,
		})
	}
	return plan
}

func (p WorkerDispatchPlan) HasWorkers() bool {
	return len(p.Tasks) > 0
}

func buildAgentNameIDMap(convAgents []model.ConversationAgent) map[string]string {
	agentNameToID := make(map[string]string, len(convAgents)*2)
	for _, ca := range convAgents {
		if ca.Name == "" || ca.AgentID == "" {
			continue
		}
		agentNameToID[ca.Name] = ca.AgentID
		agentNameToID[normalizeMentionName(ca.Name)] = ca.AgentID
	}
	return agentNameToID
}

func resolveDispatchAgentID(agentNameToID map[string]string, agentName string) (string, bool) {
	if agentID, ok := agentNameToID[agentName]; ok {
		return agentID, true
	}
	agentID, ok := agentNameToID[normalizeMentionName(agentName)]
	return agentID, ok
}
