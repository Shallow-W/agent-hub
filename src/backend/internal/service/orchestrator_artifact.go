package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
)

// AI 编辑产物相关错误（供 handler 映射 HTTP 状态码）。
var (
	// ErrArtifactEditNotFound 产物血缘根不存在
	ErrArtifactEditNotFound = errors.New("产物不存在")
	// ErrArtifactEditNoPerm 当前用户不是产物所属对话成员
	ErrArtifactEditNoPerm = errors.New("无权编辑此产物")
	// ErrArtifactEditUnsupported 仅支持 code 类型产物的 AI 编辑
	ErrArtifactEditUnsupported = errors.New("仅支持代码产物的 AI 编辑")
	// ErrArtifactEditNoAgent 对话内没有可用的已连接 Agent 执行编辑
	ErrArtifactEditNoAgent = errors.New("没有可用的已连接 Agent")
	// ErrArtifactEditInvalid 编辑指令为空等参数错误
	ErrArtifactEditInvalid = errors.New("编辑指令不能为空")
)

// AIEditArtifact 用 AI 改写一个 code 产物，把结果存为该产物血缘的新版本。
// 专用入口：复用 Dispatcher.DispatchPlan 机制（与 dispatchWorker 同款派发流程），
// 但**不**在对话里建 assistant 消息，只 createVersion。
//
// 流程：
//  1. 鉴权（rootID → conversation → 当前用户是成员）+ 取最新版本（必须是 code 产物）。
//  2. 选一个已连接 daemon 的 agent（优先产物来源 agent，取不到则用对话内任一已连接 agent）。
//  3. 构造编辑 prompt（要求只返回完整修改后代码、用代码块包裹）。
//  4. 通过 Dispatcher.DispatchPlan 派发：CreateDaemonTask → IsConnected → RegisterTaskPromise →
//     SendToMachine → waitDaemonTask，用自定义 ResultHandler 不落 message。
//  5. 从结果提取修改后代码。
//  6. CreateVersion 存为新版本并返回。
func (s *OrchestratorService) AIEditArtifact(ctx context.Context, rootID, userID, instruction, selection string) (*model.Artifact, error) {
	if s.artifactRepo == nil {
		return nil, fmt.Errorf("artifact repo not configured")
	}
	if strings.TrimSpace(instruction) == "" {
		return nil, ErrArtifactEditInvalid
	}

	// 1. 鉴权 + 取产物
	convID, err := s.artifactRepo.GetConversationIDByRoot(ctx, rootID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrArtifactEditNotFound
		}
		return nil, fmt.Errorf("resolve artifact conversation: %w", err)
	}
	if err := s.checkArtifactMember(ctx, convID, userID); err != nil {
		return nil, err
	}

	latest, err := s.artifactRepo.GetLatestByRoot(ctx, rootID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrArtifactEditNotFound
		}
		return nil, fmt.Errorf("get latest artifact: %w", err)
	}
	if latest.Type != "code" {
		return nil, ErrArtifactEditUnsupported
	}

	// 2. 选已连接 agent（优先产物来源 agent）
	agent, err := s.pickConnectedAgent(ctx, convID, userID, latest.MessageID)
	if err != nil {
		return nil, err
	}

	// 3. 构造编辑 prompt
	prompt := buildArtifactEditPrompt(latest.Language, latest.Content, selection, instruction)

	// 4. 复用 dispatchWorker 同款派发（专用入口，不建 assistant 消息）
	result, err := s.runDaemonEdit(ctx, convID, userID, agent, prompt)
	if err != nil {
		return nil, err
	}

	// 5. 提取修改后代码
	newCode := extractEditedCode(result)
	if newCode == "" {
		return nil, fmt.Errorf("agent 未返回可用的修改结果")
	}

	// 6. 存为新版本（沿用原语言/文件名）
	created, err := s.artifactRepo.CreateVersion(ctx, rootID, model.Artifact{
		Type:     "code",
		Content:  newCode,
		Language: latest.Language,
		Filename: latest.Filename,
	})
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrArtifactEditNotFound
		}
		return nil, fmt.Errorf("create artifact version: %w", err)
	}
	return created, nil
}

// checkArtifactMember 校验用户是对话成员（或创建者兜底），与 ArtifactService.checkAccess 一致。
func (s *OrchestratorService) checkArtifactMember(ctx context.Context, convID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrArtifactEditNotFound
	}
	member, err := s.convRepo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member != nil {
		return nil
	}
	// 兜底：创建者可能尚未写入成员表
	if conv.UserID == userID {
		return nil
	}
	return ErrArtifactEditNoPerm
}

// pickConnectedAgent 选一个已连接 daemon 的 agent。
// 优先用产物来源 agent（从归属消息的 artifacts_json.agent_id 取），取不到/未连接则退回对话内任一已连接 agent。
func (s *OrchestratorService) pickConnectedAgent(ctx context.Context, convID, userID, messageID string) (*model.Agent, error) {
	// 优先：产物来源 agent
	if srcID := s.sourceAgentID(ctx, messageID); srcID != "" {
		if agent := s.connectedAgentByID(ctx, srcID); agent != nil {
			return agent, nil
		}
	}

	// 退回：对话内任一已连接 agent
	convAgents, err := s.convRepo.ListAgents(ctx, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversation agents: %w", err)
	}
	for _, ca := range convAgents {
		if agent := s.connectedAgentByID(ctx, ca.AgentID); agent != nil {
			return agent, nil
		}
	}
	return nil, ErrArtifactEditNoAgent
}

