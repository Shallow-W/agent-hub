package model

import (
	"encoding/json"
	"time"
)

// UserTemplate 是用户保存的工具集 / Skill 模板。
type UserTemplate struct {
	ID        string          `json:"id" db:"id"`
	UserID    string          `json:"user_id" db:"user_id"`
	Type      string          `json:"type" db:"type"`
	Name      string          `json:"name" db:"name"`
	Content   json.RawMessage `json:"content" db:"content"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}
