package catalog

import (
	"context"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// userTemplatePlugin implements DomainPlugin for the user_template domain.
type userTemplatePlugin struct {
	repo UserTemplateStore
}

// NewUserTemplatePlugin builds a DomainPlugin backed by UserTemplateStore.
func NewUserTemplatePlugin(repo UserTemplateStore) DomainPlugin {
	return &userTemplatePlugin{repo: repo}
}

// List returns all user templates for the given user. The subtype filter is
// not available through this method (DomainPlugin interface is subtype-agnostic);
// use AdapterStore.List with a ListQuery.Subtype instead, which calls
// ListWithSubtype directly.
func (p *userTemplatePlugin) List(ctx context.Context, userID string) ([]Item, error) {
	if strings.TrimSpace(userID) == "" {
		return []Item{}, nil
	}
	// Default: list "tools" subtype when called through the generic interface.
	return p.ListWithSubtype(ctx, userID, "tools")
}

// ListWithSubtype lists user templates filtered by subtype.
func (p *userTemplatePlugin) ListWithSubtype(ctx context.Context, userID, subtype string) ([]Item, error) {
	if strings.TrimSpace(userID) == "" {
		return []Item{}, nil
	}
	if subtype == "" {
		subtype = "tools"
	}
	list, err := p.repo.ListByUserAndType(ctx, userID, subtype)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		out = append(out, userTemplateToItem(&list[i]))
	}
	return out, nil
}

func (p *userTemplatePlugin) Create(ctx context.Context, input CreateInput) (*Item, error) {
	tplType := input.Subtype
	if tplType == "" {
		tplType = "tools"
	}
	m, err := p.repo.Create(ctx, input.UserID, tplType, input.Key, input.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := userTemplateToItem(m)
	return &item, nil
}

func (p *userTemplatePlugin) Update(ctx context.Context, id, userID string, input UpdateInput) (*Item, error) {
	current, err := p.findByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	name := derefOr(input.Key, current.Name)
	content := string(current.Content)
	if input.PayloadJSON != nil {
		content = *input.PayloadJSON
	}
	m, err := p.repo.Update(ctx, id, userID, name, content)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := userTemplateToItem(m)
	return &item, nil
}

func (p *userTemplatePlugin) Delete(ctx context.Context, id, userID string) error {
	deleted, err := p.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

func (p *userTemplatePlugin) findByID(ctx context.Context, userID, id string) (model.UserTemplate, error) {
	// Search across both subtypes — Update doesn't carry subtype, so we
	// check tools and skills.
	for _, t := range []string{"tools", "skills"} {
		list, err := p.repo.ListByUserAndType(ctx, userID, t)
		if err != nil {
			return model.UserTemplate{}, err
		}
		for i := range list {
			if list[i].ID == id {
				return list[i], nil
			}
		}
	}
	return model.UserTemplate{}, ErrNotFound
}
