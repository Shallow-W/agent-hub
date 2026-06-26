package port

import (
	"context"

	"github.com/agent-hub/backend/internal/model"
)

// DeploymentStrategy 表示一种部署产物的方式（preview / github / 未来 cloudflare_pages 等）。
//
// 新增部署模式只需实现此接口并调用 DeploymentService.RegisterStrategy 注册，
// 不需要修改 handler 的 switch 或加专用 PublishXxxByConversation 方法。
//
// 实现方负责：鉴权（checkAccess）、落盘、对外发布（如 GitHub Pages 推送）、
// 写入 deployment 记录并返回已 decorate 的结果。
type DeploymentStrategy interface {
	// Mode 返回策略标识，对应 Deployment.Mode 字段与请求体 mode 字段。
	// 同一 DeploymentService 内必须唯一。
	Mode() string

	// Enabled 表示该策略当前是否可用（依赖项已配置）。
	// 例如未配置 GitHub token 时，github 策略 Enabled() 返回 false。
	// handler 在派发前据此拒绝，避免走到一半才报错。
	Enabled() bool

	// Deploy 把指定 artifact 部署出去。artifact 已由调用方按 conversation+name 查找好，
	// 策略只需关心发布本身（含鉴权与落盘）。
	// 返回已 decorate（含公网基址、下载链接）的部署记录。
	Deploy(ctx context.Context, art *model.Artifact, userID string) (*model.Deployment, error)
}
