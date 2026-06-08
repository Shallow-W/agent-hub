package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/docextract"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// OrchConvRepo queries conversation agents for @mention resolution.
type OrchConvRepo interface {
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
}

// OrchAgentRepo queries agent details and creates daemon tasks.
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

// OrchMessageCacher 缓存异步消息到 Redis，避免刷新后丢失。
type OrchMessageCacher interface {
	CacheMessage(ctx context.Context, conversationID string, msg *model.Message) error
}

// OrchestratorService handles @mention routing and orchestrated multi-agent dispatch.
type OrchestratorService struct {
	convRepo    OrchConvRepo
	agentRepo   OrchAgentRepo
	msgRepo     MsgRepo
	orchTaskRepo OrchTaskStore

	tokenIssuer *TokenIssuer
	serverURL   string
	uploadDir   string // 上传文件落盘根目录，用于服务端抽取附件文本

	kbResolver   OrchKBResolver
	daemonHub    *ws.DaemonHub
	artifactRepo OrchArtifactRepo
	notifier     MessageNotifier
	cacher       OrchMessageCacher
	taskSvc      TaskBoardSync

	// 派发并发保护：同一 agent 同时只允许一个任务在飞，其他请求排队等待
	agentQueues sync.Map // agentID → chan struct{} (buffered-1 semaphore)
}

// SetTokenIssuer sets the token issuer for generating management tool tokens.
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
func (s *OrchestratorService) BuildAttachmentContext(ctx context.Context, attachments []model.MessageAttachment, userID string) string {
	if len(attachments) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[消息附件]\n")
	sb.WriteString("用户在本条消息中附带了以下文件，已由系统抽取为文本内联在下方，请据此回答：\n\n")
	for _, a := range attachments {
		header := fmt.Sprintf("=== 附件：%s (%s, %s) ===\n", a.FileName, a.MimeType, formatFileSize(a.FileSize))
		sb.WriteString(header)
		absPath := filepath.Join(s.uploadDir, filepath.FromSlash(strings.TrimLeft(a.FilePath, "/\\")))
		if text, ok := docextract.Extract(ctx, absPath, a.FileName, attachmentTextMaxRunes); ok {
			sb.WriteString(text)
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("（该格式无法在服务端自动解析，请提示用户转换为 pptx/docx/xlsx/pdf 或纯文本后重发）\n\n")
		}
	}
	return sb.String()
}

// SetServerURL sets the server base URL for management tool API calls.
func (s *OrchestratorService) SetServerURL(url string) {
	s.serverURL = url
}

// SetKBResolver sets the knowledge base resolver for injecting KB context.
func (s *OrchestratorService) SetKBResolver(resolver OrchKBResolver) {
	s.kbResolver = resolver
}

// SetDaemonHub sets the daemon WebSocket hub for task dispatch.
func (s *OrchestratorService) SetDaemonHub(hub *ws.DaemonHub) {
	s.daemonHub = hub
}

// SetArtifactRepo sets the artifact repository used for AI editing artifacts.
func (s *OrchestratorService) SetArtifactRepo(repo OrchArtifactRepo) {
	s.artifactRepo = repo
}

// SetOrchTaskRepo sets the orchestration task store.
func (s *OrchestratorService) SetOrchTaskRepo(repo OrchTaskStore) {
	s.orchTaskRepo = repo
}

