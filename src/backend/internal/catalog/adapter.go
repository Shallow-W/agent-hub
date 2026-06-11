package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// ── Narrow repo interfaces ───────────────────────────────────────────────────
//
// We deliberately declare these locally in the catalog package (mirroring the
// pattern in service/platform_skill.go etc.) instead of importing concrete
// *repository.XxxRepo structs. The existing repository implementations already
// satisfy them. This keeps catalog → repository a one-way dependency and the
// adapter unit-testable with fakes.

// PlatformSkillStore covers the subset of PlatformSkillRepo methods used by
// catalog. The existing repository.PlatformSkillRepo already satisfies it.
// Declared locally so catalog → repository is a one-way dependency and the
// adapter is unit-testable with fakes.
type PlatformSkillStore interface {
	ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error)
	Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type ToolDefinitionLister interface {
	List(ctx context.Context) ([]model.ToolDefinition, error)
}

type AgentPromptTemplateLister interface {
	ListByUser(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error)
}

type AgentPromptTemplateStore interface {
	AgentPromptTemplateLister
	Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type UserTemplateLister interface {
	ListByUserAndType(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error)
}

type UserTemplateStore interface {
	UserTemplateLister
	Create(ctx context.Context, userID, tplType, name, content string) (*model.UserTemplate, error)
	Update(ctx context.Context, id, userID, name, content string) (*model.UserTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// AdapterStore implements Store by delegating to the four existing
// repositories. It performs the model ↔ Item conversion at the boundary so
// the rest of the catalog package sees only the unified shape.
//
// The store holds narrow interfaces (not concrete pointers) so it is safe to
// pass in *repository.XxxRepo structs directly, or mocks in tests.
type AdapterStore struct {
	platformSkill PlatformSkillStore
	toolDef       ToolDefinitionLister
	agentPrompt   AgentPromptTemplateLister
	userTemplate  UserTemplateLister
	registry      *Registry
}

// AdapterDeps bundles the four repo interfaces. Fields may be nil if the
// caller doesn't yet need that domain — List on an unset repo returns
// ErrUnknownDomain via the registry lookup before the repo is touched.
type AdapterDeps struct {
	PlatformSkill PlatformSkillStore
	ToolDef       ToolDefinitionLister
	AgentPrompt   AgentPromptTemplateLister
	UserTemplate  UserTemplateLister
	Registry      *Registry
}

// NewAdapterStore builds an AdapterStore from the given deps. The Registry
// is consulted for every operation; only domains that appear in the registry
// are reachable.
func NewAdapterStore(deps AdapterDeps) *AdapterStore {
	return &AdapterStore{
		platformSkill: deps.PlatformSkill,
		toolDef:       deps.ToolDef,
		agentPrompt:   deps.AgentPrompt,
		userTemplate:  deps.UserTemplate,
		registry:      deps.Registry,
	}
}

// Registry exposes the spec registry so Service / Handler can re-use the
// same registration data without holding a second copy.
func (s *AdapterStore) Registry() *Registry { return s.registry }

// List dispatches by domain. For system-scope domains the UserID filter is
// ignored.
func (s *AdapterStore) List(ctx context.Context, domain Domain, q ListQuery) ([]Item, error) {
	if s.registry != nil {
		if _, ok := s.registry.Get(domain); !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownDomain, domain)
		}
	}
	switch domain {
	case DomainPlatformSkill:
		if s.platformSkill == nil {
			return nil, fmt.Errorf("%w: platform_skill repo not configured", ErrUnknownDomain)
		}
		if strings.TrimSpace(q.UserID) == "" {
			return []Item{}, nil
		}
		list, err := s.platformSkill.ListByUser(ctx, q.UserID)
		if err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(list))
		for i := range list {
			out = append(out, platformSkillToItem(&list[i]))
		}
		return out, nil

	case DomainToolDefinition:
		if s.toolDef == nil {
			return nil, fmt.Errorf("%w: tool_definition repo not configured", ErrUnknownDomain)
		}
		list, err := s.toolDef.List(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(list))
		for i := range list {
			out = append(out, toolDefinitionToItem(&list[i]))
		}
		return out, nil

	case DomainAgentPromptTemplate:
		if s.agentPrompt == nil {
			return nil, fmt.Errorf("%w: agent_prompt_template repo not configured", ErrUnknownDomain)
		}
		if strings.TrimSpace(q.UserID) == "" {
			return []Item{}, nil
		}
		list, err := s.agentPrompt.ListByUser(ctx, q.UserID)
		if err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(list))
		for i := range list {
			out = append(out, agentPromptTemplateToItem(&list[i]))
		}
		return out, nil

	case DomainUserTemplate:
		if s.userTemplate == nil {
			return nil, fmt.Errorf("%w: user_template repo not configured", ErrUnknownDomain)
		}
		if strings.TrimSpace(q.UserID) == "" {
			return []Item{}, nil
		}
		subtype := q.Subtype
		if subtype == "" {
			subtype = "tools" // default subtype when caller didn't specify
		}
		list, err := s.userTemplate.ListByUserAndType(ctx, q.UserID, subtype)
		if err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(list))
		for i := range list {
			out = append(out, userTemplateToItem(&list[i]))
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownDomain, domain)
}

