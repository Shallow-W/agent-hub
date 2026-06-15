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

// Server is an MCP Server that provides tool capabilities via stdio.
type Server struct {
	name     string
	version  string
	registry *Registry
	allowed  map[string]bool
	transport *stdioTransport
	logger   *slog.Logger
}

// NewServer creates an MCP Server from tools and a shared handler.
// Kept for backward compatibility; prefer NewServerFromRegistry.
func NewServer(name, version string, tools []Tool, handler ToolHandlerFunc, logger *slog.Logger) *Server {
	r := NewRegistry()
	for _, t := range tools {
		r.Register(t, handler)
	}
	return newServerFromRegistry(name, version, r, logger)
}

// NewServerFromRegistry creates an MCP Server from a fully-wired Registry.
func NewServerFromRegistry(name, version string, r *Registry, logger *slog.Logger) *Server {
	return newServerFromRegistry(name, version, r, logger)
}

func newServerFromRegistry(name, version string, r *Registry, logger *slog.Logger) *Server {
	return &Server{
		name:     name,
		version:  version,
		registry: r,
		allowed: toolSet(func() []string {
			tools := r.Tools()
			names := make([]string, 0, len(tools))
			for _, tool := range tools {
				names = append(names, tool.Name)
			}
			return names
		}()),
		logger: logger,
	}
}

// WithAllowedTools limits tools/list and tools/call to the provided tool names.
func (s *Server) WithAllowedTools(allowed map[string]bool) *Server {
	s.allowed = allowed
	s.registry.tools = filterTools(s.registry.tools, allowed)
	return s
}

// Serve starts the MCP Server, reading requests from stdin and writing responses to stdout.
func (s *Server) Serve(ctx context.Context) error {
	s.transport = newStdioTransport(os.Stdin, os.Stdout)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
	tools := s.registry.Tools()
	list := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		list[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
	}
	return makeResponse(req.ID, map[string]interface{}{"tools": list})
}

func (s *Server) handleToolsCall(req *jsonrpcRequest) *jsonrpcResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return makeError(req.ID, errInvalidParams, "invalid params: "+err.Error())
	}

	if !s.allowed[params.Name] {
		err := fmt.Errorf("tool not authorized: %s", params.Name)
		return makeResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
			},
			"isError": true,
		})
	}

	result, err := s.registry.Dispatch(params.Name, params.Arguments)
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
