package port

// MCPToolSpec bundles a tool's identity (name, description, schema, category)
// with its API route metadata. Simple tools that only proxy to backend routes
// have non-nil RouteInfo; tools that require custom daemon-side logic return nil.
type MCPToolSpec interface {
	Name() string
	Label() string
	Description() string
	Category() string
	InputSchema() map[string]interface{}
	RouteInfo() *RouteInfo // nil means "requires custom handler in daemon"
}

// RouteInfo provides enough metadata for the daemon to auto-generate an HTTP-proxy
// handler without any daemon-side code per tool.
type RouteInfo struct {
	Method   string   // GET, POST, PUT, DELETE
	Path     string   // URL path with {param} placeholders (e.g., "/mcp/tasks/{id}/status")
	Required []string // param names that must be present
	Optional []string // param names that are optional
}
