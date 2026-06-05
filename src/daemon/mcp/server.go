package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Server 是 MCP Server，通过 stdio 提供 tool 能力
type Server struct {
	name    string
	version string
	tools   []Tool
	handler ToolHandlerFunc
	transport *stdioTransport
	logger    *slog.Logger
}

// NewServer 创建 MCP Server
func NewServer(name, version string, tools []Tool, handler ToolHandlerFunc, logger *slog.Logger) *Server {
	return &Server{
		name:    name,
		version: version,
		tools:   tools,
		handler: handler,
		logger:  logger,
	}
}

// Serve 启动 MCP Server，从 stdin 读取请求，向 stdout 写入响应
func (s *Server) Serve(ctx context.Context) error {
	s.transport = newStdioTransport(os.Stdin, os.Stdout)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 监听信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		req, err := s.transport.readMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == io.EOF {
				return nil
			}
			s.logger.Error("read message", "error", err)
			// JSON-RPC 2.0: 畸形 JSON 应返回 parse error
			if wErr := s.transport.writeMessage(makeError(nil, errParseError, "Parse error")); wErr != nil {
				return wErr
			}
			continue
		}
		if req == nil {
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			if err := s.transport.writeMessage(resp); err != nil {
				s.logger.Error("write message", "error", err)
				return err
			}
		}
	}
}

func (s *Server) handleRequest(req *jsonrpcRequest) *jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// 客户端初始化完成通知，无需响应
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return makeError(req.ID, errMethodNotFound, "method not found: "+req.Method)
	}
}

func (s *Server) handleInitialize(req *jsonrpcRequest) *jsonrpcResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    s.name,
			"version": s.version,
		},
	}
	return makeResponse(req.ID, result)
}

func (s *Server) handleToolsList(req *jsonrpcRequest) *jsonrpcResponse {
	tools := make([]map[string]interface{}, len(s.tools))
	for i, t := range s.tools {
		tools[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
	}
	return makeResponse(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(req *jsonrpcRequest) *jsonrpcResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return makeError(req.ID, errInvalidParams, "invalid params: "+err.Error())
	}

	result, err := s.handler(params.Name, params.Arguments)
	if err != nil {
		return makeResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
			},
			"isError": true,
		})
	}

	return makeResponse(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": serializeResult(result)},
		},
	})
}

func serializeResult(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
