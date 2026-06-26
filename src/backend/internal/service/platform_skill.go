package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

var (
	ErrPlatformSkillNotFound  = errors.New("平台 Skill 不存在")
	ErrPlatformSkillInvalid   = errors.New("平台 Skill 参数无效")
	ErrPlatformSkillDuplicate = errors.New("平台 Skill 名称已存在")
)

// PlatformSkillRepo 是 PlatformSkillService 依赖的仓库接口。
type PlatformSkillRepo interface {
	ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error)
	Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// PlatformSkillCatalogItem is the catalog-package-neutral representation
// of one catalog.Item for the platform_skill domain. Declared locally so
// the service package doesn't need to import internal/catalog (which would
// cause an import cycle: catalog → middleware → service → catalog).
//
// Every field on model.PlatformSkill is preserved so the /api/platform-skills
// response stays byte-equivalent regardless of which path served it.
type PlatformSkillCatalogItem struct {
	ID          string
	UserID      string
	Name        string
	Category    string
	Description string
	Trigger     string
	Detail      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PlatformSkillCatalogStore is the subset of catalog.Service consumed by
// PlatformSkillService. The method set mirrors PlatformSkillRepo so the
// service can swap repo ↔ catalog without changing its public signature.
// Wire an implementation at composition time (see main.go's
// platformSkillCatalogBridge); when nil, the service falls back to repo.
type PlatformSkillCatalogStore interface {
	ListPlatformSkills(ctx context.Context, userID string) ([]PlatformSkillCatalogItem, error)
	CreatePlatformSkill(ctx context.Context, userID, name, category, description, trigger, detail string) (*PlatformSkillCatalogItem, error)
	UpdatePlatformSkill(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*PlatformSkillCatalogItem, error)
	DeletePlatformSkill(ctx context.Context, id, userID string) error
}

type PlatformSkillService struct {
	repo    PlatformSkillRepo
	catalog PlatformSkillCatalogStore // optional; when set, all CRUD routes through catalog
}

func NewPlatformSkillService(repo PlatformSkillRepo) *PlatformSkillService {
	return &PlatformSkillService{repo: repo}
}

// SetCatalogStore wires the optional catalog.Service dependency. After
// this is called, List / Create / Update / Delete (and ImportDefaults,
// which internally calls Create) will route through catalog. Response
// bytes are unchanged: catalog Items are reverse-mapped to
// model.PlatformSkill at the service boundary.
func (s *PlatformSkillService) SetCatalogStore(store PlatformSkillCatalogStore) {
	s.catalog = store
}

func (s *PlatformSkillService) List(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrPlatformSkillInvalid
	}
	if s.catalog != nil {
		items, err := s.catalog.ListPlatformSkills(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list platform skills via catalog: %w", err)
		}
		out := make([]model.PlatformSkill, 0, len(items))
		for _, it := range items {
			out = append(out, catalogItemToPlatformSkill(it))
		}
		return out, nil
	}
	return nil, fmt.Errorf("list platform skills: catalog store not configured")
}

func (s *PlatformSkillService) ImportDefaults(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrPlatformSkillInvalid
	}
	defaultNames := make(map[string]bool, len(DefaultPlatformSkillTemplates()))
	for _, tpl := range DefaultPlatformSkillTemplates() {
		defaultNames[tpl.Name] = true
		// Create auto-routes through catalog when wired, so ImportDefaults
		// transparently inherits the catalog write path.
		if _, err := s.Create(ctx, userID, tpl.Name, tpl.Category, tpl.Description, tpl.Trigger, tpl.Detail); err != nil {
			if errors.Is(err, ErrPlatformSkillDuplicate) {
				continue
			}
			return nil, err
		}
	}
	list, err := s.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	defaults := make([]model.PlatformSkill, 0, len(defaultNames))
	for _, skill := range list {
		if defaultNames[skill.Name] {
			defaults = append(defaults, skill)
		}
	}
	return defaults, nil
}

func (s *PlatformSkillService) Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	name, category, description, trigger, detail, err := normalizePlatformSkillFields(userID, name, category, description, trigger, detail)
	if err != nil {
		return nil, err
	}
	if s.catalog != nil {
		it, cErr := s.catalog.CreatePlatformSkill(ctx, userID, name, category, description, trigger, detail)
		if cErr != nil {
			return nil, cErr
		}
		m := catalogItemToPlatformSkill(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("create platform skill: catalog store not configured")
}

func (s *PlatformSkillService) Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrPlatformSkillInvalid
	}
	name, category, description, trigger, detail, err := normalizePlatformSkillFields(userID, name, category, description, trigger, detail)
	if err != nil {
		return nil, err
	}
	if s.catalog != nil {
		it, cErr := s.catalog.UpdatePlatformSkill(ctx, id, userID, name, category, description, trigger, detail)
		if cErr != nil {
			return nil, cErr
		}
		if it == nil {
			return nil, ErrPlatformSkillNotFound
		}
		m := catalogItemToPlatformSkill(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("update platform skill: catalog store not configured")
}

func (s *PlatformSkillService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrPlatformSkillInvalid
	}
	if s.catalog != nil {
		if err := s.catalog.DeletePlatformSkill(ctx, id, userID); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("delete platform skill: catalog store not configured")
}

func normalizePlatformSkillFields(userID, name, category, description, trigger, detail string) (string, string, string, string, string, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || name == "" {
		return "", "", "", "", "", ErrPlatformSkillInvalid
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "未分类"
	}
	return truncateString(name, 80),
		truncateString(category, 60),
		truncateString(strings.TrimSpace(description), 200),
		truncateString(strings.TrimSpace(trigger), 200),
		truncateString(strings.TrimSpace(detail), 2000),
		nil
}

// catalogItemToPlatformSkill reverses the AdapterStore mapping. Every
// field on model.PlatformSkill — including CreatedAt / UpdatedAt — must
// be preserved so /api/platform-skills responses stay byte-equivalent.
// B1's pilot migration had a regression here (dropped CreatedAt) that
// trellis-check caught; this function explicitly tests both timestamps.
func catalogItemToPlatformSkill(it PlatformSkillCatalogItem) model.PlatformSkill {
	return model.PlatformSkill{
		ID:          it.ID,
		UserID:      it.UserID,
		Name:        it.Name,
		Category:    it.Category,
		Description: it.Description,
		Trigger:     it.Trigger,
		Detail:      it.Detail,
		CreatedAt:   it.CreatedAt,
		UpdatedAt:   it.UpdatedAt,
	}
}
