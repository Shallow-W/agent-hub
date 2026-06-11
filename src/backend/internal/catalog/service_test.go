package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeStore is an in-memory Store used to exercise Service without touching
// the real DB. It implements every Store method.
type fakeStore struct {
	items   map[string]Item // keyed by id
	byKey   map[string]string
	nextID  int
	createErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		items: map[string]Item{},
		byKey: map[string]string{},
	}
}

func (f *fakeStore) List(_ context.Context, domain Domain, _ ListQuery) ([]Item, error) {
	out := make([]Item, 0, len(f.items))
	for _, it := range f.items {
		if it.Domain == domain {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeStore) GetByID(_ context.Context, id string) (*Item, error) {
	it, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &it, nil
}

func (f *fakeStore) Create(_ context.Context, input CreateInput) (*Item, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextID++
	id := "fake-" + input.Domain.str() + "-" + itoa(f.nextID)
	if _, dup := f.byKey[string(input.Domain)+"|"+input.Key]; dup {
		return nil, ErrDuplicate
	}
	now := time.Now()
	it := Item{
		ID:          id,
		Domain:      input.Domain,
		Key:         input.Key,
		Label:       input.Label,
		Category:    input.Category,
		Description: input.Description,
		PayloadJSON: input.PayloadJSON,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if input.UserID != "" {
		u := input.UserID
		it.UserID = &u
	}
	f.items[id] = it
	f.byKey[string(input.Domain)+"|"+input.Key] = id
	return &it, nil
}

func (f *fakeStore) Update(_ context.Context, id string, input UpdateInput) (*Item, error) {
	it, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if input.Key != nil {
		it.Key = *input.Key
	}
	if input.Label != nil {
		it.Label = *input.Label
	}
	if input.Category != nil {
		it.Category = *input.Category
	}
	if input.Description != nil {
		it.Description = *input.Description
	}
	if input.PayloadJSON != nil {
		it.PayloadJSON = *input.PayloadJSON
	}
	it.UpdatedAt = time.Now()
	f.items[id] = it
	return &it, nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return ErrNotFound
	}
	delete(f.items, id)
	return nil
}

// (Domain).str returns the underlying string. Avoids a fmt import in test.
func (d Domain) str() string { return string(d) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// registryForTest builds a small registry that includes the production
// user_template spec plus a synthetic "test_domain" so we can prove a new
// domain can be added without touching catalog core.
func registryForTest() *Registry {
	return NewRegistry(
		DomainSpec{
			Name:            DomainUserTemplate,
			Scope:           ScopeUser,
			Subtypes:        []string{"tools", "skills"},
			MaxKeyLen:       100,
			MaxLabelLen:     100,
			DefaultCategory: "默认",
		},
		DomainSpec{
			Name:        DomainToolDefinition,
			Scope:       ScopeSystem,
		},
		DomainSpec{
			Name:        Domain("test_domain"),
			Label:       "测试域",
			Scope:       ScopeUser,
			MaxKeyLen:   50,
			MaxLabelLen: 50,
			Seeder: func() []CreateInput {
				return []CreateInput{
					{Key: "alpha", Label: "Alpha"},
					{Key: "beta", Label: "Beta"},
				}
			},
		},
	)
}

// ── Service tests ────────────────────────────────────────────────────────────

func TestService_List_UnknownDomain(t *testing.T) {
	svc := NewService(newFakeStore(), NewRegistry())
	if _, err := svc.List(context.Background(), Domain("does_not_exist"), ListQuery{}); !errors.Is(err, ErrUnknownDomain) {
		t.Fatalf("expected ErrUnknownDomain, got %v", err)
	}
}

func TestService_List_NormalizesEmpty(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	items, err := svc.List(context.Background(), DomainUserTemplate, ListQuery{})
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %d", len(items))
	}
}

func TestService_Create_Normalizes(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	item, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate,
		UserID: "u1",
		Subtype: "tools",
		Key:    "  my-tool  ",
		Label:  "",
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if item.Key != "my-tool" {
		t.Errorf("Key not trimmed: %q", item.Key)
	}
	if item.Label != "my-tool" {
		t.Errorf("Label should fall back to Key: %q", item.Label)
	}
	if item.Category != "默认" {
		t.Errorf("Category default not applied: %q", item.Category)
	}
}

func TestService_Create_RejectsEmptyKey(t *testing.T) {
	reg := registryForTest()
	svc := NewService(newFakeStore(), reg)
	if _, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate, UserID: "u1", Subtype: "tools",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for empty key, got %v", err)
	}
}

func TestService_Create_RejectsBadSubtype(t *testing.T) {
	reg := registryForTest()
	svc := NewService(newFakeStore(), reg)
	_, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate, UserID: "u1", Subtype: "bad", Key: "k",
	})
	if err == nil || !strings.Contains(err.Error(), "subtype") {
		t.Fatalf("expected subtype error, got %v", err)
	}
}