// SetNotifier injects the message notifier for pushing async messages to user WS.
func (s *OrchestratorService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetCacher injects the message cacher for persisting async messages to Redis.
func (s *OrchestratorService) SetCacher(c OrchMessageCacher) {
	s.cacher = c
}

// SetTaskSvc injects the task board sync service.
func (s *OrchestratorService) SetTaskSvc(svc TaskBoardSync) {
	s.taskSvc = svc
}

// NewOrchestratorService creates a new orchestrator service.
func NewOrchestratorService(convRepo OrchConvRepo, agentRepo OrchAgentRepo, msgRepo MsgRepo) *OrchestratorService {
	return &OrchestratorService{
		convRepo:  convRepo,
		agentRepo: agentRepo,
		msgRepo:   msgRepo,
	}
}

// RouteMention is the main entry point called when a message contains @mentions.
func (s *OrchestratorService) RouteMention(ctx context.Context, convID, userID, content string, attachments []model.MessageAttachment) (*RouteResult, error) {
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
	if len(mentions) == 0 {
		return nil, nil
	}

	mentionMap := FindMentionedAgentID(mentions, convAgents)
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

	result := &RouteResult{}

	for _, m := range mentions {
		agentID, ok := mentionMap[m.AgentName]
		if !ok {
			continue
		}

		agent, err := s.agentRepo.GetByID(ctx, agentID)
		if err != nil {
			slog.Warn("orch get agent failed", "agent_id", agentID, "error", err)
			continue
		}
		if agent == nil {
			continue
		}

		// Check conversation agent role (set per group chat, not agent type)
			var isOrchestrator bool
			for _, ca := range convAgents {
				if ca.AgentID == agentID && ca.Role == "orchestrator" {
					isOrchestrator = true
					break
				}
			}

			if isOrchestrator {
			msgs, err := s.handleOrchestratedDispatch(ctx, convID, userID, agent, content, convAgents, kbPreload)

			if err != nil {
				slog.Warn("orchestrated dispatch failed", "agent_id", agentID, "error", err)
				continue
			}
			result.AgentMessages = append(result.AgentMessages, msgs...)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: m.AgentName,
				Task:      content,
			})
		} else {
			msg, err := s.dispatchSingleAgent(ctx, convID, userID, agent, m.Task, kbPreload)
			if err != nil {
				slog.Warn("direct dispatch failed", "agent_id", agentID, "error", err)
				continue
			}
			result.AgentMessages = append(result.AgentMessages, msg)
			result.Dispatches = append(result.Dispatches, DispatchInfo{
				AgentID:   agentID,
				AgentName: m.AgentName,
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
func (s *OrchestratorService) dispatchAndWait(ctx context.Context, convID, userID string, agent *model.Agent, prompt string, contextMessages string) (*model.Message, error) {
	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, prompt, contextMessages)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}

	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
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

	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts := agentMetadata(agent)
	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, artifacts, nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
		s.persistArtifacts(ctx, msg, task.Artifacts)
	return msg, nil
}

// dispatchSingleAgent dispatches to a single non-orchestrator agent.
func (s *OrchestratorService) dispatchSingleAgent(ctx context.Context, convID, userID string, agent *model.Agent, content string, kbPreload string) (*model.Message, error) {
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

	// Dispatch guard: serialize per-agent dispatches via semaphore; block if busy
	newSem := make(chan struct{}, 1)
	actual, _ := s.agentQueues.LoadOrStore(agent.ID, newSem)
	sem := actual.(chan struct{})
	sem <- struct{}{} // blocks until slot available
	defer func() { <-sem }()

	// 使用预加载的 KB 上下文（如果非空），否则回退到实时解析
	kbCtx := kbPreload
	if kbCtx == "" {
		kbCtx = s.injectKBContext(ctx, content, "", userID)
	}

	// 注入 Agent 系统提示词和工具配置到 context
	agentCtx := s.InjectAgentConfig(agent, kbCtx, userID)

	msg, err := s.dispatchAndWait(ctx, convID, userID, agent, content, agentCtx)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// handleOrchestratedDispatch runs the full orchestrator flow: dispatch to orchestrator, parse output, fan out to workers.
// kbPreload 是从原始用户消息预解析的知识库上下文，确保 Orchestrator 改写任务后 worker 仍能获取 KB 内容。
func (s *OrchestratorService) handleOrchestratedDispatch(ctx context.Context, convID, userID string, orchAgent *model.Agent, content string, convAgents []model.ConversationAgent, kbPreload string) ([]*model.Message, error) {
	if orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}
	// 确保 orchestratorName 非空，后续 dispatchAsyncWorker 依赖它构建上下文
	if orchAgent.Name == "" {
		orchAgent.Name = "Orchestrator"
	}

	recentSummary, agentNames, err := s.buildRecentSummary(ctx, convID, convAgents)
	if err != nil {
		slog.Warn("build recent summary failed", "conv_id", convID, "error", err)
		recentSummary = ""
	}

	conv, _ := s.convRepo.GetByID(ctx, convID)
	convTitle := ""
	if conv != nil {
		convTitle = conv.Title
	}

	fullPrompt := BuildOrchestratorPrompt(convTitle, agentNames, recentSummary, content)

	// 将 Orchestrator 系统指令注入到 contextMessages 最前面。
	// 注意：不依赖 agent.Type，因为 orchestrator 身份由 conversation_agents.role 决定，
	// agent.Type 可能是 "system" 或 "custom"。
	orchCtx := "[系统指令]\n" + OrchestratorSystemPrompt + "\n\n"
	orchCtx += kbPreload
	agentConfig := s.InjectAgentConfig(orchAgent, "", userID)
	orchCtx = agentConfig + orchCtx

	orchTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, orchAgent.ID, *orchAgent.MachineID, orchAgent.CLITool, fullPrompt, orchCtx)
	if err != nil {
		return nil, fmt.Errorf("create orchestrator task: %w", err)
	}

	// Orchestrator daemon must be connected via WS to dispatch
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*orchAgent.MachineID) {
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

	orchTask, err = s.waitDaemonTask(ctx, orchTask.ID)
	if err != nil {
		return nil, err
	}
	if orchTask.Status == "failed" {
		return nil, fmt.Errorf("orchestrator task failed: %s", orchTask.Error)
	}

	dispatch := ParseOrchestratorOutput(orchTask.Result)

	// Orchestrator responded directly without dispatching
	if dispatch == nil || len(dispatch.Tasks) == 0 {
		artifacts := agentMetadata(orchAgent)
		msg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("create orchestrator reply: %w", err)
		}
			s.persistArtifacts(ctx, msg, orchTask.Artifacts)
		return []*model.Message{msg}, nil
	}

	// Post orchestrator's full dispatch message (including @mentions) to group chat
	var messages []*model.Message
	{
		artifacts := agentMetadata(orchAgent)
		dispatchMsg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, artifacts, nil, nil, nil, nil)
		if err != nil {
			slog.Warn("create orchestrator dispatch message failed", "error", err)
		} else {
				s.persistArtifacts(ctx, dispatchMsg, orchTask.Artifacts)
			messages = append(messages, dispatchMsg)
		}
	}

	// Build agent name -> ID map
	agentNameToID := make(map[string]string)
	for _, ca := range convAgents {
		agentNameToID[ca.Name] = ca.AgentID
	}

	// --- 事件驱动：创建 OrchTask，异步派发 worker，不阻塞等待 ---

	// 创建 OrchTask 记录
	orchTaskRecord := &model.OrchTask{
		ConversationID:  convID,
		UserID:          userID,
		OrchAgentID:     orchAgent.ID,
		Status:          model.OrchTaskDispatching,
		DispatchPlan:    orchTask.Result,
		OriginalMessage: content,
		KBPreload:       kbPreload,
		WorkerStatus:    "{}",
		WorkerResults:   "{}",
		RoundHistory:    "[]",
	}
	if s.orchTaskRepo != nil {
		if err := s.orchTaskRepo.Create(ctx, orchTaskRecord); err != nil {
			slog.Warn("create orch task record failed", "error", err)
		}
		// 先更新状态为 workers_running，再启动 goroutine，避免竞态
		_ = s.orchTaskRepo.UpdateStatus(ctx, orchTaskRecord.ID, model.OrchTaskWorkersRunning)
	}

	// 所有 worker 并行派发：startWorkersAndWait 内部用 WaitGroup 等待全部完成后触发 summary
	orchSender := OrchSender{ID: orchAgent.ID, Name: orchAgent.Name, Avatar: orchAgent.Avatar}
	go s.startWorkersAndWait(ctx, convID, userID, dispatch.Tasks, agentNameToID, orchAgent.Name, kbPreload, orchTaskRecord.ID, orchSender)

	return messages, nil
}

