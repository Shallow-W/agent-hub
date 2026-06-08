package model

import "time"

// Deployment 产物部署记录：将一个 artifact（按血缘根 root_id）落盘为可访问站点 / 可打包下载的产物。
// 字段名（json tag）需与前端 TS 类型对齐。
type Deployment struct {
	ID             string    `json:"id" db:"id"`
	ArtifactRootID string    `json:"artifact_root_id" db:"artifact_root_id"` // 部署的产物血缘根
	ConversationID string    `json:"conversation_id" db:"conversation_id"`   // 所属对话（鉴权与卡片归属）
	Mode           string    `json:"mode" db:"mode"`                         // preview
	Status         string    `json:"status" db:"status"`                     // pending | success | failed
	URL            string    `json:"url,omitempty" db:"url"`                 // 预览访问地址（DB 存相对路径；输出时按公网基址装饰为绝对）
	DownloadURL    string    `json:"download_url,omitempty" db:"-"`          // 源码 zip 下载地址（计算字段，不入库）
	Error          string    `json:"error,omitempty" db:"error"`             // 失败原因
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
