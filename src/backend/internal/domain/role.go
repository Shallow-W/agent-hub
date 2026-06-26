package domain

// Role 表示会话中 Agent 的角色（持久化在 conversation_agents.role 列）。
//
// 注意：与 service/tool_config.go、daemon/mcp/handlers.go 中的 "orchestrator"
// 模板名不同，本处的 Role 仅供会话内 @mention 编排使用。
type Role string

const (
	// RoleOrchestrator 群聊编排者，负责拆解任务并分发给其他 Agent。
	RoleOrchestrator Role = "orchestrator"
	// RoleWorker 群聊执行者，接收 Orchestrator 分发的子任务。
	RoleWorker Role = "worker"
	// RoleObserver 预留：只读参与，本次不使用但保留扩展。
	RoleObserver Role = "observer"
	// RoleRobot 数据库 conversation_agents.role 的默认值，
	// 表示该 Agent 尚未在群聊中被显式赋予 Orchestrator/Worker 角色。
	RoleRobot Role = "robot"
)

// Valid 判断角色值是否合法。
func (r Role) Valid() bool {
	switch r {
	case RoleOrchestrator, RoleWorker, RoleObserver, RoleRobot:
		return true
	}
	return false
}

// String 方法方便日志和 error 拼接。
func (r Role) String() string { return string(r) }

// IsAssignableRole 判断该角色是否可通过 API 赋值给 conversation agent。
// Observer、Robot 等是预留或默认值，当前 API 不允许直接设置。
func IsAssignableRole(r Role) bool {
	return r == RoleOrchestrator || r == RoleWorker
}

// HasOrchestratorRole 判断给定 role 字符串是否为 orchestrator。
// 给 repository / service 层在不 import model 的情况下使用。
func HasOrchestratorRole(role string) bool {
	return role == string(RoleOrchestrator)
}

// HasWorkerRole 判断给定 role 字符串是否为 worker。
func HasWorkerRole(role string) bool {
	return role == string(RoleWorker)
}
