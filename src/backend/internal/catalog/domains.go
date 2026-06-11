package catalog

// DefaultRegistry returns the canonical Registry used by the application.
//
// To add a new catalog domain: append a DomainSpec here and provide an
// adapter (or extend AdapterStore). You should NOT need to modify any
// other file in this package.
func DefaultRegistry() *Registry {
	return NewRegistry(
		DomainSpec{
			Name:            DomainPlatformSkill,
			Label:           "平台 Skill",
			Scope:           ScopeUser,
			MaxKeyLen:       80,
			MaxLabelLen:     80,
			MaxDescLen:      200,
			DefaultCategory: "未分类",
			Sorter:          platformSkillSorter,
			Seeder:          seederFromPlatformSkillDefaults,
		},
		DomainSpec{
			Name:  DomainToolDefinition,
			Label: "工具定义",
			Scope: ScopeSystem,
		},
		DomainSpec{
			Name:            DomainAgentPromptTemplate,
			Label:           "Agent Prompt 模板",
			Scope:           ScopeUser,
			MaxKeyLen:       80,
			MaxLabelLen:     80,
			MaxDescLen:      200,
			DefaultCategory: "通用",
			Sorter:          agentPromptTemplateSorter,
			Seeder:          seederFromAgentPromptDefaults,
		},
		DomainSpec{
			Name:        DomainUserTemplate,
			Label:       "用户模板",
			Scope:       ScopeUser,
			Subtypes:    []string{"tools", "skills"},
			MaxKeyLen:   100,
			MaxLabelLen: 100,
			Sorter:      userTemplateSorter,
		},
	)
}

// platformSkillSorter preserves the existing repo ORDER BY semantics
// (category ASC, updated_at DESC, name ASC). The repo already sorts this
// way, so the sorter is effectively a stable passthrough — but it documents
// the contract and keeps the catalog deterministic regardless of the
// underlying store.
func platformSkillSorter(items []Item) {
	sortItems(items, func(a, b Item) bool {
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.Key < b.Key
	})
}

func agentPromptTemplateSorter(items []Item) {
	sortItems(items, func(a, b Item) bool {
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.Key < b.Key
	})
}

func userTemplateSorter(items []Item) {
	sortItems(items, func(a, b Item) bool {
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.Key < b.Key
	})
}

// seederFromPlatformSkillDefaults adapts the catalog-local default-value
// data (seeders_data.go) to CreateInput values. The data is intentionally
// duplicated from service.DefaultPlatformSkillTemplates to avoid an import
// cycle (catalog ↔ service); B-later will delete the legacy copy.
func seederFromPlatformSkillDefaults() []CreateInput {
	templates := defaultPlatformSkills()
	out := make([]CreateInput, 0, len(templates))
	for _, t := range templates {
		out = append(out, CreateInput{
			Domain:      DomainPlatformSkill,
			Key:         t.Name,
			Label:       t.Name,
			Category:    t.Category,
			Description: t.Description,
			PayloadJSON: mustJSON(map[string]string{
				"trigger": t.Trigger,
				"detail":  t.Detail,
			}),
		})
	}
	return out
}

func seederFromAgentPromptDefaults() []CreateInput {
	templates := defaultAgentPrompts()
	out := make([]CreateInput, 0, len(templates))
	for _, t := range templates {
		out = append(out, CreateInput{
			Domain:      DomainAgentPromptTemplate,
			Key:         t.Name,
			Label:       t.Name,
			Category:    t.Category,
			Description: t.Description,
			PayloadJSON: mustJSON(map[string]string{
				"system_prompt": t.SystemPrompt,
			}),
		})
	}
	return out
}