// ── Read paths: GetByID ──────────────────────────────────────────────────────
//
// GetByID requires a primary key lookup that the four repos do not currently
// expose uniformly. For B1 (where only tool_definition routes through
// catalog), GetByID is implemented by List-then-match for the system-scope
// domain, which keeps the read path correct without forcing a schema change
// on the other three repos.

func (s *AdapterStore) GetByID(ctx context.Context, id string) (*Item, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalid
	}
	if s.toolDef != nil {
		list, err := s.toolDef.List(ctx)
		if err != nil {
			return nil, err
		}
		for i := range list {
			if list[i].Name == id { // tool_definition uses Name as natural key
				item := toolDefinitionToItem(&list[i])
				return &item, nil
			}
		}
	}
	return nil, ErrNotFound
}

// ── Write paths ──────────────────────────────────────────────────────────────
//
// B2 wires platform_skill through catalog with full CRUD. The other three
// domains retain their legacy service/repo paths for now; their cases fall
// through to ErrReadOnly. B3/B4 will widen them the same way.

func (s *AdapterStore) Create(ctx context.Context, input CreateInput) (*Item, error) {
	switch input.Domain {
	case DomainPlatformSkill:
		if s.platformSkill == nil {
			return nil, fmt.Errorf("%w: platform_skill repo not configured", ErrUnknownDomain)
		}
		trigger, detail, err := decodePlatformSkillPayload(input.PayloadJSON)
		if err != nil {
			return nil, err
		}
		m, err := s.platformSkill.Create(ctx, input.UserID, input.Key, input.Category, input.Description, trigger, detail)
		if err != nil {
			return nil, err
		}
		if m == nil {
			return nil, ErrNotFound
		}
		item := platformSkillToItem(m)
		return &item, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrReadOnly, input.Domain)
	}
}

