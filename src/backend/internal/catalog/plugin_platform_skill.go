package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// platformSkillPlugin implements DomainPlugin for the platform_skill domain.
type platformSkillPlugin struct {
	repo PlatformSkillStore
}

// NewPlatformSkillPlugin builds a DomainPlugin backed by PlatformSkillStore.
func NewPlatformSkillPlugin(repo PlatformSkillStore) DomainPlugin {
	return &platformSkillPlugin{repo: repo}
}

func (p *platformSkillPlugin) List(ctx context.Context, userID string) ([]Item, error) {
	if strings.TrimSpace(userID) == "" {
		return []Item{}, nil
	}
	list, err := p.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		out = append(out, platformSkillToItem(&list[i]))
	}
	return out, nil
}

func (p *platformSkillPlugin) Create(ctx context.Context, input CreateInput) (*Item, error) {
	trigger, detail, err := decodePlatformSkillPayload(input.PayloadJSON)
	if err != nil {
		return nil, err
	}
	m, err := p.repo.Create(ctx, input.UserID, input.Key, input.Category, input.Description, trigger, detail)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := platformSkillToItem(m)
	return &item, nil
}

func (p *platformSkillPlugin) Update(ctx context.Context, id, userID string, input UpdateInput) (*Item, error) {
	current, err := p.findByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	name := derefOr(input.Key, current.Name)
	category := derefOr(input.Category, current.Category)
	description := derefOr(input.Description, current.Description)
	trigger, detail := current.Trigger, current.Detail
	if input.PayloadJSON != nil {
		t, d, derr := decodePlatformSkillPayload(*input.PayloadJSON)
		if derr != nil {
			return nil, derr
		}
		trigger, detail = t, d
	}
	m, err := p.repo.Update(ctx, id, userID, name, category, description, trigger, detail)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := platformSkillToItem(m)
	return &item, nil
}

func (p *platformSkillPlugin) Delete(ctx context.Context, id, userID string) error {
	deleted, err := p.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

func (p *platformSkillPlugin) findByID(ctx context.Context, userID, id string) (model.PlatformSkill, error) {
	list, err := p.repo.ListByUser(ctx, userID)
	if err != nil {
		return model.PlatformSkill{}, err
	}
	for i := range list {
		if list[i].ID == id {
			return list[i], nil
		}
	}
	return model.PlatformSkill{}, ErrNotFound
}

// derefOr returns *ptr if non-nil, otherwise fallback.
func derefOr[T any](ptr *T, fallback T) T {
	if ptr != nil {
		return *ptr
	}
	return fallback
}

// errReadOnlyDomainf wraps ErrReadOnly with the domain name.
func errReadOnlyDomainf(domain Domain) error {
	return fmt.Errorf("%w: %s", ErrReadOnly, domain)
}
