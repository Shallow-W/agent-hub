package service

import (
	"regexp"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// mentionRe matches @AgentName patterns, supporting Unicode/CJK characters.
var mentionRe = regexp.MustCompile(`@([\p{L}\p{N}_\-.]+)`)

// MentionResult represents a single @mention parsed from message text.
type MentionResult struct {
	AgentName string // the name after @
	Task      string // text between this @mention and the next (or end of text)
}

// DispatchTask represents one task assignment from Orchestrator output.
type DispatchTask struct {
	AgentName  string // target agent name (no @)
	Task       string // task description for this agent
	Sequential bool   // true if prefixed with "→"
	DependsOn  string // agent name this task depends on
}

// OrchDispatch represents the full parsed Orchestrator output.
type OrchDispatch struct {
	Preamble string         // text before any @mention (Orch's explanation to the chat)
	Tasks    []DispatchTask // extracted task assignments
}

// ParseMentions extracts all @AgentName patterns from text.
// For each mention, Task holds the text from after the agent name to the
// start of the next @mention (or end of text), with whitespace trimmed.
func ParseMentions(text string) []MentionResult {
	matches := mentionRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	results := make([]MentionResult, 0, len(matches))
	for i, loc := range matches {
		// loc[2], loc[3] = start/end of capture group 1 (agent name)
		nameStart := loc[2]
		nameEnd := loc[3]

		taskStart := nameEnd
		var taskEnd int
		if i+1 < len(matches) {
			// task runs until the next @mention
			taskEnd = matches[i+1][0]
		} else {
			taskEnd = len(text)
		}

		results = append(results, MentionResult{
			AgentName: text[nameStart:nameEnd],
			Task:      strings.TrimSpace(text[taskStart:taskEnd]),
		})
	}
	return results
}

// ParseOrchestratorOutput parses full Orchestrator dispatch output into structured tasks.
// It identifies dispatch lines (lines starting with @mention, optionally prefixed with "→")
// and treats embedded @references in task text as dependencies.
// Returns nil if no @mentions are found (indicating a regular response, not a dispatch).
func ParseOrchestratorOutput(text string) *OrchDispatch {
	type dispatchLine struct {
		agentName  string
		taskText   string
		sequential bool
	}

	// Split into lines to identify dispatch lines vs continuation text
	lines := strings.Split(text, "\n")

	// 追踪代码块状态，忽略代码块内的 @mention
	inCodeBlock := false

	var preamble string
	var dispatches []dispatchLine
	var dispatchLineIndices []int
	dispatchStartIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检测代码块边界
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		// 代码块内不解析 @mention
		if inCodeBlock {
			continue
		}

		// Check if this line starts with optional "→" then @mention
		withoutArrow := strings.TrimPrefix(trimmed, "→")
		withoutArrow = strings.TrimSpace(withoutArrow)

		if strings.HasPrefix(withoutArrow, "@") {
			// Extract agent name after @
			name := mentionRe.FindStringSubmatch(withoutArrow)
			if name == nil {
				continue
			}
			agentName := name[1]

			// Task text is everything after @AgentName on this line
			afterAt := withoutArrow[len("@")+len(agentName):]
			taskOnLine := strings.TrimSpace(afterAt)

			sequential := strings.HasPrefix(trimmed, "→")

			if dispatchStartIdx == -1 {
				dispatchStartIdx = i
			}
			dispatches = append(dispatches, dispatchLine{
				agentName:  agentName,
				taskText:   taskOnLine,
				sequential: sequential,
			})
			dispatchLineIndices = append(dispatchLineIndices, i)
		}
	}

	if len(dispatches) == 0 {
		return nil
	}

	// Build preamble from lines before the first dispatch
	if dispatchStartIdx > 0 {
		preambleLines := lines[:dispatchStartIdx]
		preamble = strings.TrimSpace(strings.Join(preambleLines, "\n"))
	}

	// Build full task text for each dispatch (dispatch line + continuation lines)
	// Continuation lines: only indented or blank lines immediately following the dispatch line.
	// A non-blank, non-indented line (that is not itself a dispatch) terminates the task text.
	tasks := make([]DispatchTask, 0, len(dispatches))

	for i, d := range dispatches {
		var taskParts []string
		if d.taskText != "" {
			taskParts = append(taskParts, d.taskText)
		}

		thisLineIdx := dispatchLineIndices[i]
		nextLineIdx := len(lines)
		if i+1 < len(dispatches) {
			nextLineIdx = dispatchLineIndices[i+1]
		}

		for j := thisLineIdx + 1; j < nextLineIdx; j++ {
			raw := lines[j]
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				// 空行仍属于 task，保留为空字符串（join 时产生空格分隔）
				taskParts = append(taskParts, "")
				continue
			}
			// 非空白且不是缩进行 → 属于非 task 文本，停止收集
			if raw[0] != ' ' && raw[0] != '\t' {
				break
			}
			taskParts = append(taskParts, trimmed)
		}

		fullTask := strings.TrimSpace(strings.Join(taskParts, " "))

		// Extract DependsOn from embedded @references
		dependsOn := ""
		subMatches := mentionRe.FindAllStringSubmatch(fullTask, -1)
		for _, sm := range subMatches {
			if sm[1] != d.agentName {
				dependsOn = sm[1]
				break
			}
		}

		// Strip embedded @references from display task text
		cleanTask := mentionRe.ReplaceAllString(fullTask, "")
		// Collapse multiple spaces left by removal
		cleanTask = strings.Join(strings.Fields(cleanTask), " ")

		tasks = append(tasks, DispatchTask{
			AgentName:  d.agentName,
			Task:       cleanTask,
			Sequential: d.sequential,
			DependsOn:  dependsOn,
		})
	}

	return &OrchDispatch{
		Preamble: preamble,
		Tasks:    tasks,
	}
}

// FindMentionedAgentID matches mention AgentNames to actual agent IDs in the conversation.
// Returns map[agentName]agentID. Mentions that don't match any conversation agent are skipped.
func FindMentionedAgentID(mentions []MentionResult, conversationAgents []model.ConversationAgent) map[string]string {
	result := make(map[string]string)
	for _, m := range mentions {
		for _, ca := range conversationAgents {
			if ca.Name == m.AgentName {
				result[m.AgentName] = ca.AgentID
				break
			}
		}
	}
	return result
}

// KnowledgeRef 表示消息中的一个知识库引用
type KnowledgeRef struct {
	Username string // 引用的用户名
	KBName   string // 引用的知识库名称
	Raw      string // 原始文本 "{{username/kbname}}"
}

// kbRefRe 匹配 {{用户名/知识库名}} 格式的知识库引用
// 用户名和知识库名必须为非空且不含 / {} 和换行符
var kbRefRe = regexp.MustCompile(`\{\{([^/{}\s][^/{}]*?)/([^/{}\s][^/{}]*?)\}\}`)

// ParseKnowledgeRefs 从消息文本中提取所有知识库引用
func ParseKnowledgeRefs(text string) []KnowledgeRef {
	matches := kbRefRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var results []KnowledgeRef
	for _, m := range matches {
		ref := KnowledgeRef{
			Username: strings.TrimSpace(m[1]),
			KBName:   strings.TrimSpace(m[2]),
			Raw:      m[0],
		}
		key := ref.Username + "/" + ref.KBName
		if !seen[key] {
			seen[key] = true
			results = append(results, ref)
		}
	}
	return results
}
