package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

const daemonSendBuf = 64

// TaskResult daemon 任务执行结果
type TaskResult struct {
	TaskID    string           `json:"task_id"`
	Result    string           `json:"result"`
	Error     string           `json:"error"`
	Artifacts []ArtifactResult `json:"artifacts,omitempty"`
	Cards     []map[string]any `json:"cards,omitempty"` // render_card 工具渲染的交互式卡片
}

// ArtifactResult daemon 解析出的结构化产物（随 task.complete 上行）。
// 字段名必须与 backend model.Artifact 的 json tag 及前端 TS 类型对齐。
type ArtifactResult struct {
	Type     string `json:"type"` // code | webpage
	Language string `json:"language,omitempty"`
	Filename string `json:"filename,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`
	Content  string `json:"content,omitempty"`
}

// DaemonClient 封装单个 daemon WebSocket 连接
type DaemonClient struct {
	Conn      *websocket.Conn
	MachineID string // DaemonMachine.ID（DB 主键，非 hostname）
	sendCh    chan []byte
	closeOnce sync.Once
	closed    atomic.Bool
	mu        sync.Mutex
}

// NewDaemonClient 创建 DaemonClient 实例
func NewDaemonClient(conn *websocket.Conn, machineID string) *DaemonClient {
	return &DaemonClient{
		Conn:      conn,
		MachineID: machineID,
		sendCh:    make(chan []byte, daemonSendBuf),
	}
}

// WritePump 从 sendCh 读取消息写入连接
func (dc *DaemonClient) WritePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dc.sendCh:
			if !ok {
				return
			}
			dc.mu.Lock()
			err := dc.Conn.Write(ctx, websocket.MessageText, data)
			dc.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// Send 向 daemon 发送 JSON 消息（通过写缓冲区）
func (dc *DaemonClient) Send(msg WSMessage) error {
	if dc.closed.Load() {
		return errors.New("daemon client closed: " + dc.MachineID)
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case dc.sendCh <- data:
	default:
		// 背压：丢弃最旧消息腾出一个位置
		select {
		case <-dc.sendCh:
		default:
		}
		select {
		case dc.sendCh <- data:
		default:
			snippet := string(data)
			if len(snippet) > 80 {
				snippet = snippet[:80] + "..."
			}
			slog.Warn("daemon write buffer full after drain, dropping message", "machine_id", dc.MachineID, "msg", snippet)
		}
	}
	return nil
}

// --- DaemonHub 内部总线类型 ---

type daemonBusKind int

const (
	daemonBusRegister daemonBusKind = iota
	daemonBusUnregister
)

type daemonBusMsg struct {
	kind    daemonBusKind
	payload interface{}
}

// DaemonHub 管理所有 daemon WebSocket 连接，基于消息总线模式。
// 与用户 Hub 相同设计：单 goroutine 事件循环 + sync.Map + buffered bus channel。
type DaemonHub struct {
	clients      sync.Map // machineID -> *DaemonClient
	resultChans  sync.Map // taskID -> chan *TaskResult
	bus          chan daemonBusMsg
	logger       *slog.Logger
	wg           sync.WaitGroup
	draining     atomic.Bool
	shutdownOnce sync.Once
}

// NewDaemonHub 创建 DaemonHub 实例
func NewDaemonHub(logger *slog.Logger) *DaemonHub {
	return &DaemonHub{
		bus:    make(chan daemonBusMsg, 256),
		logger: logger,
	}
}

// Run 启动 DaemonHub 消息总线事件循环，应在独立 goroutine 中调用
func (dh *DaemonHub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			dh.shutdown()
			return
		case msg := <-dh.bus:
			dh.dispatch(msg)
		}
	}
}

// dispatch 根据消息类型分发处理
func (dh *DaemonHub) dispatch(msg daemonBusMsg) {
	switch msg.kind {
	case daemonBusRegister:
		dh.handleRegister(msg)
	case daemonBusUnregister:
		dh.handleUnregister(msg)
	}
}

func (dh *DaemonHub) handleRegister(msg daemonBusMsg) {
	client := msg.payload.(*DaemonClient)
	// 若同一 machineID 已有旧连接，先关闭
	if old, loaded := dh.clients.LoadAndDelete(client.MachineID); loaded {
		oldClient := old.(*DaemonClient)
		oldClient.closed.Store(true)
		oldClient.closeOnce.Do(func() { close(oldClient.sendCh) })
		oldClient.Conn.Close(websocket.StatusNormalClosure, "replaced by new connection")
		dh.logger.Info("replaced old daemon connection", "machine_id", client.MachineID)
	}
	dh.clients.Store(client.MachineID, client)
	dh.logger.Info("daemon connected", "machine_id", client.MachineID)
}

func (dh *DaemonHub) handleUnregister(msg daemonBusMsg) {
	client := msg.payload.(*DaemonClient)
	// 仅当 map 中的 client 与当前 client 一致时才删除（避免删除替代者）
	if loaded, ok := dh.clients.Load(client.MachineID); ok && loaded == client {
		dh.clients.Delete(client.MachineID)
	}
	client.closed.Store(true)
	client.closeOnce.Do(func() { close(client.sendCh) })
	client.Conn.Close(websocket.StatusNormalClosure, "disconnect")
	if !dh.draining.Load() {
		dh.wg.Done()
	}
	dh.logger.Info("daemon disconnected", "machine_id", client.MachineID)
}

