package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// OrchConvRepo queries conversation agents for @mention resolution.
type OrchConvRepo interface {
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
}

// OrchAgentRepo queries agent details and creates daemon tasks.
type OrchAgentRepo interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

// RouteResult is returned by RouteMention containing agent reply messages and dispatch info.
type RouteResult struct {
	AgentMessages []*model.Message
	Dispatches    []DispatchInfo
}

// DispatchInfo records what was dispatched for logging/debugging.
type DispatchInfo struct {
	AgentID   string
	AgentName string
	Task      string
	Parallel  bool
}

// OrchestratorService handles @mention routing and orchestrated multi-agent dispatch.
type OrchestratorService struct {
	convRepo  OrchConvRepo
	agentRepo OrchAgentRepo
	msgRepo   MsgRepo
}

// NewOrchestratorService creates a new orchestrator service.
func NewOrchestratorService(convRepo OrchConvRepo, agentRepo OrchAgentRepo, msgRepo MsgRepo) *OrchestratorService {
	return &OrchestratorService{
		convRepo:  convRepo,
		agentRepo: agentRepo,
		msgRepo:   msgRepo,
	}
}

// RouteMention is the main entry point called when a message contains @mentions.
func (s *OrchestratorService) RouteMention(ctx context.Context, convID, userID, content string) (*RouteResult, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("orch get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}

	convAgents, err := s.convRepo.ListAgents(ctx, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("orch list agents: %w", err)
	}

	mentions := ParseMentions(content)
	if len(mentions) == 0 {
		return nil, nil
	}

	mentionMap := FindMentionedAgentID(mentions, convAgents)
	if len(mentionMap) == 0 {
		return nil, nil
	}

	result := &RouteResult{}

	for _, m := range mentions {
		agentID, ok := mentionMap[m.AgentName]
		if !ok {
			continue
		}

		agent, err := s.agentRepo.GetByID(ctx, agentID)
		if err != nil {
			slog.Warn("orch get agent failed", "agent_id", agentID, "error", err)
			continue
		}
		if agent == nil {
			continue
		}

		if agent.Type == "orchestrator" {
			msgs, err := s.handleOrchestratedDispatch(ctx, convID, userID, agentID, content, convAgents)
			if err != nil {
				slog.Warn("orchestrated dispatch failed", "agent_id", agentID, "error", err)
				continue
			}
			result.AgentMessages = append(result.AgentMessages, msgs...)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: m.AgentName,
				Task:      content,
			})
		} else {
			msg, err := s.dispatchSingleAgent(ctx, convID, userID, agentID, content)
			if err != nil {
				slog.Warn("direct dispatch failed", "agent_id", agentID, "error", err)
				continue
			}
			result.AgentMessages = append(result.AgentMessages, msg)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: m.AgentName,
				Task:      content,
			})
		}
	}

	return result, nil
}

// dispatchSingleAgent dispatches to a single non-orchestrator agent.
func (s *OrchestratorService) dispatchSingleAgent(ctx context.Context, convID, userID, agentID, content string) (*model.Message, error) {
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.UserID != nil && *agent.UserID != userID {
		return nil, ErrMsgAgentNoPerm
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}

	dispatchCtx, err := s.buildDispatchContext(ctx, convID, DispatchTask{}, nil)
	if err != nil {
		slog.Warn("build dispatch context failed", "conv_id", convID, "error", err)
		dispatchCtx = ""
	}

	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, content, dispatchCtx)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}

	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts, _ := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, string(artifacts), nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	return msg, nil
}

