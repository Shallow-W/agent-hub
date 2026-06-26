package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
)

// stubSpec is a minimal MCPToolSpec for registry tests.
type stubSpec struct {
	name string
}

func (s stubSpec) Name() string                        { return s.name }
func (s stubSpec) Label() string                       { return "stub" }
func (s stubSpec) Description() string                 { return "stub desc" }
func (s stubSpec) Category() string                    { return "test" }
func (s stubSpec) InputSchema() map[string]interface{} { return nil }
func (s stubSpec) RouteInfo() *port.RouteInfo          { return nil }

// fakeUpserter records calls and can be configured to fail.
type fakeUpserter struct {
	calls   []string
	failOn  int // index of call that should fail; -1 = never
	failErr error
}

func (f *fakeUpserter) Upsert(_ context.Context, td model.ToolDefinition) error {
	idx := len(f.calls)
	f.calls = append(f.calls, td.Name)
	if idx == f.failOn && f.failErr != nil {
		return f.failErr
	}
	return nil
}

func TestRegisterAndLookup(t *testing.T) {
	ctx := context.Background()
	r := NewToolRegistry(nil)
	if err := r.Register(ctx, stubSpec{name: "alpha"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	spec, ok := r.Lookup("alpha")
	if !ok {
		t.Fatal("expected Lookup to find registered spec")
	}
	if spec.Name() != "alpha" {
		t.Fatalf("got name %q, want alpha", spec.Name())
	}
}

func TestRegisterDuplicateReturnsError(t *testing.T) {
	ctx := context.Background()
	r := NewToolRegistry(nil)
	if err := r.Register(ctx, stubSpec{name: "dup"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(ctx, stubSpec{name: "dup"})
	if err == nil {
		t.Fatal("expected duplicate register to fail")
	}
}

func TestListReturnsRegistrationOrder(t *testing.T) {
	ctx := context.Background()
	r := NewToolRegistry(nil)
	for _, n := range []string{"a", "b", "c"} {
		if err := r.Register(ctx, stubSpec{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Name() != want {
			t.Fatalf("List[%d] = %q, want %q", i, got[i].Name(), want)
		}
	}
}

func TestListReturnsCopy(t *testing.T) {
	ctx := context.Background()
	r := NewToolRegistry(nil)
	if err := r.Register(ctx, stubSpec{name: "x"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	first := r.List()
	first[0] = nil // mutate the returned slice

	second := r.List()
	if len(second) != 1 || second[0] == nil {
		t.Fatalf("registry state should not be affected by mutating List result: %#v", second)
	}
}

func TestLookupMissing(t *testing.T) {
	r := NewToolRegistry(nil)
	if _, ok := r.Lookup("nope"); ok {
		t.Fatal("expected Lookup to return ok=false for unknown name")
	}
}

// 关键回归：Register 在 Upsert 失败时不能污染内存状态——否则下一次 Register
// 同名 spec 会被错误地判定为 "duplicate tool spec"。
func TestRegisterDBFailureDoesNotMutateMemory(t *testing.T) {
	ctx := context.Background()
	upserter := &fakeUpserter{failOn: 0, failErr: errors.New("db down")}
	r := NewToolRegistry(upserter)

	err := r.Register(ctx, stubSpec{name: "alpha"})
	if err == nil {
		t.Fatal("expected Register to fail when Upsert fails")
	}

	// 内存中不应存在 alpha
	if _, ok := r.Lookup("alpha"); ok {
		t.Fatal("alpha should not be in registry after failed Upsert")
	}

	// 第二次 Register 同名 spec 应当成功（不返回 "duplicate tool spec"）
	upserter.failOn = -1 // 后续 Upsert 不再失败
	if err := r.Register(ctx, stubSpec{name: "alpha"}); err != nil {
		t.Fatalf("second register should succeed after failed upsert: %v", err)
	}
}
