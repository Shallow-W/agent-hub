package model

import "time"

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
}
