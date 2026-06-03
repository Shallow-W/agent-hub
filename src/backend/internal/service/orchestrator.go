package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/golang-jwt/jwt/v5"
)

// OrchConvRepo queries conversation agents for @mention resolution.
type OrchConvRepo interface {
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
}

// OrchAgentRepo queries agent details and creates daemon tasks.
type OrchAgentRepo interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
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

// OrchestratorService handles @mention routing and orchestrated multi-agent dispatch.
type OrchestratorService struct {
	convRepo  OrchConvRepo
	agentRepo OrchAgentRepo
	msgRepo   MsgRepo

	jwtSecret string
	serverURL string

	kbResolver OrchKBResolver

	// 编排并发保护：同一对话同时只允许一个编排流程
	mu          sync.Mutex
	activeOrchs map[string]struct{} // convID →活跃编排
}

// SetJWTSecret sets the JWT secret for generating management tool tokens.
func (s *OrchestratorService) SetJWTSecret(secret string) {
	s.jwtSecret = secret
}

// SetServerURL sets the server base URL for management tool API calls.
func (s *OrchestratorService) SetServerURL(url string) {
	s.serverURL = url
}

// SetKBResolver sets the knowledge base resolver for injecting KB context.
func (s *OrchestratorService) SetKBResolver(resolver OrchKBResolver) {
	s.kbResolver = resolver
}

// NewOrchestratorService creates a new orchestrator service.
func NewOrchestratorService(convRepo OrchConvRepo, agentRepo OrchAgentRepo, msgRepo MsgRepo) *OrchestratorService {
	return &OrchestratorService{
		convRepo:    convRepo,
		agentRepo:   agentRepo,
		msgRepo:     msgRepo,
		activeOrchs: make(map[string]struct{}),
	}
}

// RouteMention is the main entry point called when a message contains @mentions.
func (s *OrchestratorService) RouteMention(ctx context.Context, convID, userID, content string) (*RouteResult, error) {
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

		if agent.Type == "orchestrator" {
			// 并发保护：同一对话同时只允许一个编排
			s.mu.Lock()
			if _, active := s.activeOrchs[convID]; active {
				s.mu.Unlock()
				slog.Warn("orchestration already active for conversation", "conv_id", convID)
				// 返回提示消息给用户（直接构造，避免并发调用 msgRepo.Create）
				result.AgentMessages = append(result.AgentMessages, &model.Message{
					ConversationID: convID,
					Role:           "system",
					Content:        "编排器正忙，请稍后再试。",
					CreatedAt:      time.Now(),
				})
				continue
			}
			s.activeOrchs[convID] = struct{}{}
			s.mu.Unlock()

			msgs, err := s.handleOrchestratedDispatch(ctx, convID, userID, agent, content, convAgents)

			s.mu.Lock()
			delete(s.activeOrchs, convID)
			s.mu.Unlock()

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
			msg, err := s.dispatchSingleAgent(ctx, convID, userID, agent, content)
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

// dispatchSingleAgent dispatches to a single non-orchestrator agent.
func (s *OrchestratorService) dispatchSingleAgent(ctx context.Context, convID, userID string, agent *model.Agent, content string) (*model.Message, error) {
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

	// 注入知识库上下文
	kbCtx := s.injectKBContext(ctx, content, "", userID)

	// 注入 Agent 系统提示词和工具配置到 context
	agentCtx := s.injectAgentConfig(agent, kbCtx, userID)

	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, content, agentCtx)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}

	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts, _ := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, string(artifacts), nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	return msg, nil
}

