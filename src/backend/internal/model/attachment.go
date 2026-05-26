package model

import "time"

// MessageAttachment 消息附件
type MessageAttachment struct {
	ID            string    `json:"id" db:"id"`
	MessageID     string    `json:"message_id" db:"message_id"`
	FileName      string    `json:"file_name" db:"file_name"`
	MimeType      string    `json:"mime_type" db:"mime_type"`
	FileSize      int64     `json:"file_size" db:"file_size"`
	FilePath      string    `json:"file_path" db:"file_path"`
	ThumbnailPath string    `json:"thumbnail_path,omitempty" db:"thumbnail_path"`
	Width         int       `json:"width,omitempty" db:"width"`
	Height        int       `json:"height,omitempty" db:"height"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}
