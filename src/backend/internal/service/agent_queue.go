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
// 行为契约：
//   - 同一 agentID 的 Run 调用串行执行；后到的调用阻塞等待前一个释放槽位
//   - 不同 agentID 的 Run 调用彼此并行
//   - ctx 取消时立即返回 ctx.Err()（既未占用槽位，也未执行 fn）
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
// 取消语义：ctx 取消时立即返回 ctx.Err()（不持有槽位、不执行 fn）。
// 槽位已被前一个调用占用时，等待 ctx 取消或槽位释放二者先发生。
func (q *AgentQueue) Run(ctx context.Context, agentID string, fn func() error) error {
	newSem := make(chan struct{}, 1)
	actual, _ := q.queues.LoadOrStore(agentID, newSem)
	sem := actual.(chan struct{})

	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