// handleOrchestratedDispatch runs the full orchestrator flow: dispatch to orchestrator, parse output, fan out to workers.
func (s *OrchestratorService) handleOrchestratedDispatch(ctx context.Context, convID, userID string, orchAgent *model.Agent, content string, convAgents []model.ConversationAgent) ([]*model.Message, error) {
	if orchAgent.MachineID == nil || *orchAgent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}
	// 确保 orchestratorName 非空，后续 dispatchWorker 依赖它构建上下文
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

	orchTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, orchAgent.ID, *orchAgent.MachineID, orchAgent.CLITool, fullPrompt, "")
	if err != nil {
		return nil, fmt.Errorf("create orchestrator task: %w", err)
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
		artifacts, _ := json.Marshal(map[string]string{
			"agent_id":   orchAgent.ID,
			"agent_name": orchAgent.Name,
			"cli_tool":   orchAgent.CLITool,
		})
		msg, err := s.msgRepo.Create(ctx, convID, "assistant", orchTask.Result, string(artifacts), nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("create orchestrator reply: %w", err)
		}
		return []*model.Message{msg}, nil
	}

	// Post orchestrator preamble if present
	var messages []*model.Message
	if dispatch.Preamble != "" {
		artifacts, _ := json.Marshal(map[string]string{
			"agent_id":   orchAgent.ID,
			"agent_name": orchAgent.Name,
			"cli_tool":   orchAgent.CLITool,
		})
		preambleMsg, err := s.msgRepo.Create(ctx, convID, "assistant", dispatch.Preamble, string(artifacts), nil, nil, nil, nil)
		if err != nil {
			slog.Warn("create orchestrator preamble message failed", "error", err)
		} else {
			messages = append(messages, preambleMsg)
		}
	}

	// Build agent name -> ID map
	agentNameToID := make(map[string]string)
	for _, ca := range convAgents {
		agentNameToID[ca.Name] = ca.AgentID
	}

	// Dispatch worker tasks
	depResults := make(map[string]string) // agentName -> result summary

	// Collect parallel tasks and process sequential/dependent tasks in order
	var parallelTasks []DispatchTask
	for _, t := range dispatch.Tasks {
		if t.Sequential || t.DependsOn != "" {
			// First execute any accumulated parallel tasks
			if len(parallelTasks) > 0 {
				workerMsgs := s.dispatchParallel(ctx, convID, userID, parallelTasks, agentNameToID, depResults, orchAgent.Name)
				messages = append(messages, workerMsgs...)
				parallelTasks = nil
			}
			// Execute sequential/dependent task
			workerMsg := s.dispatchSequential(ctx, convID, userID, t, agentNameToID, depResults, orchAgent.Name)
			if workerMsg != nil {
				messages = append(messages, workerMsg)
			}
		} else {
			parallelTasks = append(parallelTasks, t)
		}
	}
	// Flush remaining parallel tasks
	if len(parallelTasks) > 0 {
		workerMsgs := s.dispatchParallel(ctx, convID, userID, parallelTasks, agentNameToID, depResults, orchAgent.Name)
		messages = append(messages, workerMsgs...)
	}

	return messages, nil
}

// dispatchParallel creates daemon tasks for multiple agents simultaneously and waits for all.
func (s *OrchestratorService) dispatchParallel(ctx context.Context, convID, userID string, tasks []DispatchTask, agentNameToID map[string]string, depResults map[string]string, orchestratorName string) []*model.Message {
	type taskResult struct {
		index     int
		msg       *model.Message
		agentName string
		failed    bool
	}

	// 快照 depResults 避免并发 map 读写竞争
	depSnapshot := make(map[string]string, len(depResults))
	for k, v := range depResults {
		depSnapshot[k] = v
	}

	resultCh := make(chan taskResult, len(tasks))
	for i, t := range tasks {
		go func(idx int, task DispatchTask) {
			msg, err := s.dispatchWorker(ctx, convID, userID, task, agentNameToID, depSnapshot, orchestratorName)
			if err != nil {
				slog.Warn("parallel dispatch failed", "agent", task.AgentName, "error", err)
				resultCh <- taskResult{index: idx, msg: nil, agentName: task.AgentName, failed: true}
				return
			}
			resultCh <- taskResult{index: idx, msg: msg, agentName: task.AgentName}
		}(i, t)
	}

	var results []*model.Message
	for range tasks {
		tr := <-resultCh
		if tr.msg != nil {
			depResults[tr.agentName] = truncateString(tr.msg.Content, 500)
			results = append(results, tr.msg)
		} else if tr.failed {
			depResults[tr.agentName] = "[任务失败]"
		}
	}
	return results
}

// dispatchSequential creates a daemon task for one agent, waits, then returns.
func (s *OrchestratorService) dispatchSequential(ctx context.Context, convID, userID string, task DispatchTask, agentNameToID map[string]string, depResults map[string]string, orchestratorName string) *model.Message {
	msg, err := s.dispatchWorker(ctx, convID, userID, task, agentNameToID, depResults, orchestratorName)
	if err != nil {
		slog.Warn("sequential dispatch failed", "agent", task.AgentName, "error", err)
		depResults[task.AgentName] = "[任务失败]"
		return nil
	}
	depResults[task.AgentName] = truncateString(msg.Content, 500)
	return msg
}

