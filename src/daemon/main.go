package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/agent-hub/daemon/client"
	"github.com/agent-hub/daemon/scanner"
)

const defaultDaemonWSURL = "ws://localhost:8080/daemon/ws"

func main() {
	wsURLFlag := flag.String("ws-url", "", "daemon websocket url")
	serverURLFlag := flag.String("server-url", "", "AgentHub server url")
	machineKeyFlag := flag.String("machine-key", "", "machine api key")
	apiKeyFlag := flag.String("api-key", "", "machine api key")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agents, err := scanner.New(nil).Scan(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan agents: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode agents: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "read hostname: %v\n", err)
		os.Exit(1)
	}

	registerCtx, registerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer registerCancel()
	if err := client.New(wsURL, token).Register(registerCtx, machineID, agents); err != nil {
		fmt.Fprintf(os.Stderr, "register agents: %v\n", err)
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
