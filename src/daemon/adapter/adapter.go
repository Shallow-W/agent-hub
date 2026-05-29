package adapter

import "context"

// Artifact 是从 Agent 输出中解析出的结构化产物
type Artifact struct {
	Type     string `json:"type"`
	Language string `json:"language,omitempty"`
	Filename string `json:"filename,omitempty"`
	Content  string `json:"content,omitempty"`
	URL      string `json:"url,omitempty"`
	Title    string `json:"title,omitempty"`
}

// StreamChunk 是不同 Agent 输出的统一流式格式
type StreamChunk struct {
	Type     string    `json:"type"`
	Content  string    `json:"content"`
	Artifact *Artifact `json:"artifact,omitempty"`
}

// Adapter 统一不同 Agent CLI 的生命周期和流式输出
type Adapter interface {
	Name() string
	Start(ctx context.Context, prompt string, systemPrompt string) error
	Stream() <-chan StreamChunk
	Stop() error
	IsRunning() bool
}
