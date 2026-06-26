package service

import (
	"sync"

	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/pkg/ws"
)

// fakeDaemonDispatcher 是 port.DaemonDispatcher 接口的测试替身。
//
// P8b 之前：Dispatcher / AgentService / MessageService 都直接持有 *ws.DaemonHub
// 具体类型，测试要么装配真实的 DaemonHub + RegisterTestClient（重），要么根本
// 无法注入自定义行为。
//
// P8b 之后：所有 service 层都依赖 port.DaemonDispatcher 接口。本 fake 让测试
// 可以用 closure 精确控制每个方法的返回值，并记录调用次数 / 参数，用于断言
// 「port 接口本身是可替换的契约」这一关键不变量。
//
// 设计要点：
//   - 5 个方法（IsConnected / RegisterTaskPromise / SendToMachine / AwaitTaskResult /
//     RemoveTaskPromise）各对应一个 closure 字段，nil 时返回零值
//   - calls 子结构记录每个方法的调用计数与最后一次入参，方便测试断言
//   - 互斥保护 calls，便于并发 fan-out 场景（DispatchMany）安全使用
//
// 验证契约：port.DaemonDispatcher 接口可以被任意实现替换，service / dispatcher
// 不依赖 *ws.DaemonHub 的具体实现细节。
type fakeDaemonDispatcher struct {
	// 每个方法的 closure：nil 时返回方法签名对应的零值。
	isConnected       func(machineID string) bool
	registerTaskPromise func(taskID string) chan *ws.TaskResult
	sendToMachine     func(machineID string, msg ws.WSMessage) error
	awaitTaskResult   func(taskID string) chan *ws.TaskResult
	removeTaskPromise func(taskID string)

	// calls 记录调用计数与最后一次入参（用于断言）。
	// 互斥保护，DispatchMany 等并发场景可安全使用。
	mu   sync.Mutex
	calls fakeDaemonDispatcherCalls
}

// fakeDaemonDispatcherCalls 是 fakeDaemonDispatcher 的调用计数与最近一次入参快照。
type fakeDaemonDispatcherCalls struct {
	IsConnected       int
	LastIsConnectedID string

	RegisterTaskPromise       int
	LastRegisterTaskPromiseID string

	SendToMachine       int
	LastSendMachineID   string
	LastSendMsg         ws.WSMessage

	AwaitTaskResult       int
	LastAwaitTaskResultID string

	RemoveTaskPromise       int
	LastRemoveTaskPromiseID string
}

// Calls 返回当前调用计数的快照（拷贝，调用方可自由断言）。
func (f *fakeDaemonDispatcher) Calls() fakeDaemonDispatcherCalls {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// IsConnected 报告给定 machineID 的 daemon 是否在线。
// 未注入 closure 时返回 false（与未连接语义一致）。
func (f *fakeDaemonDispatcher) IsConnected(machineID string) bool {
	f.mu.Lock()
	f.calls.IsConnected++
	f.calls.LastIsConnectedID = machineID
	f.mu.Unlock()
	if f.isConnected != nil {
		return f.isConnected(machineID)
	}
	return false
}

// RegisterTaskPromise 为 taskID 创建并返回一个结果 channel。
// 未注入 closure 时返回 nil（与「没有真实 promise 注册」语义一致）。
func (f *fakeDaemonDispatcher) RegisterTaskPromise(taskID string) chan *ws.TaskResult {
	f.mu.Lock()
	f.calls.RegisterTaskPromise++
	f.calls.LastRegisterTaskPromiseID = taskID
	f.mu.Unlock()
	if f.registerTaskPromise != nil {
		return f.registerTaskPromise(taskID)
	}
	return nil
}

// SendToMachine 向 machineID 对应的 daemon 发送 WS 消息。
// 未注入 closure 时返回 nil（发送成功）。
func (f *fakeDaemonDispatcher) SendToMachine(machineID string, msg ws.WSMessage) error {
	f.mu.Lock()
	f.calls.SendToMachine++
	f.calls.LastSendMachineID = machineID
	f.calls.LastSendMsg = msg
	f.mu.Unlock()
	if f.sendToMachine != nil {
		return f.sendToMachine(machineID, msg)
	}
	return nil
}

// AwaitTaskResult 返回 taskID 之前注册的结果 channel。
// 未注入 closure 时返回 nil。
func (f *fakeDaemonDispatcher) AwaitTaskResult(taskID string) chan *ws.TaskResult {
	f.mu.Lock()
	f.calls.AwaitTaskResult++
	f.calls.LastAwaitTaskResultID = taskID
	f.mu.Unlock()
	if f.awaitTaskResult != nil {
		return f.awaitTaskResult(taskID)
	}
	return nil
}

// RemoveTaskPromise 清理 taskID 对应的结果 channel。
func (f *fakeDaemonDispatcher) RemoveTaskPromise(taskID string) {
	f.mu.Lock()
	f.calls.RemoveTaskPromise++
	f.calls.LastRemoveTaskPromiseID = taskID
	f.mu.Unlock()
	if f.removeTaskPromise != nil {
		f.removeTaskPromise(taskID)
	}
}

// RegisterTaskMessage 存储 taskID → messageID 映射（接口要求，测试无需断言）。
func (f *fakeDaemonDispatcher) RegisterTaskMessage(taskID, messageID string) {
	// no-op：测试不依赖此映射的调用记录；service.message.go createAgentReply 会调用，
	// 满足 port.DaemonDispatcher 接口契约即可。
	_ = taskID
	_ = messageID
}

// DeleteTaskMessage 清理 taskID → messageID 映射（PR5：修复内存泄漏）。
func (f *fakeDaemonDispatcher) DeleteTaskMessage(taskID string) {
	_ = taskID
}

// 编译期保证 fakeDaemonDispatcher 满足 port.DaemonDispatcher 接口。
// 这是 P8b port 抽象的核心契约断言：任何 port.DaemonDispatcher 的注入点
// （Dispatcher / AgentService / MessageService）都可以用本 fake 替换 *ws.DaemonHub。
var _ port.DaemonDispatcher = (*fakeDaemonDispatcher)(nil)
