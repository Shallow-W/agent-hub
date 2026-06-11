package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// OrchConvRepo queries conversation agents for @mention resolution.
// Deprecated: migrate to repository.ConvStore for canonical interface.
type OrchConvRepo interface {
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
}

// OrchAgentRepo queries agent details and creates daemon tasks.
// Deprecated: migrate to repository.AgentStore for canonical interface.
type OrchAgentRepo interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	SetDaemonTaskOrch(ctx context.Context, taskID, orchTaskID, workerName string)
	CompleteDaemonTask(ctx context.Context, id, machineID, result, taskError string) (bool, error)
}

// OrchTaskStore 定义编排任务的 DB 操作。
type OrchTaskStore interface {
	Create(ctx context.Context, task *model.OrchTask) error
	GetByID(ctx context.Context, id string) (*model.OrchTask, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateDispatchMessageID(ctx context.Context, id, messageID string) error
	UpdateStatusCAS(ctx context.Context, id, fromStatus, toStatus string) (bool, error)
	UpdateWorkerResult(ctx context.Context, id, workerName, status, result string) (bool, error)
	SetSummaryAndEvaluate(ctx context.Context, id, summary string) error
	IncrementRound(ctx context.Context, id string) error
}

// RouteResult is returned by RouteMention containing agent reply messages and dispatch info.
type RouteResult struct {
	AgentMessages []*model.Message
	Dispatches    []DispatchInfo
}

// DispatchInfo records what was dispatched for logging/debugging.
type DispatchInfo struct {
	AgentID   string
	AgentName string
	Task      string
	Parallel  bool
}

// OrchKBResolver resolves knowledge base references from message text.
type OrchKBResolver interface {
	ResolveKnowledgeRef(ctx context.Context, currentUserID, username, kbName string) (*model.KnowledgeBase, []model.KnowledgeFile, error)
}

// OrchArtifactRepo 产物访问能力，用于 AI 编辑产物（取最新版本、回溯对话、创建新版本）。
type OrchArtifactRepo interface {
	GetConversationIDByRoot(ctx context.Context, rootID string) (string, error)
	GetLatestByRoot(ctx context.Context, rootID string) (*model.Artifact, error)
	CreateVersion(ctx context.Context, rootID string, in model.Artifact) (*model.Artifact, error)
}

// OrchDeliveryState stores transient delivery state for async orchestrator messages.
// Message rows are already persisted; Redis is only used for offline delivery and unread counts.
type OrchDeliveryState interface {
	EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error
	IncrementUnread(ctx context.Context, userID, conversationID string) error
}

// OrchestratorService handles @mention routing and orchestrated multi-agent dispatch.
type OrchestratorService struct {
	convRepo     OrchConvRepo
	agentRepo    OrchAgentRepo
	msgRepo      MsgRepo
	orchTaskRepo OrchTaskStore

	tokenIssuer *TokenIssuer
	serverURL   string
	uploadDir   string // 上传文件落盘根目录，用于服务端抽取附件文本

	kbResolver   OrchKBResolver
	daemonHub    *ws.DaemonHub
	artifactRepo OrchArtifactRepo
	notifier     MessageNotifier
	delivery     OrchDeliveryState
	taskSvc      TaskBoardSync

	// 三条上下文构建链：分别对应单聊直接回复、worker 派发、orch 派发三条路径。
	// 详细顺序见 NewOrchestratorServiceWithDeps。
	directReplyChain *ContextChain // 路径 A：message.asyncAgentReply
	workerChain      *ContextChain // 路径 C：dispatchSingleAgent
	orchChain        *ContextChain // 路径 D：handleOrchestratedDispatch

	// fanoutChain 对应路径 C 的异步 fanout 变体（startWorkersAndWait → dispatchOrchWorker）。
	// 与 workerChain 共享 KB / AgentConfig，但额外前置一个「群聊背景 + 调度指令」框架段。
	// 输出顺序：agentConfig + kbPreload + frame（与重构前内联拼装完全一致）。
	fanoutChain *ContextChain

	// summaryChain 对应路径 D 的 summary 阶段（startOrchSummary）。
	// 仅一个 AgentConfig builder：输出 = agentConfig + ""。
	// 不叠加 OrchestratorSystemPrompt（summary prompt 已自带 OrchestratorSummarySystemPrompt）。
	summaryChain *ContextChain

	// 派发并发保护：同一 agent 同时只允许一个任务在飞，其他请求排队等待
	// 历史实现是 agentQueues sync.Map[agentID]chan struct{}，P5 抽成 AgentQueue 类型。
	agentQueue *AgentQueue

	// router 把「消息 + 群聊 agent + @mention」解析成 []DispatchTarget。
	// 历史路径：RouteMention 内联循环做解析；P5b 抽成独立类型便于未来拓展广播/轮询策略。
	router *Router

	// dispatcher 封装 dispatchAndWait 的派发动作（task 创建 + WS + 等待 + 落库）。
	// 当前作为可注入扩展点存在；现有调用方仍直接调 dispatchAndWait 以保持零行为变更，
	// 后续清理（P6+）会把 dispatchSingleAgent / fanout / summary 迁移到走 Dispatcher。
	dispatcher *Dispatcher
}

// SetTokenIssuer sets the token issuer for generating management tool tokens.
// Deprecated: use NewOrchestratorServiceWithDeps(OrchestratorDeps{TokenIssuer: ...}).
func (s *OrchestratorService) SetTokenIssuer(ti *TokenIssuer) {
	s.tokenIssuer = ti
}

// SetUploadDir 注入上传文件根目录，用于服务端抽取附件文本。
func (s *OrchestratorService) SetUploadDir(dir string) {
	s.uploadDir = dir
}

// attachmentTextMaxRunes 单个附件注入上下文的最大字符数。
const attachmentTextMaxRunes = 6000

// BuildAttachmentContext 为带附件的消息生成「消息附件」上下文段：在服务端把每个附件抽取
// 为纯文本并直接内联，Agent 无需下载或解析二进制（避免超时）。无法解析的格式给出降级提示，
// 保证 Agent 始终能据此回复。无附件时返回空串。
//
// Façade：委托给 BuildAttachmentText（与 AttachmentBuilder 共享同一实现），保留旧 API 以
// 兼容 message.go 等历史调用方。后续 P6 清理 Setter 时会一并评估是否内联化。
func (s *OrchestratorService) BuildAttachmentContext(ctx context.Context, attachments []model.MessageAttachment, userID string) string {
	_ = userID // 历史签名保留，附件抽取不依赖 userID
	return BuildAttachmentText(ctx, attachments, s.uploadDir, attachmentTextMaxRunes)
}

// SetServerURL sets the server base URL for management tool API calls.
// Deprecated: use OrchestratorDeps.ServerURL.
func (s *OrchestratorService) SetServerURL(url string) {
	s.serverURL = url
}

// SetKBResolver sets the knowledge base resolver for injecting KB context.
// Deprecated: use OrchestratorDeps.KBResolver.
func (s *OrchestratorService) SetKBResolver(resolver OrchKBResolver) {
	s.kbResolver = resolver
}

// SetDaemonHub sets the daemon WebSocket hub for task dispatch.
// Deprecated: use OrchestratorDeps.DaemonHub.
func (s *OrchestratorService) SetDaemonHub(hub *ws.DaemonHub) {
	s.daemonHub = hub
}

// SetArtifactRepo sets the artifact repository used for AI editing artifacts.
// Deprecated: use OrchestratorDeps.ArtifactRepo.
func (s *OrchestratorService) SetArtifactRepo(repo OrchArtifactRepo) {
	s.artifactRepo = repo
}

// SetOrchTaskRepo sets the orchestration task store.
// Deprecated: use OrchestratorDeps.OrchTaskRepo.
func (s *OrchestratorService) SetOrchTaskRepo(repo OrchTaskStore) {
	s.orchTaskRepo = repo
}

// SetNotifier injects the message notifier for pushing async messages to user WS.
// Deprecated: use OrchestratorDeps.Notifier.
func (s *OrchestratorService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetDeliveryState injects transient delivery state storage.
// Deprecated: use OrchestratorDeps.Delivery.
func (s *OrchestratorService) SetDeliveryState(c OrchDeliveryState) {
	s.delivery = c
}

// SetCacher is kept for compatibility with older wiring code.
// Deprecated: use OrchestratorDeps.Delivery.
func (s *OrchestratorService) SetCacher(c OrchDeliveryState) {
	s.SetDeliveryState(c)
}

// SetTaskSvc injects the task board sync service.
// Deprecated: use OrchestratorDeps.TaskSvc.
func (s *OrchestratorService) SetTaskSvc(svc TaskBoardSync) {
	s.taskSvc = svc
}

// NewOrchestratorService creates a new orchestrator service.
// Deprecated: use NewOrchestratorServiceWithDeps for explicit dependency injection.
func NewOrchestratorService(convRepo OrchConvRepo, agentRepo OrchAgentRepo, msgRepo MsgRepo) *OrchestratorService {
	svc := &OrchestratorService{
		convRepo:   convRepo,
		agentRepo:  agentRepo,
		msgRepo:    msgRepo,
		agentQueue: NewAgentQueue(),
		router:     NewRouter(),
	}
	svc.dispatcher = NewDispatcher(svc)
	// 装配默认链（依赖项未注入时 builder 会做 nil 防护，不影响构造）
	svc.buildDefaultChains()
	return svc
}

// OrchestratorDeps bundles all optional dependencies for OrchestratorService.
// Use NewOrchestratorServiceWithDeps to construct the service with all deps at once.
type OrchestratorDeps struct {
	ConvRepo     OrchConvRepo
	AgentRepo    OrchAgentRepo
	MsgRepo      MsgRepo
	OrchTaskRepo OrchTaskStore
	TokenIssuer  *TokenIssuer
	ServerURL    string
	UploadDir    string
	KBResolver   OrchKBResolver
	DaemonHub    *ws.DaemonHub
	ArtifactRepo OrchArtifactRepo
	Notifier     MessageNotifier
	Delivery     OrchDeliveryState
	TaskSvc      TaskBoardSync

	// 可选：注入自定义上下文构建链。nil 时使用默认链（详见 buildDefaultChains）。
	DirectReplyChain *ContextChain
	WorkerChain      *ContextChain
	OrchChain        *ContextChain
	FanoutChain      *ContextChain // 路径 C 异步 fanout 变体
	SummaryChain     *ContextChain // 路径 D summary 阶段

	// 可选：注入自定义派发并发护栏。nil 时使用默认 AgentQueue。
	AgentQueue *AgentQueue

	// 可选：注入自定义路由器。nil 时使用默认 Router。
	Router *Router

	// 可选：注入自定义派发器。nil 时使用默认 Dispatcher（绑定到构造完成的 svc）。
	Dispatcher *Dispatcher
}

// NewOrchestratorServiceWithDeps creates a fully-initialized OrchestratorService.
// 未显式注入 Chain 的字段会用默认 chain 装配，保证拼装顺序与重构前完全等价：
//   - DirectReplyChain（路径 A：asyncAgentReply 单聊直接回复）
//     [Attachment, Blackboard, KB, AgentConfig]
//     最终输出: agentConfig + kb + blackboard + attach
//   - WorkerChain（路径 C：dispatchSingleAgent）
//     [KB, Blackboard, AgentConfig]
//     最终输出: agentConfig + blackboard + kbPreload
//   - OrchChain（路径 D：handleOrchestratedDispatch）
//     [KB, OrchestratorPrompt, AgentConfig]
//     最终输出: agentConfig + orchPrompt + kbPreload
//
// 注意：路径 A 与 C 的 Blackboard/KB 顺序不一致是历史遗留，
// 这里保留各自路径的现有顺序以做到零行为变更（详见 P4 任务 prd.md 的「现状」一节）。
func NewOrchestratorServiceWithDeps(deps OrchestratorDeps) *OrchestratorService {
	svc := &OrchestratorService{
		convRepo:     deps.ConvRepo,
		agentRepo:    deps.AgentRepo,
		msgRepo:      deps.MsgRepo,
		orchTaskRepo: deps.OrchTaskRepo,
		tokenIssuer:  deps.TokenIssuer,
		serverURL:    deps.ServerURL,
		uploadDir:    deps.UploadDir,
		kbResolver:   deps.KBResolver,
		daemonHub:    deps.DaemonHub,
		artifactRepo: deps.ArtifactRepo,
		notifier:     deps.Notifier,
		delivery:     deps.Delivery,
		taskSvc:      deps.TaskSvc,
		agentQueue:   deps.AgentQueue,
		router:       deps.Router,
		dispatcher:   deps.Dispatcher,
	}
	if svc.agentQueue == nil {
		svc.agentQueue = NewAgentQueue()
	}
	if svc.router == nil {
		svc.router = NewRouter()
	}
	if svc.dispatcher == nil {
		svc.dispatcher = NewDispatcher(svc)
	}
	svc.directReplyChain = deps.DirectReplyChain
	svc.workerChain = deps.WorkerChain
	svc.orchChain = deps.OrchChain
	svc.fanoutChain = deps.FanoutChain
	svc.summaryChain = deps.SummaryChain
	if svc.directReplyChain == nil || svc.workerChain == nil || svc.orchChain == nil || svc.fanoutChain == nil || svc.summaryChain == nil {
		svc.buildDefaultChains()
	}
	return svc
}

// buildDefaultChains 用 OrchestratorService 当前依赖装配默认上下文链。
// 已被显式注入（非 nil）的字段不会被覆盖。
//
// 调用时机：
//   - NewOrchestratorServiceWithDeps：构造时调用一次；deps 中显式注入的 chain 不会被覆盖
//   - NewOrchestratorService（旧构造函数）：构造时调用一次；后续 SetX 注入的依赖
//     不会自动同步到默认链，但目前没有测试在 setter 后路由非空的 KB/附件/agent-config
//     路径，因此可接受。生产代码统一使用 NewOrchestratorServiceWithDeps。
func (s *OrchestratorService) buildDefaultChains() {
	attach := &AttachmentBuilder{UploadDir: s.uploadDir, MaxRunes: attachmentTextMaxRunes}
	blackboard := &BlackboardBuilder{MsgRepo: s.msgRepo}
	kb := &KBBuilder{KBResolver: s.kbResolver, TokenIssuer: s.tokenIssuer, ServerURL: s.serverURL}
	agentCfg := &AgentConfigInjector{}
	orchPrompt := &OrchestratorPromptBuilder{}
	fanoutFrame := &FanoutFrameBuilder{}

	if s.directReplyChain == nil {
		// 路径 A：最终输出 agentConfig + kb + blackboard + attach
		s.directReplyChain = NewContextChain(attach, blackboard, kb, agentCfg)
	}
	if s.workerChain == nil {
		// 路径 C：最终输出 agentConfig + blackboard + kbPreload
		s.workerChain = NewContextChain(kb, blackboard, agentCfg)
	}
	if s.orchChain == nil {
		// 路径 D：最终输出 agentConfig + orchPrompt + kbPreload
		s.orchChain = NewContextChain(kb, orchPrompt, agentCfg)
	}
	if s.fanoutChain == nil {
		// 路径 C 异步 fanout 变体：[Frame, KB, AgentConfig]
		// 最终输出 agentConfig + kbPreload + frame（与 orchestrator_async 原内联拼装一致）
		s.fanoutChain = NewContextChain(fanoutFrame, kb, agentCfg)
	}
	if s.summaryChain == nil {
		// 路径 D summary 阶段：仅 [AgentConfig]
		// 最终输出 agentConfig + ""（summary prompt 已自带 summary system prompt）
		s.summaryChain = NewContextChain(agentCfg)
	}
}

// DirectReplyChain 返回单聊直接回复路径的上下文链（路径 A）。
func (s *OrchestratorService) DirectReplyChain() *ContextChain { return s.directReplyChain }

// WorkerChain 返回 worker 派发路径的上下文链（路径 C）。
func (s *OrchestratorService) WorkerChain() *ContextChain { return s.workerChain }

// OrchChain 返回 orchestrator 派发路径的上下文链（路径 D）。
func (s *OrchestratorService) OrchChain() *ContextChain { return s.orchChain }

// FanoutChain 返回 worker 异步 fanout 路径的上下文链（路径 C 变体）。
// 与 WorkerChain 共享 KB / AgentConfig，额外前置「群聊背景 + 调度指令」框架段。
func (s *OrchestratorService) FanoutChain() *ContextChain { return s.fanoutChain }

// SummaryChain 返回 orchestrator summary 阶段的上下文链（路径 D summary）。
// 仅一个 AgentConfig builder，不叠加 OrchestratorSystemPrompt。
func (s *OrchestratorService) SummaryChain() *ContextChain { return s.summaryChain }

// Router 返回路由器实例（解析 @mention → DispatchTarget）。
func (s *OrchestratorService) Router() *Router { return s.router }

// Dispatcher 返回派发器实例（封装 dispatchAndWait）。
func (s *OrchestratorService) Dispatcher() *Dispatcher { return s.dispatcher }

// AgentQueue 返回并发护栏实例（串行化同一 agent 的派发）。
func (s *OrchestratorService) AgentQueue() *AgentQueue { return s.agentQueue }

// RouteMention is the main entry point called when a message contains @mentions.
func (s *OrchestratorService) RouteMention(ctx context.Context, convID, userID, content string, attachments []model.MessageAttachment, sourceMessageID *string) (*RouteResult, error) {
	slog.Info(orchFlowLog, "stage", "route.start", "conversation_id", convID, "user_id", userID, "source_message_id", stringValue(sourceMessageID), "content_len", len(content))
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("orch get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}

	convAgents, err := s.convRepo.ListAgents(ctx, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("orch list agents: %w", err)
	}

	mentions := ParseMentions(content)
	slog.Info(orchFlowLog, "stage", "route.mentions_parsed", "conversation_id", convID, "mention_count", len(mentions), "mentions", mentionLogNames(mentions), "agent_count", len(convAgents), "agents", convAgentLogNames(convAgents))
	if len(mentions) == 0 {
		return nil, nil
	}

	mentionMap := FindMentionedAgentID(mentions, convAgents)
	slog.Info(orchFlowLog, "stage", "route.mentions_resolved", "conversation_id", convID, "resolved_count", len(mentionMap), "resolved", mentionMap)
	if len(mentionMap) == 0 {
		return nil, nil
	}

	// 在入口处预解析原始用户消息中的知识库引用，
	// 避免 Orchestrator 改写任务描述后丢失 {{用户名/KB}} 语法
	kbPreload := s.PreloadKBContext(ctx, content, userID)

	// 把本条消息的附件抽取为文本前置注入，让被 @ 的 Agent 直接据此回答。
	if attachCtx := s.BuildAttachmentContext(ctx, attachments, userID); attachCtx != "" {
		kbPreload = attachCtx + kbPreload
	}

	// 通过 Router 解析派发目标：mention → agent + role + task。
	// Router 只负责路由决策，不触碰上下文工程（kbPreload / attachments）。
	targets := s.router.Resolve(ctx, RouterInput{
		Content:    content,
		ConvAgents: convAgents,
		Mentions:   mentions,
		MentionMap: mentionMap,
		AgentRepo:  s.agentRepo.GetByID,
	})

	result := &RouteResult{}

	for _, t := range targets {
		agentID := t.Agent.ID
		if t.Role == DispatchRoleOrchestrator {
			slog.Info(orchFlowLog, "stage", "route.to_orchestrator", "conversation_id", convID, "agent_id", agentID, "agent_name", t.Agent.Name)
			msgs, err := s.handleOrchestratedDispatch(ctx, convID, userID, t.Agent, content, convAgents, kbPreload, sourceMessageID)
			if err != nil {
				slog.Warn(orchFlowLog, "stage", "route.orchestrator_failed", "conversation_id", convID, "agent_id", agentID, "error", err)
				continue
			}
			slog.Info(orchFlowLog, "stage", "route.orchestrator_returned", "conversation_id", convID, "agent_id", agentID, "message_count", len(msgs))
			result.AgentMessages = append(result.AgentMessages, msgs...)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: t.MentionName,
				Task:      content,
			})
		} else {
			slog.Info(orchFlowLog, "stage", "route.to_worker_direct", "conversation_id", convID, "agent_id", agentID, "agent_name", t.Agent.Name, "reply_to", stringValue(sourceMessageID))
			msg, err := s.dispatchSingleAgent(ctx, convID, userID, t.Agent, t.Task, kbPreload, sourceMessageID)
			if err != nil {
				slog.Warn(orchFlowLog, "stage", "route.worker_direct_failed", "conversation_id", convID, "agent_id", agentID, "error", err)
				continue
			}
			result.AgentMessages = append(result.AgentMessages, msg)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: t.MentionName,
				Task:      content,
				Parallel:  true,
			})
		}
	}

	return result, nil
}

