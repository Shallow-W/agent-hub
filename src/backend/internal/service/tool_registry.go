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
func (r *ToolRegistry) Register(ctx context.Context, spec port.MCPToolSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.specs[spec.Name()]; exists {
		return fmt.Errorf("duplicate tool spec: %s", spec.Name())
	}

	r.specs[spec.Name()] = spec
	r.ordered = append(r.ordered, spec)

	if r.repo != nil {
		return r.repo.Upsert(ctx, model.ToolDefinition{
			Name:        spec.Name(),
			Label:       spec.Label(),
			Category:    spec.Category(),
			Description: spec.Description(),
		})
	}
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

// SyncAllToDB re-syncs all registered specs to the DB.
func (r *ToolRegistry) SyncAllToDB(ctx context.Context) error {
	if r.repo == nil {
		return nil
	}
	r.mu.RLock()
	specs := make([]port.MCPToolSpec, len(r.ordered))
	copy(specs, r.ordered)
	r.mu.RUnlock()

	for _, spec := range specs {
		if err := r.repo.Upsert(ctx, model.ToolDefinition{
			Name:        spec.Name(),
			Label:       spec.Label(),
			Category:    spec.Category(),
			Description: spec.Description(),
		}); err != nil {
			return fmt.Errorf("sync tool %s: %w", spec.Name(), err)
		}
	}
	return nil
}
