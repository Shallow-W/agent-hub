package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// DaemonTaskCreator 是 Dispatcher 创建 daemon task 所需的 agent 仓库子集。
//
// P8a 后 OrchestratorService 持有 canonical repository.AgentStore；Dispatcher
// 仍保留此窄接口（仅 CreateDaemonTask 单方法），把「派发」与「查询」职责分离。
// repository.AgentStore 自动满足 DaemonTaskCreator（结构化接口）。
type DaemonTaskCreator interface {
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
}

// MessagePersister 是 Dispatcher 落库 assistant 消息所需的 msg 仓库子集。
//
// P8a 后 OrchestratorService 持有 canonical repository.MessageStore；Dispatcher
// 仍保留此窄接口（Create + SaveArtifacts 两方法），隔离 Dispatcher 的最小依赖面。
// repository.MessageStore 自动满足 MessagePersister。
type MessagePersister interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error
}

// DispatcherDeps 是 Dispatcher 的依赖集合。
//
// P7 之前 Dispatcher 通过 d.svc 反向依赖 OrchestratorService 来访问这些字段；
// P7 把它们显式提到 DispatcherDeps 中，解掉反向依赖，使得 Dispatcher 可独立测试与构造。
// DaemonHub 暂保持 concrete 类型（*ws.DaemonHub），端口抽象留给后续阶段。
type DispatcherDeps struct {
	AgentRepo    DaemonTaskCreator
	DaemonHub    *ws.DaemonHub
	MsgRepo      MessagePersister
	UploadDir    string // 用于 artifactsFromMarkdown fallback（目前未直接使用，预留）
}

// Dispatcher 封装「为单个 agent 创建 daemon task + WS dispatch + 等待结果 + 落库消息」
// 这一坨派发机制。它对应重构前 orchestrator.go 中的 dispatchWithHooks 私有方法。
//
// 历史路径：dispatchSingleAgent / dispatchOrchWorker / handleOrchestratedDispatch
// 各自直接调 s.dispatchAndWait。P6 把派发动作抽成独立类型并把各路径的业务
// 包装逻辑（权限校验 / OrchTaskCard 生命周期 / CAS guard / summary 落库）拆成 hook：
//   - PreDispatch：daemon task 创建前调用（权限 / CAS / 存在性校验）。返回 error 中止。
//   - OnTaskCreated：daemon task 创建成功后、WS 推送前调用（OrchTaskCard 起点）。
//   - OnMessagePersisted：assistant message 落库后调用（OrchTaskCard 完结 / summary 触发）。
//   - OnFailed：派发失败时调用（daemon 未连接 / fn error 等）。
//
// P7 进一步：Dispatcher 不再持有 *OrchestratorService 反向引用，所有依赖通过
// DispatcherDeps 注入；dispatchWithHooks 的核心实现迁到 Dispatcher.dispatchCore。
// OrchestratorService 持有独立构造的 *Dispatcher，并通过薄壳 s.dispatchAndWait
// （仅调用 s.dispatcher.Dispatch(..., DispatchHooks{})）保留两条内联路径的现有调用点。
//
// 注意：Dispatcher 不负责上下文构建（用 chain）也不负责并发护栏（用 AgentQueue）。
// 它只做「拿到 prompt + contextMessages 后把任务送到 daemon 并等待结果」这一件事。
// 并发护栏由调用方在调用 Dispatcher.Dispatch 前/后用 AgentQueue.Run 包裹。
type Dispatcher struct {
	deps DispatcherDeps
}

