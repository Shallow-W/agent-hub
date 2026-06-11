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
// 各自直接调 s.dispatchAndWait。P5b 把派发动作抽成独立类型，便于未来：
//   - 换 daemon 协议（直接 HTTP 而非 task 表）只改这里
//   - 加重试 / 熔断 / 超时策略只改这里
//   - 加派发指标 / tracing 只改这里
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

// Dispatch 把一个任务送到 daemon 并等待结果，落库为 assistant 消息后返回。
// 错误语义与原 dispatchAndWait 完全一致：
//   - daemon task 创建失败 → fmt.Errorf("create daemon task: %w", ...)
//   - daemon 未连接 → fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
//   - WS 发送失败 → fmt.Errorf("dispatch to daemon: %w", ...)
//   - 等待失败 / 超时 → 原错误透传
//   - task.Status == "failed" → fmt.Errorf("daemon task failed: %s", task.Error)
//   - 消息落库失败 → fmt.Errorf("create agent reply: %w", ...)
func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput) (*DispatchResult, error) {
	msg, err := d.svc.dispatchAndWait(ctx, in.ConvID, in.UserID, in.Agent, in.Prompt, in.ContextMessages, in.ReplyTo)
	if err != nil {
		return nil, err
	}
	return &DispatchResult{Message: msg}, nil
}

// DispatchMany 把同一份输入并发派发给多个 agent（fan-out）。
// 当前实现是简单的并发循环；调用方若需要串行化同一 agent 的派发，
// 应在调用前用 AgentQueue.Run 包裹。
//
// 返回的 results 与 agents 一一对应；任一 agent 派发失败时对应位置的 error 非 nil，
// 但不会中断其他 agent 的派发（fan-out 语义）。
func (d *Dispatcher) DispatchMany(ctx context.Context, inputs []DispatchInput) ([]*DispatchResult, []error) {
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
			res, err := d.Dispatch(ctx, din)
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