// dispatchAndWait creates a daemon task, sends it via WS, waits for the result
// via channel-based notification, creates a message, and returns it.
// This is the unified dispatch path shared by both user @mention and orch worker dispatch.
func (s *OrchestratorService) dispatchAndWait(ctx context.Context, convID, userID string, agent *model.Agent, prompt string, contextMessages string, replyTo *string) (*model.Message, error) {
	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, prompt, contextMessages)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_task_created", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID, "cli_tool", agent.CLITool, "prompt_len", len(prompt), "context_len", len(contextMessages), "reply_to", stringValue(replyTo), "prompt_preview", orchPreview(prompt))

	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		slog.Warn(orchFlowLog, "stage", "agent.daemon_not_connected", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID)
		return nil, fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
	}
	s.daemonHub.RegisterTaskPromise(task.ID)
	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
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
		return nil, fmt.Errorf("dispatch to daemon: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_sent", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID)

	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "agent.dispatch_wait_failed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "error", err)
		return nil, err
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_completed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "status", task.Status, "result_len", len(task.Result), "artifact_count", len(task.Artifacts), "result_preview", orchPreview(task.Result))
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts := agentMetadata(agent)
	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, artifacts, nil, replyTo, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.message_created", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "message_id", msg.ID, "reply_to", stringValue(replyTo), "content_len", len(msg.Content))
	s.persistArtifacts(ctx, msg, task.Artifacts)
	return msg, nil
}