// NewDispatcher 构造一个独立 Dispatcher，依赖通过 deps 显式注入。
//
// P7 前：NewDispatcher(svc *OrchestratorService)，Dispatcher 反向依赖 svc。
// P7 后：调用方负责组装 DispatcherDeps；OrchestratorService 在构造时把自身字段
// 装入 DispatcherDeps 后传给 NewDispatcher。
func NewDispatcher(deps DispatcherDeps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

// DispatchInput 是 Dispatcher.Dispatch 的输入。
type DispatchInput struct {
	ConvID          string
	UserID          string
	Agent           *model.Agent
	Prompt          string  // 任务指令（daemon 发给 agent 的 prompt 字段）
	ContextMessages string  // 上下文（context_messages 字段）
	ReplyTo         *string // 关联的消息 ID（reply_to 字段）
}

// DispatchResult 是 Dispatcher.Dispatch 的输出。
type DispatchResult struct {
	Message *model.Message // 落库的 assistant 消息
}

// DispatchHooks 是 Dispatch 的可选业务包装回调。
// 所有字段都是可选（nil-safe）；零值 DispatchHooks（全 nil）等价于
// 直接调用 dispatchCore，不引入任何额外行为。
//
// 调用顺序（与重构前 dispatchWithHooks 内部时序对齐）：
//
//	PreDispatch(input)                       ← Dispatch 入口（创建 daemon task 之前）
//	  ↳ error → 直接 return，不调用后续 hook
//	CreateDaemonTask
//	OnTaskCreated(task)                      ← daemon task 创建成功后、WS 推送前
//	IsConnected / RegisterTaskPromise / SendToMachine / waitDaemonTask / CreateMessage
//	OnMessagePersisted(msg)                  ← assistant message 落库成功后
//	  ↳ 任一中间步骤 error → OnFailed(input, err)
type DispatchHooks struct {
	// PreDispatch 在 daemon task 创建前调用，用于权限校验、CAS guard、存在性检查。
	// 返回非 nil error 立即中止派发，并直接透传给 Dispatch 调用方。
	PreDispatch func(ctx context.Context, input DispatchInput) error

	// OnTaskCreated 在 daemon task 创建成功后、WS 推送前调用。
	// 用于 OrchTaskCard 生命周期起点（创建 todo 卡片）。
	OnTaskCreated func(ctx context.Context, task *model.DaemonTask)

	// OnMessagePersisted 在 assistant message 落库成功后调用。
	// 用于 OrchTaskCard 完结、summary 落库、postPersistAsync 等后续触发。
	OnMessagePersisted func(ctx context.Context, msg *model.Message)

	// OnFailed 在派发失败时调用（daemon 未连接 / WS 失败 / 等待失败 / fn error 等）。
	// 用于 OrchTaskCard 标记失败 / OrchTask 状态回退。
	OnFailed func(ctx context.Context, input DispatchInput, err error)
}

// Dispatch 把一个任务送到 daemon 并等待结果，落库为 assistant 消息后返回。
//
// hooks 零值（全 nil）时与原 dispatchAndWait 行为完全一致；非 nil 字段会在
// 对应时序点被调用（详见 DispatchHooks 文档）。每个字段调用前都做 nil 检查。
//
// 错误语义（与原 dispatchWithHooks 完全一致）：
//   - PreDispatch 返回 error → 直接透传，不进入派发流程
//   - daemon task 创建失败 → fmt.Errorf("create daemon task: %w", ...)
//   - daemon 未连接 → fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
//   - WS 发送失败 → fmt.Errorf("dispatch to daemon: %w", ...)
//   - 等待失败 / 超时 → 原错误透传
//   - task.Status == "failed" → fmt.Errorf("daemon task failed: %s", task.Error)
//   - 消息落库失败 → fmt.Errorf("create agent reply: %w", ...)
//
// 任一错误路径上 OnFailed 都会被调用一次（若非 nil），便于调用方统一处理失败。
func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput, hooks DispatchHooks) (*DispatchResult, error) {
	if hooks.PreDispatch != nil {
		if err := hooks.PreDispatch(ctx, in); err != nil {
			if hooks.OnFailed != nil {
				hooks.OnFailed(ctx, in, err)
			}
			return nil, err
		}
	}

	msg, task, err := d.dispatchCore(ctx, in, hooks.OnTaskCreated)
	if err != nil {
		if hooks.OnFailed != nil {
			hooks.OnFailed(ctx, in, err)
		}
		return nil, err
	}
	// task == nil 表示 dispatchCore 在 PreDispatch 之后未真正创建 task（理论上不会发生）
	_ = task
	if hooks.OnMessagePersisted != nil {
		hooks.OnMessagePersisted(ctx, msg)
	}
	return &DispatchResult{Message: msg}, nil
}

