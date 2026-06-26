package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
)

// ToolRegistryReader is the read-only subset consumed by tool_config validators.
type ToolRegistryReader interface {
	Lookup(name string) (port.MCPToolSpec, bool)
	List() []port.MCPToolSpec
}

// ToolDefinitionUpserter is the narrow write interface for auto-sync.
type ToolDefinitionUpserter interface {
	Upsert(ctx context.Context, td model.ToolDefinition) error
}

// ToolRegistry is the central registry of all MCP tool specs.
// It is the SINGLE SOURCE OF TRUTH for what tools exist.
type ToolRegistry struct {
	specs   map[string]port.MCPToolSpec
	ordered []port.MCPToolSpec
	repo    ToolDefinitionUpserter
	mu      sync.RWMutex
}

// NewToolRegistry creates a ToolRegistry. Pass nil for repo if DB sync is
// not desired (tests).
func NewToolRegistry(repo ToolDefinitionUpserter) *ToolRegistry {
	return &ToolRegistry{
		specs: make(map[string]port.MCPToolSpec),
		repo:  repo,
	}
}

// Register adds a spec to the in-memory registry AND syncs its definition
// to the DB tool_definitions table. Returns error if DB write fails.
//
// 为了避免内存状态被污染：先做名称唯一性检查，再尝试 DB 写入，最后才把 spec
// 写进内存。这样当 Upsert 失败时，后续 Register 不会因为 "duplicate tool spec"
// 而失败。
func (r *ToolRegistry) Register(ctx context.Context, spec port.MCPToolSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.specs[spec.Name()]; exists {
		return fmt.Errorf("duplicate tool spec: %s", spec.Name())
	}

	// 先尝试 DB 写入；失败直接返回，不修改内存状态
	if r.repo != nil {
		if err := r.repo.Upsert(ctx, model.ToolDefinition{
			Name:        spec.Name(),
			Label:       spec.Label(),
			Category:    spec.Category(),
			Description: spec.Description(),
		}); err != nil {
			return err
		}
	}

	r.specs[spec.Name()] = spec
	r.ordered = append(r.ordered, spec)
	return nil
}

// Lookup finds a tool by name. O(1) map lookup.
func (r *ToolRegistry) Lookup(name string) (port.MCPToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[name]
	return spec, ok
}

// List returns all registered specs in registration order.
func (r *ToolRegistry) List() []port.MCPToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]port.MCPToolSpec, len(r.ordered))
	copy(out, r.ordered)
	return out
}