// dispatchSingleAgent dispatches to a single non-orchestrator agent.
func (s *OrchestratorService) dispatchSingleAgent(ctx context.Context, convID, userID string, agent *model.Agent, content string, kbPreload string, replyTo *string) (*model.Message, error) {
	if agent.UserID != nil && *agent.UserID != userID {
		return nil, ErrMsgAgentNoPerm
	}
	ok, err := s.agentRepo.IsAgentInConversation(ctx, convID, agent.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("check conversation agent: %w", err)
	}
	if !ok {
		return nil, ErrMsgAgentNoPerm
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}

	// Dispatch guard：通过 AgentQueue 串行化同一 agent 的派发（buffered-1 semaphore）。
	// 行为与原 sync.Map + `sem <- struct{}{}` 完全一致。
	var msg *model.Message
	err = s.agentQueue.Run(ctx, agent.ID, func() error {
		// 通过 worker chain 构建上下文：[KB, Blackboard, AgentConfig]
		// KB 优先使用预加载（来自 RouteMention 入口），为空时实时解析 content 中的 {{user/KB}}。
		// 最终输出顺序：agentConfig + blackboardCtx + kbCtx（与重构前完全一致）。
		agentCtx := s.WorkerChain().Build(ctx, ContextInput{
			ConvID:    convID,
			UserID:    userID,
			Agent:     agent,
			Content:   content,
			KBPreload: kbPreload,
		})

		msg, err = s.dispatchAndWait(ctx, convID, userID, agent, content, agentCtx, replyTo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// handleOrchestratedDispatch runs the full orchestrator flow: dispatch to orchestrator, parse output, fan out to workers.
// kbPreload 是从原始用户消息预解析的知识库上下文，确保 Orchestrator 改写任务后 worker 仍能获取 KB 内容。
func (s *OrchestratorService) handleOrchestratedDispatch(ctx context.Context, convID, userID string, orchAgent *model.Agent, content string, convAgents []model.ConversationAgent, kbPreload string, sourceMessageID *string) ([]*model.Message, error) {
	if orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}
	// 确保 orchestratorName 非空，后续 dispatchAsyncWorker 依赖它构建上下文
	if orchAgent.Name == "" {
		orchAgent.Name = "Orchestrator"
	}
	slog.Info(orchFlowLog, "stage", "orch.dispatch.start", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "orch_agent_name", orchAgent.Name, "machine_id", *orchAgent.MachineID, "source_message_id", stringValue(sourceMessageID), "content_len", len(content))

	recentSummary, err := s.buildRecentSummary(ctx, convID)
	if err != nil {
		slog.Warn("build recent summary failed", "conv_id", convID, "error", err)
		recentSummary = ""
	}

	conv, _ := s.convRepo.GetByID(ctx, convID)
	convTitle := ""
	if conv != nil {
		convTitle = conv.Title
	}

	blackboardCtx := s.BuildConversationBlackboardContext(ctx, convID)
	agentDetails := buildOrchestratorAgentDetails(convAgents)
	availableAgentNames := agentNames(agentDetails)
	fullPrompt := BuildOrchestratorPromptWithAgents(convTitle, agentDetails, blackboardCtx, recentSummary, content)

	// 通过 orch chain 构建 contextMessages：[KB, OrchestratorPrompt, AgentConfig]
	// 最终输出顺序：agentConfig + orchSystemPrompt + kbPreload（与重构前完全一致）。
	// 注意：blackboard 通过 fullPrompt 注入，不在 contextMessages 中（保持原行为）。
	orchCtx := s.OrchChain().Build(ctx, ContextInput{
		ConvID:         convID,
		UserID:         userID,
		Agent:          orchAgent,
		Content:        content,
		KBPreload:      kbPreload,
		IsOrchestrator: true,
	})
	slog.Info(orchFlowLog, "stage", "orch.prompt_prepared", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "prompt_len", len(fullPrompt), "context_len", len(orchCtx), "agent_count", len(convAgents))

	orchTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, orchAgent.ID, *orchAgent.MachineID, orchAgent.CLITool, fullPrompt, orchCtx)
	if err != nil {
		return nil, fmt.Errorf("create orchestrator task: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "orch.daemon_task_created", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID, "machine_id", *orchAgent.MachineID, "cli_tool", orchAgent.CLITool)

	// Orchestrator daemon must be connected via WS to dispatch
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*orchAgent.MachineID) {
		slog.Warn(orchFlowLog, "stage", "orch.daemon_not_connected", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID, "machine_id", *orchAgent.MachineID)
		return nil, fmt.Errorf("orchestrator agent %q 的 daemon 未通过 WS 连接", orchAgent.Name)
	}
	s.daemonHub.RegisterTaskPromise(orchTask.ID)
	if err := s.daemonHub.SendToMachine(*orchAgent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":          orchTask.ID,
			"cli_tool":         orchAgent.CLITool,
			"prompt":           fullPrompt,
			"context_messages": orchCtx,
			"agent_id":         orchAgent.ID,
			"conversation_id":  convID,
			"user_id":          userID,
		},
	}); err != nil {
		return nil, fmt.Errorf("dispatch to orchestrator daemon: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "orch.dispatch_sent", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID)

	orchTask, err = s.waitDaemonTask(ctx, orchTask.ID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "orch.dispatch_wait_failed", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID, "error", err)
		return nil, err
	}
	slog.Info(orchFlowLog, "stage", "orch.dispatch_completed", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID, "status", orchTask.Status, "result_len", len(orchTask.Result), "artifact_count", len(orchTask.Artifacts), "result_preview", orchPreview(orchTask.Result))
	if orchTask.Status == "failed" {
		return nil, fmt.Errorf("orchestrator task failed: %s", orchTask.Error)
	}

	dispatch := ParseOrchestratorOutputForAgents(orchTask.Result, availableAgentNames)
	if dispatch == nil {
		slog.Info(orchFlowLog, "stage", "orch.output_parsed", "conversation_id", convID, "task_count", 0, "has_dispatch", false)
	} else {
		slog.Info(orchFlowLog, "stage", "orch.output_parsed", "conversation_id", convID, "task_count", len(dispatch.Tasks), "has_dispatch", len(dispatch.Tasks) > 0, "agents", dispatchTaskLogNames(dispatch.Tasks))
	}

	// Orchestrator responded directly without dispatching
	if dispatch == nil || len(dispatch.Tasks) == 0 {
		artifacts := agentMetadata(orchAgent)
		msg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, sourceMessageID, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("create orchestrator reply: %w", err)
		}
		slog.Info(orchFlowLog, "stage", "orch.direct_message_created", "conversation_id", convID, "message_id", msg.ID, "reply_to", stringValue(sourceMessageID), "content_len", len(msg.Content))
		s.persistArtifacts(ctx, msg, orchTask.Artifacts)
		return []*model.Message{msg}, nil
	}

	// Post orchestrator's full dispatch message (including @mentions) to group chat
	var messages []*model.Message
	workerReplyTo := sourceMessageID
	{
		artifacts := agentMetadata(orchAgent)
		dispatchMsg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, sourceMessageID, nil, nil)
		if err != nil {
			slog.Warn(orchFlowLog, "stage", "orch.dispatch_message_create_failed", "conversation_id", convID, "reply_to", stringValue(sourceMessageID), "error", err)
		} else {
			s.persistArtifacts(ctx, dispatchMsg, orchTask.Artifacts)
			messages = append(messages, dispatchMsg)
			workerReplyTo = &dispatchMsg.ID
			slog.Info(orchFlowLog, "stage", "orch.dispatch_message_created", "conversation_id", convID, "message_id", dispatchMsg.ID, "reply_to", stringValue(sourceMessageID), "content_len", len(dispatchMsg.Content))
		}
	}

	plan := BuildWorkerDispatchPlan(dispatch.Tasks, convAgents)
	slog.Info(orchFlowLog, "stage", "orch.worker_plan_built", "conversation_id", convID, "parsed_count", len(dispatch.Tasks), "worker_count", len(plan.Tasks), "unknown_count", len(plan.UnknownTasks), "workers", resolvedDispatchLogNames(plan.Tasks), "unknown_agents", dispatchTaskLogNames(plan.UnknownTasks))
	for _, unknown := range plan.UnknownTasks {
		slog.Warn(orchFlowLog, "stage", "orch.worker_plan_unknown_agent", "conversation_id", convID, "agent", unknown.AgentName)
	}
	if !plan.HasWorkers() {
		slog.Warn(orchFlowLog, "stage", "orch.worker_plan_empty", "conversation_id", convID, "task_count", len(dispatch.Tasks))
		return messages, nil
	}

	orchTaskID := ""
	orchTaskRecord, ok := s.createOrchLifecycle(ctx, convID, userID, orchAgent.ID, orchTask.Result, content, kbPreload, sourceMessageID, workerReplyTo)
	if ok && orchTaskRecord != nil {
		orchTaskID = orchTaskRecord.ID
	} else {
		// 生命周期记录失败时不能吞掉 worker 派发；否则用户只能看到 Orch 的 @消息，
		// 后续 worker 完全不会收到任务。此降级路径不做自动汇总，日志会明确暴露原因。
		slog.Warn(orchFlowLog, "stage", "orch.lifecycle_degraded_no_record", "conversation_id", convID, "worker_count", len(plan.Tasks), "summary_enabled", false)
	}

	// 所有 worker 并行派发：startWorkersAndWait 内部用 WaitGroup 等待全部完成后触发 summary
	orchSender := OrchSender{ID: orchAgent.ID, Name: orchAgent.Name, Avatar: orchAgent.Avatar}
	slog.Info(orchFlowLog, "stage", "orch.worker_fanout_scheduled", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_count", len(plan.Tasks), "reply_to", stringValue(workerReplyTo), "summary_enabled", orchTaskID != "")
	go s.startWorkersAndWait(context.Background(), convID, userID, plan, orchAgent.Name, kbPreload, orchTaskID, orchSender, workerReplyTo, workerReplyTo)

	return messages, nil
}

func (s *OrchestratorService) createOrchLifecycle(ctx context.Context, convID, userID, orchAgentID, dispatchPlan, originalMessage, kbPreload string, sourceMessageID, dispatchMessageID *string) (*model.OrchTask, bool) {
	orchTaskRecord := &model.OrchTask{
		ConversationID:    convID,
		UserID:            userID,
		OrchAgentID:       orchAgentID,
		Status:            model.OrchTaskWorkersRunning,
		DispatchPlan:      dispatchPlan,
		OriginalMessage:   originalMessage,
		SourceMessageID:   stringValue(sourceMessageID),
		DispatchMessageID: stringValue(dispatchMessageID),
		KBPreload:         kbPreload,
		WorkerStatus:      "{}",
		WorkerResults:     "{}",
		RoundHistory:      "[]",
	}

	if s.orchTaskRepo == nil {
		slog.Warn(orchFlowLog, "stage", "orch.lifecycle_repo_missing", "conversation_id", convID)
		return orchTaskRecord, true
	}
	slog.Info(orchFlowLog, "stage", "orch.lifecycle_create_start", "conversation_id", convID, "orch_agent_id", orchAgentID, "source_message_id", orchTaskRecord.SourceMessageID, "dispatch_message_id", orchTaskRecord.DispatchMessageID, "status", orchTaskRecord.Status, "dispatch_plan_len", len(dispatchPlan))
	if err := s.orchTaskRepo.Create(ctx, orchTaskRecord); err != nil {
		slog.Error(orchFlowLog, "stage", "orch.lifecycle_create_failed", "conversation_id", convID, "orch_agent_id", orchAgentID, "source_message_id", orchTaskRecord.SourceMessageID, "dispatch_message_id", orchTaskRecord.DispatchMessageID, "error", err)
		return nil, false
	}
	slog.Info(orchFlowLog, "stage", "orch.lifecycle_created", "conversation_id", convID, "orch_task_id", orchTaskRecord.ID, "status", orchTaskRecord.Status)
	if err := s.orchTaskRepo.UpdateStatus(ctx, orchTaskRecord.ID, model.OrchTaskWorkersRunning); err != nil {
		slog.Warn(orchFlowLog, "stage", "orch.lifecycle_status_update_failed", "conversation_id", convID, "orch_task_id", orchTaskRecord.ID, "status", model.OrchTaskWorkersRunning, "error", err)
	}
	return orchTaskRecord, true
}

func (s *OrchestratorService) persistArtifacts(ctx context.Context, msg *model.Message, artifacts []model.Artifact) {
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
	if err := s.msgRepo.SaveArtifacts(ctx, msg.ID, artifacts); err != nil {
		slog.Warn("save orchestrator artifacts failed", "message_id", msg.ID, "error", err)
		return
	}
	msg.Artifacts = artifacts
}

// buildDispatchContext builds Layer 2 context for a worker agent dispatch.
func (s *OrchestratorService) buildDispatchContext(ctx context.Context, convID string, task DispatchTask, depResults map[string]string, orchestratorName string) (string, error) {
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	blackboardCtx := s.BuildConversationBlackboardContext(ctx, convID)

	var sb strings.Builder
	if blackboardCtx != "" {
		sb.WriteString(blackboardCtx)
	}
	sb.WriteString("[群聊背景]\n")
	for _, m := range msgs {
		role := m.Role
		if m.Role == "assistant" {
			if name := extractAgentName(m.ArtifactsJSON); name != "" {
				role = name
			}
		}
		fmt.Fprintf(&sb, "- %s: %s\n", role, truncateString(m.Content, 100))
	}
	sb.WriteString("\n")

	// 限制任务描述长度
	taskDesc := truncateString(task.Task, 2000)

	if taskDesc != "" {
		sb.WriteString("[调度指令]\n")
		fmt.Fprintf(&sb, "Orch @你，分配了以下任务：\n%s\n\n", taskDesc)
	}

	if len(depResults) > 0 {
		sb.WriteString("[依赖输出]\n")
		for name, result := range depResults {
			fmt.Fprintf(&sb, "%s 已完成，结果摘要：\n%s\n\n", name, result)
		}
	}

	if orchestratorName == "" {
		orchestratorName = "Orchestrator"
	}
	fmt.Fprintf(&sb, "请完成这个任务并在回复末尾 @%s 表示完成。", orchestratorName)

	// 总长度保护：超过 4000 字符时从最早的消息开始截断
	const maxContextLen = 6000
	result := sb.String()
	runes := []rune(result)
	if len(runes) > maxContextLen {
		result = string(runes[len(runes)-maxContextLen:])
		// 截断后跳过第一个不完整的行
		if idx := strings.IndexByte(result, '\n'); idx >= 0 {
			result = result[idx+1:]
		}
		if blackboardCtx != "" && !strings.Contains(result, "{会话上下文黑板") {
			result = blackboardCtx + result
		}
	}
	return result, nil
}

const (
	blackboardPinLimit        = 20
	blackboardMaxEntryRunes   = 800
	blackboardMaxContextRunes = 5000
	blackboardMaxManualRunes  = 3000
)

// BuildConversationBlackboardContext builds the shared conversation context block
// injected into orchestrator and worker prompts.
//
// Façade：委托给 BuildBlackboardText（与 BlackboardBuilder 共享同一实现）。
func (s *OrchestratorService) BuildConversationBlackboardContext(ctx context.Context, convID string) string {
	if s == nil || s.msgRepo == nil {
		return ""
	}
	return BuildBlackboardText(ctx, s.msgRepo, convID)
}

// buildRecentSummary formats the last 10 messages in chronological order.
func (s *OrchestratorService) buildRecentSummary(ctx context.Context, convID string) (string, error) {
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	var sb strings.Builder
	for _, m := range msgs {
		role := m.Role
		if m.Role == "assistant" {
			if name := extractAgentName(m.ArtifactsJSON); name != "" {
				role = name
			}
		}
		fmt.Fprintf(&sb, "- %s: %s\n", role, truncateString(m.Content, 100))
	}

	return sb.String(), nil
}

func buildOrchestratorAgentDetails(convAgents []model.ConversationAgent) []OrchestratorAgentDetail {
	details := make([]OrchestratorAgentDetail, 0, len(convAgents))
	for _, ca := range convAgents {
		details = append(details, OrchestratorAgentDetail{
			Name:        ca.Name,
			Role:        ca.Role,
			Status:      ca.Status,
			Description: dispatchSafeAgentDescription(ca),
			Tags:        ca.Tags,
		})
	}
	return details
}

func dispatchSafeAgentDescription(agent model.ConversationAgent) string {
	if strings.TrimSpace(agent.Description) != "" {
		return truncateString(normalizePromptLine(agent.Description), 160)
	}
	switch agent.CLITool {
	case "claude":
		return "Claude Code CLI Agent，适合代码生成、项目理解、重构、评审与任务拆解。"
	case "codex":
		return "Codex CLI Agent，适合代码实现、补丁生成、测试修复和工程化任务。"
	case "opencode":
		return "OpenCode CLI Agent，适合通用代码任务和命令行开发工作流。"
	default:
		return ""
	}
}

// waitDaemonTask waits for a daemon task to complete via channel-based
// notification through DaemonHub. Returns an error if the daemon is not
// connected or the context times out.
func (s *OrchestratorService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	if s.daemonHub == nil {
		return nil, fmt.Errorf("daemon hub not available")
	}

	ch := s.daemonHub.AwaitTaskResult(taskID)
	if ch == nil {
		return nil, fmt.Errorf("daemon not connected for task %s", taskID)
	}
	defer s.daemonHub.RemoveTaskPromise(taskID)

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

// kbMaxInlineChars 单个文本文件内联注入的最大字符数（超过则截断并提示使用工具）
const kbMaxInlineChars = 4000

// PreloadKBContext 在 RouteMention 入口处预解析用户消息中的知识库引用，
// 生成 KB 上下文字符串。确保 Orchestrator 改写任务后 worker 仍能获取 KB 内容。
// 返回空字符串表示无引用或解析失败。
//
// Façade：委托给 KBBuilder.resolveKB（与 chain 内 KBBuilder 共享同一实现）。
func (s *OrchestratorService) PreloadKBContext(ctx context.Context, content string, userID string) string {
	b := &KBBuilder{KBResolver: s.kbResolver, TokenIssuer: s.tokenIssuer, ServerURL: s.serverURL}
	return b.resolveKB(ctx, content, userID)
}

func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
	)
	switch {
	case size >= MB:
		return fmt.Sprintf("%.1fMB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1fKB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// agentMetadata serializes the agent identity fields into a JSON string
// for storing in the ArtifactsJSON column of a message.
func agentMetadata(agent *model.Agent) string {
	b, _ := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})
	return string(b)
}

// resolveMemberIDs returns the member IDs for a conversation, falling back to
// a single-element slice containing fallbackUserID if the query fails or returns empty.
func (s *OrchestratorService) resolveMemberIDs(ctx context.Context, convID, fallbackUserID string) []string {
	ids, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil || len(ids) == 0 {
		return []string{fallbackUserID}
	}
	return ids
}

// extractAgentName parses the agent_name field from an ArtifactsJSON string.
// Returns empty string on parse failure or missing field.
func extractAgentName(artifactsJSON string) string {
	var m map[string]string
	if err := json.Unmarshal([]byte(artifactsJSON), &m); err == nil {
		return m["agent_name"]
	}
	return ""
}
