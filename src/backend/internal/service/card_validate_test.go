package service

import (
	"testing"
)

// TestValidateCard_SupportedTypes 验证 6 个已知类型 + 必填字段齐全时通过校验。
func TestValidateCard_SupportedTypes(t *testing.T) {
	cases := []map[string]any{
		{"type": "plan", "id": "p1", "questions": []any{}},
		{"type": "approval", "id": "a1", "actions": []any{}},
		{"type": "progress", "id": "pr1", "tasks": []any{}},
		{"type": "info", "id": "i1", "fields": map[string]any{}},
		{"type": "diff", "id": "d1", "workDir": "/path", "files": []any{"App.tsx"}},
		{"type": "project", "id": "pj1", "workDir": "/path"},
	}
	for i, card := range cases {
		vc, err := ValidateCard(card)
		if err != nil {
			t.Errorf("case %d (%s): expected pass, got error: %v", i, card["type"], err)
			continue
		}
		if vc == nil {
			t.Errorf("case %d (%s): returned nil card", i, card["type"])
		}
	}
}

// TestValidateCard_UnknownType 验证未知 type 被拒绝。
func TestValidateCard_UnknownType(t *testing.T) {
	cases := []map[string]any{
		{"type": "unknown", "id": "x1"},
		{"type": "render_card", "id": "x2"}, // 历史 type，已废弃
		{"type": "", "id": "x3"},
		{"type": 123, "id": "x4"},
	}
	for i, card := range cases {
		if _, err := ValidateCard(card); err == nil {
			t.Errorf("case %d: expected error for card %+v, got nil", i, card)
		}
	}
}

// TestValidateCard_MissingRequired 验证 diff / project 缺 workDir 时拒绝（严格）。
func TestValidateCard_MissingRequired(t *testing.T) {
	cases := []map[string]any{
		{"type": "diff", "id": "d1", "files": []any{"x.ts"}},    // 缺 workDir
		{"type": "diff", "id": "d2", "workDir": "/p"},           // 缺 files
		{"type": "project", "id": "p1"},                         // 缺 workDir
		{"type": "plan", "id": ""},                              // 空 id
	}
	for i, card := range cases {
		if _, err := ValidateCard(card); err == nil {
			t.Errorf("case %d: expected error for card %+v, got nil", i, card)
		}
	}
}

// TestValidateCard_MissingOptionalWarn 验证可选字段缺失时只 warn 不拒绝。
// LLM 偶尔会漏 questions/actions 等字段——前端组件已有 ?? [] 兜底，不应阻塞入库。
func TestValidateCard_MissingOptionalWarn(t *testing.T) {
	cases := []map[string]any{
		{"type": "plan", "id": "p1"},       // 缺 questions（前端 PlanCard 已 ?? []）
		{"type": "approval", "id": "a1"},   // 缺 actions
		{"type": "progress", "id": "pr1"},  // 缺 tasks
		{"type": "info", "id": "i1"},       // 缺 fields
	}
	for i, card := range cases {
		vc, err := ValidateCard(card)
		if err != nil {
			t.Errorf("case %d (%s): optional field missing should warn not reject, got error: %v",
				i, card["type"], err)
			continue
		}
		if vc == nil {
			t.Errorf("case %d (%s): returned nil", i, card["type"])
		}
	}
}

// TestValidateCards_FiltersInvalid 验证批量过滤场景：好的保留，坏的剔除。
func TestValidateCards_FiltersInvalid(t *testing.T) {
	cards := []map[string]any{
		{"type": "info", "id": "ok1", "fields": map[string]any{}},
		{"type": "unknown", "id": "bad1"},                  // 拒绝（未知 type）
		{"type": "diff", "id": "bad2", "files": []any{}},   // 拒绝（缺 workDir）
		{"type": "info", "id": "ok2"},                      // 保留（缺 fields 只 warn）
		nil,                                                 // 拒绝（nil）
	}
	valid := ValidateCards(cards)
	if len(valid) != 2 {
		t.Fatalf("expected 2 valid cards (ok1 + ok2), got %d: %+v", len(valid), valid)
	}
	// 检查 id 集合
	ids := map[string]bool{}
	for _, c := range valid {
		if id, ok := c["id"].(string); ok {
			ids[id] = true
		}
	}
	if !ids["ok1"] || !ids["ok2"] {
		t.Errorf("expected ok1 and ok2 in valid set, got %v", ids)
	}
}

// TestSupportedCardTypes_FourSourceConsistency 单一事实源断言——
// 新增类型时必须同步四处：SupportedCardTypes（本文件）、context_agent_config.go 系统提示词、
// 前端 types/card.ts CardType union、前端 CardRegistry.tsx registerCard。
// 本测试只能直接断言后端这一侧；前端两侧由前端测试覆盖（见 cards.test.ts）。
func TestSupportedCardTypes_FourSourceConsistency(t *testing.T) {
	// 后端支持的类型集合
	expected := []string{"plan", "approval", "progress", "info", "diff", "project"}
	for _, tp := range expected {
		if !IsSupportedCardType(tp) {
			t.Errorf("SupportedCardTypes missing required type %q", tp)
		}
	}
	listed := ListSupportedCardTypes()
	if len(listed) != len(expected) {
		t.Errorf("ListSupportedCardTypes returned %d types, expected %d (got %v)",
			len(listed), len(expected), listed)
	}
}
