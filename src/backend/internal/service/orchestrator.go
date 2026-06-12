package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/internal/repository"
)

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
//
// P8a: 此 subset interface 保留，因为 ResolveKnowledgeRef 横跨「KB + 用户名校验」
// 两个领域，且当前实现是 KnowledgeService（service 层），不属于 repository.KnowledgeStore
// 的纯持久化范畴。强行把它合并到 canonical KnowledgeStore 会污染 domain 边界。
type OrchKBResolver interface {
	ResolveKnowledgeRef(ctx context.Context, currentUserID, username, kbName string) (*model.KnowledgeBase, []model.KnowledgeFile, error)
}

// OrchDeliveryState stores transient delivery state for async orchestrator messages.
// Message rows are already persisted; Redis is only used for offline delivery and unread counts.
//
// P8a: 此 subset interface 保留，因为它属于 Redis 投递层（offline queue + unread counters），
// 与 repository.MessageStore（PG 持久化）职责正交，合到 canonical 会混淆持久化与缓存边界。
type OrchDeliveryState interface {
	EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error
	IncrementUnread(ctx context.Context, userID, conversationID string) error
}

// OrchestratorService handles @mention routing and orchestrated multi-agent dispatch.
type OrchestratorService struct {
	convRepo     repository.ConvStore
	agentRepo    repository.AgentStore
	msgRepo      repository.MessageStore
	orchTaskRepo repository.OrchTaskStoreCanon

	tokenIssuer port.TokenIssuerPort
	serverURL   string
	uploadDir   string // 上传文件落盘根目录，用于服务端抽取附件文本

	kbResolver   OrchKBResolver
	daemonHub    port.DaemonDispatcher
	artifactRepo repository.ArtifactStore
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
	// P7 进一步把 Router 从具体 struct 改为 interface，允许注入自定义实现（零行为变更）。
	router Router

	// dispatcher 封装 dispatchAndWait 的派发动作（task 创建 + WS + 等待 + 落库）。
	// 当前作为可注入扩展点存在；现有调用方仍直接调 dispatchAndWait 以保持零行为变更，
	// 后续清理（P6+）会把 dispatchSingleAgent / fanout / summary 迁移到走 Dispatcher。
	dispatcher *Dispatcher
}

// SetUploadDir 注入上传文件根目录，用于服务端抽取附件文本。
func (s *OrchestratorService) SetUploadDir(dir string) {
	s.uploadDir = dir
}

// attachmentTextMaxRunes 单个附件注入上下文的最大字符数。
const attachmentTextMaxRunes = 6000

// NewOrchestratorService creates a new orchestrator service.
// Deprecated: use NewOrchestratorServiceWithDeps for explicit dependency injection.
func NewOrchestratorService(convRepo repository.ConvStore, agentRepo repository.AgentStore, msgRepo repository.MessageStore) *OrchestratorService {
	svc := &OrchestratorService{
		convRepo:   convRepo,
		agentRepo:  agentRepo,
		msgRepo:    msgRepo,
		agentQueue: NewAgentQueue(),
		router:     NewRouter(),
	}
	svc.dispatcher = NewDispatcher(DispatcherDeps{
		AgentRepo: svc.agentRepo,
		DaemonHub: svc.daemonHub,
		MsgRepo:   svc.msgRepo,
		UploadDir: svc.uploadDir,
	})
	// 装配默认链（依赖项未注入时 builder 会做 nil 防护，不影响构造）
	svc.buildDefaultChains()
	return svc
}

