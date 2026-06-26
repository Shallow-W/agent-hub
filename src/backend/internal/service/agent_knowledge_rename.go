package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

const knowledgeFilenameSuggestionTimeout = 90 * time.Second

// SuggestKnowledgeFilename 选择任意可用在线智能体，为知识库文件生成一个明确的新文件名。
func (s *AgentService) SuggestKnowledgeFilename(ctx context.Context, userID string, file model.KnowledgeFile, text string) (string, error) {
	if userID == "" || file.ID == "" {
		return "", ErrAgentInvalidInput
	}
	agent, err := s.pickKnowledgeRenameAgent(ctx, userID)
	if err != nil {
		return "", err
	}

	prompt := buildKnowledgeFilenamePrompt(file, text)
	task, err := s.repo.CreateDaemonTask(ctx, userID, "", agent.ID, *agent.MachineID, agent.CLITool, prompt, "")
	if err != nil {
		return "", fmt.Errorf("create knowledge rename task: %w", err)
	}

	ch := s.daemonHub.RegisterTaskPromise(task.ID)
	defer s.daemonHub.RemoveTaskPromise(task.ID)

	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":         task.ID,
			"cli_tool":        agent.CLITool,
			"prompt":          prompt,
			"agent_id":        agent.ID,
			"conversation_id": "",
			"user_id":         userID,
		},
	}); err != nil {
		return "", fmt.Errorf("dispatch knowledge rename task: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, knowledgeFilenameSuggestionTimeout)
	defer cancel()

	select {
	case result := <-ch:
		if result == nil {
			return "", ErrMsgAgentTimeout
		}
		if result.Error != "" {
			return "", fmt.Errorf("daemon task failed: %s", result.Error)
		}
		return strings.TrimSpace(result.Result), nil
	case <-waitCtx.Done():
		return "", ErrMsgAgentTimeout
	}
}

func (s *AgentService) pickKnowledgeRenameAgent(ctx context.Context, userID string) (*model.Agent, error) {
	if s.daemonHub == nil {
		return nil, ErrKBRenameNoAgent
	}
	agents, err := s.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range agents {
		agent := &agents[i]
		if agent.Status != "online" || agent.MachineID == nil || *agent.MachineID == "" || agent.CLITool == "" {
			continue
		}
		if s.daemonHub.IsConnected(*agent.MachineID) {
			return agent, nil
		}
	}
	return nil, ErrKBRenameNoAgent
}

func buildKnowledgeFilenamePrompt(file model.KnowledgeFile, text string) string {
	content := strings.TrimSpace(text)
	if content == "" {
		content = "（该文件没有可抽取的正文预览，请根据原文件名、MIME 类型和文件大小概括命名。）"
	}

	return fmt.Sprintf(`你正在为知识库文件做智能重命名。请扫读文件内容，概括出一个清晰、明确、便于检索的文件名。

要求：
- 只输出一个文件名，不要解释，不要 Markdown，不要 JSON。
- 文件名应概括主题，避免“文档”“资料”“未命名”等空泛词。
- 保持简洁，建议 8 到 30 个中文字符或等价长度。
- 不要输出路径。
- 不需要强行保留扩展名，系统会使用原文件扩展名。

原文件名：%s
MIME 类型：%s
文件大小：%d bytes
预览类型：%s

文件内容：
%s`, file.Filename, file.MimeType, file.FileSize, file.PreviewType, content)
}
