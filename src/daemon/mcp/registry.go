package mcp

import "fmt"

// Tool defines an MCP tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// ToolHandlerFunc handles a tool call.
type ToolHandlerFunc func(toolName string, arguments map[string]interface{}) (interface{}, error)

// Registry manages tool definitions and their handlers.
// Register tools at init; dispatch is O(1) map lookup.
// To add a new tool category, write a RegisterXxx function and add one
// line to BuildRegistry — zero core code changes.
type Registry struct {
	tools    []Tool
	handlers map[string]ToolHandlerFunc
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]ToolHandlerFunc)}
}

func (r *Registry) Register(tool Tool, handler ToolHandlerFunc) {
	r.tools = append(r.tools, tool)
	r.handlers[tool.Name] = handler
}

func (r *Registry) Tools() []Tool { return r.tools }

func (r *Registry) Dispatch(toolName string, args map[string]interface{}) (interface{}, error) {
	h, ok := r.handlers[toolName]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	return h(toolName, args)
}

// Handler returns a ToolHandlerFunc that dispatches through the registry.
func (r *Registry) Handler() ToolHandlerFunc {
	return r.Dispatch
}
