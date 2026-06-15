package tool_specs

import (
	"github.com/agent-hub/backend/internal/port"
)

// ── Helpers ──

func strProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}

func intProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}

func enumProp(desc string, values ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc, "enum": values}
}

func arrayProp(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"items":       map[string]interface{}{"type": "string"},
		"description": desc,
	}
}

func schema(props map[string]map[string]interface{}, required ...string) map[string]interface{} {
	p := make(map[string]interface{}, len(props))
	for k, v := range props {
		p[k] = v
	}
	s := map[string]interface{}{"type": "object", "properties": p}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func noParams() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

// ── Base structs ──

type routeSpec struct {
	name        string
	label       string
	description string
	category    string
	inputSchema map[string]interface{}
	routeInfo   *port.RouteInfo
}

func (s routeSpec) Name() string                        { return s.name }
func (s routeSpec) Label() string                       { return s.label }
func (s routeSpec) Description() string                 { return s.description }
func (s routeSpec) Category() string                    { return s.category }
func (s routeSpec) InputSchema() map[string]interface{} { return s.inputSchema }
func (s routeSpec) RouteInfo() *port.RouteInfo          { return s.routeInfo }
