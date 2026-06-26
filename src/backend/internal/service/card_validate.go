package service

import (
	"fmt"
	"log/slog"
	"strings"
)

// SupportedCardTypes 是平台当前支持的 card_type 集合——单一事实源。
// 新增卡片类型在此追加，配合：
//   - context_agent_config.go 的系统提示词（教 agent 何时输出）
//   - 前端 types/card.ts 的 CardType union + 接口
//   - 前端 CardRegistry.tsx 的 registerCard
//
// 四处不一致时：前端静默丢弃未注册 type 的卡（getCardSpec 返回 undefined），
// 后端不校验时则把脏数据入库——所以新增类型必须四处同步改。
var SupportedCardTypes = map[string]struct{}{
	"plan":      {},
	"approval":  {},
	"progress":  {},
	"info":      {},
	"diff":      {},
	"project":   {},
}

// IsSupportedCardType 报告 t 是否为平台支持的 card_type。
func IsSupportedCardType(t string) bool {
	_, ok := SupportedCardTypes[t]
	return ok
}

// ListSupportedCardTypes 返回排序后的支持类型列表（供 API 暴露给前端 / 测试 assert）。
func ListSupportedCardTypes() []string {
	out := make([]string, 0, len(SupportedCardTypes))
	for t := range SupportedCardTypes {
		out = append(out, t)
	}
	return out
}

// ValidateCard 对一张 map[string]any 形态的卡做 best-effort 校验。
// 设计哲学：
//   - 严格拒绝明显坏掉的卡（缺 type / 缺必填字段且无可恢复兜底）——避免脏数据入库
//   - 宽容对待可选字段缺失——LLM 偶尔会漏字段，前端组件已有兜底
//   - 不深校验字段值的类型细节（如 options 数组里的元素结构）——会让协议僵化，
//     且前端组件已做防御（card.questions ?? []）
//
// 返回 (card, error)：拒绝时返回 nil + error；接受时返回原 card（可能补了默认值）。
// 调用方应在 extractCardsFromContent / Drain 之后、入库之前调用。
func ValidateCard(card map[string]any) (map[string]any, error) {
	if card == nil {
		return nil, fmt.Errorf("card is nil")
	}

	// type 必填 + 必须是已知类型。未知 type 直接拒绝——
	// 避免幽灵卡片入库（前端会静默丢弃但 DB 已污染）。
	rawType, ok := card["type"]
	if !ok {
		return nil, fmt.Errorf("missing required field: type")
	}
	cardType, ok := rawType.(string)
	if !ok || cardType == "" {
		return nil, fmt.Errorf("field type must be non-empty string, got %T", rawType)
	}
	if !IsSupportedCardType(cardType) {
		return nil, fmt.Errorf("unsupported card type %q (supported: %s)", cardType, strings.Join(ListSupportedCardTypes(), ", "))
	}

	// id 必填——extractCardsFromContent 上游已补 UUID，但 MCP subprocess
	// 直接走 TaskCardQueue 时未必补；此处兜底。
	if _, ok := card["id"]; !ok {
		return nil, fmt.Errorf("missing required field: id (type=%s)", cardType)
	}
	idStr, ok := card["id"].(string)
	if !ok || idStr == "" {
		return nil, fmt.Errorf("field id must be non-empty string, got %T", card["id"])
	}

	// 按 type 校验必填字段。LLM 漏字段时记 warn 但仍接受——
	// 前端组件已有防御（如 DiffCard 用 card.files?.length），不致命。
	switch cardType {
	case "plan":
		if _, ok := card["questions"]; !ok {
			slog.Warn("card validation: plan card missing questions", "card_id", idStr)
		}
	case "approval":
		if _, ok := card["actions"]; !ok {
			slog.Warn("card validation: approval card missing actions", "card_id", idStr)
		}
	case "progress":
		if _, ok := card["tasks"]; !ok {
			slog.Warn("card validation: progress card missing tasks", "card_id", idStr)
		}
	case "info":
		if _, ok := card["fields"]; !ok {
			slog.Warn("card validation: info card missing fields", "card_id", idStr)
		}
	case "diff":
		// workDir + files 是关键——缺了前端无法启动 git 查询。严格拒绝。
		if _, ok := card["workDir"]; !ok {
			return nil, fmt.Errorf("diff card missing required field: workDir (card_id=%s)", idStr)
		}
		if _, ok := card["files"]; !ok {
			return nil, fmt.Errorf("diff card missing required field: files (card_id=%s)", idStr)
		}
	case "project":
		if _, ok := card["workDir"]; !ok {
			return nil, fmt.Errorf("project card missing required field: workDir (card_id=%s)", idStr)
		}
	}
	return card, nil
}

// ValidateCards 过滤掉无效卡（保留有效卡），用于批量场景。
// 至少返回有效卡切片（可能为空）；无效卡的错误聚合为一条 warn 日志。
// 不返回 error——best-effort：哪怕全部无效也不阻塞消息创建。
func ValidateCards(cards []map[string]any) []map[string]any {
	if len(cards) == 0 {
		return cards
	}
	valid := make([]map[string]any, 0, len(cards))
	var rejected []string
	for _, c := range cards {
		vc, err := ValidateCard(c)
		if err != nil {
			rejected = append(rejected, err.Error())
			continue
		}
		valid = append(valid, vc)
	}
	if len(rejected) > 0 {
		slog.Warn("card validation rejected some cards", "rejected_count", len(rejected), "reasons", rejected)
	}
	return valid
}