func (s *AdapterStore) Update(ctx context.Context, id string, input UpdateInput) (*Item, error) {
	switch input.Domain {
	case DomainPlatformSkill:
		if s.platformSkill == nil {
			return nil, fmt.Errorf("%w: platform_skill repo not configured", ErrUnknownDomain)
		}
		// platform_skill repo.Update is full-replace (every column required).
		// Resolve nil pointers by reading the current row via List-then-match —
		// the repo has no GetByID method, but ListByUser + id match is cheap
		// and matches the existing read pattern.
		current, err := s.findPlatformSkillByID(ctx, input.UserID, id)
		if err != nil {
			return nil, err
		}
		name := current.Name
		if input.Key != nil {
			name = *input.Key
		}
		category := current.Category
		if input.Category != nil {
			category = *input.Category
		}
		description := current.Description
		if input.Description != nil {
			description = *input.Description
		}
		trigger, detail := current.Trigger, current.Detail
		if input.PayloadJSON != nil {
			t, d, derr := decodePlatformSkillPayload(*input.PayloadJSON)
			if derr != nil {
				return nil, derr
			}
			trigger, detail = t, d
		}
		m, err := s.platformSkill.Update(ctx, id, input.UserID, name, category, description, trigger, detail)
		if err != nil {
			return nil, err
		}
		if m == nil {
			return nil, ErrNotFound
		}
		item := platformSkillToItem(m)
		return &item, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrReadOnly, input.Domain)
	}
}

func (s *AdapterStore) Delete(ctx context.Context, domain Domain, userID, id string) error {
	switch domain {
	case DomainPlatformSkill:
		if s.platformSkill == nil {
			return fmt.Errorf("%w: platform_skill repo not configured", ErrUnknownDomain)
		}
		deleted, err := s.platformSkill.Delete(ctx, id, userID)
		if err != nil {
			return err
		}
		if !deleted {
			return ErrNotFound
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrReadOnly, domain)
	}
}

// findPlatformSkillByID is a List-then-match helper for platform_skill,
// required because repository.PlatformSkillRepo exposes no GetByID. Used
// only by Update to resolve partial-merge source values.
func (s *AdapterStore) findPlatformSkillByID(ctx context.Context, userID, id string) (model.PlatformSkill, error) {
	list, err := s.platformSkill.ListByUser(ctx, userID)
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

// decodePlatformSkillPayload parses the JSON payload carried by Create /
// Update for the platform_skill domain into the (trigger, detail) pair the
// underlying repo expects. An empty payload yields two empty strings. A
// malformed payload yields ErrInvalid.
func decodePlatformSkillPayload(raw string) (trigger, detail string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", nil
	}
	var m map[string]string
	if e := json.Unmarshal([]byte(raw), &m); e != nil {
		return "", "", fmt.Errorf("%w: payload: %v", ErrInvalid, e)
	}
	return m["trigger"], m["detail"], nil
}

// ── model → Item converters ──────────────────────────────────────────────────

func platformSkillToItem(m *model.PlatformSkill) Item {
	payload := map[string]string{
		"trigger": m.Trigger,
		"detail":  m.Detail,
	}
	return Item{
		ID:          m.ID,
		Domain:      DomainPlatformSkill,
		UserID:      strPtr(m.UserID),
		Key:         m.Name,
		Label:       m.Name,
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: mustJSON(payload),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func toolDefinitionToItem(m *model.ToolDefinition) Item {
	// tool_definition carries no JSON payload today (input_schema lives in
	// migrations). We surface category + description as the canonical fields.
	return Item{
		ID:          m.Name, // system scope: Name is the natural key
		Domain:      DomainToolDefinition,
		UserID:      nil,
		Key:         m.Name,
		Label:       firstNonEmpty(m.Label, m.Name),
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: "",
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.CreatedAt, // tool_definitions are created once; no updated_at column
	}
}

func agentPromptTemplateToItem(m *model.AgentPromptTemplate) Item {
	payload := map[string]string{
		"system_prompt": m.SystemPrompt,
	}
	return Item{
		ID:          m.ID,
		Domain:      DomainAgentPromptTemplate,
		UserID:      strPtr(m.UserID),
		Key:         m.Name,
		Label:       m.Name,
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: mustJSON(payload),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func userTemplateToItem(m *model.UserTemplate) Item {
	return Item{
		ID:          m.ID,
		Domain:      DomainUserTemplate,
		UserID:      strPtr(m.UserID),
		Subtype:     m.Type,
		Key:         m.Name,
		Label:       m.Name,
		PayloadJSON: string(m.Content),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// ── small helpers ────────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
