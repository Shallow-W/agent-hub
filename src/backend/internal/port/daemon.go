package port

import (
	"github.com/agent-hub/backend/pkg/ws"
)

// Compile-time assertion that the concrete infra type *ws.DaemonHub satisfies
// DaemonDispatcher via Go structural typing. If any DaemonHub method signature
// drifts away from the interface contract, this build fails with a clear error
// at the assertion site rather than a cryptic error at a call site.
//
// Note: the corresponding TokenIssuerPort assertion lives in package service
// (next to *TokenIssuer) to avoid an import cycle (service -> port -> service).
var _ DaemonDispatcher = (*ws.DaemonHub)(nil)

// DaemonDispatcher is the port interface for dispatching tasks to daemon agents
// via WebSocket. It abstracts *ws.DaemonHub so the service layer never imports
// concrete infrastructure types (pkg/ws).
//
// *ws.DaemonHub satisfies this interface via structured typing (all methods have
// matching signatures), so no adapter is needed.
type DaemonDispatcher interface {
	// IsConnected reports whether a daemon with the given machineID is
	// currently connected via WebSocket.
	IsConnected(machineID string) bool

	// RegisterTaskPromise creates a buffered result channel for taskID and
	// stores it so the daemon can deliver results via ResolveTask.
	// Returns the channel so callers that need to race with the daemon
	// (e.g. agent_skill_open) can also use it directly.
	RegisterTaskPromise(taskID string) chan *ws.TaskResult

	// SendToMachine sends a WebSocket message to the daemon identified by
	// machineID. Returns an error if no daemon with that machineID is
	// connected.
	SendToMachine(machineID string, msg ws.WSMessage) error

	// AwaitTaskResult retrieves the result channel for taskID previously
	// created by RegisterTaskPromise. Returns nil if no promise was
	// registered for this taskID.
	AwaitTaskResult(taskID string) chan *ws.TaskResult

	// RemoveTaskPromise removes the result channel for taskID, typically
	// called after the task result has been consumed or on timeout.
	RemoveTaskPromise(taskID string)

	// RegisterTaskMessage stores a taskID → messageID mapping so
	// handleTaskProgress can resolve the streaming message when the
	// daemon omits the message_id field in task.progress messages.
	RegisterTaskMessage(taskID, messageID string)

	// DeleteTaskMessage clears the taskID → messageID mapping after
	// the streaming message is finalized. PR5: prevents the sync.Map
	// from growing without bound across long-running backends.
	// 对称于 RegisterTaskMessage；createAgentReply 在所有终态路径 defer 调用。
	DeleteTaskMessage(taskID string)
}
