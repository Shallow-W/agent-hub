// Package service: block_card_extractor.go
//
// SplitTextBlocksByCardFences 把 streaming 累积的 blocks 里的 text block 按
// ```agenthub {"cards":[...]}``` fenced block 切分成 [text-before, card-block, text-middle,
// card-block, text-after]，让卡片成为 first-class block kind（与 text / thinking /
// tool_use / tool_result / error 平级）。
//
// 切分算法复用 extractCardsFromContent 的 fence 识别逻辑（fenced block 协议契约
// 三方一致：backend extractCardsFromContent / backend SplitTextBlocksByCardFences /
// agent 系统提示词，统一使用 ```agenthub fence 标记）。本文件不重写识别算法，
// 而是把切分指令从 extractCardsFromContent 的「strippedContent + cards」表达
// 升级为「blocks 数组」表达。
//
// 设计原则：
//   - 非 text block 原样保留（thinking / tool_use / tool_result / error / card）
//   - text block 无 fence → 原样保留
//   - text block 含完整 fence → 切分；多个 fence 全部识别
//   - text block 跨 fence 但 fence 未闭合 → 原样返回（不部分切分，避免污染）
//   - 一个 fence 内多张卡 → 每张卡独立 card block（保留位置语义，前端可单独 render）
//   - 切分后重新计算所有 block 的 Index（保证单调递增无空洞，与 streaming_reducer 一致）
package service

import (
	"encoding/json"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// fenceOpenMarker 是卡片协议的 fence 开启标记。
// 用 agenthub 而非 json，避免与普通 JSON 代码块歧义——agent 写 ```json 时是
// 普通代码产物，写 ```agenthub 时是卡片协议。
// 协议三方一致：context_agent_config.go（agent 系统提示词）/ block_card_extractor.go
// （block 切分）/ message.go extractCardsFromContent（content 提取）。
const fenceOpenMarker = "```agenthub"

// SplitTextBlocksByCardFences 扫描 blocks 数组，对每个 text block 调用 splitTextBlockByCardFences
// 切分。返回新 blocks 数组（含 card kind block）。非 text block 原样保留。所有 block 的
// Index 重新编号（保证单调递增无空洞）。
//
// 纯函数：不修改入参 blocks（cloneBlocks + append 新 slice）。
func SplitTextBlocksByCardFences(blocks []model.MessageBlock) []model.MessageBlock {
	if len(blocks) == 0 {
		return blocks
	}
	out := make([]model.MessageBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Kind != model.BlockKindText {
			out = append(out, block)
			continue
		}
		parts := splitTextBlockByCardFences(block.Text)
		for _, part := range parts {
			// 跳过空文本片段：text 切分时 before/middle/after 可能为空串，落库无意义。
			if part.Kind == model.BlockKindText && strings.TrimSpace(part.Text) == "" {
				continue
			}
			out = append(out, part)
		}
	}
	// 重新计算 Index：保证单调递增无空洞（与 streaming_reducer.nextIndex 行为一致）。
	for i := range out {
		out[i].Index = i
	}
	return out
}

// splitTextBlockByCardFences 扫描单个 text block 的内容，识别所有 ```agenthub
// {"cards":[...]}``` fenced block，切分成 [text, card, text, card, text] 序列。
// 返回的 slice 元素 Index 未设置（由上层 SplitTextBlocksByCardFences 统一编号）。
//
// 识别算法与 extractCardsFromContent 完全一致（避免双源）：
//   - 仅识别 fenced block（```agenthub 开启，``` 闭合）
//   - block 内 JSON 解析失败 → 该 fence 原样保留（不切分）
//   - block 无 cards 字段或 cards 非数组 → 该 fence 原样保留
//   - block 未闭合（inBlock 仍 true 到末尾）→ 原样返回整个 text（不部分切分）
//
// 多卡处理：单个 fence 内 cards 数组长度 N → 拆成 N 个独立 card block（每张卡精确
// 占位，前端 BlockRegistry 单独 render）。这是与 extractCardsFromContent 的细微差异——
// 后者把多卡合并到单一 cards 数组，本函数把每张卡提升为独立 block。
func splitTextBlockByCardFences(text string) []model.MessageBlock {
	if !strings.Contains(text, fenceOpenMarker) {
		// 快速路径：无 fence 标记，原样返回。
		return []model.MessageBlock{{Kind: model.BlockKindText, Text: text}}
	}

	lines := strings.Split(text, "\n")

	// 第一遍：识别所有有效 fence block 的 (startLine, endLine, cards)。
	type fenceMatch struct {
		startLine int
		endLine   int
		cards     []map[string]any
	}
	var fences []fenceMatch

	inBlock := false
	blockStart := -1
	var jsonBuf strings.Builder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == fenceOpenMarker {
				inBlock = true
				blockStart = i
				jsonBuf.Reset()
			}
			continue
		}
		// inBlock == true
		if trimmed == "```" {
			// 闭合 fence——尝试解析这个 block
			var probe struct {
				Cards []map[string]any `json:"cards"`
			}
			if err := json.Unmarshal([]byte(jsonBuf.String()), &probe); err == nil && probe.Cards != nil {
				fences = append(fences, fenceMatch{
					startLine: blockStart,
					endLine:   i,
					cards:     probe.Cards,
				})
			}
			inBlock = false
			blockStart = -1
			continue
		}
		jsonBuf.WriteString(line)
		jsonBuf.WriteString("\n")
	}

	// fence 未闭合（inBlock 仍 true）→ 原样返回整个 text，不部分切分。
	// 这避免了流式期间 fence 半写导致 block 边界污染。
	if inBlock || len(fences) == 0 {
		return []model.MessageBlock{{Kind: model.BlockKindText, Text: text}}
	}

	// 第二遍：按 fence 切分。fence 内多卡拆成 N 个独立 card block。
	out := make([]model.MessageBlock, 0, len(fences)+1)
	cursor := 0
	for _, f := range fences {
		// fence 之前的 text 段
		if f.startLine > cursor {
			before := strings.Join(lines[cursor:f.startLine], "\n")
			// 保留原文（包括换行），但不要在 text 末尾追加 fence 之前的换行——
			// fence 行本身被吞掉，前后文本之间至少有一个换行被消除。
			before = strings.TrimSuffix(before, "\n")
			if before != "" {
				out = append(out, model.MessageBlock{Kind: model.BlockKindText, Text: before})
			}
		}
		// fence 内每张卡独立一个 card block（多卡 → N 个 block）
		for _, c := range f.cards {
			if c == nil {
				continue
			}
			out = append(out, model.MessageBlock{
				Kind: model.BlockKindCard,
				Card: c,
			})
		}
		cursor = f.endLine + 1
	}
	// 尾部 text
	if cursor < len(lines) {
		tail := strings.Join(lines[cursor:], "\n")
		tail = strings.TrimPrefix(tail, "\n")
		if tail != "" {
			out = append(out, model.MessageBlock{Kind: model.BlockKindText, Text: tail})
		}
	}

	if len(out) == 0 {
		return []model.MessageBlock{{Kind: model.BlockKindText, Text: text}}
	}
	return out
}
