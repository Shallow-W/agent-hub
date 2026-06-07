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

// dispatchOrchWorker dispatches a single worker task using the unified dispatchAndWait path.
// Runs in a goroutine: synchronously waits for the WS result, creates message, pushes to user,
// then updates OrchTask worker state and triggers summary if all workers are done.
func (s *OrchestratorService) dispatchOrchWorker(convID, userID string, task DispatchTask, agentID, orchestratorName, kbPreload, orchTaskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		slog.Warn("orch worker: agent not found", "agent_id", agentID, "error", err)
		s.markWorkerFailed(orchTaskID, task.AgentName, "agent not found")
		return
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		slog.Warn("orch worker: agent offline", "agent", task.AgentName)
		s.markWorkerFailed(orchTaskID, task.AgentName, "agent offline")
		return
	}

	// 权限校验
	if agent.UserID != nil && *agent.UserID != userID {
		s.markWorkerFailed(orchTaskID, task.AgentName, "permission denied")
		return
	}

	// 构建 dispatch 上下文
	dispatchCtx := fmt.Sprintf("[群聊背景]\n- Orchestrator: %s\n\n[调度指令]\nOrch @你，分配了以下任务：\n%s\n\n请完成这个任务并在回复末尾 @%s 表示完成。",
		orchestratorName, truncateString(task.Task, 2000), orchestratorName)

	if kbPreload != "" {
		dispatchCtx = kbPreload + dispatchCtx
	}
	dispatchCtx = s.InjectAgentConfig(agent, dispatchCtx, userID)

	// 统一路径：创建 daemon task → WS dispatch → channel wait → 创建消息
	msg, err := s.dispatchAndWait(ctx, convID, userID, agent, task.Task, dispatchCtx)
	if err != nil {
		slog.Warn("orch worker: dispatch failed", "agent", task.AgentName, "error", err)
		s.markWorkerFailed(orchTaskID, task.AgentName, err.Error())
		return
	}

	// 推送 worker 消息到用户 WS
	if msg != nil {
		s.postPersistAsync(convID, userID, msg)
	}

	// 更新 OrchTask worker 状态
	if s.orchTaskRepo != nil && orchTaskID != "" {
		allDone, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, task.AgentName, "completed", truncateString(msg.Content, 2000))
		if err != nil {
			slog.Warn("update worker result failed", "orch_task", orchTaskID, "worker", task.AgentName, "error", err)
			return
		}
		if allDone {
			s.goStartOrchSummary(orchTaskID)
		}
	}
}

// markWorkerFailed 标记 worker 失败并检查是否全部完成。
func (s *OrchestratorService) markWorkerFailed(orchTaskID, workerName, reason string) {
	if s.orchTaskRepo == nil || orchTaskID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allDone, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, workerName, "failed", reason)
	if err != nil {
		slog.Warn("mark worker failed error", "orch_task", orchTaskID, "worker", workerName, "error", err)
		return
	}
	if allDone {
		s.goStartOrchSummary(orchTaskID)
	}
}

// goStartOrchSummary launches startOrchSummary in a goroutine with panic recovery.
func (s *OrchestratorService) goStartOrchSummary(orchTaskID string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("startOrchSummary panicked", "orch_task", orchTaskID, "panic", r)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if s.orchTaskRepo != nil {
					_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
				}
			}
		}()
		s.startOrchSummary(orchTaskID)
	}()
}

// startOrchSummary 执行 Orch 汇总阶段：收集所有 worker 结果，调 Orch 生成汇总。
func (s *OrchestratorService) startOrchSummary(orchTaskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// CAS guard: only one goroutine can transition workers_running → summarizing
	if s.orchTaskRepo != nil {
		ok, err := s.orchTaskRepo.UpdateStatusCAS(ctx, orchTaskID, model.OrchTaskWorkersRunning, model.OrchTaskSummarizing)
		if err != nil {
			slog.Warn("start orch summary: CAS check failed", "id", orchTaskID, "error", err)
			return
		}
		if !ok {
			slog.Debug("start orch summary: another goroutine already started", "id", orchTaskID)
			return
		}
	}

	orchTask, err := s.orchTaskRepo.GetByID(ctx, orchTaskID)
	if err != nil || orchTask == nil {
		slog.Warn("start orch summary: task not found", "id", orchTaskID)
		return
	}

	orchAgent, err := s.agentRepo.GetByID(ctx, orchTask.OrchAgentID)
	if err != nil || orchAgent == nil || orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		slog.Warn("start orch summary: orch agent unavailable", "id", orchTask.OrchAgentID)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// 构建汇总 prompt
	var sb strings.Builder
	sb.WriteString(OrchestratorSystemPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("[汇总任务]\n")
	sb.WriteString("所有 Agent 已完成你分配的任务，以下是它们的执行结果。\n")
	sb.WriteString("请汇总各 Agent 的成果，给出最终结论。\n\n")
	sb.WriteString("[原始用户请求]\n")
	sb.WriteString(truncateString(orchTask.OriginalMessage, 1000))
	sb.WriteString("\n\n[Agent 执行结果]\n")

	// 解析 worker_results
	var workerResults map[string]string
	if orchTask.WorkerResults != "" {
		_ = json.Unmarshal([]byte(orchTask.WorkerResults), &workerResults)
	}
	for name, result := range workerResults {
		fmt.Fprintf(&sb, "### %s\n%s\n\n", name, truncateString(result, 2000))
	}
	sb.WriteString("请发布最终汇总。")

	// 使用统一路径 dispatchAndWait 进行汇总
	summaryPrompt := sb.String()
	summaryCtx := s.InjectAgentConfig(orchAgent, "", orchTask.UserID)

	msg, err := s.dispatchAndWait(ctx, orchTask.ConversationID, orchTask.UserID, orchAgent, summaryPrompt, summaryCtx)
	if err != nil {
		slog.Warn("orch summary dispatch failed", "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// 更新 OrchTask
	_ = s.orchTaskRepo.SetSummary(ctx, orchTaskID, msg.Content)

	// 推送汇总消息
	if msg != nil {
		s.postPersistAsync(orchTask.ConversationID, orchTask.UserID, msg)
	}
}

// postPersistAsync 推送消息到用户 WS 并缓存到 Redis（异步安全版）。
func (s *OrchestratorService) postPersistAsync(convID, userID string, msg *model.Message) {
	if msg == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 缓存到 Redis（避免刷新后丢失）
	if s.cacher != nil {
		if err := s.cacher.CacheMessage(ctx, convID, msg); err != nil {
			slog.Warn("cache async message failed", "conv", convID, "error", err)
		}
	}

	if s.notifier == nil {
		return
	}

	memberIDs, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil || len(memberIDs) == 0 {
		memberIDs = []string{userID}
	}

	s.notifier.PushToConversation(convID, memberIDs, msg)
}
