package adapter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// CommandAdapter 用最小公共能力包装 stdin/stdout 型 CLI
type CommandAdapter struct {
	name    string
	command string
	args    []string

	cmd     *exec.Cmd
	cancel  context.CancelFunc
	stream  chan StreamChunk
	running bool
	mu      sync.RWMutex
}

// NewCommandAdapter 创建通用 CLI 适配器
func NewCommandAdapter(name, command string, args []string) *CommandAdapter {
	return &CommandAdapter{
		name:    name,
		command: command,
		args:    args,
		stream:  make(chan StreamChunk, 32),
	}
}

// Name 返回适配器名称
func (a *CommandAdapter) Name() string {
	return a.name
}

// Start 启动 CLI 并把提示词写入 stdin
func (a *CommandAdapter) Start(ctx context.Context, prompt string, systemPrompt string) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("%s already running", a.name)
	}

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, a.command, a.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		a.mu.Unlock()
		return fmt.Errorf("open stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		a.mu.Unlock()
		return fmt.Errorf("open stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		a.mu.Unlock()
		return fmt.Errorf("open stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		a.mu.Unlock()
		return fmt.Errorf("start %s: %w", a.command, err)
	}

	a.cmd = cmd
	a.cancel = cancel
	a.running = true
	a.mu.Unlock()

	go a.writePrompt(stdin, prompt, systemPrompt)
	go a.readPipe(stdout, "text")
	go a.readPipe(stderr, "error")
	go a.wait()
	return nil
}

// Stream 返回统一流式输出通道
func (a *CommandAdapter) Stream() <-chan StreamChunk {
	return a.stream
}

// Stop 停止正在运行的 CLI
func (a *CommandAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return nil
	}
	if a.cancel != nil {
		a.cancel()
	}
	a.running = false
	return nil
}

// IsRunning 返回进程状态
func (a *CommandAdapter) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

func (a *CommandAdapter) writePrompt(stdin io.WriteCloser, prompt string, systemPrompt string) {
	defer stdin.Close()
	parts := []string{}
	if strings.TrimSpace(systemPrompt) != "" {
		parts = append(parts, systemPrompt)
	}
	parts = append(parts, prompt)
	if _, err := io.WriteString(stdin, strings.Join(parts, "\n\n")); err != nil {
		a.stream <- StreamChunk{Type: "error", Content: err.Error()}
	}
}

func (a *CommandAdapter) readPipe(pipe io.Reader, chunkType string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		a.stream <- StreamChunk{
			Type:    chunkType,
			Content: scanner.Text() + "\n",
		}
	}
}

func (a *CommandAdapter) wait() {
	err := a.cmd.Wait()
	if err != nil {
		a.stream <- StreamChunk{Type: "error", Content: err.Error()}
	}
	a.stream <- StreamChunk{Type: "done"}

	a.mu.Lock()
	a.running = false
	a.cmd = nil
	a.cancel = nil
	a.mu.Unlock()
}
