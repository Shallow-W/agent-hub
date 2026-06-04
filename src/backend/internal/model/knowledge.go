package model

import "time"

// KnowledgeBase 知识库模型
type KnowledgeBase struct {
	ID          string          `json:"id" db:"id"`
	UserID      string          `json:"user_id" db:"user_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	Visibility  string          `json:"visibility" db:"visibility"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
	Files       []KnowledgeFile `json:"files" db:"-"`
	FileCount   int             `json:"file_count" db:"file_count"`
	Username    string          `json:"username,omitempty" db:"username"`
}

// KnowledgeFile 知识库文件模型
type KnowledgeFile struct {
	ID              string    `json:"id" db:"id"`
	KnowledgeBaseID string    `json:"knowledge_base_id" db:"knowledge_base_id"`
	Filename        string    `json:"filename" db:"filename"`
	FilePath        string    `json:"-" db:"file_path"`
	FileSize        int64     `json:"size" db:"file_size"`
	MimeType        string    `json:"mime_type" db:"mime_type"`
	CreatedAt       time.Time `json:"uploaded_at" db:"created_at"`
}