// sourceAgentID 从归属消息的 artifacts_json 取产物来源 agent_id，失败返回空串。
func (s *OrchestratorService) sourceAgentID(ctx context.Context, messageID string) string {
	if messageID == "" {
		return ""
	}
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil || msg == nil || msg.ArtifactsJSON == "" {
		return ""
	}
	var meta struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(msg.ArtifactsJSON), &meta); err != nil {
		return ""
	}
	return meta.AgentID
}

// connectedAgentByID 取 agent 并校验其 daemon 已通过 WS 连接，未连接返回 nil。
func (s *OrchestratorService) connectedAgentByID(ctx context.Context, agentID string) *model.Agent {
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return nil
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil
	}
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		return nil
	}
	return agent
}

// runDaemonEdit 复用 dispatchWorker 的派发机制执行一次 daemon 任务，返回结果文本（含产物时优先取 code 产物 content）。
//
// P9 后通过 Dispatcher.DispatchPlan 走统一派发路径（CreateDaemonTask → WS → wait），
// 但用自定义 ResultHandler 在 daemon 返回后**不落 message**，只把 task 通过 closure
// 传出供调用方做 CreateVersion。零行为变更：错误码语义、agentQueue 串行化、code 产物
// 优先取值策略完全保留。
func (s *OrchestratorService) runDaemonEdit(ctx context.Context, convID, userID string, agent *model.Agent, prompt string) (string, error) {
	// 派发护栏：通过 AgentQueue 串行化同一 agent 的派发（与 dispatchSingleAgent 一致）。
	var result string
	err := s.agentQueue.Run(ctx, agent.ID, func() error {
		// 通过 closure 捕获 task，让 ResultHandler 把 daemon 返回的 task 透出给外层。
		var capturedTask *model.DaemonTask
		plan := DispatchPlan{
			Input: DispatchInput{
				ConvID:          convID,
				UserID:          userID,
				Agent:           agent,
				Prompt:          prompt,
				ContextMessages: "", // 编辑场景不带历史上下文（与原行为一致）
			},
			// PromptBuilder 留 nil → defaultPromptBuilder 直接用 input.Prompt。
			ResultHandler: func(_ context.Context, task *model.DaemonTask) (*model.Message, error) {
				capturedTask = task
				// 不落 message：调用方会用 task.Artifacts / task.Result 做 CreateVersion。
				return nil, nil
			},
		}
		res, derr := s.dispatcher.DispatchPlan(ctx, plan, DispatchHooks{})
		if derr != nil {
			// 错误码语义保留：daemon 未连接时映射回 ErrArtifactEditNoAgent
			// （AIEditArtifact 的 HTTP 错误响应依赖此 sentinel）。
			if errors.Is(derr, ErrDaemonNotConnected) {
				return ErrArtifactEditNoAgent
			}
			return derr
		}
		// DispatchPlan 返回的 res.Task 与 closure 捕获的 capturedTask 是同一对象，
		// 二者均可读，这里以 closure 捕获的为准（语义更明确）。
		task := capturedTask
		if task == nil {
			task = res.Task
		}
		if task == nil {
			return fmt.Errorf("edit daemon task returned no result")
		}

		// 优先取产物里第一个 code 产物的 content（与原 runDaemonEdit 行为一致）。
		for i := range task.Artifacts {
			if task.Artifacts[i].Type == "code" && task.Artifacts[i].Content != "" {
				result = task.Artifacts[i].Content
				return nil
			}
		}
		result = task.Result
		return nil
	})
	return result, err
}

// buildArtifactEditPrompt 构造代码编辑 prompt：要求只返回完整修改后代码、用代码块包裹。
func buildArtifactEditPrompt(language, baseContent, selection, instruction string) string {
	lang := language
	if lang == "" {
		lang = "代码"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "你是代码编辑助手。下面是一段 %s，请按用户要求修改，然后只返回**完整的修改后代码**（用 ``` 代码块包裹），不要解释。\n\n", lang)
	sb.WriteString("【完整代码】\n")
	sb.WriteString(baseContent)
	sb.WriteString("\n\n")
	if strings.TrimSpace(selection) != "" {
		sb.WriteString("【用户选中的片段】（若有）\n")
		sb.WriteString(selection)
		sb.WriteString("\n\n")
	}
	sb.WriteString("【修改要求】\n")
	sb.WriteString(instruction)
	return sb.String()
}

// extractEditedCode 从结果文本提取修改后代码：
// 优先抽第一个围栏代码块内容；没有围栏则用整段 trim。
func extractEditedCode(result string) string {
	if code, ok := firstFencedBlock(result); ok {
		return code
	}
	return strings.TrimSpace(result)
}

// firstFencedBlock 提取第一个 ``` 围栏代码块的内容（剥离起始 ```lang 行与结尾 ```）。
func firstFencedBlock(text string) (string, bool) {
	idx := strings.Index(text, "```")
	if idx < 0 {
		return "", false
	}
	rest := text[idx+3:]
	// 跳过开围栏后的语言标识行
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		return "", false
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", false
	}
	code := strings.TrimRight(rest[:end], "\n")
	if strings.TrimSpace(code) == "" {
		return "", false
	}
	return code, true
}