// dispatchCore 是 dispatchWithHooks 的核心实现迁移：CreateDaemonTask → onTaskCreated →
// IsConnected/RegisterTaskPromise/SendToMachine → waitDaemonTask → CreateMessage。
//
// 与原 OrchestratorService.dispatchWithHooks 时序完全一致（零行为变更）：
//
//	CreateDaemonTask
//	  ↳ err → return ("create daemon task")
//	onTaskCreated(task)               ← 若回调非 nil
//	IsConnected / RegisterTaskPromise / SendToMachine / waitDaemonTask / CreateMessage
//
// onTaskCreated 为 nil 时与原 dispatchWithHooks 行为完全等价。
func (d *Dispatcher) dispatchCore(ctx context.Context, in DispatchInput, onTaskCreated func(context.Context, *model.DaemonTask)) (*model.Message, *model.DaemonTask, error) {
	convID, userID := in.ConvID, in.UserID
	agent, prompt, contextMessages, replyTo := in.Agent, in.Prompt, in.ContextMessages, in.ReplyTo

	task, err := d.deps.AgentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, prompt, contextMessages)
	if err != nil {
		return nil, nil, fmt.Errorf("create daemon task: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_task_created", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID, "cli_tool", agent.CLITool, "prompt_len", len(prompt), "context_len", len(contextMessages), "reply_to", stringValue(replyTo), "prompt_preview", orchPreview(prompt))

	// OrchTaskCard 起点回调：拿到 daemon task ID 后立即登记（WS 推送前）。
	if onTaskCreated != nil {
		onTaskCreated(ctx, task)
	}

	if d.deps.DaemonHub == nil || !d.deps.DaemonHub.IsConnected(*agent.MachineID) {
		slog.Warn(orchFlowLog, "stage", "agent.daemon_not_connected", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID)
		return nil, task, fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
	}
	d.deps.DaemonHub.RegisterTaskPromise(task.ID)
	if err := d.deps.DaemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":          task.ID,
			"cli_tool":         agent.CLITool,
			"prompt":           prompt,
			"context_messages": contextMessages,
			"agent_id":         agent.ID,
			"conversation_id":  convID,
			"user_id":          userID,
		},
	}); err != nil {
		return nil, task, fmt.Errorf("dispatch to daemon: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_sent", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID)

	task, err = d.waitDaemonTask(ctx, task.ID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "agent.dispatch_wait_failed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "error", err)
		return nil, task, err
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_completed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "status", task.Status, "result_len", len(task.Result), "artifact_count", len(task.Artifacts), "result_preview", orchPreview(task.Result))
	if task.Status == "failed" {
		return nil, task, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts := agentMetadata(agent)
	msg, err := d.deps.MsgRepo.Create(ctx, convID, "assistant", task.Result, artifacts, nil, replyTo, nil, nil)
	if err != nil {
		return nil, task, fmt.Errorf("create agent reply: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.message_created", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "message_id", msg.ID, "reply_to", stringValue(replyTo), "content_len", len(msg.Content))
	d.persistArtifacts(ctx, msg, task.Artifacts)
	return msg, task, nil
}

// waitDaemonTask 通过 DaemonHub 等待 daemon task 完成（channel-based 通知）。
//
// 迁移自 OrchestratorService.waitDaemonTask（零行为变更）：
//   - DaemonHub 为 nil → fmt.Errorf("daemon hub not available")
//   - AwaitTaskResult 返回 nil → fmt.Errorf("daemon not connected for task %s", taskID)
//   - 400s 超时 → ErrMsgAgentTimeout
func (d *Dispatcher) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	if d.deps.DaemonHub == nil {
		return nil, fmt.Errorf("daemon hub not available")
	}

	ch := d.deps.DaemonHub.AwaitTaskResult(taskID)
	if ch == nil {
		return nil, fmt.Errorf("daemon not connected for task %s", taskID)
	}
	defer d.deps.DaemonHub.RemoveTaskPromise(taskID)

	ctx, cancel := context.WithTimeout(ctx, 400*time.Second)
	defer cancel()

	select {
	case result := <-ch:
		task := &model.DaemonTask{
			ID:        result.TaskID,
			Status:    "completed",
			Result:    result.Result,
			Artifacts: artifactsFromTaskResult(result.Artifacts),
		}
		if result.Error != "" {
			task.Status = "failed"
			task.Error = result.Error
		}
		return task, nil
	case <-ctx.Done():
		return nil, ErrMsgAgentTimeout
	}
}

// persistArtifacts 把 daemon 返回的 artifacts（或从 markdown 抽取）落库并回填 msg.Artifacts。
//
// 迁移自 OrchestratorService.persistArtifacts（零行为变更）。
func (d *Dispatcher) persistArtifacts(ctx context.Context, msg *model.Message, artifacts []model.Artifact) {
	if msg == nil {
		return
	}
	if len(artifacts) == 0 {
		artifacts = artifactsFromMarkdown(msg.Content)
	} else if !hasCodeArtifact(artifacts) {
		artifacts = append(artifacts, codeArtifactsFromMarkdown(msg.Content)...)
	}
	if len(artifacts) == 0 {
		return
	}
	if err := d.deps.MsgRepo.SaveArtifacts(ctx, msg.ID, artifacts); err != nil {
		slog.Warn("save orchestrator artifacts failed", "message_id", msg.ID, "error", err)
		return
	}
	msg.Artifacts = artifacts
}

// DispatchMany 把同一份输入并发派发给多个 agent（fan-out）。
// 当前实现是简单的并发循环；调用方若需要串行化同一 agent 的派发，
// 应在调用前用 AgentQueue.Run 包裹。
//
// 返回的 results 与 agents 一一对应；任一 agent 派发失败时对应位置的 error 非 nil，
// 但不会中断其他 agent 的派发（fan-out 语义）。hooks 对每个 input 都生效。
func (d *Dispatcher) DispatchMany(ctx context.Context, inputs []DispatchInput, hooks DispatchHooks) ([]*DispatchResult, []error) {
	results := make([]*DispatchResult, len(inputs))
	errs := make([]error, len(inputs))
	if len(inputs) == 0 {
		return results, errs
	}

	// 并发 fan-out：每个 agent 独立 goroutine
	type outcome struct {
		idx int
		res *DispatchResult
		err error
	}
	ch := make(chan outcome, len(inputs))
	for i, in := range inputs {
		go func(idx int, din DispatchInput) {
			res, err := d.Dispatch(ctx, din, hooks)
			ch <- outcome{idx: idx, res: res, err: err}
		}(i, in)
	}

	for i := 0; i < len(inputs); i++ {
		o := <-ch
		results[o.idx] = o.res
		errs[o.idx] = o.err
	}
	return results, errs
}

// FormatDispatchError 统一格式化派发错误，便于上层 slog。
// 当前仅 fmt.Errorf 包装，保留作为未来统一错误码的扩展点。
func FormatDispatchError(agentName string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("dispatch to %q failed: %w", agentName, err)
}