// handleOrchestratedDispatch runs the full orchestrator flow: dispatch to orchestrator, parse output, fan out to workers.
func (s *OrchestratorService) handleOrchestratedDispatch(ctx context.Context, convID, userID, orchAgentID, content string, convAgents []model.ConversationAgent) ([]*model.Message, error) {
	orchAgent, err := s.agentRepo.GetByID(ctx, orchAgentID)
	if err != nil {
		return nil, fmt.Errorf("get orchestrator agent: %w", err)
	}
	if orchAgent == nil {
		return nil, ErrAgentNotFound
	}
	if orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}

	recentSummary, agentNames, err := s.buildRecentSummary(ctx, convID, convAgents)
	if err != nil {
		slog.Warn("build recent summary failed", "conv_id", convID, "error", err)
		recentSummary = ""
	}

	conv, _ := s.convRepo.GetByID(ctx, convID)
	convTitle := ""
	if conv != nil {
		convTitle = conv.Title
	}

	fullPrompt := BuildOrchestratorPrompt(convTitle, agentNames, recentSummary, content)

	orchTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, orchAgent.ID, *orchAgent.MachineID, orchAgent.CLITool, fullPrompt, "")
	if err != nil {
		return nil, fmt.Errorf("create orchestrator task: %w", err)
	}

	orchTask, err = s.waitDaemonTask(ctx, orchTask.ID)
	if err != nil {
		return nil, err
	}
	if orchTask.Status == "failed" {
		return nil, fmt.Errorf("orchestrator task failed: %s", orchTask.Error)
	}

	dispatch := ParseOrchestratorOutput(orchTask.Result)

	// Orchestrator responded directly without dispatching
	if dispatch == nil || len(dispatch.Tasks) == 0 {
		artifacts, _ := json.Marshal(map[string]string{
			"agent_id":   orchAgent.ID,
			"agent_name": orchAgent.Name,
			"cli_tool":   orchAgent.CLITool,
		})
		msg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, string(artifacts), nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("create orchestrator reply: %w", err)
		}
		return []*model.Message{msg}, nil
	}

	// Post orchestrator preamble if present
	var messages []*model.Message
	if dispatch.Preamble != "" {
		artifacts, _ := json.Marshal(map[string]string{
			"agent_id":   orchAgent.ID,
			"agent_name": orchAgent.Name,
			"cli_tool":   orchAgent.CLITool,
		})
		preambleMsg, err := s.msgRepo.Create(ctx, convID, "assistant", dispatch.Preamble, string(artifacts), nil, nil, nil, nil)
		if err != nil {
			slog.Warn("create orchestrator preamble message failed", "error", err)
		} else {
			messages = append(messages, preambleMsg)
		}
	}

	// Build agent name -> ID map
	agentNameToID := make(map[string]string)
	for _, ca := range convAgents {
		agentNameToID[ca.Name] = ca.AgentID
	}

	// Dispatch worker tasks
	depResults := make(map[string]string) // agentName -> result summary

	// Collect parallel tasks (non-sequential) and process sequential tasks in order
	var parallelTasks []DispatchTask
	for _, t := range dispatch.Tasks {
		if t.Sequential {
			// First execute any accumulated parallel tasks
			if len(parallelTasks) > 0 {
				workerMsgs := s.dispatchParallel(ctx, convID, userID, parallelTasks, agentNameToID, depResults)
				messages = append(messages, workerMsgs...)
				parallelTasks = nil
			}
			// Execute sequential task
			workerMsg := s.dispatchSequential(ctx, convID, userID, t, agentNameToID, depResults)
			if workerMsg != nil {
				messages = append(messages, workerMsg)
			}
		} else {
			parallelTasks = append(parallelTasks, t)
		}
	}
	// Flush remaining parallel tasks
	if len(parallelTasks) > 0 {
		workerMsgs := s.dispatchParallel(ctx, convID, userID, parallelTasks, agentNameToID, depResults)
		messages = append(messages, workerMsgs...)
	}

	return messages, nil
}

// dispatchParallel creates daemon tasks for multiple agents simultaneously and waits for all.
func (s *OrchestratorService) dispatchParallel(ctx context.Context, convID, userID string, tasks []DispatchTask, agentNameToID map[string]string, depResults map[string]string) []*model.Message {
	type taskResult struct {
		index     int
		msg       *model.Message
		agentName string
	}

	resultCh := make(chan taskResult, len(tasks))
	for i, t := range tasks {
		go func(idx int, task DispatchTask) {
			msg, err := s.dispatchWorker(ctx, convID, userID, task, agentNameToID, depResults)
			if err != nil {
				slog.Warn("parallel dispatch failed", "agent", task.AgentName, "error", err)
				resultCh <- taskResult{index: idx, msg: nil}
				return
			}
			resultCh <- taskResult{index: idx, msg: msg, agentName: task.AgentName}
		}(i, t)
	}

	var results []*model.Message
	for range tasks {
		tr := <-resultCh
		if tr.msg != nil {
			depResults[tr.agentName] = truncateString(tr.msg.Content, 500)
			results = append(results, tr.msg)
		}
	}
	return results
}

