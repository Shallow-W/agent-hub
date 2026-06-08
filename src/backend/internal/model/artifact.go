package model

import "time"

// Artifact 结构化产物：从 Agent 回复中提取的代码 / 网页等一等对象。
// 关联 message_id，预留 version 为后续版本历史铺路。
// 字段名（json tag）必须与 daemon(JS) 上行的产物 JSON 及前端 TS 类型三层对齐。
type Artifact struct {
	ID        string    `json:"id" db:"id"`
	MessageID string    `json:"message_id" db:"message_id"`
	RootID    string    `json:"root_id" db:"root_id"` // 版本血缘根（v1 的 id），同一逻辑产物各版本共享
	Version   int       `json:"version" db:"version"`
	Type      string    `json:"type" db:"type"`               // code | webpage | document | file
	Language  string    `json:"language,omitempty" db:"language"` // code 产物的语言，如 go/ts/python
	Filename  string    `json:"filename,omitempty" db:"filename"` // 可选文件名
	Title     string    `json:"title,omitempty" db:"title"`       // 可选标题（webpage）
	URL       string    `json:"url,omitempty" db:"url"`           // webpage 产物的链接
	Content   string    `json:"content,omitempty" db:"content"`   // code 源码 / webpage HTML
	SortOrder int       `json:"-" db:"sort_order"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
