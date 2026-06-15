package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// OrchSender 包含 Orch agent 的身份信息，在创建任务卡片时写入。
type OrchSender struct {
	ID     string
	Name   string
	Avatar string
}

// dispatchOrchWorker dispatches a single worker task using the unified dispatchAndWait path.
// Runs in a goroutine: synchronously waits for the WS result, creates message, pushes to user,
// then updates OrchTask worker state and triggers summary if all workers are done.
func (s *OrchestratorService) dispatchOrchWorker(convID, userID string, task DispatchTask, agentID, orchestratorName, kbPreload, orchTaskID string, orchSender OrchSender, replyTo *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Second)
	defer cancel()

	slog.Info(orchFlowLog, "stage", "worker.dispatch_start", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID, "reply_to", stringValue(replyTo), "task_len", len(task.Task), "task_preview", orchPreview(task.Task))

	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		slog.Warn(orchFlowLog, "stage", "worker.agent_not_found", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID, "error", err)
		s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, "agent not found")
		return
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		slog.Warn(orchFlowLog, "stage", "worker.agent_offline", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID)
		s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, "agent offline")
		return
	}

	if agent.Status == "stopped" {
		slog.Warn(orchFlowLog, "stage", "worker.agent_stopped", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID)
		s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, "agent stopped")
		return
	}

	// 权限校验
	if agent.UserID != nil && *agent.UserID != userID {
		slog.Warn(orchFlowLog, "stage", "worker.permission_denied", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID, "user_id", userID)
		s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, "permission denied")
		return
	}

	// 创建 OrchTaskCard 卡片 + 推送 WS 信号
	taskSummary := truncateString(task.Task, 80)
	taskHash := computeTaskHash(orchTaskID, task.AgentName, taskSummary)
	if s.taskSvc != nil && orchTaskID != "" {
		card := &model.OrchTaskCard{
			ConversationID: convID,
			OrchTaskID:     orchTaskID,
			SenderID:       orchSender.ID,
			SenderName:     orchSender.Name,
			SenderAvatar:   orchSender.Avatar,
			WorkerID:       agentID,
			WorkerName:     task.AgentName,
			WorkerAvatar:   agent.Avatar,
			TaskContent:    task.Task,
			TaskSummary:    taskSummary,
			Status:         "todo",
			Priority:       "medium",
			TaskHash:       taskHash,
			DispatchedAt:   time.Now(),
		}
		if _, err := s.taskSvc.CreateOrchWorkerTask(ctx, card); err != nil {
			slog.Warn(orchFlowLog, "stage", "worker.task_card_create_failed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "error", err)
		} else {
			slog.Info(orchFlowLog, "stage", "worker.task_card_created", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "task_hash", taskHash)
			s.pushTaskChanged(ctx, convID, userID)
		}
	}

	// 标记为执行中
	if s.taskSvc != nil && orchTaskID != "" {
		if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, taskHash, "in_progress", ""); err != nil {
			slog.Warn(orchFlowLog, "stage", "worker.task_card_progress_failed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "error", err)
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
	dispatchCtx = s.InjectAgentConfig(agent, dispatchCtx, userID, task.Task)

	// 统一路径：创建 daemon task → WS dispatch → channel wait → 创建消息
	msg, err := s.dispatchAndWait(ctx, convID, userID, agent, task.Task, dispatchCtx, replyTo)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "worker.dispatch_failed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID, "error", err)
		s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, err.Error())
		return
	}

	slog.Info(orchFlowLog, "stage", "worker.dispatch_completed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "worker_id", agentID, "message_id", msg.ID, "content_len", len(msg.Content))

	// 推送 worker 消息到用户 WS
	if msg != nil {
		slog.Info(orchFlowLog, "stage", "worker.message_push_scheduled", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "message_id", msg.ID)
		s.postPersistAsync(convID, userID, msg)
	}

	// 更新 OrchTask worker 状态
	if s.orchTaskRepo != nil && orchTaskID != "" {
		_, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, task.AgentName, "completed", truncateString(msg.Content, 2000))
		if err != nil {
			slog.Warn(orchFlowLog, "stage", "worker.result_update_failed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "error", err)
			return
		}
		slog.Info(orchFlowLog, "stage", "worker.result_updated", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName)

		// 同步 OrchTaskCard 状态为 done
		if s.taskSvc != nil {
			if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, taskHash, "done", truncateString(msg.Content, 4000)); err != nil {
				slog.Warn(orchFlowLog, "stage", "worker.task_card_done_failed", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "error", err)
			} else {
				s.pushTaskChanged(ctx, convID, userID)
			}
		}
	}
}

