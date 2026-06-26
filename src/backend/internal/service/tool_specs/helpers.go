package tool_specs

import (
	"fmt"
	"regexp"
	"strings"

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

// pathPlaceholderRe extracts `{param}` placeholders from a RouteInfo.Path.
var pathPlaceholderRe = regexp.MustCompile(`\{(\w+)\}`)

// newRouteSpec 构造 routeSpec 并做必要的格式校验。
//
// 校验规则（RouteInfo 非 nil 时）：
//   - Path 中的每个 `{param}` 占位符都必须出现在 Required 或 Optional 中
//   - Required 与 Optional 不能有交集
//
// 校验失败会 panic：所有 routeSpec 都在包初始化阶段构造，错误应在编译/启动
// 时立即暴露，避免运行时静默不一致。
func newRouteSpec(name, label, category, description string, inputSchema map[string]interface{}, routeInfo *port.RouteInfo) routeSpec {
	if strings.TrimSpace(name) == "" {
		panic(fmt.Sprintf("tool_specs: newRouteSpec requires non-empty name"))
	}
	if routeInfo != nil {
		// 校验 Required ∩ Optional 为空
		seen := make(map[string]bool, len(routeInfo.Required))
		for _, r := range routeInfo.Required {
			seen[r] = true
		}
		for _, opt := range routeInfo.Optional {
			if seen[opt] {
				panic(fmt.Sprintf("tool_specs: %s has %q in both Required and Optional", name, opt))
			}
		}
		// 校验 Path 中的每个占位符都出现在 Required ∪ Optional 中
		allowed := make(map[string]bool, len(routeInfo.Required)+len(routeInfo.Optional))
		for _, r := range routeInfo.Required {
			allowed[r] = true
		}
		for _, opt := range routeInfo.Optional {
			allowed[opt] = true
		}
		for _, match := range pathPlaceholderRe.FindAllStringSubmatch(routeInfo.Path, -1) {
			placeholder := match[1]
			if !allowed[placeholder] {
				panic(fmt.Sprintf("tool_specs: %s Path placeholder %q not declared in Required/Optional", name, placeholder))
			}
		}
	}
	return routeSpec{
		name:        name,
		label:       label,
		category:    category,
		description: description,
		inputSchema: inputSchema,
		routeInfo:   routeInfo,
	}
}
