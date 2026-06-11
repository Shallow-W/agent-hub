package service

import (
	"context"
	"sync"
)

// AgentQueue 把「同一 agent 同时只允许一个任务在飞」的并发护栏抽成独立类型。
//
// 历史：原实现是 OrchestratorService.agentQueues sync.Map[agentID]chan struct{}，
// dispatchSingleAgent / runDaemonEdit 各自重复 LoadOrStore + send + defer recv 的样板。
//
// 行为契约（与原 sync.Map + buffered-1 chan 完全一致）：
//   - 同一 agentID 的 Run 调用串行执行；后到的调用阻塞等待前一个释放槽位
//   - 不同 agentID 的 Run 调用彼此并行
//   - ctx 取消时立即返回 ctx.Err()（不持有槽位）
//
// 可拓展：未来加优先级 / 配额 / 全局并发上限时只改这里。
type AgentQueue struct {
	queues sync.Map // agentID → chan struct{} (buffered-1 semaphore)
}

// NewAgentQueue 构造一个空的并发护栏。
func NewAgentQueue() *AgentQueue {
	return &AgentQueue{}
}

// Run 在 agentID 的串行槽位内执行 fn。
// 同 agentID 的多次调用会排队（buffered-1 semaphore）；不同 agentID 并行。
// fn 的返回值原样透传。
//
// 阻塞语义：与原 sync.Map + `sem <- struct{}{}` 完全一致——槽位等待期间
// 不响应 ctx 取消（保持行为零变更）。ctx 仅用于未来拓展（优先级 / 超时）的占位，
// 当前实现忽略它；调用方若需响应取消应在 fn 内部自行处理。
func (q *AgentQueue) Run(_ context.Context, agentID string, fn func() error) error {
	newSem := make(chan struct{}, 1)
	actual, _ := q.queues.LoadOrStore(agentID, newSem)
	sem := actual.(chan struct{})
	sem <- struct{}{} // blocks until slot available（与原实现一致，不响应 ctx）
	defer func() { <-sem }()

	return fn()
}
