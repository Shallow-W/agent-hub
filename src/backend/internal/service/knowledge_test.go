package service

import (
	"testing"
)

func TestParseKnowledgeRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []KnowledgeRef
	}{
		{
			name:     "无引用",
			input:    "这是一条普通消息",
			expected: nil,
		},
		{
			name:  "单个引用",
			input: "请使用 {{alice/项目文档}} 来帮助完成任务",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "项目文档", Raw: "{{alice/项目文档}}"},
			},
		},
		{
			name:  "多个引用",
			input: "参考 {{bob/API文档}} 和 {{alice/设计稿}} 来完成",
			expected: []KnowledgeRef{
				{Username: "bob", KBName: "API文档"},
				{Username: "alice", KBName: "设计稿"},
			},
		},
		{
			name:     "去重",
			input:    "{{alice/共享}} 和 {{alice/共享}}",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "共享"},
			},
		},
		{
			name:     "空括号不匹配",
			input:    "{{}} 和 {{ / }} 不应匹配",
			expected: nil,
		},
		{
			name:  "带@mention混合",
			input: "@Agent1 请查阅 {{alice/技术文档}} 后回答",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "技术文档"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKnowledgeRefs(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d refs, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i].Username != exp.Username {
					t.Errorf("ref[%d].Username = %q, want %q", i, result[i].Username, exp.Username)
				}
				if result[i].KBName != exp.KBName {
					t.Errorf("ref[%d].KBName = %q, want %q", i, result[i].KBName, exp.KBName)
				}
			}
		})
	}
}
