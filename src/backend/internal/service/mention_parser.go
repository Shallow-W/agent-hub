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
	var preamble string
	var dispatches []dispatchLine
	dispatchStartIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

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

	// Collect continuation lines between dispatch lines for each task
	// Map dispatch index -> list of continuation line indices
	dispatchLineIndices := make([]int, len(dispatches))
	idx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		withoutArrow := strings.TrimPrefix(trimmed, "→")
		withoutArrow = strings.TrimSpace(withoutArrow)
		if strings.HasPrefix(withoutArrow, "@") && mentionRe.FindStringSubmatch(withoutArrow) != nil {
			if idx < len(dispatches) {
				dispatchLineIndices[idx] = i
				idx++
			}
		}
	}

	// Now build full task text for each dispatch (dispatch line + continuation lines)
	dependsRe := regexp.MustCompile(`@([\p{L}\p{N}_\-.]+)`)
	tasks := make([]DispatchTask, 0, len(dispatches))

	for i, d := range dispatches {
		// Collect continuation lines until next dispatch or end
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
			taskParts = append(taskParts, strings.TrimSpace(lines[j]))
		}

		fullTask := strings.TrimSpace(strings.Join(taskParts, " "))

		// Extract DependsOn from embedded @references
		dependsOn := ""
		subMatches := dependsRe.FindAllStringSubmatch(fullTask, -1)
		for _, sm := range subMatches {
			if sm[1] != d.agentName {
				dependsOn = sm[1]
				break
			}
		}

		// Strip embedded @references from display task text
		cleanTask := dependsRe.ReplaceAllString(fullTask, "")
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
