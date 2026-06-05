package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config 应用配置结构
type Config struct {
	Server struct {
		Port int `koanf:"port"`
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
		Dir        string `koanf:"dir"`
		MaxImageMB int    `koanf:"max_image_mb"`
		MaxPDFMB   int    `koanf:"max_pdf_mb"`
	} `koanf:"upload"`
	Log struct {
		Level string `koanf:"level"`
	} `koanf:"log"`
	RateLimit struct {
		RPS   float64 `koanf:"rps"`
		Burst int     `koanf:"burst"`
	} `koanf:"rate_limit"`
}

// loadConfig 从 YAML 文件加载配置
func loadConfig(path string) (*Config, error) {
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