// dispatchWorker dispatches to a single worker agent and returns its reply message.
func (s *OrchestratorService) dispatchWorker(ctx context.Context, convID, userID string, task DispatchTask, agentNameToID map[string]string, depResults map[string]string, orchestratorName string) (*model.Message, error) {
	agentID, ok := agentNameToID[task.AgentName]
	if !ok {
		return nil, fmt.Errorf("agent %q not found in conversation", task.AgentName)
	}

	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get worker agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("worker agent %q not found", task.AgentName)
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, fmt.Errorf("worker agent %q offline", task.AgentName)
	}

	dispatchCtx, err := s.buildDispatchContext(ctx, convID, task, depResults, orchestratorName)
	if err != nil {
		slog.Warn("build worker dispatch context failed", "agent", task.AgentName, "error", err)
		dispatchCtx = ""
	}

	// 注入知识库上下文
	dispatchCtx = s.injectKBContext(ctx, task.Task, dispatchCtx, userID)

	// 注入 Agent 的系统提示词和工具配置
	dispatchCtx = s.injectAgentConfig(agent, dispatchCtx, userID)

	daemonTask, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, task.Task, dispatchCtx)
	if err != nil {
		return nil, fmt.Errorf("create worker daemon task: %w", err)
	}

	daemonTask, err = s.waitDaemonTask(ctx, daemonTask.ID)
	if err != nil {
		return nil, err
	}
	if daemonTask.Status == "failed" {
		return nil, fmt.Errorf("worker daemon task failed: %s", daemonTask.Error)
	}

	artifacts, _ := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", daemonTask.Result, string(artifacts), nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create worker reply: %w", err)
	}
	return msg, nil
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
			// Extract agent name from artifacts if available
			var a struct {
				AgentName string `json:"agent_name"`
			}
			if m.ArtifactsJSON != "" {
				_ = json.Unmarshal([]byte(m.ArtifactsJSON), &a)
			}
			if a.AgentName != "" {
				role = a.AgentName
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
			var a struct {
				AgentName string `json:"agent_name"`
			}
			if m.ArtifactsJSON != "" {
				_ = json.Unmarshal([]byte(m.ArtifactsJSON), &a)
			}
			if a.AgentName != "" {
				role = a.AgentName
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

// waitDaemonTask polls for daemon task completion (600ms interval, 120s timeout).
func (s *OrchestratorService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := s.agentRepo.GetDaemonTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get daemon task: %w", err)
		}
		if task != nil && (task.Status == "completed" || task.Status == "failed") {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrMsgAgentTimeout
		case <-ticker.C:
		}
	}
}

// injectAgentConfig 将 Agent 的系统提示词和工具配置注入到 dispatch 上下文前面。
// 如果 agent.EnableManagementTools 为 true，还会生成管理工具并追加到工具配置中。
// 如果消息中包含 {{用户名/知识库名}} 引用且 kbResolver 已设置，会解析引用并注入文件上下文。
func (s *OrchestratorService) injectAgentConfig(agent *model.Agent, contextStr string, userID string) string {
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
	if agent.EnableManagementTools && s.jwtSecret != "" && s.serverURL != "" {
		token, _, err := s.generateMgmtToken(userID)
		if err != nil {
			slog.Warn("generate management token failed", "agent_id", agent.ID, "error", err)
		} else {
			sb.WriteString("[可用工具]\n")
			sb.WriteString(GenerateManagementTools(s.serverURL, token))
			sb.WriteString("\n\n")
		}
	}
	sb.WriteString(contextStr)
	return sb.String()
}

// injectKBContext 解析消息中的 {{用户名/知识库名}} 引用并将文件列表注入到上下文中。
// 如果没有引用则原样返回 contextStr。
func (s *OrchestratorService) injectKBContext(ctx context.Context, content string, contextStr string, userID string) string {
	if s.kbResolver == nil {
		return contextStr
	}

	refs := ParseKnowledgeRefs(content)
	if len(refs) == 0 {
		return contextStr
	}

	var kbSection strings.Builder
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
				kbSection.WriteString(fmt.Sprintf("- %s (%s)\n", f.Filename, formatFileSize(f.FileSize)))
			}
		}
		kbSection.WriteString("\n")
	}

	if kbSection.Len() == 0 {
		return contextStr
	}

	var sb strings.Builder
	sb.WriteString("[引用的知识库]\n")
	sb.WriteString(kbSection.String())
	sb.WriteString(contextStr)
	return sb.String()
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

// generateMgmtToken generates a scoped JWT token for management tool usage.
// Token has a short 5-minute lifetime and "agent_management" scope to distinguish
// from regular user tokens.
func (s *OrchestratorService) generateMgmtToken(userID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)
	claims := jwt.MapClaims{
		"user_id": userID,
		"scope":   "agent_management",
		"iat":     now.Unix(),
		"exp":     expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign management token: %w", err)
	}
	return tokenStr, expiresAt, nil
}

// truncateString truncates s to maxRunes runes, appending "..." if truncated.
func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
