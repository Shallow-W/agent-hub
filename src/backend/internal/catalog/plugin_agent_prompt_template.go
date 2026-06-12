package catalog

import (
	"context"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// agentPromptTemplatePlugin implements DomainPlugin for the agent_prompt_template domain.
type agentPromptTemplatePlugin struct {
	repo AgentPromptTemplateStore
}

// NewAgentPromptTemplatePlugin builds a DomainPlugin backed by AgentPromptTemplateStore.
func NewAgentPromptTemplatePlugin(repo AgentPromptTemplateStore) DomainPlugin {
	return &agentPromptTemplatePlugin{repo: repo}
}

func (p *agentPromptTemplatePlugin) List(ctx context.Context, userID string) ([]Item, error) {
	if strings.TrimSpace(userID) == "" {
		return []Item{}, nil
	}
	list, err := p.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		out = append(out, agentPromptTemplateToItem(&list[i]))
	}
	return out, nil
}

func (p *agentPromptTemplatePlugin) Create(ctx context.Context, input CreateInput) (*Item, error) {
	systemPrompt, err := decodeAgentPromptPayload(input.PayloadJSON)
	if err != nil {
		return nil, err
	}
	m, err := p.repo.Create(ctx, input.UserID, input.Key, input.Category, input.Description, systemPrompt)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := agentPromptTemplateToItem(m)
	return &item, nil
}

func (p *agentPromptTemplatePlugin) Update(ctx context.Context, id, userID string, input UpdateInput) (*Item, error) {
	current, err := p.findByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	name := derefOr(input.Key, current.Name)
	category := derefOr(input.Category, current.Category)
	description := derefOr(input.Description, current.Description)
	systemPrompt := current.SystemPrompt
	if input.PayloadJSON != nil {
		sp, derr := decodeAgentPromptPayload(*input.PayloadJSON)
		if derr != nil {
			return nil, derr
		}
		systemPrompt = sp
	}
	m, err := p.repo.Update(ctx, id, userID, name, category, description, systemPrompt)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	item := agentPromptTemplateToItem(m)
	return &item, nil
}

func (p *agentPromptTemplatePlugin) Delete(ctx context.Context, id, userID string) error {
	deleted, err := p.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

func (p *agentPromptTemplatePlugin) findByID(ctx context.Context, userID, id string) (model.AgentPromptTemplate, error) {
	list, err := p.repo.ListByUser(ctx, userID)
	if err != nil {
		return model.AgentPromptTemplate{}, err
	}
	for i := range list {
		if list[i].ID == id {
			return list[i], nil
		}
	}
	return model.AgentPromptTemplate{}, ErrNotFound
}
