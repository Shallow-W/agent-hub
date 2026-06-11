package service

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// Dispatcher 封装「为单个 agent 创建 daemon task + WS dispatch + 等待结果 + 落库消息」
// 这一坨派发机制。它对应 orchestrator.go 中的 dispatchAndWait 私有方法。
//
// 历史路径：dispatchSingleAgent / dispatchOrchWorker / handleOrchestratedDispatch
// 各自直接调 s.dispatchAndWait。P6 把派发动作抽成独立类型并把各路径的业务
// 包装逻辑（权限校验 / OrchTaskCard 生命周期 / CAS guard / summary 落库）拆成 hook：
//   - PreDispatch：daemon task 创建前调用（权限 / CAS / 存在性校验）。返回 error 中止。
//   - OnTaskCreated：daemon task 创建成功后、WS 推送前调用（OrchTaskCard 起点）。
//   - OnMessagePersisted：assistant message 落库后调用（OrchTaskCard 完结 / summary 触发）。
//   - OnFailed：派发失败时调用（daemon 未连接 / fn error 等）。
//
// 注意：Dispatcher 不负责上下文构建（用 chain）也不负责并发护栏（用 AgentQueue）。
// 它只做「拿到 prompt + contextMessages 后把任务送到 daemon 并等待结果」这一件事。
// 并发护栏由调用方在调用 Dispatcher.Dispatch 前/后用 AgentQueue.Run 包裹。
type Dispatcher struct {
	svc *OrchestratorService
}

// NewDispatcher 构造绑定到 svc 的 Dispatcher。
// svc 用于访问 dispatchAndWait 依赖的 agentRepo / daemonHub / msgRepo。
func NewDispatcher(svc *OrchestratorService) *Dispatcher {
	return &Dispatcher{svc: svc}
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
// 直接调用 dispatchAndWait，不引入任何额外行为。
//
// 调用顺序（与 dispatchAndWait 内部时序对齐）：
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
// 错误语义（与原 dispatchAndWait 完全一致）：
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

	msg, task, err := d.svc.dispatchWithHooks(ctx, in, hooks.OnTaskCreated)
	if err != nil {
		if hooks.OnFailed != nil {
			hooks.OnFailed(ctx, in, err)
		}
		return nil, err
	}
	// task == nil 表示 dispatchWithHooks 在 PreDispatch 之后未真正创建 task（理论上不会发生）
	_ = task
	if hooks.OnMessagePersisted != nil {
		hooks.OnMessagePersisted(ctx, msg)
	}
	return &DispatchResult{Message: msg}, nil
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
