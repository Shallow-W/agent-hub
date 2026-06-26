package mcp

import (
	"fmt"
	"net/url"
	"strings"
)

// RouteDef describes a REST API route for declarative tool proxying.
// For tools that simply extract args and forward to the backend API.
type RouteDef struct {
	Method   string // GET, POST, PUT, DELETE
	Path     string // URL path with {param} placeholders
	Required []string
	Optional []string
}

// RouteEntry pairs a Tool definition with its RouteDef.
type RouteEntry struct {
	Tool  Tool
	Route RouteDef
}

// RegisterRoutes registers multiple route-based tools into the registry.
func RegisterRoutes(r *Registry, api *APIClient, entries ...RouteEntry) {
	for _, e := range entries {
		r.Register(e.Tool, RouteHandler(api, e.Route))
	}
}

// RouteHandler creates a ToolHandlerFunc from a declarative RouteDef.
// Path params ({name}) are substituted from args; non-path Required/Optional
// params become query params (GET) or body fields (POST/PUT).
func RouteHandler(api *APIClient, def RouteDef) ToolHandlerFunc {
	pp := extractPathParams(def.Path)
	return func(_ string, args map[string]interface{}) (interface{}, error) {
		for _, p := range def.Required {
			if v, _ := args[p].(string); strings.TrimSpace(v) == "" {
				return nil, fmt.Errorf("%s is required", p)
			}
		}
		path := def.Path
		for _, p := range pp {
			v, _ := args[p].(string)
			path = strings.ReplaceAll(path, "{"+p+"}", url.PathEscape(v))
		}
		switch def.Method {
		case "GET":
			return routeGet(api, path, def, args, pp)
		case "POST":
			return routeBody(api.doPost, path, def, args, pp)
		case "PUT":
			return routeBody(api.doPut, path, def, args, pp)
		case "DELETE":
			return api.doDelete(path)
		}
		return nil, fmt.Errorf("unsupported method: %s", def.Method)
	}
}

func extractPathParams(path string) []string {
	var params []string
	for i := 0; i < len(path); {
		s := strings.Index(path[i:], "{")
		if s < 0 {
			break
		}
		s += i
		e := strings.Index(path[s:], "}")
		if e < 0 {
			break
		}
		e += s
		params = append(params, path[s+1:e])
		i = e + 1
	}
	return params
}

func routeGet(api *APIClient, path string, def RouteDef, args map[string]interface{}, pp []string) (interface{}, error) {
	query := map[string]string{}
	skip := make(map[string]bool, len(pp))
	for _, p := range pp {
		skip[p] = true
	}
	for _, p := range def.Required {
		if skip[p] {
			continue
		}
		if v, _ := args[p].(string); v != "" {
			query[p] = v
		}
	}
	for _, p := range def.Optional {
		if skip[p] {
			continue
		}
		if v, _ := args[p].(string); v != "" {
			query[p] = v
		}
		if v, ok := args[p].(float64); ok && v > 0 {
			query[p] = fmt.Sprintf("%d", int(v))
		}
	}
	return api.doGet(path, query)
}

type doBodyFunc func(string, interface{}) (interface{}, error)

func routeBody(fn doBodyFunc, path string, def RouteDef, args map[string]interface{}, pp []string) (interface{}, error) {
	body := map[string]interface{}{}
	skip := make(map[string]bool, len(pp))
	for _, p := range pp {
		skip[p] = true
	}
	for _, p := range def.Required {
		if skip[p] {
			continue
		}
		if v, ok := args[p]; ok {
			body[p] = v
		}
	}
	for _, p := range def.Optional {
		if skip[p] {
			continue
		}
		switch v := args[p].(type) {
		case string:
			if v != "" {
				body[p] = v
			}
		case []interface{}:
			if len(v) > 0 {
				body[p] = v
			}
		}
	}
	if len(body) == 0 {
		return fn(path, nil)
	}
	return fn(path, body)
}
