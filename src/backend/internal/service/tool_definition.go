package service

import (
	"context"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// ToolDefinitionRepo 定义 ToolDefinitionService 需要的仓库接口。
type ToolDefinitionRepo interface {
	List(ctx context.Context) ([]model.ToolDefinition, error)
	ListBuiltinTemplates(ctx context.Context) ([]model.BuiltinToolsetTemplate, error)
	ListBuiltinSkillTemplates(ctx context.Context) ([]model.BuiltinSkillTemplate, error)
}

// ToolDefinitionCatalogItem is the catalog-package-neutral representation
// of one catalog.Item for the tool_definition domain. Declared locally so
// the service package doesn't need to import internal/catalog (which would
// cause an import cycle: catalog → middleware → service → catalog).
type ToolDefinitionCatalogItem struct {
	Name         string
	Label        string
	Category     string
	Description  string
	IsManagement bool
	CreatedAt    time.Time // set by the catalog bridge from Item.CreatedAt
}

// ToolDefinitionCatalogLister is the subset of catalog.Service consumed by
// ToolDefinitionService. Wire an implementation at composition time (see
// main.go); when nil, the service falls back to the direct repo call.
type ToolDefinitionCatalogLister interface {
	ListToolDefinitions(ctx context.Context) ([]ToolDefinitionCatalogItem, error)
}

type ToolDefinitionService struct {
	repo    ToolDefinitionRepo
	catalog ToolDefinitionCatalogLister // optional; when set, ListDefinitions routes through catalog
}

func NewToolDefinitionService(repo ToolDefinitionRepo) *ToolDefinitionService {
	return &ToolDefinitionService{repo: repo}
}

// SetCatalogLister wires the optional catalog.Service dependency. After
// this is called, ListDefinitions will route through catalog and then map
// the returned Items back to model.ToolDefinition so the response shape
// stays byte-equivalent to the legacy implementation.
func (s *ToolDefinitionService) SetCatalogLister(lister ToolDefinitionCatalogLister) {
	s.catalog = lister
}

func (s *ToolDefinitionService) ListDefinitions(ctx context.Context) ([]model.ToolDefinition, error) {
	// Pilot migration (B1): when a catalog lister is wired, route through it.
	// The returned items are reverse-mapped into []model.ToolDefinition so
	// /api/tools/definitions response bytes are unchanged.
	if s.catalog != nil {
		items, err := s.catalog.ListToolDefinitions(ctx)
		if err != nil {
			return nil, fmt.Errorf("list definitions via catalog: %w", err)
		}
		out := make([]model.ToolDefinition, 0, len(items))
		for _, it := range items {
			out = append(out, catalogItemToToolDefinition(it))
		}
		return out, nil
	}
	return nil, fmt.Errorf("list definitions: catalog lister not configured")
}

func (s *ToolDefinitionService) ListBuiltinTemplates(ctx context.Context) ([]model.BuiltinToolsetTemplate, error) {
	list, err := s.repo.ListBuiltinTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list builtin templates: %w", err)
	}
	if list == nil {
		return []model.BuiltinToolsetTemplate{}, nil
	}
	return list, nil
}

func (s *ToolDefinitionService) ListBuiltinSkillTemplates(ctx context.Context) ([]model.BuiltinSkillTemplate, error) {
	list, err := s.repo.ListBuiltinSkillTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list builtin skill templates: %w", err)
	}
	if list == nil {
		return []model.BuiltinSkillTemplate{}, nil
	}
	return list, nil
}

// catalogItemToToolDefinition reverses the AdapterStore mapping. Used only
// by the tool_definition pilot migration to preserve legacy response shape.
// All fields from the catalog Item (including CreatedAt and IsManagement)
// are preserved so the /api/tools/definitions response stays byte-equivalent
// to the legacy direct-repo path.
func catalogItemToToolDefinition(it ToolDefinitionCatalogItem) model.ToolDefinition {
	return model.ToolDefinition{
		Name:         it.Name,
		Label:        it.Label,
		Category:     it.Category,
		Description:  it.Description,
		IsManagement: it.IsManagement,
		CreatedAt:    it.CreatedAt,
	}
}