func (s *OrchestratorService) persistArtifacts(ctx context.Context, msg *model.Message, artifacts []model.Artifact) {
	if msg == nil || len(artifacts) == 0 {
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

	var sb strings.Builder
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
	const maxContextLen = 4000
	result := sb.String()
	runes := []rune(result)
	if len(runes) > maxContextLen {
		result = string(runes[len(runes)-maxContextLen:])
		// 截断后跳过第一个不完整的行
		if idx := strings.IndexByte(result, '\n'); idx >= 0 {
			result = result[idx+1:]
		}
	}
	return result, nil
}

// buildRecentSummary formats the last 10 messages and returns agent names in the conversation.
func (s *OrchestratorService) buildRecentSummary(ctx context.Context, convID string, convAgents []model.ConversationAgent) (string, []string, error) {
	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		return "", nil, fmt.Errorf("list messages: %w", err)
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

	agentNames := make([]string, 0, len(convAgents))
	for _, ca := range convAgents {
		agentNames = append(agentNames, ca.Name)
	}

	return sb.String(), agentNames, nil
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

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
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

// InjectAgentConfig 将 Agent 的自定义系统提示词和工具配置注入到 dispatch 上下文前面。
// Orchestrator 系统指令由 handleOrchestratedDispatch 单独注入，此处只处理自定义配置。
func (s *OrchestratorService) InjectAgentConfig(agent *model.Agent, contextStr string, userID string) string {
	var sb strings.Builder
	if agent.SystemPrompt != "" {
		sb.WriteString("[系统指令]\n")
		sb.WriteString(agent.SystemPrompt)
		sb.WriteString("\n\n")
	}

	if agent.ToolsConfig != "" {
		sb.WriteString("[可用工具]\n")
		sb.WriteString(agent.ToolsConfig)
		sb.WriteString("\n\n")
	}

	sb.WriteString(contextStr)
	return sb.String()
}

// kbMaxInlineChars 单个文本文件内联注入的最大字符数（超过则截断并提示使用工具）
const kbMaxInlineChars = 4000

// PreloadKBContext 在 RouteMention 入口处预解析用户消息中的知识库引用，
// 生成 KB 上下文字符串。确保 Orchestrator 改写任务后 worker 仍能获取 KB 内容。
// 返回空字符串表示无引用或解析失败。
func (s *OrchestratorService) PreloadKBContext(ctx context.Context, content string, userID string) string {
	if s.kbResolver == nil {
		return ""
	}

	refs := ParseKnowledgeRefs(content)
	if len(refs) == 0 {
		return ""
	}

	var kbSection strings.Builder
	needTool := false

	for _, ref := range refs {
		kb, files, err := s.kbResolver.ResolveKnowledgeRef(ctx, userID, ref.Username, ref.KBName)
		if err != nil {
			slog.Warn("resolve knowledge ref failed", "ref", ref.Raw, "error", err)
			continue
		}
		kbSection.WriteString(fmt.Sprintf("[知识库: %s/%s (%s)]\n", ref.Username, ref.KBName, kb.Visibility))
		if len(files) == 0 {
			kbSection.WriteString("（空知识库，无文件）\n")
		} else {
			for _, f := range files {
				switch f.PreviewType {
				case "text":
					// 文本内容：限制内联长度，防止 context 膨胀导致截断
					text := f.PreviewText
					if len(text) > kbMaxInlineChars {
						text = text[:kbMaxInlineChars] + "\n...[内容已截断，使用 kb_read_file 工具读取完整内容]"
						needTool = true
					}
					kbSection.WriteString(fmt.Sprintf("- %s (%s):\n```\n%s\n```\n", f.Filename, formatFileSize(f.FileSize), text))
				case "image":
					kbSection.WriteString(fmt.Sprintf("- %s (%s, %s, 使用 kb_read_file 工具获取)\n", f.Filename, formatFileSize(f.FileSize), f.PreviewText))
					needTool = true
				case "too_large":
					kbSection.WriteString(fmt.Sprintf("- %s (%s, 文件过大，使用 kb_read_file 工具读取)\n", f.Filename, formatFileSize(f.FileSize)))
					needTool = true
				default:
					kbSection.WriteString(fmt.Sprintf("- %s (%s, 使用 kb_read_file 工具读取)\n", f.Filename, formatFileSize(f.FileSize)))
					needTool = true
				}
			}
		}
		kbSection.WriteString("\n")
	}

	if kbSection.Len() == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[引用的知识库]\n")
	sb.WriteString(kbSection.String())

	// 如果有需要工具读取的文件，注入知识库读取工具
	if needTool && s.tokenIssuer != nil && s.serverURL != "" {
		token, _, err := s.tokenIssuer.IssueAgentToken(userID)
		if err != nil {
			slog.Warn("generate kb tool token failed", "error", err)
		} else {
			sb.WriteString(GenerateKBReadTool(s.serverURL, token))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// injectKBContext 是 preloadKBContext 的包装，将预加载内容拼接到 contextStr 前面。
// 注意：该方法仅在未使用 preloadKBContext 的回退路径中调用。
func (s *OrchestratorService) injectKBContext(ctx context.Context, content string, contextStr string, userID string) string {
	preload := s.PreloadKBContext(ctx, content, userID)
	if preload == "" {
		return contextStr
	}
	return preload + contextStr
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

// truncateString truncates s to maxRunes runes, appending "..." if truncated.
func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
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