// dispatchSequential creates a daemon task for one agent, waits, then returns.
func (s *OrchestratorService) dispatchSequential(ctx context.Context, convID, userID string, task DispatchTask, agentNameToID map[string]string, depResults map[string]string) *model.Message {
	msg, err := s.dispatchWorker(ctx, convID, userID, task, agentNameToID, depResults)
	if err != nil {
		slog.Warn("sequential dispatch failed", "agent", task.AgentName, "error", err)
		return nil
	}
	depResults[task.AgentName] = truncateString(msg.Content, 500)
	return msg
}

// dispatchWorker dispatches to a single worker agent and returns its reply message.
func (s *OrchestratorService) dispatchWorker(ctx context.Context, convID, userID string, task DispatchTask, agentNameToID map[string]string, depResults map[string]string) (*model.Message, error) {
	agentID, ok := agentNameToID[task.AgentName]
	if !ok {
		return nil, fmt.Errorf("agent %q not found in conversation", task.AgentName)
	}

	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get worker agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("worker agent %q not found", task.AgentName)
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, fmt.Errorf("worker agent %q offline", task.AgentName)
	}

	dispatchCtx, err := s.buildDispatchContext(ctx, convID, task, depResults)
	if err != nil {
		slog.Warn("build worker dispatch context failed", "agent", task.AgentName, "error", err)
		dispatchCtx = ""
	}

	daemonTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, task.Task, dispatchCtx)
	if err != nil {
		return nil, fmt.Errorf("create worker daemon task: %w", err)
	}

	daemonTask, err = s.waitDaemonTask(ctx, daemonTask.ID)
	if err != nil {
		return nil, err
	}
	if daemonTask.Status == "failed" {
		return nil, fmt.Errorf("worker daemon task failed: %s", daemonTask.Error)
	}

	artifacts, _ := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", daemonTask.Result, string(artifacts), nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create worker reply: %w", err)
	}
	return msg, nil
}

// buildDispatchContext builds Layer 2 context for a worker agent dispatch.
func (s *OrchestratorService) buildDispatchContext(ctx context.Context, convID string, task DispatchTask, depResults map[string]string) (string, error) {
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	var sb strings.Builder
	sb.WriteString("[群聊背景]\n")
	for _, m := range msgs {
		role := m.Role
		if m.Role == "assistant" {
			// Extract agent name from artifacts if available
			var a struct {
				AgentName string `json:"agent_name"`
			}
			if m.ArtifactsJSON != "" {
				_ = json.Unmarshal([]byte(m.ArtifactsJSON), &a)
			}
			if a.AgentName != "" {
				role = a.AgentName
			}
		}
		fmt.Fprintf(&sb, "- %s: %s\n", role, truncateString(m.Content, 100))
	}
	sb.WriteString("\n")

	if task.Task != "" {
		sb.WriteString("[调度指令]\n")
		fmt.Fprintf(&sb, "Orch @你，分配了以下任务：\n%s\n\n", task.Task)
	}

	if len(depResults) > 0 {
		sb.WriteString("[依赖输出]\n")
		for name, result := range depResults {
			fmt.Fprintf(&sb, "%s 已完成，结果摘要：\n%s\n\n", name, result)
		}
	}

	sb.WriteString("请完成这个任务并在回复末尾 @Orchestrator 表示完成。")

	return sb.String(), nil
}

// buildRecentSummary formats the last 10 messages and returns agent names in the conversation.
func (s *OrchestratorService) buildRecentSummary(ctx context.Context, convID string, convAgents []model.ConversationAgent) (string, []string, error) {
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		return "", nil, fmt.Errorf("list messages: %w", err)
	}

	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	var sb strings.Builder
	for _, m := range msgs {
		role := m.Role
		if m.Role == "assistant" {
			var a struct {
				AgentName string `json:"agent_name"`
			}
			if m.ArtifactsJSON != "" {
				_ = json.Unmarshal([]byte(m.ArtifactsJSON), &a)
			}
			if a.AgentName != "" {
				role = a.AgentName
			}
		}
		fmt.Fprintf(&sb, "- %s: %s\n", role, truncateString(m.Content, 100))
	}

	agentNames := make([]string, 0, len(convAgents))
	for _, ca := range convAgents {
		agentNames = append(agentNames, ca.Name)
	}

	return sb.String(), agentNames, nil
}

// waitDaemonTask polls for daemon task completion (600ms interval, 120s timeout).
func (s *OrchestratorService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := s.agentRepo.GetDaemonTask(ctx, taskID)
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

// truncateString truncates s to maxRunes runes, appending "..." if truncated.
func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
