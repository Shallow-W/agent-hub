package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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

	// 创建 WorkspaceTask 卡片 + 推送 WS 信号
	if s.taskSvc != nil && orchTaskID != "" {
		if _, err := s.taskSvc.CreateOrchWorkerTask(ctx, convID, userID, agentID, truncateString(task.Task, 80), task.Task, orchTaskID, task.AgentName); err != nil {
			slog.Warn("create orch worker task board card failed", "orch_task", orchTaskID, "worker", task.AgentName, "error", err)
		} else {
			s.pushTaskChanged(ctx, convID, userID)
		}
	}

	// 标记为执行中
	if s.taskSvc != nil && orchTaskID != "" {
		if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, orchTaskID, task.AgentName, "in_progress"); err != nil {
			slog.Warn("update orch worker task to in_progress failed", "orch_task", orchTaskID, "worker", task.AgentName, "error", err)
		} else {
			s.pushTaskChanged(ctx, convID, userID)
		}
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
		_, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, task.AgentName, "completed", truncateString(msg.Content, 2000))
		if err != nil {
			slog.Warn("update worker result failed", "orch_task", orchTaskID, "worker", task.AgentName, "error", err)
			return
		}

		// 同步 WorkspaceTask 状态为 done
		if s.taskSvc != nil {
			if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, orchTaskID, task.AgentName, "done"); err != nil {
				slog.Warn("update orch worker task board status failed", "orch_task", orchTaskID, "worker", task.AgentName, "error", err)
			} else {
				s.pushTaskChanged(ctx, convID, userID)
			}
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

	_, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, workerName, "failed", reason)
	if err != nil {
		slog.Warn("mark worker failed error", "orch_task", orchTaskID, "worker", workerName, "error", err)
		return
	}

	// 同步 WorkspaceTask 状态为 blocked
	if s.taskSvc != nil {
		if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, orchTaskID, workerName, "blocked"); err != nil {
			slog.Warn("update orch worker task board status (failed) error", "orch_task", orchTaskID, "worker", workerName, "error", err)
		} else {
			// 获取 convID 用于推送 WS 信号（仅在更新成功时推送）
			orchTask, _ := s.orchTaskRepo.GetByID(ctx, orchTaskID)
			if orchTask != nil {
				s.pushTaskChanged(ctx, orchTask.ConversationID, orchTask.UserID)
			}
		}
	}

}

// startWorkersAndWait 派发所有 worker 并等待全部完成（包括 WS 推送），然后触发 summary。
func (s *OrchestratorService) startWorkersAndWait(ctx context.Context, convID, userID string, tasks []DispatchTask, agentNameToID map[string]string, orchAgentName, kbPreload, orchTaskID string) {
	var wg sync.WaitGroup
	for _, t := range tasks {
		agentID, ok := agentNameToID[t.AgentName]
		if !ok {
			slog.Warn("unknown agent in dispatch", "agent", t.AgentName)
			continue
		}
		wg.Add(1)
		go func(task DispatchTask, agentID string) {
			defer wg.Done()
			s.dispatchOrchWorker(convID, userID, task, agentID, orchAgentName, kbPreload, orchTaskID)
		}(t, agentID)
	}
	wg.Wait()
	// 所有 worker 完整执行完毕（包括 postPersistAsync），现在可以安全地启动 summary
	s.goStartOrchSummary(orchTaskID)
}

