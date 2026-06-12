package service

import (
	"context"
	"log/slog"

	"github.com/agent-hub/backend/internal/model"
)

// DispatchRole 标识一个派发目标走哪条派发路径。
type DispatchRole string

const (
	// DispatchRoleWorker 表示走 dispatchSingleAgent / FanoutChain 路径。
	DispatchRoleWorker DispatchRole = "worker"
	// DispatchRoleOrchestrator 表示走 handleOrchestratedDispatch / OrchChain 路径。
	DispatchRoleOrchestrator DispatchRole = "orchestrator"
)

// DispatchTarget 是 Router 解析出的单个派发目标。
//
// 字段语义：
//   - Agent：已解析的 agent 实体（路由阶段已 GetByID）
//   - MentionName：原始 @mention 文本中的名字（保留用于日志 / DispatchInfo 回填）
//   - Task：该 agent 的任务描述（worker 取 mention 之间的文本；orch 取整条消息）
//   - Role：派发角色，决定走 worker 还是 orchestrator 路径
type DispatchTarget struct {
	Agent       *model.Agent
	MentionName string
	Task        string
	Role        DispatchRole
}

// RouterInput 是 Router.Resolve 的输入。
type RouterInput struct {
	Content    string                    // 用户原始消息文本
	ConvAgents []model.ConversationAgent // 当前群聊 agent 列表（含 role）
	Mentions   []MentionResult           // 已解析的 @mention
	MentionMap map[string]string         // mention name → agentID（由 FindMentionedAgentID 产出）
	AgentRepo  func(ctx context.Context, id string) (*model.Agent, error)
}

// Router 把「消息 + 群聊 agent 列表 + @mention」解析成 []DispatchTarget。
//
// 历史路径：RouteMention 内联做了三件事——解析 mention、查 agent、判定 orch/worker 角色。
// P5b 把判定逻辑抽成独立类型；P7 进一步把具体实现 defaultRouter 与 interface 解耦，
// 便于未来拓展广播 / 轮询 / 负载均衡策略时只注入自定义 Router 实现（零行为变更）。
//
// 行为契约（与 RouteMention 原内联循环完全一致）：
//   - 遍历 mentions（不是 convAgents），保留 mention 在文本中的出现顺序
//   - mentionMap 命中失败 → 跳过
//   - GetByID 失败或返回 nil → 跳过
//   - 角色：ConversationAgent.IsOrchestrator() 为 true → orch；否则 worker
//   - Task：worker 取 mention.Task；orch 取整条 content
type Router interface {
	Resolve(ctx context.Context, in RouterInput) []DispatchTarget
}

// defaultRouter 是 Router interface 的默认实现。
//
// 历史上 Router 是一个 struct（type Router struct{}）；P7 将其改为 interface，
// 旧 struct 改名 defaultRouter 并继续承载原 Resolve 实现，保证零行为变更。
type defaultRouter struct{}

// NewDefaultRouter 构造默认 Router 实现。
func NewDefaultRouter() Router { return defaultRouter{} }

// NewRouter 是 NewDefaultRouter 的兼容别名，便于已有调用点零迁移。
//
// Deprecated: 优先使用 NewDefaultRouter。
func NewRouter() Router { return NewDefaultRouter() }

// Resolve 把 input 解析为有序的 []DispatchTarget。
// ctx 透传给 AgentRepo 查询；查询失败的 agent 会被跳过（与原 RouteMention 行为一致）。
func (defaultRouter) Resolve(ctx context.Context, in RouterInput) []DispatchTarget {
	targets := make([]DispatchTarget, 0, len(in.Mentions))
	for _, m := range in.Mentions {
		agentID, ok := in.MentionMap[m.AgentName]
		if !ok {
			continue
		}

		agent, err := in.AgentRepo(ctx, agentID)
		if err != nil {
			slog.Warn("orch get agent failed", "agent_id", agentID, "error", err)
			continue
		}
		if agent == nil {
			continue
		}

		ca, _ := model.ConversationAgents(in.ConvAgents).FindByAgentID(agentID)
		role := DispatchRoleWorker
		if ca != nil && ca.IsOrchestrator() {
			role = DispatchRoleOrchestrator
		}

		task := m.Task
		if role == DispatchRoleOrchestrator {
			task = in.Content
		}
		targets = append(targets, DispatchTarget{
			Agent:       agent,
			MentionName: m.AgentName,
			Task:        task,
			Role:        role,
		})
	}
	return targets
}
