package service

import (
	"context"

	"github.com/agent-hub/backend/internal/model"
)

// ContextInput 收集所有上下文构建可能用到的原料。
// 每个 ContextBuilder 按需读取自己关心的字段，忽略其他。
type ContextInput struct {
	ConvID         string
	UserID         string
	Agent          *model.Agent // 可能为 nil（RouteMention 入口时尚未解析到具体 agent）
	Content        string       // 用户原始消息内容（用于解析 {{user/KB}} 引用等）
	Attachments    []model.MessageAttachment
	KBPreload      string // 已预加载的 KB 上下文（避免重复解析）
	IsOrchestrator bool   // 是否用于 orch 角色（影响是否叠加 OrchestratorSystemPrompt）

	// FanoutFrame 是 FanoutFrameBuilder 读取的专用原料。nil 时 builder 返回 current 不变。
	// 由调用方按需填充（路径 C 异步 fanout 变体 dispatchOrchWorker）。
	FanoutFrame *FanoutFrameInput
}

// ContextBuilder 构建一段上下文，追加/前置到 current 上。
// 实现方决定自己的位置（前置/后置），返回新的累积字符串。
// 当 Builder 没有内容要注入时应原样返回 current（不要额外补 \n\n）。
type ContextBuilder interface {
	Build(ctx context.Context, in ContextInput, current string) string
}

// ContextChain 把多个 ContextBuilder 串成管道。
// Build 顺序 = 注册顺序；每个 builder 拿到上一个的输出作为 current。
type ContextChain struct {
	builders []ContextBuilder
}

// NewContextChain 构造一个 chain。注册顺序决定执行顺序；
// 越靠后的 builder 其输出越靠外（最左侧）。
func NewContextChain(builders ...ContextBuilder) *ContextChain {
	return &ContextChain{builders: builders}
}

// Build 依次执行所有 builder，返回最终累积的上下文字符串。
func (c *ContextChain) Build(ctx context.Context, in ContextInput) string {
	var current string
	for _, b := range c.builders {
		current = b.Build(ctx, in, current)
	}
	return current
}
