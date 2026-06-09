package service

import (
	"regexp"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

type fencedBlock struct {
	language string
	content  string
}

var (
	fenceOpenRe  = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})([^`~]*)$")
	fileHintRe   = regexp.MustCompile(`(?i)^\s*(?://|#|<!--)\s*file:\s*(.+?)\s*(?:-->)?\s*$`)
	documentLang = map[string]bool{
		"markdown": true,
		"md":       true,
		"txt":      true,
		"text":     true,
		"json":     true,
		"csv":      true,
	}
)

func artifactsFromMarkdown(text string) []model.Artifact {
	blocks := extractFenceBlocks(text)
	if len(blocks) == 0 {
		return nil
	}

	artifacts := make([]model.Artifact, 0, len(blocks))
	for _, block := range blocks {
		language := block.language
		content := block.content
		filename := ""

		firstLine := content
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		if match := fileHintRe.FindStringSubmatch(firstLine); len(match) == 2 {
			filename = strings.TrimSpace(match[1])
			if idx := strings.IndexByte(content, '\n'); idx >= 0 {
				content = content[idx+1:]
			} else {
				content = ""
			}
		}

		content = strings.TrimRight(content, " \t\r\n")
		if strings.TrimSpace(content) == "" {
			continue
		}

		switch {
		case language == "html" && looksLikeHTML(content):
			artifacts = append(artifacts, model.Artifact{
				Version: 1,
				Type:    "webpage",
				Title:   firstNonEmptyString(filename, "HTML Preview"),
				Content: content,
			})
		case documentLang[language]:
			artifacts = append(artifacts, model.Artifact{
				Version:  1,
				Type:     "document",
				Language: firstNonEmptyString(language, "text"),
				Filename: filename,
				Title:    firstNonEmptyString(filename, "Document Preview"),
				Content:  content,
			})
		default:
			artifacts = append(artifacts, model.Artifact{
				Version:  1,
				Type:     "code",
				Language: language,
				Filename: filename,
				Content:  content,
			})
		}
	}

	return artifacts
}

func extractFenceBlocks(text string) []fencedBlock {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	blocks := []fencedBlock{}

	for i := 0; i < len(lines); i++ {
		open := fenceOpenRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if len(open) != 3 {
			continue
		}
		marker := open[1]
		markerChar := marker[:1]
		language := strings.ToLower(strings.TrimSpace(open[2]))
		if fields := strings.Fields(language); len(fields) > 0 {
			language = fields[0]
		}

		start := i + 1
		closeIdx := -1
		for j := start; j < len(lines); j++ {
			line := strings.TrimSpace(lines[j])
			if strings.HasPrefix(line, marker) && strings.Trim(line, markerChar) == "" {
				closeIdx = j
				break
			}
		}
		if closeIdx == -1 {
			break
		}

		blocks = append(blocks, fencedBlock{
			language: language,
			content:  strings.Join(lines[start:closeIdx], "\n"),
		})
		i = closeIdx
	}

	return blocks
}

func looksLikeHTML(content string) bool {
	return regexp.MustCompile(`(?i)<\s*(?:html|!doctype|body|head|div)\b`).MatchString(content)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
