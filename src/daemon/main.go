package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/agent-hub/daemon/client"
	"github.com/agent-hub/daemon/mcp"
	"github.com/agent-hub/daemon/scanner"
)

const defaultDaemonWSURL = "ws://localhost:8080/daemon/ws"

func main() {
	wsURLFlag := flag.String("ws-url", "", "daemon websocket url")
	serverURLFlag := flag.String("server-url", "", "AgentHub server url")
	machineKeyFlag := flag.String("machine-key", "", "machine api key")
	apiKeyFlag := flag.String("api-key", "", "machine api key")
	agentIDFlag := flag.String("agent-id", "", "current agent id for MCP tool authorization")
	mcpFlag := flag.Bool("mcp", false, "启动 MCP Server 模式（stdio）")
	flag.Parse()

	// 全局 slog 初始化——两条路径（MCP / daemon）共享同一结构化 JSON 日志。
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if *mcpFlag {
		runMCP(*agentIDFlag)
		return
	}

	runDaemon(wsURLFlag, serverURLFlag, machineKeyFlag, apiKeyFlag)
}

// runMCP 启动 MCP Server，通过 stdio 对外提供 tool 能力
func runMCP(agentID string) {
	logger := slog.Default()

	serverURL := os.Getenv("AGENTHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	token := firstNonEmpty(
		os.Getenv("AGENTHUB_DAEMON_TOKEN"),
		os.Getenv("AGENTHUB_MACHINE_KEY"),
	)
	if token == "" {
		logger.Error("缺少认证 token，设置 AGENTHUB_DAEMON_TOKEN 环境变量")
		os.Exit(1)
	}

	api := mcp.NewAPIClient(serverURL, token)

	currentAgentID := firstNonEmpty(agentID, mcp.AgentIDFromEnv())
	registry := mcp.BuildRegistry(api, currentAgentID)
	allowed := api.AllowedToolsForAgent(currentAgentID)
	server := mcp.NewServerFromRegistry("agenthub", "0.1.0", registry, logger).WithAllowedTools(allowed)

	ctx := context.Background()

	logger.Info("MCP server starting", "server", serverURL)
	if err := server.Serve(ctx); err != nil {
		logger.Error("MCP server error", "error", err)
		os.Exit(1)
	}
}

// runDaemon 正常模式：扫描 agent + 注册到后端
func runDaemon(wsURLFlag, serverURLFlag, machineKeyFlag, apiKeyFlag *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agents, err := scanner.New(nil).Scan(ctx)
	if err != nil {
		slog.Error("scan agents failed", "error", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		slog.Error("encode agents failed", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))

	token := firstNonEmpty(*apiKeyFlag, *machineKeyFlag)
	if token == "" {
		token = os.Getenv("AGENTHUB_MACHINE_KEY")
	}
	if token == "" {
		token = os.Getenv("AGENTHUB_DAEMON_TOKEN")
	}
	if token == "" {
		return
	}
	wsURL := *wsURLFlag
	if wsURL == "" && *serverURLFlag != "" {
		wsURL = buildDaemonWSURL(*serverURLFlag)
	}
	if wsURL == "" {
		wsURL = os.Getenv("AGENTHUB_DAEMON_WS_URL")
	}
	if wsURL == "" {
		wsURL = defaultDaemonWSURL
	}
	machineID, err := os.Hostname()
	if err != nil {
		slog.Error("read hostname failed", "error", err)
		os.Exit(1)
	}

	registerCtx, registerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer registerCancel()
	if err := client.New(wsURL, token).Register(registerCtx, machineID, agents); err != nil {
		slog.Error("register agents failed", "error", err, "machine_id", machineID)
		os.Exit(1)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildDaemonWSURL(serverURL string) string {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return serverURL
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/daemon/ws"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