// markWorkerFailed 标记 worker 失败并检查是否全部完成。
func (s *OrchestratorService) markWorkerFailed(orchTaskID, workerName, taskDesc, reason string) {
	if s.orchTaskRepo == nil || orchTaskID == "" {
		slog.Warn(orchFlowLog, "stage", "worker.failed_without_lifecycle", "orch_task_id", orchTaskID, "worker_name", workerName, "reason", reason)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.orchTaskRepo.UpdateWorkerResult(ctx, orchTaskID, workerName, "failed", reason)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "worker.mark_failed_error", "orch_task_id", orchTaskID, "worker_name", workerName, "error", err)
		return
	}
	slog.Warn(orchFlowLog, "stage", "worker.marked_failed", "orch_task_id", orchTaskID, "worker_name", workerName, "reason", reason)

	// 同步 OrchTaskCard 状态为 failed
	if s.taskSvc != nil {
		taskHash := computeTaskHash(orchTaskID, workerName, truncateString(taskDesc, 80))
		if err := s.taskSvc.UpdateOrchWorkerStatus(ctx, taskHash, "failed", reason); err != nil {
			slog.Warn(orchFlowLog, "stage", "worker.task_card_failed_update_failed", "orch_task_id", orchTaskID, "worker_name", workerName, "error", err)
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
func (s *OrchestratorService) startWorkersAndWait(ctx context.Context, convID, userID string, plan WorkerDispatchPlan, orchAgentName, kbPreload, orchTaskID string, orchSender OrchSender, replyTo *string, summaryReplyTo *string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(orchFlowLog, "stage", "worker_fanout.panicked", "orch_task_id", orchTaskID, "panic", r)
			if s.orchTaskRepo != nil && orchTaskID != "" {
				failCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.orchTaskRepo.UpdateStatus(failCtx, orchTaskID, model.OrchTaskFailed)
			}
		}
	}()

	slog.Info(orchFlowLog, "stage", "worker_fanout.start", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_count", len(plan.Tasks), "unknown_count", len(plan.UnknownTasks), "workers", resolvedDispatchLogNames(plan.Tasks), "summary_enabled", orchTaskID != "")
	for _, unknown := range plan.UnknownTasks {
		slog.Warn(orchFlowLog, "stage", "worker_fanout.unknown_agent", "conversation_id", convID, "orch_task_id", orchTaskID, "agent", unknown.AgentName)
	}

	var wg sync.WaitGroup
	for _, t := range plan.Tasks {
		wg.Add(1)
		go func(task ResolvedDispatchTask) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error(orchFlowLog, "stage", "worker.panicked", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_name", task.AgentName, "panic", r)
					s.markWorkerFailed(orchTaskID, task.AgentName, task.Task, fmt.Sprintf("worker panic: %v", r))
				}
			}()
			s.dispatchOrchWorker(convID, userID, task.DispatchTask, task.AgentID, orchAgentName, kbPreload, orchTaskID, orchSender, replyTo)
		}(t)
	}

	if !plan.HasWorkers() {
		slog.Warn(orchFlowLog, "stage", "worker_fanout.empty", "conversation_id", convID, "orch_task_id", orchTaskID, "unknown_count", len(plan.UnknownTasks))
		if s.orchTaskRepo != nil && orchTaskID != "" {
			_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskCompleted)
		}
		return
	}

	wg.Wait()
	if s.orchTaskRepo == nil || orchTaskID == "" {
		slog.Warn(orchFlowLog, "stage", "worker_fanout.completed_without_lifecycle", "conversation_id", convID, "orch_task_id", orchTaskID, "summary_enabled", false)
		return
	}
	slog.Info(orchFlowLog, "stage", "worker_fanout.completed", "conversation_id", convID, "orch_task_id", orchTaskID, "summary_enabled", true)
	// 所有 worker 完整执行完毕（包括 postPersistAsync），现在可以安全地启动 summary
	s.goStartOrchSummary(orchTaskID, summaryReplyTo)
}