// shutdown 优雅关闭：排空消息、关闭所有连接（通过 sync.Once 保证只执行一次）
func (dh *DaemonHub) shutdown() {
	dh.shutdownOnce.Do(func() {
		dh.logger.Info("daemon hub shutting down, draining messages")

		dh.draining.Store(true)

		// 排空待发消息（最多等待 2 秒）
		drainTimer := time.NewTimer(2 * time.Second)
		drainDone := false
		for !drainDone {
			select {
			case msg := <-dh.bus:
				dh.dispatch(msg)
			case <-drainTimer.C:
				drainDone = true
			}
		}

		// 关闭所有 daemon 连接
		dh.clients.Range(func(key, value interface{}) bool {
			client := value.(*DaemonClient)
			client.closed.Store(true)
			client.closeOnce.Do(func() { close(client.sendCh) })
			if client.Conn != nil {
				client.Conn.Close(websocket.StatusNormalClosure, "server shutdown")
			}
			return true
		})

		// 等待所有连接 goroutine 结束
		dh.wg.Wait()

		dh.logger.Info("daemon hub shutdown complete")
	})
}

// --- 公开 API ---

// Register 注册 daemon 连接（异步通过 bus）
func (dh *DaemonHub) Register(client *DaemonClient) {
	if !dh.draining.Load() {
		dh.wg.Add(1)
	}
	select {
	case dh.bus <- daemonBusMsg{kind: daemonBusRegister, payload: client}:
	default:
		if !dh.draining.Load() {
			dh.wg.Done()
		}
		dh.logger.Warn("daemon hub bus full, dropping register", "machine_id", client.MachineID)
	}
}

// Unregister 注销 daemon 连接（异步通过 bus）
func (dh *DaemonHub) Unregister(client *DaemonClient) {
	select {
	case dh.bus <- daemonBusMsg{kind: daemonBusUnregister, payload: client}:
	default:
		dh.logger.Warn("daemon hub bus full, force-closing connection", "machine_id", client.MachineID)
		client.closed.Store(true)
		client.closeOnce.Do(func() { close(client.sendCh) })
		client.Conn.Close(websocket.StatusNormalClosure, "disconnect")
		if !dh.draining.Load() {
			dh.wg.Done()
		}
	}
}

// SendToMachine 向指定 daemon 发送消息（同步查询 + 异步写入）
func (dh *DaemonHub) SendToMachine(machineID string, msg WSMessage) error {
	val, ok := dh.clients.Load(machineID)
	if !ok {
		return errors.New("daemon not connected: " + machineID)
	}
	client := val.(*DaemonClient)
	return client.Send(msg)
}

// IsConnected 检查 daemon 是否 WS 连接中
func (dh *DaemonHub) IsConnected(machineID string) bool {
	_, ok := dh.clients.Load(machineID)
	return ok
}

// RegisterTaskPromise 创建并存储任务结果 channel（带 buffer=1）
func (dh *DaemonHub) RegisterTaskPromise(taskID string) chan *TaskResult {
	ch := make(chan *TaskResult, 1)
	dh.resultChans.Store(taskID, ch)
	return ch
}

// AwaitTaskResult 获取任务结果 promise channel
// 返回 nil 表示该任务在 WS 之前创建（无 promise）
func (dh *DaemonHub) AwaitTaskResult(taskID string) chan *TaskResult {
	val, ok := dh.resultChans.Load(taskID)
	if !ok {
		return nil
	}
	return val.(chan *TaskResult)
}

// ResolveTask 发送结果到 promise channel；清理由等待方 RemoveTaskPromise 完成。
func (dh *DaemonHub) ResolveTask(taskID string, result *TaskResult) {
	val, ok := dh.resultChans.Load(taskID)
	if !ok {
		return
	}
	ch := val.(chan *TaskResult)
	select {
	case ch <- result:
	default:
		// channel 已满（已有结果或无人消费），跳过
	}
}

// RemoveTaskPromise 清理 promise channel（超时场景）
func (dh *DaemonHub) RemoveTaskPromise(taskID string) {
	dh.resultChans.Delete(taskID)
}

// RegisterTestClient inserts a client directly into the clients map.
// For use in tests only — bypasses the bus and avoids needing a real WebSocket.
func (dh *DaemonHub) RegisterTestClient(machineID string, client *DaemonClient) {
	dh.clients.Store(machineID, client)
}

// UpdateMachineID 更新 daemon 客户端的 machineID 标识（全局 token 连接时，收到
// daemon.register 后才知道真实 machineID，需要更新 DaemonHub 注册）。
func (dh *DaemonHub) UpdateMachineID(client *DaemonClient, newMachineID string) {
	oldID := client.MachineID
	if oldID == newMachineID {
		return
	}
	dh.clients.Delete(oldID)
	client.MachineID = newMachineID
	dh.clients.Store(newMachineID, client)
	dh.logger.Info("daemon machine_id updated", "old", oldID, "new", newMachineID)
}

// Shutdown 外部调用关闭 DaemonHub（委托给内部 shutdown，sync.Once 保证幂等）
func (dh *DaemonHub) Shutdown(ctx context.Context) {
	dh.shutdown()
}
