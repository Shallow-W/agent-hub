package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/pkg/ws"
	"github.com/google/uuid"
)

// daemonBrowseFilesTool 是 daemon 端 executeTask 的内置 cli_tool 分支，
// 走文件浏览只读 RPC（与 __agenthub_open_path__ 同一通道）。
const daemonBrowseFilesTool = "__agenthub_browse_files__"

// BrowseAgentFilesTimeout 文件浏览 RPC 同步等待上限。daemon 端 git 操作另有
// 10s 超时兜底，这里给足余量容纳多目录列表 + 大文件读取。
const BrowseAgentFilesTimeout = 20 * time.Second

// BrowseRequest 文件浏览请求。Action 决定 daemon 端分支：
//   - tree：根目录快照 + git 改动文件清单（打开抽屉时调）
//   - list：单层展开子目录（懒加载）
//   - read：读单文件内容（点文件查看）
//   - zip ：收集整目录文件数组（后端打包下载）
//   - log ：git 历史（某文件的 commit 列表，供版本切换）
//   - show：读某 commit 下某文件的内容（git show rev:path）
//   - status：查多文件 git 状态（added/modified/deleted）
//   - diff：拿某文件前后内容（默认工作区 vs HEAD）
type BrowseRequest struct {
	Action  string   // tree | list | read | zip | log | show | status | diff
	Path    string   // 绝对路径（list/read/zip/log/show/diff 用，相对 repoRoot 校验）
	Rev     string   // git base ref（tree 用）/ commit hash（show/diff 用）
	WorkDir string   // 来自前端 project/diff 卡片的绝对路径（解耦：路径生产与浏览分离）
	Files   []string // status action 用：要查状态的文件相对路径列表
}

// BrowseResult daemon 返回的原始 JSON。后端只透传不解析——三层契约由前端/daemon
// 直接对齐，后端不成为中间数据结构的维护者（避免 daemon 字段变化时三层都要改）。
type BrowseResult = json.RawMessage

// browseFilesPayload 发给 daemon 的 task.dispatch prompt（JSON 字符串）。
type browseFilesPayload struct {
	WorkDir string   `json:"work_dir"` // 来自卡片，daemon 端不再 fallback process.cwd()
	Action  string   `json:"action"`
	Path    string   `json:"path,omitempty"`
	Rev     string   `json:"rev,omitempty"`
	Files   []string `json:"files,omitempty"` // status action 用
}

// BrowseAgentFiles 通过同步 RPC 让 agent 所在 daemon 浏览其机器上的文件。
// 复用 OpenDaemonSkillLocation 的 taskID promise 通道：
// RegisterTaskPromise → SendToMachine(task.dispatch) → 等 task.complete → ResolveTask 投递结果。
//
// 鉴权：agent 必须属于当前 user 且 MachineID 已连接 daemon。
// work_dir 来源：前端 project 卡片（agent 通过 render_card 上报），
// 是路径生产与文件浏览解耦的唯一契约——后端不关心它怎么来，只透传给 daemon。
func (s *AgentService) BrowseAgentFiles(ctx context.Context, userID, agentID string, req BrowseRequest) (*BrowseResult, error) {
	action := strings.TrimSpace(req.Action)
	if userID == "" || agentID == "" || action == "" {
		return nil, ErrAgentInvalidInput
	}
	// work_dir 来自卡片，所有 action 都依赖它（daemon 不再 fallback process.cwd()）
	if strings.TrimSpace(req.WorkDir) == "" {
		return nil, ErrAgentInvalidInput
	}
	switch action {
	case "tree", "list", "read", "zip", "log", "show", "status", "diff":
		// 合法 action
	default:
		return nil, ErrAgentInvalidInput
	}

	agent, err := s.repo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	// 鉴权：agent 属于当前 user（系统 agent UserID 为 nil，禁止浏览）
	if agent.UserID == nil || *agent.UserID != userID {
		return nil, ErrAgentNotFound
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrAgentOffline
	}
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		return nil, ErrAgentOffline
	}

	payload, err := json.Marshal(browseFilesPayload{
		WorkDir: req.WorkDir,
		Action:  action,
		Path:    req.Path,
		Rev:     req.Rev,
		Files:   req.Files,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal browse payload: %w", err)
	}

	taskID := uuid.NewString()
	ch := s.daemonHub.RegisterTaskPromise(taskID)
	defer s.daemonHub.RemoveTaskPromise(taskID)

	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":         taskID,
			"cli_tool":        daemonBrowseFilesTool,
			"prompt":          string(payload),
			"agent_id":        agentID,
			"conversation_id": "",
		},
	}); err != nil {
		return nil, fmt.Errorf("send browse task: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, BrowseAgentFilesTimeout)
	defer cancel()

	select {
	case result := <-ch:
		if result == nil {
			return nil, ErrMsgAgentTimeout
		}
		if result.Error != "" {
			return nil, fmt.Errorf("%s", result.Error)
		}
		// daemon 的 browseFiles 返回 JSON 字符串，直接透传给前端
		raw := json.RawMessage(result.Result)
		return &raw, nil
	case <-ctx.Done():
		return nil, ErrMsgAgentTimeout
	}
}
