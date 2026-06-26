package mcp

// Prop creates a string property descriptor.
func Prop(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}

// IntProp creates an integer property descriptor.
func IntProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}

// EnumProp creates a string enum property descriptor.
func EnumProp(desc string, values ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc, "enum": values}
}

// ArrayProp creates a string array property descriptor.
func ArrayProp(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"items":       map[string]interface{}{"type": "string"},
		"description": desc,
	}
}

// Schema creates an input schema for a tool.
func Schema(props map[string]map[string]interface{}, required ...string) map[string]interface{} {
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

// NoParams creates an empty input schema.
func NoParams() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

// T is a shorthand for creating a Tool.
func T(name, desc string, schema map[string]interface{}) Tool {
	return Tool{Name: name, Description: desc, InputSchema: schema}
}
