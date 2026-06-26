package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config 应用配置结构
type Config struct {
	Server struct {
		Port        int    `koanf:"port"`
		ExternalURL string `koanf:"external_url"` // 局域网/公网可达地址，如 http://10.0.0.1:8080；为空则自动检测
	} `koanf:"server"`
	Database struct {
		Host     string `koanf:"host"`
		Port     int    `koanf:"port"`
		User     string `koanf:"user"`
		Password string `koanf:"password"`
		DBName   string `koanf:"dbname"`
		SSLMode  string `koanf:"sslmode"`
	} `koanf:"database"`
	JWT struct {
		Secret      string `koanf:"secret"`
		ExpiryHours int    `koanf:"expiry_hours"`
	} `koanf:"jwt"`
	Daemon struct {
		Token string `koanf:"token"`
	} `koanf:"daemon"`
	CORS struct {
		AllowedOrigins []string `koanf:"allowed_origins"`
	} `koanf:"cors"`
	Redis struct {
		Host     string `koanf:"host"`
		Port     int    `koanf:"port"`
		Password string `koanf:"password"`
		DB       int    `koanf:"db"`
	} `koanf:"redis"`
	Upload struct {
		Dir           string `koanf:"dir"`
		MaxImageMB    int    `koanf:"max_image_mb"`
		MaxPDFMB      int    `koanf:"max_pdf_mb"`
		PublicBaseURL string `koanf:"public_base_url"`
	} `koanf:"upload"`
	Log struct {
		Level string `koanf:"level"`
	} `koanf:"log"`
	RateLimit struct {
		RPS   float64 `koanf:"rps"`
		Burst int     `koanf:"burst"`
	} `koanf:"rate_limit"`
	GitHub struct {
		Token     string `koanf:"token"`      // PAT（classic，repo 权限）；建议用环境变量 GITHUB_TOKEN 注入
		Owner     string `koanf:"owner"`      // 仓库归属账号，如 Shallow-W；可用 GITHUB_PAGES_OWNER 覆盖
		PagesRepo string `koanf:"pages_repo"` // 专用公开仓库名，如 agent-hub-sites；可用 GITHUB_PAGES_REPO 覆盖
	} `koanf:"github"`
}

// loadConfig 从 YAML 文件加载配置
func loadConfig(path string) (*Config, error) {
	if envPath := os.Getenv("AGENTHUB_CONFIG"); envPath != "" {
		path = envPath
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.Server.Port <= 0 {
		return fmt.Errorf("server.port must be positive")
	}
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	return nil
}

// parseLogLevel 将字符串转为 slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// firstNonEmpty 返回第一个非空字符串（用于环境变量覆盖配置文件）。
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// isFalsy 判断环境变量是否表示「关闭」（false/0/no/off，忽略大小写与首尾空白）。
func isFalsy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}
