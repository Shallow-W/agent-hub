package catalog

import (
	"context"
)

// toolDefinitionPlugin implements DomainPlugin for the tool_definition domain.
// It is read-only: Create, Update, and Delete return ErrReadOnly.
type toolDefinitionPlugin struct {
	repo ToolDefinitionLister
}

// NewToolDefinitionPlugin builds a read-only DomainPlugin backed by ToolDefinitionLister.
func NewToolDefinitionPlugin(repo ToolDefinitionLister) DomainPlugin {
	return &toolDefinitionPlugin{repo: repo}
}

func (p *toolDefinitionPlugin) List(ctx context.Context, _ string) ([]Item, error) {
	list, err := p.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		out = append(out, toolDefinitionToItem(&list[i]))
	}
	return out, nil
}

func (p *toolDefinitionPlugin) Create(_ context.Context, _ CreateInput) (*Item, error) {
	return nil, errReadOnlyDomainf(DomainToolDefinition)
}

func (p *toolDefinitionPlugin) Update(_ context.Context, _ string, _ string, _ UpdateInput) (*Item, error) {
	return nil, errReadOnlyDomainf(DomainToolDefinition)
}

func (p *toolDefinitionPlugin) Delete(_ context.Context, _ string, _ string) error {
	return errReadOnlyDomainf(DomainToolDefinition)
}

// toolDefFindByID scans the repo list to find a tool definition by name.
// This is used internally by AdapterStore.GetByID.
func toolDefFindByID(repo ToolDefinitionLister, ctx context.Context, id string) (*Item, error) {
	list, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == id { // tool_definition uses Name as natural key
			item := toolDefinitionToItem(&list[i])
			return &item, nil
		}
	}
	return nil, nil // not found, no error
}