// pushTaskChanged 推送 task.changed WS 信号通知前端刷新任务列表。
func (s *OrchestratorService) pushTaskChanged(ctx context.Context, convID, userID string) {
	if s.notifier == nil {
		return
	}
	memberIDs := s.resolveMemberIDs(ctx, convID, userID)
	s.notifier.PushCustomEvent(convID, memberIDs, "task.changed", map[string]any{
		"conversation_id": convID,
	})
}

// goStartOrchSummary launches startOrchSummary in a goroutine with panic recovery.
func (s *OrchestratorService) goStartOrchSummary(orchTaskID string, replyTo *string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error(orchFlowLog, "stage", "summary.panicked", "orch_task_id", orchTaskID, "panic", r)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if s.orchTaskRepo != nil {
					_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
				}
				// 将关联的 OrchTaskCard 标记为 failed
				if s.taskSvc != nil {
					if err := s.taskSvc.FailAllTasksForOrchTask(ctx, orchTaskID); err != nil {
						slog.Warn(orchFlowLog, "stage", "summary.fail_cards_after_panic_failed", "orch_task_id", orchTaskID, "error", err)
					}
				}
			}
		}()
		slog.Info(orchFlowLog, "stage", "summary.scheduled", "orch_task_id", orchTaskID, "reply_to", stringValue(replyTo))
		s.startOrchSummary(orchTaskID, replyTo)
	}()
}

// startOrchSummary 执行 Orch 汇总阶段：收集所有 worker 结果，调 Orch 生成汇总。
func (s *OrchestratorService) startOrchSummary(orchTaskID string, replyTo *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Second)
	defer cancel()

	// CAS guard: only one goroutine can transition workers_running → summarizing
	if s.orchTaskRepo != nil {
		ok, err := s.orchTaskRepo.UpdateStatusCAS(ctx, orchTaskID, model.OrchTaskWorkersRunning, model.OrchTaskSummarizing)
		if err != nil {
			slog.Warn(orchFlowLog, "stage", "summary.cas_failed", "orch_task_id", orchTaskID, "from_status", model.OrchTaskWorkersRunning, "to_status", model.OrchTaskSummarizing, "error", err)
			return
		}
		if !ok {
			slog.Info(orchFlowLog, "stage", "summary.cas_skipped", "orch_task_id", orchTaskID, "from_status", model.OrchTaskWorkersRunning, "to_status", model.OrchTaskSummarizing)
			return
		}
	}

	orchTask, err := s.orchTaskRepo.GetByID(ctx, orchTaskID)
	if err != nil || orchTask == nil {
		slog.Warn(orchFlowLog, "stage", "summary.task_not_found", "orch_task_id", orchTaskID, "error", err)
		return
	}
	slog.Info(orchFlowLog, "stage", "summary.start", "conversation_id", orchTask.ConversationID, "orch_task_id", orchTaskID, "round", orchTask.Round, "reply_to", stringValue(replyTo))

	orchAgent, err := s.agentRepo.GetByID(ctx, orchTask.OrchAgentID)
	if err != nil || orchAgent == nil || orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		slog.Warn(orchFlowLog, "stage", "summary.orch_agent_unavailable", "orch_task_id", orchTaskID, "orch_agent_id", orchTask.OrchAgentID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// 构建汇总+决策 prompt（支持多轮上下文）
	summaryPrompt := BuildSummaryPrompt(orchTask)
	summaryCtx := s.InjectAgentConfig(orchAgent, "", orchTask.UserID, summaryPrompt)

	if replyTo == nil {
		replyTo = optionalStringPtr(orchTask.DispatchMessageID)
	}
	msg, err := s.dispatchAndWait(ctx, orchTask.ConversationID, orchTask.UserID, orchAgent, summaryPrompt, summaryCtx, replyTo)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "summary.dispatch_failed", "conversation_id", orchTask.ConversationID, "orch_task_id", orchTaskID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}

	// 保存 summary 并过渡到 evaluating 状态（决策点）
	_ = s.orchTaskRepo.SetSummaryAndEvaluate(ctx, orchTaskID, msg.Content)
	if msg != nil {
		_ = s.orchTaskRepo.UpdateDispatchMessageID(ctx, orchTaskID, msg.ID)
		orchTask.DispatchMessageID = msg.ID
	}
	slog.Info(orchFlowLog, "stage", "summary.message_created", "conversation_id", orchTask.ConversationID, "orch_task_id", orchTaskID, "message_id", msg.ID, "content_len", len(msg.Content), "result_preview", orchPreview(msg.Content))

	// 推送汇总/决策消息
	if msg != nil {
		s.postPersistAsync(orchTask.ConversationID, orchTask.UserID, msg)
	}

	// 评估 Orch 回复，决定继续派发还是完成
	s.evaluateOrchResponse(orchTaskID, orchTask, orchAgent.Name, msg.Content, msg.ID)
}

