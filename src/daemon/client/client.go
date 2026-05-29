package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/agent-hub/daemon/scanner"
	"nhooyr.io/websocket"
)

// Client 与后端 daemon WebSocket 通信
type Client struct {
	baseURL string
	token   string
}

// RegisterRequest 是 daemon 首次连接后的注册消息
type RegisterRequest struct {
	MachineID string              `json:"machine_id"`
	Agents    []scanner.AgentInfo `json:"agents"`
}

// New 创建 daemon 后端客户端
func New(baseURL string, token string) *Client {
	return &Client{baseURL: baseURL, token: token}
}

// Register 上报本机可用 Agent 列表
func (c *Client) Register(ctx context.Context, machineID string, agents []scanner.AgentInfo) error {
	target, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse daemon websocket url: %w", err)
	}
	q := target.Query()
	q.Set("token", c.token)
	target.RawQuery = q.Encode()

	conn, _, err := websocket.Dial(ctx, target.String(), nil)
	if err != nil {
		return fmt.Errorf("connect daemon websocket: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "register complete")

	payload := struct {
		Type string          `json:"type"`
		Data RegisterRequest `json:"data"`
	}{
		Type: "daemon.register",
		Data: RegisterRequest{
			MachineID: machineID,
			Agents:    agents,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal register payload: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("send register payload: %w", err)
	}
	return nil
}