// OrchestratorDeps bundles all optional dependencies for OrchestratorService.
// Use NewOrchestratorServiceWithDeps to construct the service with all deps at once.
type OrchestratorDeps struct {
	ConvRepo     repository.ConvStore
	AgentRepo    repository.AgentStore
	MsgRepo      repository.MessageStore
	OrchTaskRepo repository.OrchTaskStoreCanon
	TokenIssuer  port.TokenIssuerPort
	ServerURL    string
	UploadDir    string
	KBResolver   OrchKBResolver
	DaemonHub    port.DaemonDispatcher
	ArtifactRepo repository.ArtifactStore
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

	// 可选：注入自定义路由器。nil 时使用默认 Router（NewDefaultRouter）。
	Router Router

	// 可选：注入自定义派发器。nil 时使用默认 Dispatcher（基于 svc 当前依赖构造 DispatcherDeps）。
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
		svc.router = NewDefaultRouter()
	}
	if svc.dispatcher == nil {
		// P7：Dispatcher 不再反向依赖 svc，构造时从 svc 装配 DispatcherDeps。
		// 此处取 svc 当前依赖快照；SetDaemonHub 等后续 setter 修改的是 svc.daemonHub，
		// 但 Dispatcher 内部持有的 *ws.DaemonHub 是指针，setter 修改的 hub 实例需要
		// 调用方重新构造 Dispatcher 或直接通过 OrchestratorDeps.DaemonHub 注入。
		svc.dispatcher = NewDispatcher(DispatcherDeps{
			AgentRepo: svc.agentRepo,
			DaemonHub: svc.daemonHub,
			MsgRepo:   svc.msgRepo,
			UploadDir: svc.uploadDir,
		})
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
func (s *OrchestratorService) Router() Router { return s.router }

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
	// （直接用 KBBuilder 纯函数，与 chain 内 KBBuilder 共享同一实现）
	kbPreload := (&KBBuilder{KBResolver: s.kbResolver, TokenIssuer: s.tokenIssuer, ServerURL: s.serverURL}).resolveKB(ctx, content, userID)

	// 把本条消息的附件抽取为文本前置注入，让被 @ 的 Agent 直接据此回答。
	// （直接调纯函数 BuildAttachmentText，与 AttachmentBuilder 共享同一实现）
	if attachCtx := BuildAttachmentText(ctx, attachments, s.uploadDir, attachmentTextMaxRunes); attachCtx != "" {
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
//
// P7 后核心实现已迁移到 Dispatcher.dispatchCore（dispatcher.go）；
// P8d 后 handleOrchestratedDispatch / runDaemonEdit 改走 Dispatcher.DispatchPlan，
// 此薄壳仅服务 dispatchSingleAgent (worker) 路径，等价于调用
// Dispatcher.Dispatch(..., DispatchHooks{})。零行为变更。
//
// 业务包装（权限 / 卡片生命周期 / CAS guard / summary 落库）应通过 Dispatcher.Dispatch
// + DispatchHooks 暴露，而不是在这里扩张。
func (s *OrchestratorService) dispatchAndWait(ctx context.Context, convID, userID string, agent *model.Agent, prompt string, contextMessages string, replyTo *string) (*model.Message, error) {
	res, err := s.dispatcher.Dispatch(ctx, DispatchInput{
		ConvID:          convID,
		UserID:          userID,
		Agent:           agent,
		Prompt:          prompt,
		ContextMessages: contextMessages,
		ReplyTo:         replyTo,
	}, DispatchHooks{})
	if err != nil {
		return nil, err
	}
	return res.Message, nil
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

		// 走 Dispatcher.Dispatch（零 hooks：等价于直接 dispatchAndWait，无业务包装）。
		res, derr := s.Dispatcher().Dispatch(ctx, DispatchInput{
			ConvID:          convID,
			UserID:          userID,
			Agent:           agent,
			Prompt:          content,
			ContextMessages: agentCtx,
			ReplyTo:         replyTo,
		}, DispatchHooks{})
		if derr != nil {
			return derr
		}
		msg = res.Message
		return nil
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// handleOrchestratedDispatch runs the full orchestrator flow: dispatch to orchestrator, parse output, fan out to workers.
// kbPreload 是从原始用户消息预解析的知识库上下文，确保 Orchestrator 改写任务后 worker 仍能获取 KB 内容。
//
// P9 后通过 Dispatcher.DispatchPlan 走统一派发路径（CreateDaemonTask → WS → wait），
// 但用自定义 PromptBuilder 把 fullPrompt 注入 daemon task，用自定义 ResultHandler
// 在 daemon 返回后：
//   - 解析输出（ParseOrchestratorOutputForAgents）
//   - 直答路径：落 message(replyTo=sourceMessageID)
//   - fanout 路径：落 dispatch message(replyTo=sourceMessageID) + 触发 startWorkersAndWait
//
// 错误码语义保留：daemon 未连接时映射回 "orchestrator agent %q 的 daemon 未通过 WS 连接"
// 原始错误格式（与 P9 前完全一致）。
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

	blackboardCtx := BuildBlackboardText(ctx, s.msgRepo, convID)
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

	// 通过闭包捕获解析结果与落库消息，供 ResultHandler 后续处理。
	var (
		parsedDispatch     *OrchDispatch
		messages           []*model.Message
		orchTaskArtifacts  []model.Artifact
	)

	plan := DispatchPlan{
		Input: DispatchInput{
			ConvID:          convID,
			UserID:          userID,
			Agent:           orchAgent,
			Prompt:          fullPrompt, // 通过 defaultPromptBuilder 注入到 daemon task / WS payload
			ContextMessages: orchCtx,
			ReplyTo:         sourceMessageID,
		},
		// PromptBuilder 留 nil → defaultPromptBuilder 直接用 input.Prompt（= fullPrompt）。
		ResultHandler: func(_ context.Context, orchTask *model.DaemonTask) (*model.Message, error) {
			slog.Info(orchFlowLog, "stage", "orch.dispatch_completed", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "daemon_task_id", orchTask.ID, "status", orchTask.Status, "result_len", len(orchTask.Result), "artifact_count", len(orchTask.Artifacts), "result_preview", orchPreview(orchTask.Result))

			// task.Artifacts 在两条路径都需要，先存到 closure 变量供后续 persistArtifacts 使用。
			orchTaskArtifacts = orchTask.Artifacts

			parsedDispatch = ParseOrchestratorOutputForAgents(orchTask.Result, availableAgentNames)
			if parsedDispatch == nil {
				slog.Info(orchFlowLog, "stage", "orch.output_parsed", "conversation_id", convID, "task_count", 0, "has_dispatch", false)
			} else {
				slog.Info(orchFlowLog, "stage", "orch.output_parsed", "conversation_id", convID, "task_count", len(parsedDispatch.Tasks), "has_dispatch", len(parsedDispatch.Tasks) > 0, "agents", dispatchTaskLogNames(parsedDispatch.Tasks))
			}

			// 直答路径：无 @mention 派发任务，落 message(replyTo=sourceMessageID)。
			if parsedDispatch == nil || len(parsedDispatch.Tasks) == 0 {
				artifacts := agentMetadata(orchAgent)
				msg, cerr := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, sourceMessageID, nil, nil)
				if cerr != nil {
					return nil, fmt.Errorf("create orchestrator reply: %w", cerr)
				}
				slog.Info(orchFlowLog, "stage", "orch.direct_message_created", "conversation_id", convID, "message_id", msg.ID, "reply_to", stringValue(sourceMessageID), "content_len", len(msg.Content))
				s.persistArtifacts(ctx, msg, orchTaskArtifacts)
				messages = []*model.Message{msg}
				// 返回 msg 让 DispatchPlan 触发 OnMessagePersisted（这里 hooks 为零值，无副作用）。
				return msg, nil
			}

			// fanout 路径：先落 dispatch message（replyTo=sourceMessageID），作为 workerReplyTo。
			artifacts := agentMetadata(orchAgent)
			dispatchMsg, cerr := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, sourceMessageID, nil, nil)
			if cerr != nil {
				slog.Warn(orchFlowLog, "stage", "orch.dispatch_message_create_failed", "conversation_id", convID, "reply_to", stringValue(sourceMessageID), "error", cerr)
				// 创建失败时返回 nil msg（与原行为一致：不 append，workerReplyTo 退回 sourceMessageID），
				// 但仍继续 fanout（原代码也继续，只是没 message 落库）。
			} else {
				s.persistArtifacts(ctx, dispatchMsg, orchTaskArtifacts)
				messages = append(messages, dispatchMsg)
				slog.Info(orchFlowLog, "stage", "orch.dispatch_message_created", "conversation_id", convID, "message_id", dispatchMsg.ID, "reply_to", stringValue(sourceMessageID), "content_len", len(dispatchMsg.Content))
			}
			// 返回 dispatchMsg（可能 nil，对应创建失败的降级路径）；后续 fanout 在 DispatchPlan 返回后触发。
			if dispatchMsg != nil {
				return dispatchMsg, nil
			}
			return nil, nil
		},
	}

	// orch 路径不用任何 hook：无 PreDispatch（machineID 已在入口校验）、无 OnTaskCreated
	// （orch 路径不创建 OrchTaskCard）、无 OnMessagePersisted（消息已通过 closure 处理）、
	// 无 OnFailed（错误直接透传，调用方处理）。
	res, err := s.dispatcher.DispatchPlan(ctx, plan, DispatchHooks{})
	if err != nil {
		// 错误码语义保留：daemon 未连接时映射回原始 orch 错误格式
		// （"orchestrator agent %q 的 daemon 未通过 WS 连接"）。
		if errors.Is(err, ErrDaemonNotConnected) {
			slog.Warn(orchFlowLog, "stage", "orch.daemon_not_connected", "conversation_id", convID, "orch_agent_id", orchAgent.ID, "machine_id", *orchAgent.MachineID)
			return nil, fmt.Errorf("orchestrator agent %q 的 daemon 未通过 WS 连接", orchAgent.Name)
		}
		// 其他错误（CreateDaemonTask / WS / wait / task failed / ResultHandler err）原样透传：
		// 现有测试只检查 err != nil 与 errors.Is 下层 sentinel，不依赖具体 fmt.Errorf 文案。
		// 保留 dispatcher 内部的 stage 日志（dispatch_task_created / dispatch_sent / dispatch_wait_failed）
		// 即可让人类阅读日志时定位失败点，错误文案差异不影响程序行为。
		return nil, err
	}

	// 直答路径：ResultHandler 已经把 message 落库并 append 到 messages，无需 fanout。
	if parsedDispatch == nil || len(parsedDispatch.Tasks) == 0 {
		return messages, nil
	}

	// fanout 路径：dispatchMsg 已落（如果创建成功）；现在计算 workerReplyTo + 触发 fanout。
	workerReplyTo := sourceMessageID
	if res != nil && res.Message != nil {
		workerReplyTo = &res.Message.ID
	}

	wplan := BuildWorkerDispatchPlan(parsedDispatch.Tasks, convAgents)
	slog.Info(orchFlowLog, "stage", "orch.worker_plan_built", "conversation_id", convID, "parsed_count", len(parsedDispatch.Tasks), "worker_count", len(wplan.Tasks), "unknown_count", len(wplan.UnknownTasks), "workers", resolvedDispatchLogNames(wplan.Tasks), "unknown_agents", dispatchTaskLogNames(wplan.UnknownTasks))
	for _, unknown := range wplan.UnknownTasks {
		slog.Warn(orchFlowLog, "stage", "orch.worker_plan_unknown_agent", "conversation_id", convID, "agent", unknown.AgentName)
	}
	if !wplan.HasWorkers() {
		slog.Warn(orchFlowLog, "stage", "orch.worker_plan_empty", "conversation_id", convID, "task_count", len(parsedDispatch.Tasks))
		return messages, nil
	}

	orchTaskID := ""
	orchTaskRecord, ok := s.createOrchLifecycle(ctx, convID, userID, orchAgent.ID, res.Task.Result, content, kbPreload, sourceMessageID, workerReplyTo)
	if ok && orchTaskRecord != nil {
		orchTaskID = orchTaskRecord.ID
	} else {
		// 生命周期记录失败时不能吞掉 worker 派发；否则用户只能看到 Orch 的 @消息，
		// 后续 worker 完全不会收到任务。此降级路径不做自动汇总，日志会明确暴露原因。
		slog.Warn(orchFlowLog, "stage", "orch.lifecycle_degraded_no_record", "conversation_id", convID, "worker_count", len(wplan.Tasks), "summary_enabled", false)
	}

	// 所有 worker 并行派发：startWorkersAndWait 内部用 WaitGroup 等待全部完成后触发 summary
	orchSender := OrchSender{ID: orchAgent.ID, Name: orchAgent.Name, Avatar: orchAgent.Avatar}
	slog.Info(orchFlowLog, "stage", "orch.worker_fanout_scheduled", "conversation_id", convID, "orch_task_id", orchTaskID, "worker_count", len(wplan.Tasks), "reply_to", stringValue(workerReplyTo), "summary_enabled", orchTaskID != "")
	go s.startWorkersAndWait(context.Background(), convID, userID, wplan, orchAgent.Name, kbPreload, orchTaskID, orchSender, workerReplyTo, workerReplyTo)

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

// persistArtifacts 把 daemon 返回的 artifacts（或从 markdown 抽取）落库并回填 msg.Artifacts。
//
// P7：核心实现已迁移到 Dispatcher.persistArtifacts（dispatcher.go）；
// 这里保留薄壳给 handleOrchestratedDispatch / runDaemonEdit 等内联路径继续调用。零行为变更。
func (s *OrchestratorService) persistArtifacts(ctx context.Context, msg *model.Message, artifacts []model.Artifact) {
	s.dispatcher.persistArtifacts(ctx, msg, artifacts)
}

const (
	blackboardPinLimit        = 20
	blackboardMaxEntryRunes   = 800
	blackboardMaxContextRunes = 5000
	blackboardMaxManualRunes  = 3000
)

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
//
// P7：核心实现已迁移到 Dispatcher.waitDaemonTask（dispatcher.go）；
// 这里保留薄壳给 handleOrchestratedDispatch / runDaemonEdit 等内联路径继续调用。零行为变更。
func (s *OrchestratorService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	return s.dispatcher.waitDaemonTask(ctx, taskID)
}

// kbMaxInlineChars 单个文本文件内联注入的最大字符数（超过则截断并提示使用工具）
const kbMaxInlineChars = 4000

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