func TestService_Create_ReadOnlyDomain(t *testing.T) {
	reg := registryForTest()
	svc := NewService(newFakeStore(), reg)
	_, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainToolDefinition, Key: "x",
	})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestService_Create_Truncates(t *testing.T) {
	reg := registryForTest()
	svc := NewService(newFakeStore(), reg)
	long := strings.Repeat("a", 200)
	item, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate, UserID: "u1", Subtype: "tools",
		Key: long, Label: long,
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if len([]rune(item.Key)) != 103 { // 100 runes + "..."
		t.Errorf("Key not truncated to 100 runes + ellipsis: %d", len([]rune(item.Key)))
	}
	if !strings.HasSuffix(item.Key, "...") {
		t.Errorf("Key should end with ... after truncation: %q", item.Key)
	}
}

func TestService_Create_Duplicate(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	in := CreateInput{Domain: DomainUserTemplate, UserID: "u1", Subtype: "tools", Key: "k"}
	if _, err := svc.Create(context.Background(), in); err != nil {
		t.Fatalf("first Create err: %v", err)
	}
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc := NewService(newFakeStore(), registryForTest())
	if _, err := svc.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Update(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	item, err := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate, UserID: "u1", Subtype: "tools", Key: "k",
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	newLabel := "renamed"
	updated, err := svc.Update(context.Background(), item.ID, UpdateInput{Label: &newLabel})
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if updated.Label != "renamed" {
		t.Errorf("Label not updated: %q", updated.Label)
	}
}

func TestService_Delete(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	item, _ := svc.Create(context.Background(), CreateInput{
		Domain: DomainUserTemplate, UserID: "u1", Subtype: "tools", Key: "k",
	})
	if err := svc.Delete(context.Background(), item.ID); err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if _, err := svc.Get(context.Background(), item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestService_ImportDefaults(t *testing.T) {
	reg := registryForTest()
	store := newFakeStore()
	svc := NewService(store, reg)
	items, err := svc.ImportDefaults(context.Background(), Domain("test_domain"), "u1")
	if err != nil {
		t.Fatalf("ImportDefaults err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 seeded items, got %d", len(items))
	}
	// Second import should be idempotent — duplicates are skipped.
	items2, _ := svc.ImportDefaults(context.Background(), Domain("test_domain"), "u1")
	if len(items2) != 0 {
		t.Fatalf("idempotent re-import should yield 0 new items, got %d", len(items2))
	}
}

func TestService_ImportDefaults_NoSeeder(t *testing.T) {
	reg := registryForTest()
	svc := NewService(newFakeStore(), reg)
	items, err := svc.ImportDefaults(context.Background(), DomainUserTemplate, "u1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

// New domain extensibility proof: registering an extra DomainSpec at the
// call site (not in domains.go) lets the catalog serve it without any
// changes to the catalog core (service.go / handler.go / store.go).
func TestService_Extensibility_NewDomain(t *testing.T) {
	reg := NewRegistry(
		DomainSpec{Name: Domain("future_domain"), Scope: ScopeUser, MaxKeyLen: 30},
	)
	store := newFakeStore()
	svc := NewService(store, reg)
	item, err := svc.Create(context.Background(), CreateInput{
		Domain: Domain("future_domain"), UserID: "u1", Key: "future",
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if item.Domain != Domain("future_domain") {
		t.Errorf("wrong domain: %s", item.Domain)
	}
	list, _ := svc.List(context.Background(), Domain("future_domain"), ListQuery{})
	if len(list) != 1 {
		t.Errorf("expected 1 item in new domain, got %d", len(list))
	}
}