// pushTaskChanged 推送 task.changed WS 信号通知前端刷新任务列表。
func (s *OrchestratorService) pushTaskChanged(ctx context.Context, convID, userID string) {
	if s.notifier == nil {
		return
	}
	memberIDs, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil || len(memberIDs) == 0 {
		memberIDs = []string{userID}
	}
	s.notifier.PushCustomEvent(convID, memberIDs, "task.changed", map[string]any{
		"conversation_id": convID,
	})
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
				// 将关联的 WorkspaceTask 标记为 blocked
				if s.taskSvc != nil {
					if err := s.taskSvc.FailAllTasksForOrchTask(ctx, orchTaskID); err != nil {
						slog.Warn("fail all workspace tasks after panic", "orch_task", orchTaskID, "error", err)
					}
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

	// 构建汇总+决策 prompt（支持多轮上下文）
	summaryPrompt := BuildSummaryPrompt(orchTask)
	summaryCtx := s.InjectAgentConfig(orchAgent, "", orchTask.UserID)

	msg, err := s.dispatchAndWait(ctx, orchTask.ConversationID, orchTask.UserID, orchAgent, summaryPrompt, summaryCtx)
	if err != nil {
		slog.Warn("orch summary dispatch failed", "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// 保存 summary 并过渡到 evaluating 状态（决策点）
	_ = s.orchTaskRepo.SetSummaryAndEvaluate(ctx, orchTaskID, msg.Content)

	// 推送汇总/决策消息
	if msg != nil {
		s.postPersistAsync(orchTask.ConversationID, orchTask.UserID, msg)
	}

	// 评估 Orch 回复，决定继续派发还是完成
	s.evaluateOrchResponse(orchTaskID, orchTask, orchAgent.Name, msg.Content)
}

// evaluateOrchResponse parses the orchestrator's summary response for @mention dispatches.
// If @mentions are found and max rounds not reached, archives current round and dispatches new workers.
// Otherwise marks the task as completed.
func (s *OrchestratorService) evaluateOrchResponse(orchTaskID string, orchTask *model.OrchTask, orchestratorName, orchResponse string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dispatch := ParseOrchestratorOutput(orchResponse)

	// CAS guard: only proceed if status is still "evaluating"
	if s.orchTaskRepo != nil {
		ok, err := s.orchTaskRepo.UpdateStatusCAS(ctx, orchTaskID, model.OrchTaskEvaluating, model.OrchTaskCompleted)
		if err != nil {
			slog.Warn("evaluate orch response: CAS check failed", "id", orchTaskID, "error", err)
			return
		}
		if !ok {
			slog.Debug("evaluate orch response: status no longer evaluating", "id", orchTaskID)
			return
		}
	}

	// No @mentions -> orchestrator is done (already set to completed by CAS above)
	if dispatch == nil || len(dispatch.Tasks) == 0 {
		slog.Info("orch task completed (no further dispatch)", "orch_task", orchTaskID, "round", orchTask.Round)
		return
	}

	// Max rounds reached -> already completed by CAS, stay completed
	if orchTask.Round >= model.MaxOrchRounds-1 {
		slog.Info("orch task completed (max rounds reached)", "orch_task", orchTaskID, "round", orchTask.Round)
		return
	}

	// Transition evaluating -> workers_running via IncrementRound (archives current round, resets workers)
	if err := s.orchTaskRepo.IncrementRound(ctx, orchTaskID); err != nil {
		slog.Error("increment round failed", "orch_task", orchTaskID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// Resolve @mentions to agent IDs
	convAgents, err := s.convRepo.ListAgents(ctx, orchTask.ConversationID, orchTask.UserID)
	if err != nil {
		slog.Error("list agents for re-dispatch failed", "orch_task", orchTaskID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}
	agentNameToID := make(map[string]string)
	for _, ca := range convAgents {
		agentNameToID[ca.Name] = ca.AgentID
	}

	// Collect valid tasks for re-dispatch
	var validTasks []DispatchTask
	for _, t := range dispatch.Tasks {
		agentID, ok := agentNameToID[t.AgentName]
		if !ok {
			slog.Warn("re-dispatch: agent not found", "agent", t.AgentName, "orch_task", orchTaskID)
			continue
		}
		_ = agentID // validated by startWorkersAndWait via agentNameToID
		validTasks = append(validTasks, t)
	}

	// Zero valid workers -> force complete to avoid empty loop
	if len(validTasks) == 0 {
		slog.Warn("re-dispatch: no valid agents found, completing", "orch_task", orchTaskID)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskCompleted)
		return
	}

	slog.Info("orch re-dispatching workers", "orch_task", orchTaskID, "round", orchTask.Round+1, "workers", len(validTasks))
	go s.startWorkersAndWait(ctx, orchTask.ConversationID, orchTask.UserID, validTasks, agentNameToID, orchestratorName, orchTask.KBPreload, orchTaskID)
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