// evaluateOrchResponse parses the orchestrator's summary response for @mention dispatches.
// If @mentions are found and max rounds not reached, archives current round and dispatches new workers.
// Otherwise marks the task as completed.
func (s *OrchestratorService) evaluateOrchResponse(orchTaskID string, orchTask *model.OrchTask, orchestratorName, orchResponse, summaryMessageID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Resolve @mentions to agent IDs
	convAgents, err := s.convRepo.ListAgents(ctx, orchTask.ConversationID, orchTask.UserID)
	if err != nil {
		slog.Error(orchFlowLog, "stage", "summary.redispatch_list_agents_failed", "orch_task_id", orchTaskID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}
	agentNames := make([]string, 0, len(convAgents))
	for _, ca := range convAgents {
		agentNames = append(agentNames, ca.Name)
	}

	dispatch := ParseOrchestratorOutputForAgents(orchResponse, agentNames)
	if dispatch == nil {
		slog.Info(orchFlowLog, "stage", "summary.output_parsed", "orch_task_id", orchTaskID, "task_count", 0, "has_dispatch", false)
	} else {
		slog.Info(orchFlowLog, "stage", "summary.output_parsed", "orch_task_id", orchTaskID, "task_count", len(dispatch.Tasks), "has_dispatch", len(dispatch.Tasks) > 0, "agents", dispatchTaskLogNames(dispatch.Tasks))
	}

	// CAS guard: only proceed if status is still "evaluating"
	if s.orchTaskRepo != nil {
		ok, err := s.orchTaskRepo.UpdateStatusCAS(ctx, orchTaskID, model.OrchTaskEvaluating, model.OrchTaskWorkersRunning)
		if err != nil {
			slog.Warn(orchFlowLog, "stage", "summary.evaluate_cas_failed", "orch_task_id", orchTaskID, "from_status", model.OrchTaskEvaluating, "to_status", model.OrchTaskWorkersRunning, "error", err)
			return
		}
		if !ok {
			slog.Info(orchFlowLog, "stage", "summary.evaluate_cas_skipped", "orch_task_id", orchTaskID, "from_status", model.OrchTaskEvaluating, "to_status", model.OrchTaskWorkersRunning)
			return
		}
	}

	// No @mentions -> orchestrator is done (already set to completed by CAS above)
	if dispatch == nil || len(dispatch.Tasks) == 0 {
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskCompleted)
		slog.Info(orchFlowLog, "stage", "summary.completed_no_further_dispatch", "orch_task_id", orchTaskID, "round", orchTask.Round)
		return
	}

	// Max rounds reached -> already completed by CAS, stay completed
	if orchTask.Round >= model.MaxOrchRounds-1 {
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskCompleted)
		slog.Info(orchFlowLog, "stage", "summary.completed_max_rounds", "orch_task_id", orchTaskID, "round", orchTask.Round)
		return
	}

	// Transition evaluating -> workers_running via IncrementRound (archives current round, resets workers)
	if err := s.orchTaskRepo.IncrementRound(ctx, orchTaskID); err != nil {
		slog.Error(orchFlowLog, "stage", "summary.increment_round_failed", "orch_task_id", orchTaskID, "error", err)
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskFailed)
		return
	}
	_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskWorkersRunning)

	plan := BuildWorkerDispatchPlan(dispatch.Tasks, convAgents)

	// Zero valid workers -> force complete to avoid empty loop
	if !plan.HasWorkers() {
		slog.Warn(orchFlowLog, "stage", "summary.redispatch_empty_plan", "orch_task_id", orchTaskID, "task_count", len(dispatch.Tasks), "unknown_count", len(plan.UnknownTasks), "unknown_agents", dispatchTaskLogNames(plan.UnknownTasks))
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskID, model.OrchTaskCompleted)
		return
	}

	slog.Info(orchFlowLog, "stage", "summary.redispatch_scheduled", "conversation_id", orchTask.ConversationID, "orch_task_id", orchTaskID, "round", orchTask.Round+1, "worker_count", len(plan.Tasks), "unknown_count", len(plan.UnknownTasks), "workers", resolvedDispatchLogNames(plan.Tasks), "reply_to", summaryMessageID)
	// 解析 Orch agent 身份信息用于创建任务卡片
	orchAgent, _ := s.agentRepo.GetByID(ctx, orchTask.OrchAgentID)
	orchSender := OrchSender{ID: orchTask.OrchAgentID, Name: orchestratorName}
	if orchAgent != nil {
		orchSender.Avatar = orchAgent.Avatar
	}
	nextReplyTo := optionalStringPtr(summaryMessageID)
	go s.startWorkersAndWait(context.Background(), orchTask.ConversationID, orchTask.UserID, plan, orchestratorName, orchTask.KBPreload, orchTaskID, orchSender, nextReplyTo, nextReplyTo)
}

