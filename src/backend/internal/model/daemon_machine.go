package model

import (
	"encoding/json"
	"time"
)

// DaemonMachine 表示一台接入 AgentHub 的本地或远端电脑
type DaemonMachine struct {
	ID         string     `json:"id" db:"id"`
	UserID     string     `json:"user_id" db:"user_id"`
	Name       string     `json:"name" db:"name"`
	APIKeyHash string     `json:"-" db:"api_key_hash"`
	MachineID  string     `json:"machine_id" db:"machine_id"`
	Status     string     `json:"status" db:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	// CapabilitiesRaw 机器能力清单（JSON 数组字符串，如 '["docker"]'），daemon 上报。
	// DB 存 TEXT，sqlx 直接扫描；HasCapability 方法解析。
	CapabilitiesRaw string `json:"-" db:"capabilities"`
	// Capabilities 解析后的能力清单，JSON 序列化时输出给前端。
	Capabilities []string `json:"capabilities,omitempty" db:"-"`
}

// HasCapability 判断机器是否具备某项能力（如 "docker"）。
func (m *DaemonMachine) HasCapability(cap string) bool {
	var caps []string
	if m.CapabilitiesRaw == "" {
		return false
	}
	if err := json.Unmarshal([]byte(m.CapabilitiesRaw), &caps); err != nil {
		return false
	}
	m.Capabilities = caps // 缓存解析结果，供 JSON 输出
	for _, c := range caps {
		if c == cap {
			return true
		}
	}
	return false
}

// ParseCapabilities 把 CapabilitiesRaw 解析到 Capabilities 字段（供 JSON 序列化输出）。
func (m *DaemonMachine) ParseCapabilities() {
	if m.CapabilitiesRaw == "" {
		m.Capabilities = []string{}
		return
	}
	var caps []string
	if err := json.Unmarshal([]byte(m.CapabilitiesRaw), &caps); err != nil {
		m.Capabilities = []string{}
		return
	}
	m.Capabilities = caps
}