// postPersistAsync pushes a persisted async message and records transient
// delivery state. Message history recovery is always DB-backed.
func (s *OrchestratorService) postPersistAsync(convID, userID string, msg *model.Message) {
	if msg == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	memberIDs := s.resolveMemberIDs(ctx, convID, userID)
	slog.Info(orchFlowLog, "stage", "message.async_push", "conversation_id", convID, "user_id", userID, "message_id", msg.ID, "reply_to", stringValue(msg.ReplyTo), "member_count", len(memberIDs), "notifier_enabled", s.notifier != nil)

	if s.notifier != nil {
		s.notifier.PushToConversation(convID, memberIDs, msg)
	}

	if s.delivery == nil {
		return
	}
	for _, uid := range memberIDs {
		if uid == userID {
			continue
		}
		if s.notifier != nil && !s.notifier.IsOnline(uid) {
			if err := s.delivery.EnqueueOffline(ctx, uid, convID, msg); err != nil {
				slog.Warn("enqueue async offline failed", "user_id", uid, "conversation_id", convID, "error", err)
			}
		}
		if err := s.delivery.IncrementUnread(ctx, uid, convID); err != nil {
			slog.Warn("increment async unread failed", "user_id", uid, "conversation_id", convID, "error", err)
		}
	}
}
